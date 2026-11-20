import json
import math
import threading
import urllib.error
import urllib.parse
import urllib.request
from concurrent.futures import ThreadPoolExecutor
from contextlib import contextmanager
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import numpy as np
import pytest

from icf_worker import mastering, wav
from icf_worker.engines import (
    CosyVoiceEngine,
    DevToneEngine,
    EngineError,
    GPTSoVITSEngine,
    VoxCPMEngine,
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


def test_gpt_sovits_refuses_an_untranscribed_chunk_before_tts(monkeypatch):
    # Even on a prepared worker, a missing transcript must never reach /tts.
    def no_network(*_args, **_kwargs):
        raise AssertionError("the guard must run before any provider call")

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


def test_revision_measures_actual_bundle_once_per_instance(tmp_path, monkeypatch):
    # Same bytes on different mounts match; same-sized edits to nested weights
    # do not. The operator's release label can never mask a byte change.
    monkeypatch.setenv("ICF_CHECKPOINT_REVISION", "reviewed-build")
    for name in ("replica-a", "replica-b"):
        directory = tmp_path / name / "nested"
        directory.mkdir(parents=True)
        (directory / "weights.pt").write_bytes(b"ABCD")
    a, b = tmp_path / "replica-a", tmp_path / "replica-b"
    monkeypatch.setenv("ICF_COSYVOICE_MODEL_DIR", str(a))
    old = CosyVoiceEngine()
    first = old.version
    assert first.startswith("0.reviewed-build+sha256.") and len(first.split("+sha256.")[1]) == 64
    monkeypatch.setenv("ICF_COSYVOICE_MODEL_DIR", str(b))
    assert CosyVoiceEngine().version == first
    (b / "nested/weights.pt").write_bytes(b"ABCE")
    assert CosyVoiceEngine().version != first
    (a / "nested/weights.pt").write_bytes(b"ABCF")
    assert old.version == first, "a long-lived instance must not relabel already-loaded weights"
    monkeypatch.delenv("ICF_COSYVOICE_MODEL_DIR")
    with pytest.raises(EngineError, match="checkpoint bundle"):
        _ = CosyVoiceEngine().version
    monkeypatch.setenv("ICF_VOXCPM_MODEL", str(b))
    assert VoxCPMEngine().version.startswith("1.reviewed-build+sha256.")
    assert DevToneEngine().version == "dev"


def test_gpt_sovits_binds_both_weights_on_every_render(tmp_path, monkeypatch):
    monkeypatch.setenv("ICF_GPTSOVITS_MODEL_DIR", str(tmp_path))
    gpt, sov = tmp_path / "model.ckpt", tmp_path / "model.pth"
    gpt.write_bytes(b"gpt")
    sov.write_bytes(b"sov")
    e = GPTSoVITSEngine()
    calls = []
    monkeypatch.setattr(e, "_get", lambda path, args: calls.append((path, args["weights_path"])))
    for ckpt in (sov, None):
        e.prepare_render(ckpt)
    assert calls == [("/set_gpt_weights", str(gpt)), ("/set_sovits_weights", str(sov))] * 2
    with pytest.raises(EngineError, match="outside"):
        e.prepare_render(tmp_path.parent / "other.pth")
    (tmp_path / "other.pth").write_bytes(b"other")
    with pytest.raises(EngineError, match="exactly one"):
        e.prepare_render(None)
    assert len(calls) == 4


def test_gpt_sovits_verifies_the_upstream_that_returns_audio(tmp_path, monkeypatch):
    # The worker is a proxy, not the process holding the weights. A plain
    # api_v2 setter success isn't proof, nor is a receipt from the wrong replica.
    bundle = tmp_path / "bundle"
    bundle.mkdir()
    (bundle / "model.ckpt").write_bytes(b"fixture-a")
    (bundle / "model.pth").write_bytes(b"fixture-b")
    monkeypatch.setenv("ICF_GPTSOVITS_MODEL_DIR", str(bundle))
    monkeypatch.setattr("icf_worker.engines.resolve_checkpoint", lambda _: tmp_path / "ref.wav")
    audio = wav.encode(tone(0.1, secs=0.5), RATE)
    paths = []
    fault = "missing setter proof"

    class Upstream(BaseHTTPRequestHandler):
        def do_GET(self):
            path = urllib.parse.urlsplit(self.path).path
            paths.append(path)
            self.send_response(200)
            if path == "/tts" or (path == "/set_sovits_weights" and fault != "missing setter proof"):
                self.send_header("X-Engine", "gptsovits")
                wrong = path == "/tts" and fault == "different renderer"
                self.send_header("X-Engine-Version", "wrong" if wrong else engine.version)
            self.end_headers()
            if path == "/tts":
                self.wfile.write(audio)

        def log_message(self, *_args):
            pass  # the TTS query includes text

    with ThreadingHTTPServer(("127.0.0.1", 0), Upstream) as upstream:
        monkeypatch.setenv("ICF_GPTSOVITS_API", f"http://127.0.0.1:{upstream.server_port}")
        engine = GPTSoVITSEngine()
        thread = threading.Thread(target=upstream.serve_forever, daemon=True)
        thread.start()
        try:
            with serve(engine) as worker:
                req = {"expected_worker": {"engine": "gptsovits", "revision": engine.version},
                       "chunks": [{"text": "Test."}], "reference": REF}
                stream = worker + "/v1/synthesize/stream"
                for fault in ("missing setter proof", "different renderer"):
                    paths.clear()
                    with pytest.raises(urllib.error.HTTPError) as exc:
                        call(stream, req)
                    assert exc.value.code == 409 and json.loads(exc.value.read())["error"]["class"] == "identity"
                    assert ("/tts" in paths) == (fault == "different renderer")
                fault = ""
                with call(stream, req) as resp:
                    assert resp.headers["X-Engine"] == "gptsovits" and resp.headers["X-Engine-Version"] == engine.version
                    assert resp.read().startswith(b"RIFF")
        finally:
            upstream.shutdown()
            thread.join()


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


@contextmanager
def serve(engine):
    with ThreadingHTTPServer(("127.0.0.1", 0), make_handler(engine, "s3cret")) as srv:
        thread = threading.Thread(target=srv.serve_forever, daemon=True)
        thread.start()
        try:
            yield f"http://127.0.0.1:{srv.server_port}"
        finally:
            srv.shutdown()
            thread.join()


@pytest.fixture
def worker():
    with serve(DevToneEngine()) as url:
        yield url


def call(url, body=None, token="s3cret", *, pin=True):
    if pin and body is not None and "/v1/synthesize" in url:
        body = {"expected_worker": {"engine": "dev-tone", "revision": "dev"}, **body}
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, headers={"Authorization": f"Bearer {token}"})
    return urllib.request.urlopen(req)


@pytest.mark.parametrize("active", ["/v1/synthesize", "/v1/synthesize/stream"])
def test_busy_worker_refuses_overlap_without_entering_engine(active):
    entered, release = threading.Event(), threading.Event()
    calls = []

    class BlockingTone(DevToneEngine):
        def synthesize_chunk(self, text, **kwargs):
            calls.append(text)
            if text == "Second chunk.":
                entered.set()
                assert release.wait(5), "test did not release engine"
            return super().synthesize_chunk(text, **kwargs)

    with serve(BlockingTone()) as base:
        single = {"chunks": [{"text": "Next render."}]}

        def original():
            with call(base + active, {"chunks": [{"text": "First chunk."}, {"text": "Second chunk."}]}) as resp:
                return resp.read()

        with ThreadPoolExecutor(max_workers=1) as pool:
            first = pool.submit(original)
            try:
                assert entered.wait(5), "original request did not enter inference"
                # Health is still reachable and neither endpoint waits on inference.
                assert json.loads(call(base + "/v1/health").read())["status"] == "ok"
                for path in ("/v1/synthesize", "/v1/synthesize/stream"):
                    with pytest.raises(urllib.error.HTTPError) as exc:
                        call(base + path, single)
                    assert exc.value.code == 503
                    assert json.loads(exc.value.read())["error"]["class"] == "transient"
                assert calls == ["First chunk.", "Second chunk."]
            finally:
                release.set()
            assert first.result(timeout=5).startswith(b"RIFF")
        assert call(base + "/v1/synthesize", single).read().startswith(b"RIFF")
        assert calls[-1] == "Next render."


def test_stream_fault_after_headers_releases_slot_without_appending_json():
    class FailingTone(DevToneEngine):
        def synthesize_chunk(self, text, **kwargs):
            if text == "Fail here.":
                raise EngineError("gpu", "inference failed")
            return super().synthesize_chunk(text, **kwargs)

    with serve(FailingTone()) as base:
        first = {"text": "First chunk."}
        stream = base + "/v1/synthesize/stream"
        expected = call(stream, {"chunks": [first]}).read()
        assert call(stream, {"chunks": [first, {"text": "Fail here."}]}).read() == expected
        assert call(base + "/v1/synthesize", {"chunks": [first]}).read().startswith(b"RIFF")


@pytest.mark.parametrize("path", ["/v1/synthesize", "/v1/synthesize/stream"])
def test_missing_or_mismatched_pin_never_enters_engine(path):
    class CountingTone(DevToneEngine):
        calls = 0

        def synthesize_chunk(self, text, **kwargs):
            self.calls += 1
            return super().synthesize_chunk(text, **kwargs)

    engine = CountingTone()
    with serve(engine) as base:
        req = {"chunks": [{"text": "Amen."}]}
        for fields in ({}, {"expected_worker": {"engine": "cosyvoice", "revision": "dev"}},
                       {"expected_worker": {"engine": "dev-tone", "revision": "other"}}):
            with pytest.raises(urllib.error.HTTPError) as exc:
                call(base + path, {**req, **fields}, pin=False)
            assert exc.value.code == 409 and json.loads(exc.value.read())["error"]["class"] == "identity"
        assert engine.calls == 0
        resp = call(base + path, req)
        assert (resp.headers["X-Engine"], resp.headers["X-Engine-Version"]) == ("dev-tone", "dev")
        assert resp.read().startswith(b"RIFF") and engine.calls == 1


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


def test_synthesize_reports_measured_seconds(worker):
    """VE-017: cost telemetry, measured rather than estimated.

    The API prices a render from these two numbers, so they have to be
    observations and not decoration: non-negative, nested (the request cannot
    take less time than the inference inside it), and responsive to how much
    work was asked for.
    """

    def secs(url, text):
        resp = call(url, {"chunks": [{"text": text}], "params": {}})
        resp.read()
        return float(resp.headers["X-Inference-Seconds"]), float(resp.headers["X-Worker-Seconds"])

    short_inf, short_all = secs(worker + "/v1/synthesize", "Grace and peace.")
    long_inf, long_all = secs(worker + "/v1/synthesize", "Be still and know. " * 120)
    for inf, all_ in ((short_inf, short_all), (long_inf, long_all)):
        assert inf >= 0 and all_ >= 0
        assert all_ >= inf, "a request cannot be shorter than the inference inside it"
    assert long_inf > short_inf, "inference seconds did not move with the work: it is a constant"
    assert long_all > short_all


def test_stream_carries_no_cost_header(worker):
    """Live preview is metered nowhere, and that is stated rather than implied.

    A total cannot be reported before it is spent, and the render is discarded
    rather than stored, so the stream path deliberately has no cost header. If
    someone adds one, this test is where the contract gets re-read.
    """
    resp = call(worker + "/v1/synthesize/stream", {"chunks": [{"text": "Grace and peace."}]})
    resp.read()
    assert "X-Inference-Seconds" not in resp.headers
    assert "X-Worker-Seconds" not in resp.headers


@pytest.mark.parametrize("path", ["/v1/synthesize", "/v1/synthesize/stream"])
def test_errors_are_classified_and_release_slot(worker, path):
    with pytest.raises(urllib.error.HTTPError) as e:
        call(worker + path, {"chunks": []})
    assert e.value.code == 422
    assert json.loads(e.value.read())["error"]["class"] == "content"
    assert call(worker + path, {"chunks": [{"text": "Next render."}]}).read().startswith(b"RIFF")


def test_capabilities_advertise_the_stream_endpoint(worker):
    # The Go orchestrator gates /voices/stream on this flag. The worker's
    # stream endpoint works for every engine (chunk by chunk), so it must say so
    # even for engines that can't stream within a chunk.
    caps = json.loads(call(worker + "/v1/capabilities").read())
    assert caps["streaming"] is True
    assert caps["streaming_granularity"] == "chunk"


def test_capabilities_report_runtime_flags_live(tmp_path, monkeypatch):
    # VE-006: whether style instructions are honoured is only knowable once an
    # engine has tried, so it is reported as a runtime capability rather than a
    # static claim in the engine's table. The value has to change when the engine
    # learns otherwise, or it is decoration.
    monkeypatch.setenv("ICF_COSYVOICE_MODEL_DIR", str(tmp_path))
    (tmp_path / "weights.pt").write_bytes(b"fixture")
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
