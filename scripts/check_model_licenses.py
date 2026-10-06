#!/usr/bin/env python3
"""Verify the model licence register before it is trusted at runtime.

`docs/model_licenses.json` is the only thing standing between an uncleared TTS
model and a production deployment: `icf_worker.licenses.check` refuses to load,
infer or train on an engine whose entry has `production_allowed: false`, and
raises SystemExit when ICF_ENV=production.

That makes the register load-bearing, and a load-bearing file of hand-edited
JSON needs a checker. Two failure modes matter:

  * A malformed or incomplete entry, which the runtime gate reads as
    "not cleared" and so fails safely - annoying, but safe.
  * An entry flipped to `production_allowed: true` without the evidence the
    register's own header demands (who verified, when, against which commit and
    weights hash). This one fails *unsafely*: the worker starts, synthesises
    with a model nobody cleared, and the platform is distributing audio it may
    have no right to distribute.

This script makes the second mode impossible to merge by accident.

Usage:
    python scripts/check_model_licenses.py             # report
    python scripts/check_model_licenses.py --strict    # CI: fail on bad evidence
    python scripts/check_model_licenses.py --gate      # CI: runtime gate still refuses

Exit status is 0 when the register is sound, 1 otherwise.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_REGISTER = ROOT / "docs" / "model_licenses.json"

# The register's header requires the exact checkpoint to be identified, and a
# checkpoint is identified by its weights hash. Anything else is a guess.
SHA256_RE = re.compile(r"^[0-9a-f]{64}$")

# An engine whose role is "never linked, benchmark only" is not expected to
# carry checkpoints, so it is exempt from the checkpoint-evidence rule - but it
# is never exempt from production_allowed=false.
CHECKPOINT_EXEMPT_ROLE_MARKERS = ("offline", "benchmark", "tool only")

REQUIRED_ENGINE_FIELDS = ("engine", "role", "production_allowed")
REQUIRED_CHECKPOINT_FIELDS = (
    "name",
    "source",
    "weights_sha256",
    "weights_license",
    "commercial_use_verified",
    "verified_by",
    "verified_at",
)


class Report:
    """Collects errors and notes so a run reports everything, not just the first."""

    def __init__(self) -> None:
        self.errors: list[str] = []
        self.notes: list[str] = []

    def error(self, msg: str) -> None:
        self.errors.append(msg)

    def note(self, msg: str) -> None:
        self.notes.append(msg)

    def emit(self) -> None:
        for n in self.notes:
            print(f"  note   {n}")
        for e in self.errors:
            print(f"  ERROR  {e}", file=sys.stderr)


def is_blank(value: object) -> bool:
    """The register uses null for 'not verified'; treat empty strings the same."""
    if value is None:
        return True
    if isinstance(value, str) and not value.strip():
        return True
    return False


def load_register(path: Path) -> dict:
    try:
        return json.loads(path.read_text())
    except OSError as e:
        raise SystemExit(f"cannot read licence register {path}: {e}")
    except ValueError as e:
        raise SystemExit(f"licence register {path} is not valid JSON: {e}")


def check_checkpoint(engine: str, idx: int, ckpt: dict, rep: Report) -> bool:
    """Return True when a checkpoint carries full commercial-use evidence."""
    label = f"{engine}.checkpoints[{idx}]"
    if not isinstance(ckpt, dict):
        rep.error(f"{label} is not an object")
        return False

    missing = [f for f in REQUIRED_CHECKPOINT_FIELDS if f not in ckpt]
    if missing:
        rep.error(f"{label} is missing field(s): {', '.join(missing)}")

    blank = [f for f in REQUIRED_CHECKPOINT_FIELDS if is_blank(ckpt.get(f))]
    if blank:
        rep.error(f"{label} has unverified (null/empty): {', '.join(blank)}")

    sha = ckpt.get("weights_sha256")
    if not is_blank(sha) and not SHA256_RE.match(str(sha)):
        rep.error(f"{label}.weights_sha256 is not a 64-char lowercase hex digest: {sha!r}")

    # Must be a literal true, not a truthy string: "no" is truthy in Python.
    if ckpt.get("commercial_use_verified") is not True:
        rep.error(
            f"{label}.commercial_use_verified must be exactly true "
            f"(got {ckpt.get('commercial_use_verified')!r})"
        )
        return False

    return not blank


def check_register(data: dict, strict: bool, rep: Report) -> None:
    if data.get("schema_version") != 1:
        rep.error(f"unsupported schema_version {data.get('schema_version')!r} (want 1)")

    engines = data.get("engines")
    if not isinstance(engines, list) or not engines:
        rep.error("register has no 'engines' list")
        return

    seen: set[str] = set()
    cleared: list[str] = []

    for entry in engines:
        if not isinstance(entry, dict):
            rep.error(f"engine entry is not an object: {entry!r}")
            continue

        name = entry.get("engine")
        if is_blank(name):
            rep.error(f"engine entry has no 'engine' name: {sorted(entry)[:4]}...")
            continue
        name = str(name)

        if name in seen:
            rep.error(f"duplicate engine entry {name!r}")
        seen.add(name)

        missing = [f for f in REQUIRED_ENGINE_FIELDS if f not in entry]
        if missing:
            rep.error(f"{name} is missing field(s): {', '.join(missing)}")

        allowed = entry.get("production_allowed")
        if allowed is not True and allowed is not False:
            rep.error(f"{name}.production_allowed must be exactly true or false (got {allowed!r})")
            continue

        ckpts = entry.get("checkpoints")
        if not isinstance(ckpts, list):
            ckpts = []

        if allowed is False:
            rep.note(f"{name}: production_allowed=false (blocked) - {len(ckpts)} checkpoint(s) recorded")
            continue

        # ---- production_allowed: true. Demand the evidence. ----
        cleared.append(name)

        if is_blank(entry.get("code_license_reported")):
            rep.error(f"{name} is cleared for production but code_license_reported is unverified")
        if is_blank(entry.get("code_license_verified_commit")):
            rep.error(
                f"{name} is cleared for production but code_license_verified_commit is unset; "
                f"record the exact commit the licence was read against"
            )

        role = str(entry.get("role") or "").lower()
        exempt = any(m in role for m in CHECKPOINT_EXEMPT_ROLE_MARKERS)

        if exempt:
            # A benchmark-only tool has no weights, but it also must never be
            # cleared: clearing it would let AGPL code near the service.
            rep.error(
                f"{name} is an offline/benchmark-only tool and must not be production_allowed=true"
            )
            continue

        if not ckpts:
            rep.error(
                f"{name} is cleared for production but records no checkpoint; "
                f"the licence attaches to specific weights, not to the repo"
            )
            continue

        if not any(check_checkpoint(name, i, c, rep) for i, c in enumerate(ckpts)):
            rep.error(
                f"{name} is cleared for production but no checkpoint carries complete "
                f"commercial-use evidence (name, source, weights_sha256, weights_license, "
                f"commercial_use_verified, verified_by, verified_at)"
            )

    if cleared and strict:
        rep.note(f"cleared for production: {', '.join(cleared)}")
    elif not cleared:
        rep.note("no engine is cleared for production; the worker cannot run with ICF_ENV=production")


def check_gate(register_path: Path, rep: Report) -> None:
    """Assert the runtime gate still refuses every uncleared engine in production.

    A future edit that makes `licenses.check` permissive is a legal exposure
    rather than a bug, so it is worth an explicit assertion.
    """
    sys.path.insert(0, str(ROOT / "voice-engine"))
    os.environ["ICF_ENV"] = "production"
    os.environ.setdefault("ICF_LICENSE_REGISTER", str(register_path))

    try:
        from icf_worker import licenses  # noqa: PLC0415
    except ImportError as e:
        rep.error(f"cannot import icf_worker.licenses for the gate check: {e}")
        return

    register = licenses.load_register(register_path)
    if not register:
        rep.error("gate check found an empty register")
        return

    for name, entry in sorted(register.items()):
        if entry.get("production_allowed"):
            rep.note(f"gate check: {name} is cleared, so refusal is not expected")
            continue
        try:
            licenses.check(name, "inference", register=register)
        except SystemExit:
            continue
        except Exception as e:  # noqa: BLE001 - any escape still counts as a refusal
            rep.note(f"gate check: {name} raised {type(e).__name__} rather than SystemExit")
            continue
        rep.error(
            f"gate check FAILED: {name} has production_allowed=false but "
            f"licenses.check() did not refuse it under ICF_ENV=production"
        )


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--register", default=os.environ.get("ICF_LICENSE_REGISTER", str(DEFAULT_REGISTER)))
    ap.add_argument("--strict", action="store_true", help="fail on incomplete production evidence")
    ap.add_argument("--gate", action="store_true", help="assert the runtime gate refuses uncleared engines")
    args = ap.parse_args(argv)

    path = Path(args.register)
    print(f"licence register: {path}")
    data = load_register(path)

    rep = Report()
    check_register(data, args.strict or args.gate, rep)
    if args.gate:
        check_gate(path, rep)

    rep.emit()

    engines = data.get("engines") or []
    print(f"engines checked: {len(engines)}")
    if rep.errors:
        print(f"FAILED: {len(rep.errors)} problem(s)", file=sys.stderr)
        return 1
    print("OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
