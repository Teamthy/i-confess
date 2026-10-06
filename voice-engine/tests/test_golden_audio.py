"""The golden-audio gate (audit VE-020) and its capture mode.

Run normally in CI: it replays tests/golden/manifest.json through the worker's
audio path and fails on any change. Run with ICF_GOLDEN_UPDATE=1 to re-capture:

    ICF_WORKER_DEV=1 ICF_GOLDEN_UPDATE=1 python -m pytest -q tests/test_golden_audio.py

Committing a re-capture means you *decided* the output should move: the diff the
test prints is the review artefact, and the commit message has to say why. A
silent re-capture turns this gate back into the thing it replaces.
"""

from __future__ import annotations

import copy
import os

import numpy as np
import pytest

from icf_worker import golden
from icf_worker.engines import DevToneEngine

UPDATE = os.environ.get("ICF_GOLDEN_UPDATE") == "1"


@pytest.fixture(scope="module")
def engine():
    return DevToneEngine()


@pytest.fixture(scope="module")
def prompts():
    return golden.load_prompts()


def _capture(engine, prompts):
    man = golden.capture(engine, prompts)
    if UPDATE:
        path = golden.write_manifest(man)
        pytest.skip(f"golden manifest rewritten at {path}; review the diff before committing")
    if not golden.MANIFEST_PATH.exists():
        pytest.fail("tests/golden/manifest.json is missing. Capture one with ICF_GOLDEN_UPDATE=1.")
    return golden.read_manifest()


def test_every_golden_prompt_has_a_case(engine, prompts):
    """Adding a prompt to the shared golden set must not slip past the worker.

    The gate is only worth having if it covers the set the benchmark covers, so
    the two lists are pinned against each other in both directions.
    """
    man = _capture(engine, prompts)
    have = {c["id"] for c in man["cases"]}
    want = {p["id"] for p in prompts}
    assert have == want, f"missing={sorted(want - have)} stale={sorted(have - want)}"


def test_golden_audio_matches_the_reference_manifest(engine, prompts):
    man = _capture(engine, prompts)
    tol = man.get("tolerance") or golden.TOLERANCE
    by_id = {c["id"]: c for c in man["cases"]}
    failures = []
    for p in prompts:
        case = by_id.get(p["id"])
        if case is None:
            failures.append(f"{p['id']}: no case in the manifest")
            continue
        body, headers = golden.render_case(engine, case["chunks"])
        diffs = golden.compare(case["expect"], golden.measure(body, headers), tol)
        if diffs:
            failures.append(f"{p['id']}: " + "; ".join(diffs))
    assert not failures, "golden audio moved:\n  " + "\n  ".join(failures)


def test_the_manifest_declares_what_it_cannot_prove(engine, prompts):
    """The provenance block is part of the test.

    A manifest captured from dev-tone is not a speech reference, and a reader
    must not have to know that. If someone re-captures against a real engine,
    `synthetic` has to change with it - which is the signal that the review of
    what those bytes now prove actually happened.
    """
    man = _capture(engine, prompts)
    prov = man["provenance"]
    assert prov["engine"] == "dev-tone"
    assert prov["synthetic"] is True
    assert "NOT voice quality" in prov["note"]


def test_render_is_deterministic(engine, prompts):
    """Two runs, one process: the gate is meaningless if the path is not stable."""
    case = golden.split_prompt(prompts[0]["text"])
    a, ah = golden.render_case(engine, case)
    b, bh = golden.render_case(engine, case)
    assert a == b
    assert {k: v for k, v in ah.items() if not k.endswith("Seconds")} == {k: v for k, v in bh.items() if not k.endswith("Seconds")}


def test_split_prompt_keeps_markers_as_pauses():
    chunks = golden.split_prompt("Be still. {pause 600} And know.")
    assert [c["text"] for c in chunks] == ["Be still.", "And know."]
    assert chunks[0]["silence_after_ms"] == 600
    assert all(len(c["text"]) <= golden.MAX_CHUNK_CHARS for c in chunks)


def test_compare_reports_a_chain_that_misreports_itself(engine, prompts):
    """The claim/measure cross-check is the point, not decoration.

    A chain that reports -22 LUFS while producing -16 still passes a gate that
    only trusts headers, and the API stores the header value as the asset's
    mastering record.
    """
    case = golden.split_prompt(prompts[0]["text"])
    body, headers = golden.render_case(engine, case)
    honest = golden.measure(body, headers)
    assert golden.compare(honest, honest) == []  # silent when the chain tells the truth
    lying = copy.deepcopy(honest)
    lying["claimed_loudness_lufs"] = round(honest["loudness_lufs"] - 6.0, 2)
    diffs = golden.compare(honest, lying)
    assert any("claims" in d for d in diffs), diffs


def test_compare_ignores_noise_inside_the_tolerance(engine, prompts):
    case = golden.split_prompt(prompts[0]["text"])
    body, headers = golden.render_case(engine, case)
    m = golden.measure(body, headers)
    jittered = copy.deepcopy(m)
    jittered["loudness_lufs"] = round(m["loudness_lufs"] + 0.1, 2)
    jittered["true_peak_dbtp"] = round(m["true_peak_dbtp"] - 0.1, 2)
    assert golden.compare(m, jittered) == []
    beyond = copy.deepcopy(m)
    beyond["loudness_lufs"] = round(m["loudness_lufs"] + 0.4, 2)
    assert any("loudness_lufs" in d for d in golden.compare(m, beyond))


def test_duration_change_always_fails(engine, prompts):
    case = golden.split_prompt(prompts[0]["text"])
    body, headers = golden.render_case(engine, case)
    m = golden.measure(body, headers)
    off = copy.deepcopy(m)
    off["duration_ms"] = m["duration_ms"] + 1
    assert any("duration_ms" in d for d in golden.compare(m, off))
    # The chain reports what it trimmed; a different trim is a different audio.
    off2 = copy.deepcopy(m)
    off2["trimmed_ms"] = m["trimmed_ms"] + 1
    assert any("trimmed_ms" in d for d in golden.compare(m, off2))
    assert np.isfinite(m["loudness_lufs"])
