#!/usr/bin/env python3
"""Fail when a dependency audit reports a finding the baseline does not know.

`pip-audit --strict` is the right gate for the worker's own dependencies: they
are few, resolved, and clean. It is the wrong gate for the engine manifests
(VE-014), because those mirror an upstream project whose pins already carry known
advisories — most of them in the demo and training stack, which is why that stack
is excluded from the manifest. Turning the step red on 91 inherited findings
would make the gate meaningless, and turning it non-failing would make it
decoration.

So the known set is enumerated, checked in, and visible, and this script fails on
anything else. A new advisory against a pinned engine dependency is a real signal
and stops the build; the inherited ones are triaged once, in writing, where a
reviewer can see exactly what was accepted and why.

Usage:
    pip-audit --no-deps --disable-pip --format=json -r <manifest> -o audit.json
    python3 scripts/check_engine_audit.py <audit.json> [more audit.json ...]

Exit codes: 0 when every finding is baselined, 1 when something is new or the
baseline is malformed.
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

BASELINE = Path(__file__).resolve().parents[1] / "voice-engine" / "engine-audit-baseline.json"


def load_baseline(path: Path = BASELINE) -> set[tuple[str, str]]:
    try:
        data = json.loads(path.read_text())
    except FileNotFoundError:
        raise SystemExit(f"baseline not found: {path}") from None
    except json.JSONDecodeError as e:
        raise SystemExit(f"baseline is not valid JSON: {e}") from None
    entries = data.get("accepted", [])
    if not isinstance(entries, list):
        raise SystemExit("baseline: 'accepted' must be a list")
    out = set()
    for e in entries:
        if not isinstance(e, dict) or "package" not in e or "id" not in e:
            raise SystemExit(f"baseline entry is missing package/id: {e!r}")
        if not str(e.get("reason", "")).strip():
            raise SystemExit(f"baseline entry has no reason recorded: {e!r}")
        out.add((e["package"].lower(), e["id"]))
    return out


def findings(doc: dict) -> list[tuple[str, str, str]]:
    out = []
    for dep in doc.get("dependencies", []):
        name = (dep.get("name") or "").lower()
        for v in dep.get("vulns", []) or []:
            out.append((name, v.get("id", ""), v.get("fix_versions") and ", ".join(v["fix_versions"]) or "no fix released"))
    return out


def main(argv: list[str]) -> int:
    if len(argv) < 2:
        raise SystemExit(__doc__)
    baseline = load_baseline()
    new: list[tuple[str, str, str]] = []
    total = 0
    for arg in argv[1:]:
        doc = json.loads(Path(arg).read_text())
        for package, vid, fix in findings(doc):
            total += 1
            if (package, vid) not in baseline:
                new.append((package, vid, fix))
    if new:
        print(f"::error::{len(new)} dependency finding(s) outside the accepted baseline:")
        for package, vid, fix in sorted(set(new)):
            print(f"  {package}: {vid} (fix: {fix})")
        print("\nIf the finding is understood and accepted, add it to "
              "voice-engine/engine-audit-baseline.json with a reason - the point of the file is that "
              "accepting a vulnerability is a decision somebody makes on the record.")
        return 1
    print(f"engine dependency audit: {total} finding(s), all in the accepted baseline")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
