import json
import math
import threading
import urllib.error
import urllib.request
from http.server import ThreadingHTTPServer

import numpy as np
import pytest

from icf_worker import mastering, wav
from icf_worker.engines import DevToneEngine, EngineError, resolve_checkpoint
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
