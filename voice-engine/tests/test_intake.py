import json
import sys
import textwrap
import threading
import time
import urllib.error
import urllib.request
from http.server import ThreadingHTTPServer

import numpy as np
import pytest

from icf_worker import ingest, licenses, storage, wav
from icf_worker.engines import DevToneEngine, EngineError, resolve_checkpoint
from icf_worker.server import make_handler
from icf_worker.train import TrainingError, TrainingManager

RATE = 24000


@pytest.fixture
def store(tmp_path, monkeypatch):
    monkeypatch.setenv("ICF_STORAGE", "local")
    monkeypatch.setenv("ICF_STORAGE_ROOT", str(tmp_path / "objects"))
    storage.reset_store()
    yield storage.get_store()
    storage.reset_store()


def voice(f0, secs, rng, rate=RATE):
    """A crude voiced sound: harmonics of f0 with a formant-ish envelope and jitter."""
    t = np.arange(int(secs * rate)) / rate
    f = f0 * (1 + 0.02 * np.sin(2 * np.pi * 3 * t))
    ph = 2 * np.pi * np.cumsum(f) / rate
    x = sum((1 / k) * np.sin(k * ph) * (1 if k * f0 < 3500 else 0.1) for k in range(1, 20))
    return 0.2 * x / np.max(np.abs(x)) * (0.6 + 0.4 * np.abs(np.sin(2 * np.pi * 2.5 * t))) + 0.001 * rng.standard_normal(t.size)


def recording(rng, speakers):
    parts = []
    for f0, secs in speakers:
        parts += [voice(f0, secs, rng), 0.001 * rng.standard_normal(int(0.8 * RATE))]
    return np.concatenate([0.001 * rng.standard_normal(RATE)] + parts)


def test_ingest_segments_scores_and_flags_other_speaker(store):
    rng = np.random.default_rng(1)
    main, other = 110, 260
    x = recording(rng, [(main, 4), (main, 5), (other, 4), (main, 3.5), (main, 20)])
    store.put("voice-private/voices/v/recordings/r.wav", wav.encode(x, RATE))
    store.put("voice-private/voices/v/refs/ref.wav", wav.encode(voice(main, 6, rng), RATE))
    res = ingest.ingest({"audio_key": "voice-private/voices/v/recordings/r.wav",
                         "reference_key": "voice-private/voices/v/refs/ref.wav",
                         "segment_prefix": "voice-private/voices/v/segments/r", "language": "en"})
    segs = res.segments
    # 20 s of continuous speech is split at <= 15 s.
    assert all(s["end_ms"] - s["start_ms"] <= 15000 for s in segs)
    assert len(segs) >= 6
    assert res.report["asr"] == "not_run" and "reference" in res.report["speaker_check"]
    # The segment that overlaps the other speaker (starts ~ 12.6 s) scores lowest.
    by_conf = sorted(segs, key=lambda s: s["speaker_confidence"])
    assert 11000 < by_conf[0]["start_ms"] < 14500
    assert by_conf[0]["speaker_confidence"] < min(s["speaker_confidence"] for s in by_conf[1:])
    for s in segs:
        assert s["snr_db"] > 20 and 0 <= s["quality"] <= 1
        assert store.local_path(s["audio_key"]).exists()


def test_quality_heuristic_orders_sensibly():
    good = ingest.quality_score(40, 0, 6000, -20)
    noisy = ingest.quality_score(12, 0, 6000, -20)
    clipped = ingest.quality_score(40, 0.01, 6000, -20)
    assert good > noisy and good > clipped and good == pytest.approx(1.0)


def test_disallowed_keys_are_permanent_errors(store):
    with pytest.raises(EngineError) as e:
        resolve_checkpoint("../../etc/passwd")
    assert e.value.cls == "permanent"
    with pytest.raises(EngineError) as e:
        resolve_checkpoint("voice-private/missing.wav")
    assert e.value.cls == "model"


FAKE_TRAINER = textwrap.dedent("""
    import json, sys, pathlib
    args = dict(zip(sys.argv[1::2], sys.argv[2::2]))
    rows = [json.loads(l) for l in open(args["--manifest"])]
    assert all(pathlib.Path(r["audio"]).exists() for r in rows)
    print("ICF_PROGRESS 0.5 loss=1.5", flush=True)
    out = pathlib.Path(args["--out"])
    (out / "model.pth").write_bytes(b"weights:" + str(len(rows)).encode())
    print("ICF_PROGRESS 1.0 loss=0.4", flush=True)
""")


def entries(store, n):
    out = []
    for i in range(n):
        key = f"voice-private/voices/v/segments/r/{i:04d}.wav"
        store.put(key, wav.encode(np.zeros(100), RATE))
        split = "test" if i == 0 else ("validation" if i == 1 else "train")
        out.append({"segment_id": f"s{i}", "audio_key": key, "transcript": "Amen.", "split": split})
    return out


def wait(run, secs=10):
    end = time.time() + secs
    while run.status == "RUNNING" and time.time() < end:
        time.sleep(0.05)
    return run


def test_training_runs_wrapped_command_and_uploads_hashed_checkpoint(store, tmp_path, monkeypatch):
    script = tmp_path / "trainer.py"
    script.write_text(FAKE_TRAINER)
    monkeypatch.setenv("ICF_TRAIN_CMD_GPT_SOVITS", f"{sys.executable} {script} --manifest {{manifest}} --out {{out}}")
    tm = TrainingManager(str(tmp_path))
    req = {"run_id": "run_1", "engine": "gpt_sovits", "base_model": "v2", "entries": entries(store, 5),
           "checkpoint_key": "voice-private/voices/v/checkpoints/run_1"}
    run = wait(tm.submit(req))
    assert tm.submit(req) is run  # idempotent
    assert run.status == "COMPLETED", run.error
    assert run.loss == pytest.approx(0.4)
    assert run.checkpoint_key.endswith("/model.pth") and len(run.checkpoint_sha256) == 64
    # Test-split audio is withheld from the trainer: 5 entries, 3 train.
    assert store.local_path(run.checkpoint_key).read_bytes() == b"weights:3"


def test_training_not_configured_and_failures_are_reported(store, tmp_path, monkeypatch):
    tm = TrainingManager(str(tmp_path))
    with pytest.raises(TrainingError) as e:
        tm.submit({"run_id": "r2", "engine": "cosyvoice", "entries": entries(store, 3)})
    assert e.value.cls == "model"
    monkeypatch.setenv("ICF_TRAIN_CMD_COSYVOICE", f"{sys.executable} -c \"import sys; print('boom'); sys.exit(3)\"")
    run = wait(tm.submit({"run_id": "r3", "engine": "cosyvoice", "entries": entries(store, 3)}))
    assert run.status == "FAILED" and "exited 3" in run.error


def test_licence_gate_blocks_uncleared_engines_in_production(monkeypatch, tmp_path):
    reg = {"cosyvoice": {"engine": "cosyvoice", "production_allowed": False},
           "gpt_sovits": {"engine": "gpt_sovits", "production_allowed": True}}
    monkeypatch.setenv("ICF_ENV", "production")
    with pytest.raises(licenses.LicenseError):
        licenses.check("cosyvoice", register=reg)
    with pytest.raises(licenses.LicenseError):
        licenses.check("unknown", register=reg)
    licenses.check("gpt_sovits", register=reg)
    monkeypatch.setenv("ICF_ENV", "development")
    licenses.check("cosyvoice", register=reg)  # warns only
    # The checked-in register clears nothing for production yet.
    assert not any(e.get("production_allowed") for e in licenses.load_register().values())


def test_training_refused_in_production_when_uncleared(store, tmp_path, monkeypatch):
    monkeypatch.setenv("ICF_ENV", "production")
    monkeypatch.setenv("ICF_TRAIN_CMD_GPT_SOVITS", "true")
    with pytest.raises(TrainingError) as e:
        TrainingManager(str(tmp_path)).submit({"run_id": "r4", "engine": "gpt_sovits", "entries": entries(store, 3)})
    assert "production" in str(e.value)


@pytest.fixture
def worker(store, tmp_path):
    srv = ThreadingHTTPServer(("127.0.0.1", 0), make_handler(DevToneEngine(), "t", TrainingManager(str(tmp_path))))
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    yield f"http://127.0.0.1:{srv.server_port}"
    srv.shutdown()


def post(url, body):
    req = urllib.request.Request(url, data=json.dumps(body).encode(), headers={"Authorization": "Bearer t"})
    return urllib.request.urlopen(req)


def test_stream_endpoint_yields_playable_wav(worker):
    resp = post(worker + "/v1/synthesize/stream", {"chunks": [{"text": "Grace and peace.", "silence_after_ms": 200},
                                                             {"text": "Be still."}]})
    body = resp.read()
    assert resp.headers["X-Synthetic"] == "true"
    assert body[:4] == b"RIFF" and body[8:12] == b"WAVE"
    pcm = np.frombuffer(body[44:], dtype="<i2") / 32768
    assert pcm.size > 16000 and np.max(np.abs(pcm)) <= 10 ** (-1 / 20) + 1e-3


def test_ingest_endpoint_and_errors(worker, store):
    rng = np.random.default_rng(2)
    store.put("voice-private/voices/v/recordings/a.wav", wav.encode(recording(rng, [(120, 4), (120, 3)]), RATE))
    out = json.loads(post(worker + "/v1/ingest", {"audio_key": "voice-private/voices/v/recordings/a.wav",
                                                  "segment_prefix": "voice-private/voices/v/segments/a"}).read())
    assert len(out["segments"]) == 2 and out["report"]["speaker_check"].startswith("majority")
    with pytest.raises(urllib.error.HTTPError) as e:
        post(worker + "/v1/ingest", {"audio_key": "voice-private/nope.wav", "segment_prefix": "voice-private/x"})
    assert e.value.code == 404
