"""Golden-audio regression set (audit VE-020).

Two questions get confused in a voice pipeline, so this module separates them
and is honest about which one it can answer without a GPU.

* "Did the audio path change?" - the DSP chain, loudness mastering, the limiter,
  the WAV writer, the delivery encoders. This is what this module gates. The
  reference set is captured from the `dev-tone` backend, whose output is a pure
  function of its input, so a difference is a difference in *our* code and not
  in a model's mood. It runs in CI in seconds.
* "Does it still sound like the minister?" - not gateable here, and this module
  does not pretend otherwise. That needs a licence-cleared engine on a GPU host
  (P0-1 / P0-2). The manifest shape below is exactly what such a capture would
  write (`provenance.engine` naming a real backend, real speech in the cases),
  which is why the capture path takes an engine name instead of hard-coding
  dev-tone.

Signal-level measurements are compared with tolerances rather than by hashing
the bytes: numpy/scipy are declared as ranges, not pins, and a BLAS that rounds
one float differently must not turn CI red. Duration is compared exactly - the
chain's trimming is integer-sample arithmetic - and the worker's own claims
about loudness and true peak are compared against what the bytes actually say,
so a chain that misreports itself fails even when the audio is fine.
"""

from __future__ import annotations

import hashlib
import json
import math
import os
import re
from pathlib import Path

import numpy as np

from . import mastering, wav

REPO = Path(__file__).resolve().parents[2]
GOLDEN_SET_PATH = REPO / "docs" / "voice" / "golden_set.json"
MANIFEST_PATH = Path(__file__).resolve().parents[1] / "tests" / "golden" / "manifest.json"

PAUSE_RE = re.compile(r"\{\s*pause\s+(\d+)\s*\}")
MARKER_RE = re.compile(r"\{[^{}]*\}")

# How far a measurement may move before the gate fails. dB tolerances absorb
# float noise between numpy/scipy builds; the exact ones do not, because they
# are counts.
TOLERANCE = {"duration_ms": 0, "loudness_db": 0.15, "true_peak_db": 0.15, "rms_db": 0.25, "claim_db": 0.10}

MAX_CHUNK_CHARS = 400  # dev-tone's advertised limit; keep cases inside it


def load_prompts(path: Path | None = None) -> list[dict]:
    """The shared golden prompt set (docs/voice/golden_set.json)."""
    doc = json.loads(Path(path or GOLDEN_SET_PATH).read_text())
    prompts = doc.get("prompts") or []
    if not prompts:
        raise ValueError("golden set has no prompts")
    return prompts


def split_prompt(text: str) -> list[dict]:
    """Turn one prompt into the chunk list the worker is given in production.

    This is deliberately NOT a reimplementation of the Go orchestrator's
    chunker. It exists only to produce a stable fixture: the sentences and the
    `{pause N}` markers are flattened into `chunks` once, at capture time, and
    committed. From then on the gate replays the committed chunks, so a change
    in Go's chunking cannot make this test red - and cannot make it green
    either, which is the point of gating the worker on its own contract.

    Other inline markers ({emph}...) are dropped: production sends them to the
    engine as parameters, never as text the model has to read.
    """
    parts: list[dict] = []
    for tok in _tokens(text):
        m = PAUSE_RE.fullmatch(tok)
        if m:
            if parts:
                parts[-1]["silence_after_ms"] = parts[-1].get("silence_after_ms", 0) + int(m.group(1))
            continue
        for line in _wrap(tok):
            parts.append({"text": line})
    return parts or [{"text": ""}]


def _wrap(sentence: str) -> list[str]:
    """Hard-wrap an over-long sentence on word boundaries."""
    if len(sentence) <= MAX_CHUNK_CHARS:
        return [sentence]
    out, buf = [], ""
    for w in sentence.split():
        if buf and len(buf) + 1 + len(w) > MAX_CHUNK_CHARS:
            out.append(buf)
            buf = w
        else:
            buf = f"{buf} {w}".strip()
    if buf:
        out.append(buf)
    return out


def _tokens(text: str):
    """Sentences and `{pause N}` markers, in order.

    A sentence splitter for a fixture generator, not a tokenizer: it cuts on
    sentence-final punctuation after whitespace, which is enough for the golden
    set and stays stable for years because its output is committed once.
    """
    pos = 0
    for m in PAUSE_RE.finditer(text):
        yield from _sentences(text[pos:m.start()])
        yield m.group(0)
        pos = m.end()
    yield from _sentences(text[pos:])


def _sentences(chunk: str) -> list[str]:
    body = MARKER_RE.sub(" ", chunk)
    body = re.sub(r"\s+", " ", body).strip()
    if not body:
        return []
    return [s.strip() for s in re.split(r"(?<=[.!?])\s+", body) if s.strip()]


def render_case(engine, chunks: list[dict]) -> tuple[bytes, dict]:
    """One case through the same entry point the HTTP handler uses."""
    from .server import render

    return render(engine, {"chunks": chunks, "params": {}})


def measure(body: bytes, headers: dict, rate_fallback: int = 16000) -> dict:
    """What the produced bytes actually are, plus what the worker claimed."""
    x, rate = wav.decode(body)
    if rate <= 0:
        rate = rate_fallback
    n = int(x.size)
    peak = float(np.max(np.abs(x))) if n else 0.0
    rms = float(np.sqrt(np.mean(np.square(x)))) if n else 0.0
    lin = mastering.integrated_loudness(x, rate) if n else float("-inf")
    tp = mastering.true_peak_dbtp(x) if n else float("-inf")
    clipped = int(np.count_nonzero(np.abs(x) >= 0.999)) if n else 0
    f = lambda v: None if (v is None or not math.isfinite(float(v))) else round(float(v), 2)  # noqa: E731
    return {
        "duration_ms": int(round(1000.0 * n / rate)),
        "sample_rate": int(rate),
        "bytes": len(body),
        "sha256": hashlib.sha256(body).hexdigest(),
        "peak_dbfs": f(20 * math.log10(peak) if peak > 0 else float("-inf")),
        "rms_dbfs": f(20 * math.log10(rms) if rms > 0 else float("-inf")),
        "loudness_lufs": f(lin),
        "true_peak_dbtp": f(tp),
        "clipped_samples": clipped,
        "chain": headers.get("X-DSP-Chain", ""),
        "trimmed_ms": int(float(headers.get("X-Trimmed-Ms", "0") or 0)),
        "limiter_max_db": headers.get("X-Limiter-Max-dB", ""),
        "peak_limited": headers.get("X-Peak-Limited", "0") == "1",
        # The worker's own claims, kept so the gate can show a disagreement
        # between the header and the audio instead of silently trusting it.
        "claimed_loudness_lufs": f(headers.get("X-Loudness-LUFS", "nan")),
        "claimed_true_peak_dbtp": f(headers.get("X-True-Peak-dBTP", "nan")),
    }


def capture(engine, prompts: list[dict]) -> dict:
    """Reference measurements for every prompt, with provenance attached."""
    cases = []
    for p in prompts:
        chunks = split_prompt(p["text"])
        body, headers = render_case(engine, chunks)
        cases.append({"id": p["id"], "category": p.get("category", ""), "style": p.get("style", ""),
                      "chunks": chunks, "expect": measure(body, headers)})
    import scipy

    return {
        "version": 1,
        "measure_version": 1,
        "tolerance": TOLERANCE,
        "provenance": {
            "engine": engine.name,
            "engine_version": engine.version,
            "synthetic": engine.name == "dev-tone",
            "numpy": np.__version__,
            "scipy": scipy.__version__,
            "target_lufs": os.environ.get("ICF_MASTER_TARGET_LUFS", "-16"),
            "ceiling_dbtp": os.environ.get("ICF_MASTER_CEILING_DBTP", "-1"),
            "note": ("dev-tone renders a tone, never speech. These references gate the audio path "
                     "(DSP chain, mastering, WAV) - NOT voice quality. Voice quality is gated by "
                     "docs/voice/golden_set.json + server/cmd/voice-bench on a GPU host, which "
                     "P0-1/P0-2 keep out of CI."),
        },
        "cases": cases,
    }


def compare(expected: dict, actual: dict, tol: dict | None = None) -> list[str]:
    """Differences as human-readable lines; empty means the case still matches."""
    t = dict(TOLERANCE)
    t.update(tol or {})
    diffs: list[str] = []
    if actual["duration_ms"] != expected["duration_ms"]:
        diffs.append(f"duration_ms {expected['duration_ms']} -> {actual['duration_ms']}")
    for key, allowed in (("loudness_lufs", t["loudness_db"]), ("true_peak_dbtp", t["true_peak_db"]),
                         ("rms_dbfs", t["rms_db"]), ("peak_dbfs", t["true_peak_db"])):
        e, a = expected.get(key), actual.get(key)
        if e is None or a is None:
            if e != a:
                diffs.append(f"{key} {e} -> {a}")
            continue
        if abs(a - e) > allowed:
            diffs.append(f"{key} {e} -> {a} (allowed ±{allowed})")
    for key in ("sample_rate", "chain", "clipped_samples", "trimmed_ms", "limiter_max_db", "peak_limited"):
        if expected.get(key) != actual.get(key):
            diffs.append(f"{key} {expected.get(key)!r} -> {actual.get(key)!r}")
    # A chain that measures its own output differently from the bytes is a
    # reporting bug even when the audio is fine, and the header is what the
    # API stores and what the mastering review reads.
    for claim, real in (("claimed_loudness_lufs", "loudness_lufs"), ("claimed_true_peak_dbtp", "true_peak_dbtp")):
        c, m = actual.get(claim), actual.get(real)
        if c is not None and m is not None and abs(c - m) > t["claim_db"]:
            diffs.append(f"worker claims {claim.split('_', 1)[1]}={c} but the audio measures {m}")
    return diffs


def write_manifest(manifest: dict, path: Path | None = None) -> Path:
    p = Path(path or MANIFEST_PATH)
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(json.dumps(manifest, indent=1, sort_keys=True) + "\n")
    return p


def read_manifest(path: Path | None = None) -> dict:
    return json.loads(Path(path or MANIFEST_PATH).read_text())
