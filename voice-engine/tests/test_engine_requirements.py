"""The engine dependency manifests (audit VE-014).

`requirements.txt` cannot produce speech: it declares numpy and scipy, which is
enough to master and encode audio but not to synthesise it, so every deployment
used to reconstruct the engine's environment by hand. The manifests tested here
are the answer, and these tests exist because a dependency file is exactly the
kind of document that rots silently: a truncated file, a package pinned twice at
different versions, or a torch/torchaudio pair that disagrees all look fine until
a GPU host fails to boot.

What is NOT asserted: that these pins install, or that they produce speech. There
is no GPU in this environment and no licence-cleared checkpoint, so a test that
claimed either would be false. The tests assert the properties that can be
checked from the files themselves.
"""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path

import pytest

ROOT = Path(__file__).resolve().parents[1]
MANIFESTS = {
    "requirements-cosyvoice.txt": "cosyvoice",
    "requirements-voxcpm.txt": "voxcpm",
}

REQUIREMENT = re.compile(r"^(?P<name>[A-Za-z0-9._-]+)==(?P<version>[^\s;]+)(?P<marker>\s*;.*)?$")


def entries(path: Path) -> list[tuple[str, str]]:
    """Pinned (name, version) pairs, lowercased, markers stripped."""
    out = []
    for line in path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#") or line.startswith("--"):
            continue
        m = REQUIREMENT.match(line)
        assert m, f"{path.name}: not a pin: {line!r}"
        out.append((m.group("name").lower(), m.group("version")))
    return out


@pytest.mark.parametrize("filename", sorted(MANIFESTS))
def test_manifest_is_a_reviewable_list_of_pins(filename):
    path = ROOT / filename
    assert path.exists(), f"{filename} is missing"
    rows = entries(path)
    assert len(rows) >= 3, f"{filename} has {len(rows)} entries; a truncated file must not pass"

    names = [n for n, _ in rows]
    duplicates = {n for n in names if names.count(n) > 1}
    assert not duplicates, f"{filename} pins the same package twice: {sorted(duplicates)}"


@pytest.mark.parametrize("filename", sorted(MANIFESTS))
def test_manifest_says_where_its_pins_came_from_and_what_is_unproven(filename):
    text = (ROOT / filename).read_text()
    lowered = text.lower()
    # Provenance and the untested caveat are the point of the file, not
    # decoration: without them the next reader cannot tell an upstream pin from a
    # guess, or a verified pin from one nobody has ever installed.
    for required in ("ve-014", "upstream", "not verified on a gpu", "dockerfile"):
        assert required in lowered, f"{filename} does not mention {required!r}"


def test_cosyvoice_carries_the_upstream_extra_indexes():
    # torch/torchaudio cu121 wheels and the CUDA onnxruntime feed are not on PyPI;
    # dropping these lines turns a working manifest into an unresolvable one.
    text = (ROOT / "requirements-cosyvoice.txt").read_text()
    assert "--extra-index-url https://download.pytorch.org/whl/cu121" in text
    assert "onnxruntime-cuda-12" in text


def test_torch_and_torchaudio_are_a_matched_pair():
    # Mismatched versions of these two are a runtime failure rather than an
    # install failure, which makes them expensive to discover on a GPU host.
    for filename in MANIFESTS:
        pins = dict(entries(ROOT / filename))
        assert pins.get("torch") == pins.get("torchaudio"), (
            f"{filename}: torch={pins.get('torch')} torchaudio={pins.get('torchaudio')}"
        )


def test_the_two_engines_declare_their_incompatibility():
    # CosyVoice pins torch 2.3.1; voxcpm requires >= 2.5.0. A deployment that puts
    # both in one environment cannot resolve, so the constraint is written down
    # where the installer will read it.
    cosy = dict(entries(ROOT / "requirements-cosyvoice.txt"))
    vox = dict(entries(ROOT / "requirements-voxcpm.txt"))
    assert cosy["torch"] != vox["torch"]
    assert "ONE HOST, ONE ENGINE ENVIRONMENT" in (ROOT / "requirements-voxcpm.txt").read_text()


def test_the_production_image_installs_neither_engine():
    # The image stays free of engine code so an uncleared engine cannot ship
    # inside it (docs/model_licenses.json is the gate). If a Dockerfile edit ever
    # installed these manifests, the licence posture would change silently.
    dockerfile = (ROOT / "Dockerfile").read_text()
    installs = [ln.strip() for ln in dockerfile.splitlines() if "pip install" in ln]
    assert any("requirements.txt" in ln for ln in installs), "the worker's own deps are not installed"
    for filename in MANIFESTS:
        assert filename not in dockerfile, f"Dockerfile installs {filename}; the image must stay engine-free"


def test_the_demo_stack_stays_out_of_the_worker():
    # Upstream's requirements pulled in 116 known advisories, 60+ of them in
    # gradio - the model's public demo UI, which this worker never serves. The
    # subtraction is deliberate; a future "sync with upstream" must not undo it
    # silently.
    text = (ROOT / "requirements-cosyvoice.txt").read_text()
    for demo in ("gradio==", "fastapi==", "uvicorn==", "tensorboard==", "wget=="):
        assert demo not in text, f"{demo} is the demo/training stack, not the worker's"
    # ...and the reason has to be written where the next person will look.
    for dropped in ("gradio", "tensorboard", "openai-whisper"):
        assert dropped in text, f"{dropped} is not listed among the excluded packages"


def test_the_accepted_advisories_are_enumerated_with_reasons():
    # The manifests knowingly pin versions upstream has left behind. Accepting a
    # vulnerability is a decision, so it is recorded with a reason and a fix
    # version, and the CI gate fails on anything not on the list.
    baseline = json.loads((ROOT / "engine-audit-baseline.json").read_text())
    accepted = baseline["accepted"]
    assert accepted, "the baseline is empty but the manifests are not known-clean"
    ids = {(e["package"].lower(), e["id"]) for e in accepted}
    assert len(ids) == len(accepted), "duplicate entries in the baseline"
    for e in accepted:
        assert e["package"] and e["id"], e
        assert e["reason"].strip(), f"{e['package']}: no reason recorded"
        assert e["manifests"], e
        for m in e["manifests"]:
            assert m in MANIFESTS, f"{e['package']}: unknown manifest {m}"
    for filename in MANIFESTS:
        assert any(filename in e["manifests"] for e in accepted), f"no baseline entries for {filename}"


def test_the_gate_script_rejects_an_unknown_finding(tmp_path):
    # The gate is only worth having if it fails. A finding absent from the
    # baseline must produce a non-zero exit; the same document with a known id
    # must not.
    sys.path.insert(0, str(ROOT.parent / "scripts"))
    import check_engine_audit as gate

    baseline = gate.load_baseline()
    assert baseline, "baseline loaded empty"

    doc = {"dependencies": [{"name": "torch", "version": "2.3.1",
                             "vulns": [{"id": "PYSEC-2025-194", "fix_versions": ["2.13.0"]}]}]}
    known = tmp_path / "known.json"
    known.write_text(json.dumps(doc))
    assert gate.main(["check_engine_audit.py", str(known)]) == 0, "a baselined finding failed the gate"

    doc["dependencies"].append({"name": "brand-new", "version": "1.0",
                                "vulns": [{"id": "PYSEC-TEST-0001", "fix_versions": ["1.0.1"]}]})
    unknown = tmp_path / "unknown.json"
    unknown.write_text(json.dumps(doc))
    assert gate.main(["check_engine_audit.py", str(unknown)]) == 1, "a new finding passed the gate"


def test_readme_points_at_the_manifests():
    readme = (ROOT / "README.md").read_text()
    for filename in MANIFESTS:
        assert filename in readme, f"README does not tell a deployment about {filename}"
