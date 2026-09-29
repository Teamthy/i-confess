"""GPU worker HTTP server implementing the voice-engine wire protocol.

    GET  /v1/health        -> 200 {"status":"ok"} | 503
    GET  /v1/capabilities  -> capabilities JSON
    POST /v1/synthesize    <- GenerateRequest JSON -> mastered audio/wav
    POST /v1/clone         <- CloneRequest JSON  -> {"speaker_handle": ...}

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

from . import mastering, wav
from .engines import Engine, EngineError, load_engine, resolve_checkpoint

log = logging.getLogger("icf_worker")
MAX_BODY = 2 << 20


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
    y, rep = mastering.master(x, rate)
    body = wav.encode(y, rate)
    headers = {
        "Content-Type": "audio/wav",
        "X-Sample-Rate": str(rate),
        "X-Duration-Ms": str(int(1000 * y.size / rate)),
        "X-Engine-Version": f"{engine.name}-{engine.version}",
        "X-Loudness-LUFS": f"{rep.output_lufs:.2f}",
        "X-True-Peak-dBTP": f"{rep.output_true_peak_dbtp:.2f}",
        "X-Peak-Limited": "1" if rep.peak_limited else "0",
        # Provenance travels with the bytes (Go also writes it to the DB).
        "X-Synthetic": "true",
        "X-Audio-SHA256": hashlib.sha256(body).hexdigest(),
    }
    return body, headers


def make_handler(engine: Engine, token: str):
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
            if self.path == "/v1/capabilities":
                caps = dict(engine.capabilities, engine=engine.name, engine_version=engine.version)
                return self._json(200, caps)
            self._err(404, "permanent", "not found")

        def do_POST(self):
            if not self._authorized():
                return self._err(401, "permanent", "unauthorized")
            n = int(self.headers.get("Content-Length") or 0)
            if n <= 0 or n > MAX_BODY:
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
                if self.path == "/v1/clone":
                    ref = req.get("reference") or {}
                    resolve_checkpoint(ref.get("uri", ""))  # must exist locally
                    handle = hashlib.sha256(f"{req.get('voice_id')}|{ref.get('id')}".encode()).hexdigest()[:24]
                    return self._json(200, {"speaker_handle": handle})
                self._err(404, "permanent", "not found")
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
    srv = ThreadingHTTPServer((args.host, args.port), make_handler(engine, token))
    log.info("voice worker %s listening on %s:%d", engine.name, args.host, args.port)
    srv.serve_forever()


if __name__ == "__main__":
    main()
