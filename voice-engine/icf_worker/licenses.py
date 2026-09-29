"""Licence gate backed by docs/model_licenses.json.

In production (ICF_ENV=production) an engine may be loaded, used for
inference, or used for training only when its register entry has
"production_allowed": true. That flag should be set only after the code AND
the exact checkpoint licences have been verified (see docs/MODEL_LICENSES.md).
Outside production the gate logs a warning and allows the engine, so that
engineers can benchmark.
"""

from __future__ import annotations

import json
import logging
import os
from pathlib import Path

log = logging.getLogger("icf_worker.licenses")

DEFAULT_REGISTER = Path(__file__).resolve().parents[2] / "docs" / "model_licenses.json"


class LicenseError(SystemExit):
    pass


def load_register(path: str | os.PathLike | None = None) -> dict:
    p = Path(path or os.environ.get("ICF_LICENSE_REGISTER", DEFAULT_REGISTER))
    try:
        data = json.loads(p.read_text())
    except (OSError, ValueError) as e:
        raise LicenseError(f"cannot read licence register {p}: {e}")
    return {e["engine"]: e for e in data.get("engines", [])}


def check(engine: str, purpose: str = "inference", register: dict | None = None) -> None:
    reg = register if register is not None else load_register()
    entry = reg.get(engine)
    production = os.environ.get("ICF_ENV") == "production"
    if entry is None:
        msg = f"engine {engine!r} has no entry in the licence register"
        if production:
            raise LicenseError(msg)
        log.warning("%s (allowed outside production)", msg)
        return
    if not entry.get("production_allowed"):
        msg = f"engine {engine!r} is not cleared for production {purpose} (production_allowed=false)"
        if production:
            raise LicenseError(msg)
        log.warning("%s; allowed because ICF_ENV is not production", msg)
