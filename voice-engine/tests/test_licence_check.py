"""Regression tests for scripts/check_model_licenses.py.

That script is what stops an uncleared TTS model reaching production: it fails
CI when an engine is marked production_allowed=true without the checkpoint
evidence the licence register demands, and it asserts that the runtime gate in
icf_worker.licenses still refuses an uncleared engine.

A checker that has only ever been seen to pass proves nothing, so these tests
assert the failures it exists to produce. Each bad-register case below was
produced by hand and confirmed to exit non-zero before being written here.
"""

from __future__ import annotations

import hashlib
import importlib.util
import json
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts" / "check_model_licenses.py"
REAL_REGISTER = ROOT / "docs" / "model_licenses.json"

GOOD_SHA = hashlib.sha256(b"checkpoint-bytes").hexdigest()


def load_checker():
    spec = importlib.util.spec_from_file_location("check_model_licenses", SCRIPT)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


chk = load_checker()


def run(data: dict, tmp_path: Path, gate: bool = False) -> tuple[int, list[str]]:
    """Write a register, check it, return (exit_code, errors)."""
    path = tmp_path / "model_licenses.json"
    path.write_text(json.dumps(data))
    rep = chk.Report()
    chk.check_register(data, True, rep)
    if gate:
        chk.check_gate(path, rep)
    return (1 if rep.errors else 0), rep.errors


def engine(**over) -> dict:
    base = {
        "engine": "cosyvoice",
        "role": "primary zero-shot candidate",
        "code_license_reported": "Apache-2.0",
        "code_license_verified_commit": None,
        "checkpoints": [
            {
                "name": None,
                "source": None,
                "weights_sha256": None,
                "weights_license": None,
                "commercial_use_verified": None,
                "verified_by": None,
                "verified_at": None,
            }
        ],
        "production_allowed": False,
    }
    base.update(over)
    return base


def cleared_checkpoint() -> dict:
    return {
        "name": "CosyVoice2-0.5B",
        "source": "hf:FunAudioLLM/CosyVoice2-0.5B",
        "weights_sha256": GOOD_SHA,
        "weights_license": "Apache-2.0",
        "commercial_use_verified": True,
        "verified_by": "counsel@example.com",
        "verified_at": "2026-10-01",
    }


def test_script_and_register_exist():
    assert SCRIPT.exists(), "scripts/check_model_licenses.py is missing"
    assert REAL_REGISTER.exists(), "docs/model_licenses.json is missing"


def test_shipped_register_is_valid_and_blocked():
    """The register in the repo must pass, and must not clear any engine."""
    data = json.loads(REAL_REGISTER.read_text())
    rep = chk.Report()
    chk.check_register(data, True, rep)
    assert rep.errors == [], f"shipped register has problems: {rep.errors}"
    assert all(e["production_allowed"] is False for e in data["engines"]), (
        "an engine is cleared for production in the committed register; that must "
        "be a deliberate, counsel-verified change, not a drive-by edit"
    )


def test_shipped_register_gate_refuses_every_engine(tmp_path):
    """With the shipped register, the runtime gate must refuse all four engines."""
    exit_code, errors = run(json.loads(REAL_REGISTER.read_text()), tmp_path, gate=True)
    assert errors == [], errors
    assert exit_code == 0


def test_flip_without_evidence_is_rejected(tmp_path):
    """The case that matters: someone flips the flag and changes nothing else."""
    data = {"schema_version": 1, "engines": [engine(production_allowed=True)]}
    exit_code, errors = run(data, tmp_path)
    assert exit_code == 1
    assert any("code_license_verified_commit" in e for e in errors)
    assert any("no checkpoint carries complete commercial-use evidence" in e for e in errors)


def test_fully_cleared_engine_is_accepted(tmp_path):
    data = {
        "schema_version": 1,
        "engines": [
            engine(
                production_allowed=True,
                code_license_verified_commit="a1b2c3d4",
                checkpoints=[cleared_checkpoint()],
            )
        ],
    }
    exit_code, errors = run(data, tmp_path)
    assert errors == [], errors
    assert exit_code == 0


def test_agpl_benchmark_tool_can_never_be_cleared(tmp_path):
    """VoiceStudio is AGPL and offline-only; clearing it would be a legal bug."""
    data = {
        "schema_version": 1,
        "engines": [
            {
                "engine": "voicestudio",
                "role": "offline development and benchmarking tool only",
                "code_license_reported": "AGPL-3.0",
                "code_license_verified_commit": "abc123",
                "checkpoints": [],
                "production_allowed": True,
            }
        ],
    }
    exit_code, errors = run(data, tmp_path)
    assert exit_code == 1
    assert any("offline/benchmark-only tool" in e for e in errors)


def test_malformed_checkpoint_hash_is_rejected(tmp_path):
    ckpt = cleared_checkpoint()
    ckpt["weights_sha256"] = "not-a-real-hash"
    data = {
        "schema_version": 1,
        "engines": [
            engine(
                production_allowed=True,
                code_license_verified_commit="abc123",
                checkpoints=[ckpt],
            )
        ],
    }
    exit_code, errors = run(data, tmp_path)
    assert exit_code == 1
    assert any("not a 64-char lowercase hex digest" in e for e in errors)


def test_truthy_string_is_not_a_verified_true(tmp_path):
    """'yes' is truthy in Python; only a literal true counts as verified."""
    ckpt = cleared_checkpoint()
    ckpt["commercial_use_verified"] = "yes"
    data = {
        "schema_version": 1,
        "engines": [
            engine(
                production_allowed=True,
                code_license_verified_commit="abc123",
                checkpoints=[ckpt],
            )
        ],
    }
    exit_code, errors = run(data, tmp_path)
    assert exit_code == 1
    assert any("must be exactly true" in e for e in errors)


def test_missing_production_allowed_is_rejected(tmp_path):
    data = {"schema_version": 1, "engines": [{"engine": "cosyvoice", "role": "x"}]}
    _, errors = run(data, tmp_path)
    assert any("production_allowed must be exactly true or false" in e for e in errors)


def test_duplicate_engine_entries_are_rejected(tmp_path):
    data = {"schema_version": 1, "engines": [engine(), engine()]}
    _, errors = run(data, tmp_path)
    assert any("duplicate engine entry" in e for e in errors)


def test_neutered_runtime_gate_is_detected(tmp_path, monkeypatch):
    """If licenses.check() stops refusing, that is a legal exposure, not a bug.

    Simulated by monkeypatching check() to a no-op, which is exactly what a
    careless future edit would amount to.
    """
    import sys

    sys.path.insert(0, str(ROOT / "voice-engine"))
    from icf_worker import licenses

    monkeypatch.setenv("ICF_ENV", "production")
    monkeypatch.setattr(licenses, "check", lambda *a, **k: None)

    path = tmp_path / "model_licenses.json"
    path.write_text(json.dumps(json.loads(REAL_REGISTER.read_text())))
    rep = chk.Report()
    chk.check_gate(path, rep)

    assert rep.errors, "a permissive gate was not detected"
    assert any("did not refuse it" in e for e in rep.errors)


@pytest.mark.parametrize("bad", [None, "", "   "])
def test_blank_treated_as_unverified(bad):
    assert chk.is_blank(bad) is True


def test_non_blank_values_are_not_unverified():
    assert chk.is_blank("Apache-2.0") is False
    assert chk.is_blank(0) is False
