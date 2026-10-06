"""Tests for the container health probe.

The probe exists because an earlier inline `python -c` in the Dockerfile got
401 from /v1/health: the worker authenticates every endpoint, and urllib raises
HTTPError on non-2xx rather than returning a response, so the probe both failed
to authenticate and could never have reported a non-200 status. A worker that
is fine would have been marked permanently unhealthy and pulled from rotation.

These tests pin both halves of that: the token is sent, and a non-200 is a
failure rather than an exception.
"""

from __future__ import annotations

import http.server
import threading

import pytest

from icf_worker import healthcheck


class Handler(http.server.BaseHTTPRequestHandler):
    """Minimal stand-in that mimics the worker's auth-then-status behaviour."""

    token = "secret"
    status = 200

    def do_GET(self):  # noqa: N802 - http.server naming
        if self.path != "/v1/health":
            self.send_response(404)
            self.end_headers()
            return
        if self.headers.get("Authorization") != f"Bearer {self.token}":
            self.send_response(401)
            self.end_headers()
            self.wfile.write(b'{"error":{"class":"permanent","message":"unauthorized"}}')
            return
        self.send_response(self.status)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(b'{"status":"ok"}' if self.status == 200 else b'{"status":"unavailable"}')

    def log_message(self, *args):  # keep test output readable
        pass


@pytest.fixture
def server():
    Handler.token = "secret"
    Handler.status = 200
    srv = http.server.HTTPServer(("127.0.0.1", 0), Handler)
    t = threading.Thread(target=srv.serve_forever, daemon=True)
    t.start()
    yield srv.server_address[1]
    srv.shutdown()
    srv.server_close()


def test_healthy_worker_passes(server, monkeypatch):
    monkeypatch.setenv("ICF_WORKER_PORT", str(server))
    monkeypatch.setenv("ICF_WORKER_TOKEN", "secret")
    assert healthcheck.probe() == 0


def test_missing_token_fails(server, monkeypatch):
    """The original bug: no bearer token means 401, which must be a failure."""
    monkeypatch.setenv("ICF_WORKER_PORT", str(server))
    monkeypatch.setenv("ICF_WORKER_TOKEN", "")
    assert healthcheck.probe() == 1


def test_wrong_token_fails(server, monkeypatch):
    monkeypatch.setenv("ICF_WORKER_PORT", str(server))
    monkeypatch.setenv("ICF_WORKER_TOKEN", "wrong")
    assert healthcheck.probe() == 1


def test_engine_unhealthy_503_fails(server, monkeypatch):
    """503 means the model could not load - a real readiness signal."""
    Handler.status = 503
    monkeypatch.setenv("ICF_WORKER_PORT", str(server))
    monkeypatch.setenv("ICF_WORKER_TOKEN", "secret")
    assert healthcheck.probe() == 1


def test_nothing_listening_fails(monkeypatch):
    monkeypatch.setenv("ICF_WORKER_PORT", "1")
    monkeypatch.setenv("ICF_WORKER_TOKEN", "secret")
    assert healthcheck.probe() == 1


def test_probe_returns_int_not_raises(server, monkeypatch):
    """A probe that raises leaves the container's health to accident."""
    monkeypatch.setenv("ICF_WORKER_PORT", str(server))
    monkeypatch.setenv("ICF_WORKER_TOKEN", "wrong")
    assert isinstance(healthcheck.probe(), int)
