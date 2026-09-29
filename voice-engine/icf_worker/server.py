"""GPU worker HTTP server implementing the voice-engine wire protocol.

    GET  /v1/health        -> 200 {"status":"ok"} | 503
    GET  /v1/capabilities  -> capabilities JSON
    POST /v1/synthesize    <- GenerateRequest JSON -> mastered audio/wav
    POST /v1/synthesize/stream <- GenerateRequest -> streamed audio/wav (unknown length)
    POST /v1/clone         <- CloneRequest JSON  -> {"speaker_handle": ...}
    POST /v1/ingest        <- IngestRequest -> segments + report (see ingest.py)
    POST /v1/train         <- TrainRequest  -> 202          (see train.py)
    GET  /v1/train/{id}    -> run status
    POST /v1/train/{id}/cancel

The Go API never performs inference: it enqueues a job, and its queue worker
calls this server. Only the Go side authorizes rights; this process trusts a
shared bearer token (ICF_WORKER_TOKEN) and must not be exposed publicly.

Run:  python -m icf_worker.server --engine cosyvoice --port 8601
"""

from __future__ import annotations

import argparse
import hashlib
import hmac
import json
import logging
import os
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import numpy as np

import math
import re
import struct

from . import ingest as ingest_mod
import base64

from . import dsp, encode as encode_mod, mastering, wav
from .engines import Engine, EngineError, load_engine, resolve_checkpoint
from .storage import StorageError
from .train import TrainingError, TrainingManager

log = logging.getLogger("icf_worker")
MAX_BODY = 2 << 20
# /v1/encode carries a whole WAV master (base64): allow up to ~160 MB.
ENCODE_MAX_BODY = int(os.environ.get("ICF_ENCODE_MAX_BODY", str(160 << 20)))


def render(engine: Engine, req: dict) -> tuple[bytes, dict]:
    chunks = req.get("chunks") or []
    if not chunks:
        raise EngineError("content", "no chunks to synthesize")
    checkpoint = resolve_checkpoint(req.get("checkpoint_uri", ""))
    params = dict(req.get("params") or {})
    rate = engine.sample_rate
    parts: list[np.ndarray] = []
    for c in chunks:
        text = (c.get("text") or "").strip()
        if text:
            audio = engine.synthesize_chunk(text, reference=req.get("reference"), checkpoint=checkpoint,
                                            params=params, chunk=c)
            rate = engine.sample_rate
            parts.append(np.asarray(audio, dtype=np.float64))
        pause = int(c.get("silence_after_ms") or 0)
        if pause > 0:
            parts.append(np.zeros(int(rate * min(pause, 5000) / 1000)))
    x = np.concatenate(parts) if parts else np.zeros(0)
    if x.size == 0:
        raise EngineError("model", "engine produced no audio")
    y, chain = dsp.process(x, rate)
    rep = chain.master
    body = wav.encode(y, rate)
    headers = {
        "Content-Type": "audio/wav",
        "X-Sample-Rate": str(rate),
        "X-Duration-Ms": str(int(1000 * y.size / rate)),
        "X-Engine-Version": f"{engine.name}-{engine.version}",
        "X-Loudness-LUFS": f"{rep.output_lufs:.2f}",
        "X-True-Peak-dBTP": f"{rep.output_true_peak_dbtp:.2f}",
        "X-Peak-Limited": "1" if rep.peak_limited else "0",
        "X-DSP-Chain": ",".join(chain.stages),
        "X-Limiter-Max-dB": f"{chain.limiter_max_db:.2f}",
        "X-Trimmed-Ms": f"{chain.trimmed_ms:.0f}",
        # Provenance travels with the bytes (Go also writes it to the DB).
        "X-Synthetic": "true",
        "X-Audio-SHA256": hashlib.sha256(body).hexdigest(),
    }
    return body, headers


def _stream_header(rate: int) -> bytes:
    """WAV header with 0xFFFFFFFF sizes: 'length unknown', which players accept."""
    return (b"RIFF" + struct.pack("<I", 0xFFFFFFFF) + b"WAVEfmt " + struct.pack("<IHHIIHH", 16, 1, 1, rate, rate * 2, 2, 16)
            + b"data" + struct.pack("<I", 0xFFFFFFFF))


def render_stream(engine: Engine, req: dict):
    """Yield WAV bytes chunk by chunk.

    Whole-file loudness mastering is impossible before the audio exists, so
    the gain is set from the first chunk's loudness (clamped to +/-20 dB), and
    each chunk is scaled down if its sample peak would exceed the ceiling. The
    cached, queued render remains the canonical mastered asset.
    """
    chunks = req.get("chunks") or []
    if not chunks:
        raise EngineError("content", "no chunks to synthesize")
    checkpoint = resolve_checkpoint(req.get("checkpoint_uri", ""))
    params = dict(req.get("params") or {})
    target, ceiling = mastering.config_from_env()
    ceiling_lin = 10 ** (ceiling / 20)
    gain, header_sent = None, False
    for c in chunks:
        text = (c.get("text") or "").strip()
        parts = []
        if text:
            parts.append(np.asarray(engine.synthesize_chunk(text, reference=req.get("reference"), checkpoint=checkpoint,
                                                            params=params, chunk=c), dtype=np.float64))
        rate = engine.sample_rate
        pause = int(c.get("silence_after_ms") or 0)
        if pause > 0:
            parts.append(np.zeros(int(rate * min(pause, 5000) / 1000)))
        if not parts:
            continue
        x = np.concatenate(parts)
        if gain is None and text:
            lin = mastering.integrated_loudness(x, rate)
            gain = 0.0 if not math.isfinite(lin) else float(np.clip(target - lin, -20, 20))
        y = x * (10 ** ((gain or 0.0) / 20))
        peak = float(np.max(np.abs(y))) if y.size else 0.0
        if peak > ceiling_lin:
            y *= ceiling_lin / peak
        if not header_sent:
            yield _stream_header(rate)
            header_sent = True
        yield (np.clip(y, -1, 1) * 32767).astype("<i2").tobytes()


TRAIN_PATH = re.compile(r"^/v1/train/([A-Za-z0-9_-]{1,80})(/cancel)?$")


def make_handler(engine: Engine, token: str, trainer: TrainingManager | None = None):
    trainer = trainer or TrainingManager()

    class Handler(BaseHTTPRequestHandler):
        server_version = "icf-voice-worker/1"

        def log_message(self, fmt, *args):  # route through logging, never log bodies
            log.info("%s %s", self.address_string(), fmt % args)

        def _json(self, code: int, obj: dict):
            b = json.dumps(obj).encode()
            self.send_response(code)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(b)))
            self.end_headers()
            self.wfile.write(b)

        def _err(self, code: int, cls: str, msg: str):
            self._json(code, {"error": {"class": cls, "message": msg}})

        def _authorized(self) -> bool:
            if not token:
                return True  # dev only; main() refuses this in production
            got = self.headers.get("Authorization", "")
            return hmac.compare_digest(got.encode(), f"Bearer {token}".encode())

        def do_GET(self):
            if not self._authorized():
                return self._err(401, "permanent", "unauthorized")
            if self.path == "/v1/health":
                ok = engine.healthy()
                return self._json(200 if ok else 503, {"status": "ok" if ok else "unavailable"})
            m = TRAIN_PATH.match(self.path)
            if m and not m.group(2):
                run = trainer.get(m.group(1))
                if not run:
                    return self._err(404, "permanent", "unknown run")
                return self._json(200, run.view())
            if self.path == "/v1/capabilities":
                caps = dict(engine.capabilities, engine=engine.name, engine_version=engine.version)
                # /v1/synthesize/stream works for every engine: it emits audio
                # chunk by chunk even when the engine itself can't stream inside
                # a chunk. The granularity tells callers which of the two they get.
                caps["streaming_granularity"] = "native" if caps.get("streaming") else "chunk"
                caps["streaming"] = True
                return self._json(200, caps)
            self._err(404, "permanent", "not found")

        def do_POST(self):
            if not self._authorized():
                return self._err(401, "permanent", "unauthorized")
            n = int(self.headers.get("Content-Length") or 0)
            limit = ENCODE_MAX_BODY if self.path == "/v1/encode" else MAX_BODY
            if n <= 0 or n > limit:
                return self._err(413, "content", "request body missing or too large")
            try:
                req = json.loads(self.rfile.read(n))
            except ValueError:
                return self._err(400, "content", "invalid JSON")
            try:
                if self.path == "/v1/synthesize":
                    body, headers = render(engine, req)
                    self.send_response(200)
                    for k, v in headers.items():
                        self.send_header(k, v)
                    self.send_header("Content-Length", str(len(body)))
                    self.end_headers()
                    self.wfile.write(body)
                    return
                if self.path == "/v1/synthesize/stream":
                    gen = render_stream(engine, req)
                    first = next(gen, None)  # surface engine errors before headers
                    if first is None:
                        return self._err(422, "content", "nothing to synthesize")
                    self.send_response(200)
                    self.send_header("Content-Type", "audio/wav")
                    self.send_header("X-Sample-Rate", str(engine.sample_rate))
                    self.send_header("X-Synthetic", "true")
                    self.send_header("Cache-Control", "no-store")
                    self.end_headers()
                    try:
                        self.wfile.write(first)
                        for b in gen:
                            self.wfile.write(b)
                            self.wfile.flush()
                    except (BrokenPipeError, ConnectionResetError):
                        log.info("stream client disconnected; stopping synthesis")
                    self.close_connection = True
                    return
                if self.path == "/v1/ingest":
                    for k in ("audio_key", "segment_prefix"):
                        if not req.get(k):
                            return self._err(400, "permanent", f"{k} is required")
                    res = ingest_mod.ingest(req)
                    return self._json(200, {"duration_ms": res.duration_ms, "segments": res.segments,
                                            "report": res.report})
                if self.path == "/v1/encode":
                    formats = req.get("formats") or ["aac", "opus", "mp3"]
                    if req.get("audio_b64"):
                        try:
                            master = base64.b64decode(req["audio_b64"], validate=True)
                        except ValueError:
                            return self._err(400, "permanent", "audio_b64 is not valid base64")
                        out = {}
                        for fmt, (data, ctype, ext) in encode_mod.encode_bytes(master, formats).items():
                            out[fmt] = {"b64": base64.b64encode(data).decode(), "sha256": hashlib.sha256(data).hexdigest(),
                                        "bytes": len(data), "content_type": ctype, "ext": ext}
                        return self._json(200, {"variants": out})
                    if req.get("key"):
                        return self._json(200, {"variants": encode_mod.encode(req["key"], formats)})
                    return self._err(400, "permanent", "send audio_b64 or key")
                if self.path == "/v1/train":
                    trainer.submit(req)
                    return self._json(202, {"run_id": req.get("run_id"), "status": "RUNNING"})
                m = TRAIN_PATH.match(self.path)
                if m and m.group(2):
                    ok = trainer.cancel(m.group(1))
                    return self._json(200 if ok else 404, {"cancelled": ok})
                if self.path == "/v1/clone":
                    ref = req.get("reference") or {}
                    resolve_checkpoint(ref.get("uri", ""))  # must exist locally
                    handle = hashlib.sha256(f"{req.get('voice_id')}|{ref.get('id')}".encode()).hexdigest()[:24]
                    return self._json(200, {"speaker_handle": handle})
                self._err(404, "permanent", "not found")
            except TrainingError as e:
                code = {"content": 422, "permanent": 400, "model": 503}.get(e.cls, 500)
                self._err(code, e.cls, str(e))
            except StorageError as e:
                self._err(404, "storage", str(e))
            except ValueError as e:
                self._err(422, "content", str(e))
            except EngineError as e:
                code = {"content": 422, "permanent": 400, "model": 503, "gpu": 503}.get(e.cls, 500)
                self._err(code, e.cls, str(e))
            except MemoryError:
                self._err(503, "gpu", "out of memory")
            except Exception:  # noqa: BLE001 - classify, never leak a traceback
                log.exception("synthesis failed")
                self._err(500, "transient", "internal worker error")

    return Handler


def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--engine", default=os.environ.get("ICF_ENGINE", "cosyvoice"))
    ap.add_argument("--host", default=os.environ.get("ICF_WORKER_HOST", "0.0.0.0"))
    ap.add_argument("--port", type=int, default=int(os.environ.get("ICF_WORKER_PORT", "8601")))
    args = ap.parse_args(argv)
    logging.basicConfig(level=logging.INFO)
    token = os.environ.get("ICF_WORKER_TOKEN", "")
    if not token and os.environ.get("ICF_ENV") == "production":
        raise SystemExit("ICF_WORKER_TOKEN is required in production")
    engine = load_engine(args.engine)
    srv = ThreadingHTTPServer((args.host, args.port), make_handler(engine, token, TrainingManager()))
    log.info("voice worker %s listening on %s:%d", engine.name, args.host, args.port)
    srv.serve_forever()


if __name__ == "__main__":
    main()
