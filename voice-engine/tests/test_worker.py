import json
import math
import threading
import urllib.error
import urllib.request
from http.server import ThreadingHTTPServer

import numpy as np
import pytest

from icf_worker import mastering, wav
from icf_worker.engines import (
    CosyVoiceEngine,
    DevToneEngine,
    EngineError,
    GPTSoVITSEngine,
    VoxCPMEngine,
    checkpoint_revision,
    reference_transcript,
    resolve_checkpoint,
)
from icf_worker.server import make_handler

RATE = 48000


def tone(amp, secs=3.0, freq=1000.0, rate=RATE):
    t = np.arange(int(secs * rate)) / rate
    return amp * np.sin(2 * np.pi * freq * t)


def test_bs1770_reference_tone():
    # BS.1770: a 0 dBFS-peak 1 kHz sine measures about -3.01 LUFS.
    assert mastering.integrated_loudness(tone(1.0), RATE) == pytest.approx(-3.01, abs=0.1)


def test_master_hits_configured_target():
    y, rep = mastering.master(tone(0.05), RATE, target_lufs=-16, ceiling_dbtp=-1)
    assert rep.output_lufs == pytest.approx(-16, abs=0.1)
    assert rep.output_true_peak_dbtp <= -1 + 1e-6
    assert not rep.peak_limited


def test_ceiling_wins_and_is_reported():
    # A crest-heavy signal cannot reach -9 LUFS under a -1 dBTP ceiling.
    x = tone(0.02)
    x[::4800] = 0.9
    y, rep = mastering.master(x, RATE, target_lufs=-9, ceiling_dbtp=-1)
    assert rep.peak_limited
    assert rep.output_true_peak_dbtp <= -1 + 1e-6
    assert rep.output_lufs < -9


def test_silence_is_never_amplified():
    y, rep = mastering.master(np.zeros(RATE), RATE)
    assert rep.gain_db == 0 and not np.any(y)


def test_checkpoint_traversal_refused(tmp_path, monkeypatch):
    from icf_worker import storage
    monkeypatch.setenv("ICF_STORAGE_ROOT", str(tmp_path))
    storage.reset_store()
    monkeypatch.setenv("ICF_CHECKPOINT_ROOT", str(tmp_path))
    with pytest.raises(EngineError) as e:
        resolve_checkpoint("../../etc/passwd")
    assert e.value.cls == "permanent"


def test_missing_reference_transcript_is_a_classified_content_error():
    # VE-016: the engines used to index the transcript key directly, so a
    # reference without one raised KeyError - which the handler can only report
    # as an unexpected fault, for a request that can never succeed.
    for reference in (None, {}, {"uri": "voice-private/a.wav"}, {"transcript": "   "}):
        with pytest.raises(EngineError) as e:
            reference_transcript(reference)
        assert e.value.cls == "content"


def test_gpt_sovits_refuses_an_untranscribed_reference_before_any_network_call(monkeypatch):
    # The guard runs before the reference path is resolved and before the
    # upstream api_v2 server is contacted, so the failure is classified rather
    # than a storage or connection error.
    def no_network(*_args, **_kwargs):
        raise AssertionError("the guard must run before any provider call")

    monkeypatch.setattr(GPTSoVITSEngine, "_ensure_weights", lambda self, checkpoint: None)
    monkeypatch.setattr(GPTSoVITSEngine, "_get", no_network)

    with pytest.raises(EngineError) as e:
        GPTSoVITSEngine().synthesize_chunk(
            "Peace be still.",
            reference={"uri": "voice-private/ref.wav"},
            checkpoint=None,
            params={},
            chunk={},
        )
    assert e.value.cls == "content"


def test_engine_version_names_the_checkpoint(tmp_path, monkeypatch):
    # VE-012: engine_version was the hardcoded string "0" (CosyVoice) or "1"
    # (VoxCPM), so two different checkpoints produced indistinguishable asset
    # records - and the content hash, which includes engine_version, would have
    # treated a render from the old checkpoint and the new one as the same audio.
    def make(directory, weight_bytes):
        directory.mkdir()
        (directory / "config.yaml").write_text("model: x\n")
        (directory / "weights.pt").write_bytes(b"0" * weight_bytes)
        return directory

    a = make(tmp_path / "ckpt-A", 2048)
    b = make(tmp_path / "ckpt-B", 4096)

    monkeypatch.setenv("ICF_COSYVOICE_MODEL_DIR", str(a))
    monkeypatch.delenv("ICF_CHECKPOINT_REVISION", raising=False)
    first = CosyVoiceEngine().version
    same = CosyVoiceEngine().version
    assert first == same, "the same checkpoint must produce the same version"
    assert first.startswith("0+") and len(first) == len("0+") + 12

    monkeypatch.setenv("ICF_COSYVOICE_MODEL_DIR", str(b))
    assert CosyVoiceEngine().version != first, "a different checkpoint must change the version"

    # A swapped file of a different size changes it too - that is the point.
    (a / "weights.pt").write_bytes(b"0" * 8192)
    monkeypatch.setenv("ICF_COSYVOICE_MODEL_DIR", str(a))
    assert CosyVoiceEngine().version != first

    # With no checkpoint configured the version stays the plain generation
    # rather than gaining a meaningless suffix.
    monkeypatch.delenv("ICF_COSYVOICE_MODEL_DIR", raising=False)
    assert CosyVoiceEngine().version == "0"

    # An explicit revision wins: operators who know what is deployed should not
    # have their answer second-guessed by a directory listing.
    monkeypatch.setenv("ICF_CHECKPOINT_REVISION", "cosyvoice2-2026-09")
    assert CosyVoiceEngine().version == "0+cosyvoice2-2026-09"
    assert checkpoint_revision(str(a)) == "cosyvoice2-2026-09"


def test_every_backend_derives_its_own_version(tmp_path, monkeypatch):
    monkeypatch.delenv("ICF_CHECKPOINT_REVISION", raising=False)
    monkeypatch.setenv("ICF_VOXCPM_MODEL", str(tmp_path / "voxcpm"))
    monkeypatch.delenv("ICF_COSYVOICE_MODEL_DIR", raising=False)
    assert VoxCPMEngine().version.startswith("1+")
    # The dev backend is not a model and must not pretend to have a checkpoint.
    assert DevToneEngine().version == "dev"
    # GPT-SoVITS names the upstream generation until a fine-tune is pushed.
    assert GPTSoVITSEngine().version == "v2"


def _FakeCosyVoiceModel(instruct2="ok"):
    """Records which inference path the engine chose, without a GPU.

    instruct2="ok" is a build with the documented API; "missing" is one that
    never had it; "raises" is one whose signature differs, which only shows up
    when you call it.
    """

    class _Model:
        sample_rate = 24000

        def __init__(self):
            self.calls: list[tuple] = []

        def _record(self, kind, *rest):
            self.calls.append((kind, *rest))
            return [{"tts_speech": _fake_tensor()}]

        def inference_zero_shot(self, text, transcript, prompt, **kw):
            return self._record("zero_shot", text, transcript, prompt, kw)

    if instruct2 == "ok":

        def inference_instruct2(self, text, instruction, prompt, **kw):
            return self._record("instruct2", text, instruction, prompt, kw)

        _Model.inference_instruct2 = inference_instruct2
    elif instruct2 == "raises":

        def inference_instruct2(self, text, *args, **kw):
            raise TypeError("inference_instruct2() got an unexpected keyword argument 'stream'")

        _Model.inference_instruct2 = inference_instruct2

    return _Model()


def _fake_tensor():
    import numpy as _np

    class _T:
        def squeeze(self):
            return self

        def cpu(self):
            return self

        def numpy(self):
            return _np.zeros(64)

    return _T()


def _cosyvoice_with(model):
    e = CosyVoiceEngine()
    e._model = model  # bypass _load: this test is about which path is chosen
    e.sample_rate = 24000
    return e


REF = {"uri": "voice-private/ref.wav", "transcript": "Let us pray."}


def test_style_instruction_reaches_the_engine(tmp_path, monkeypatch):
    # VE-006: inference_instruct2 is CosyVoice's expressive control and the
    # adapter never called it. The Go side already sent the style name on the
    # wire; the instruction is what the model actually reads.
    model = _FakeCosyVoiceModel()
    engine = _cosyvoice_with(model)
    monkeypatch.setenv("ICF_STORAGE_ROOT", str(tmp_path))
    from icf_worker import storage as _storage

    _storage.reset_store()
    (tmp_path / "voice-private").mkdir()
    (tmp_path / "voice-private" / "ref.wav").write_bytes(b"x")

    engine.synthesize_chunk("Our Father, who art in heaven", reference=REF, checkpoint=None,
                            params={"speed": 0.95, "instruct": "Speak as a prayer: slow, quiet and reverent.",
                                    "instruct_style": "prayer"}, chunk={})

    assert len(model.calls) == 1, model.calls
    kind, text, instruction, prompt, kw = model.calls[0]
    assert kind == "instruct2", "the style instruction was not used"
    assert instruction == "Speak as a prayer: slow, quiet and reverent."
    assert text == "Our Father, who art in heaven"
    assert kw.get("speed") == 0.95
    assert prompt.endswith("ref.wav")
    assert engine.runtime_capabilities()["style_instruction"] is True


def test_no_instruction_renders_zero_shot(tmp_path, monkeypatch):
    model = _FakeCosyVoiceModel()
    engine = _cosyvoice_with(model)
    monkeypatch.setenv("ICF_STORAGE_ROOT", str(tmp_path))
    from icf_worker import storage as _storage

    _storage.reset_store()
    (tmp_path / "voice-private").mkdir()
    (tmp_path / "voice-private" / "ref.wav").write_bytes(b"x")

    engine.synthesize_chunk("plain", reference=REF, checkpoint=None, params={"speed": 1.0}, chunk={})
    assert model.calls[0][0] == "zero_shot"
    assert model.calls[0][2] == "Let us pray."


@pytest.mark.parametrize("shape", ["missing", "raises"])
def test_a_build_without_instruct2_still_renders_and_says_so(tmp_path, monkeypatch, caplog, shape):
    # A CosyVoice build that lacks the API, or has a different signature for it,
    # must not fail the request - but the dropped instruction must be visible
    # rather than silent, which is the whole reason VE-006 existed.
    model = _FakeCosyVoiceModel(instruct2=shape)
    engine = _cosyvoice_with(model)
    monkeypatch.setenv("ICF_STORAGE_ROOT", str(tmp_path))
    from icf_worker import storage as _storage

    _storage.reset_store()
    (tmp_path / "voice-private").mkdir()
    (tmp_path / "voice-private" / "ref.wav").write_bytes(b"x")

    with caplog.at_level("WARNING"):
        engine.synthesize_chunk("first", reference=REF, checkpoint=None,
                                params={"instruct": "Speak as a prayer."}, chunk={})
        engine.synthesize_chunk("second", reference=REF, checkpoint=None,
                                params={"instruct": "Speak as a prayer."}, chunk={})

    assert [c[0] for c in model.calls] == ["zero_shot", "zero_shot"]
    assert engine.runtime_capabilities()["style_instruction"] is False
    assert caplog.text.count("style instructions cannot be used") == 1, "warned more than once"


@pytest.fixture
def worker():
    srv = ThreadingHTTPServer(("127.0.0.1", 0), make_handler(DevToneEngine(), "s3cret"))
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    yield f"http://127.0.0.1:{srv.server_port}"
    srv.shutdown()


def call(url, body=None, token="s3cret"):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, headers={"Authorization": f"Bearer {token}"})
    return urllib.request.urlopen(req)


def test_requires_token(worker):
    with pytest.raises(urllib.error.HTTPError) as e:
        call(worker + "/v1/health", token="wrong")
    assert e.value.code == 401


def test_synthesize_returns_mastered_provenanced_wav(worker):
    resp = call(worker + "/v1/synthesize", {"chunks": [{"text": "Grace and peace.", "silence_after_ms": 300}],
                                            "params": {"speed": 1.0}})
    assert resp.headers["X-Synthetic"] == "true"
    assert resp.headers["Content-Type"] == "audio/wav"
    x, rate = wav.decode(resp.read())
    assert rate == 16000 and x.size > 0
    assert float(resp.headers["X-Loudness-LUFS"]) == pytest.approx(-16, abs=0.5)
    assert float(resp.headers["X-True-Peak-dBTP"]) <= -0.9


def test_errors_are_classified(worker):
    with pytest.raises(urllib.error.HTTPError) as e:
        call(worker + "/v1/synthesize", {"chunks": []})
    assert e.value.code == 422
    assert json.loads(e.value.read())["error"]["class"] == "content"


def test_capabilities_advertise_the_stream_endpoint(worker):
    # The Go orchestrator gates /voices/stream on this flag. The worker's
    # stream endpoint works for every engine (chunk by chunk), so it must say so
    # even for engines that can't stream within a chunk.
    caps = json.loads(call(worker + "/v1/capabilities").read())
    assert caps["streaming"] is True
    assert caps["streaming_granularity"] == "chunk"


def test_capabilities_report_runtime_flags_live():
    # VE-006: whether style instructions are honoured is only knowable once an
    # engine has tried, so it is reported as a runtime capability rather than a
    # static claim in the engine's table. The value has to change when the engine
    # learns otherwise, or it is decoration.
    engine = _cosyvoice_with(_FakeCosyVoiceModel())
    srv = ThreadingHTTPServer(("127.0.0.1", 0), make_handler(engine, "s3cret"))
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    try:
        base = f"http://127.0.0.1:{srv.server_port}"
        caps = json.loads(call(base + "/v1/capabilities").read())
        assert caps["style_instruction"] is True
        assert caps["engine_version"].startswith("0")

        engine._drop_instruct("test: pretend this build lacks instruct2")
        caps = json.loads(call(base + "/v1/capabilities").read())
        assert caps["style_instruction"] is False
    finally:
        srv.shutdown()
