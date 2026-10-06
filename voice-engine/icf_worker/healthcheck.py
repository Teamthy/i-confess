"""Container health probe for the voice worker.

A separate module rather than an inline `python -c` in the Dockerfile for three
reasons, all learned the hard way:

  * The worker authenticates every endpoint, /v1/health included. A probe that
    does not send the bearer token gets 401 and the container is permanently
    unhealthy, which takes the pod out of rotation for a worker that is fine.
  * urllib raises HTTPError for any non-2xx instead of returning a response, so
    `urlopen(...).status == 200` never evaluates for the failure case it exists
    to detect - it raises, and the probe's exit code becomes an accident.
  * A 503 here means the engine could not load its model, which is a real
    readiness signal and must be distinguishable from "nobody is listening".

Exit 0 when the worker reports healthy, 1 otherwise.
"""

from __future__ import annotations

import os
import sys
import urllib.error
import urllib.request


def probe() -> int:
    port = os.environ.get("ICF_WORKER_PORT", "8601")
    host = os.environ.get("ICF_HEALTH_HOST", "127.0.0.1")
    url = f"http://{host}:{port}/v1/health"

    req = urllib.request.Request(url)
    token = os.environ.get("ICF_WORKER_TOKEN", "")
    if token:
        req.add_header("Authorization", f"Bearer {token}")

    try:
        with urllib.request.urlopen(req, timeout=8) as resp:  # noqa: S310 - fixed http URL
            if resp.status == 200:
                return 0
            print(f"unhealthy: /v1/health returned {resp.status}", file=sys.stderr)
            return 1
    except urllib.error.HTTPError as e:
        # 503 = engine unhealthy (model missing, licence gate). 401 = the probe
        # is misconfigured and the token does not match; that is an operator
        # problem, not a model problem, and it should not look like either.
        print(f"unhealthy: /v1/health returned {e.code} {e.reason}", file=sys.stderr)
        return 1
    except Exception as e:  # noqa: BLE001 - any failure means "not ready"
        print(f"unhealthy: cannot reach {url}: {e}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(probe())
