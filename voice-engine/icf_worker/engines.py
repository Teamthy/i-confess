"""Engine backends. Each wraps one self-hosted TTS model.

Backends import their heavy ML dependencies lazily so the worker can start,
report health, and be tested without a GPU. No backend downloads weights at
runtime: checkpoints are resolved from ICF_CHECKPOINT_ROOT (populated from the
private bucket by deployment), and are never returned to callers.

VoiceStudio is deliberately absent: it is AGPL and is used only as an offline
benchmarking tool (see docs/VOICESTUDIO_LICENSE.md), never linked in here.
"""

from __future__ import annotations

import os
from pathlib import Path

import numpy as np


class EngineError(Exception):
    """Carries a worker error class understood by the Go adapter."""

    def __init__(self, cls: str, message: str):
        super().__init__(message)
        self.cls = cls


class Engine:
    name = "base"
    version = "0"
    sample_rate = 24000
    capabilities: dict = {}

    def synthesize_chunk(self, text: str, *, reference: dict | None, checkpoint: Path | None,
                         params: dict, chunk: dict) -> np.ndarray:
        raise NotImplementedError

    def healthy(self) -> bool:
        return True


def resolve_checkpoint(key: str) -> Path | None:
    """Map a private object key to a local path, refusing path traversal."""
    if not key:
        return None
    root = Path(os.environ.get("ICF_CHECKPOINT_ROOT", "/models")).resolve()
    p = (root / key).resolve()
    if root not in p.parents:
        raise EngineError("permanent", "checkpoint key escapes checkpoint root")
    if not p.exists():
        raise EngineError("model", "checkpoint not present on this worker")
    return p


class CosyVoiceEngine(Engine):
    """CosyVoice zero-shot / instruct inference.

    Requires the CosyVoice repository on PYTHONPATH and a pretrained model
    directory at ICF_COSYVOICE_MODEL_DIR. Verify the model licence in
    docs/MODEL_LICENSES.md before production use.
    """

    name = "cosyvoice"
    capabilities = {"zero_shot": True, "fine_tune": True, "streaming": True, "languages": ["en", "zh"],
                    "max_chunk_chars": 300, "self_hosted": True, "license": "see MODEL_LICENSES.md"}

    def __init__(self):
        self._model = None

    def _load(self):
        if self._model is None:
            try:
                from cosyvoice.cli.cosyvoice import AutoModel  # type: ignore
            except ImportError as e:  # pragma: no cover - needs the real engine
                raise EngineError("model", f"CosyVoice is not installed on this worker: {e}")
            model_dir = os.environ.get("ICF_COSYVOICE_MODEL_DIR")
            if not model_dir:
                raise EngineError("model", "ICF_COSYVOICE_MODEL_DIR is not set")
            self._model = AutoModel(model_dir=model_dir)
            self.sample_rate = int(getattr(self._model, "sample_rate", 24000))
        return self._model

    def healthy(self) -> bool:
        try:
            self._load()
            return True
        except EngineError:
            return False

    def synthesize_chunk(self, text, *, reference, checkpoint, params, chunk):  # pragma: no cover
        model = self._load()
        if not reference:
            raise EngineError("model", "zero-shot synthesis requires a reference clip")
        ref_path = resolve_checkpoint(reference["uri"])
        speed = float(params.get("speed") or 1.0)
        out = []
        for piece in model.inference_zero_shot(text, reference["transcript"], str(ref_path),
                                               stream=False, speed=speed):
            out.append(piece["tts_speech"].squeeze().cpu().numpy())
        return np.concatenate(out) if out else np.zeros(0)


class DevToneEngine(Engine):
    """Plumbing-only backend: emits a quiet tone, never speech.

    Enabled only with ICF_WORKER_DEV=1 and refuses to run when
    ICF_ENV=production, so it cannot be mistaken for a voice.
    """

    name = "dev-tone"
    version = "dev"
    sample_rate = 16000
    capabilities = {"zero_shot": True, "languages": ["en"], "max_chunk_chars": 400, "self_hosted": True,
                    "license": "internal test backend"}

    def synthesize_chunk(self, text, *, reference, checkpoint, params, chunk):
        secs = max(0.4, 0.06 * len(text) / float(params.get("speed") or 1.0))
        t = np.arange(int(secs * self.sample_rate)) / self.sample_rate
        return 0.1 * np.sin(2 * np.pi * 220 * t)


def load_engine(name: str) -> Engine:
    if name == "cosyvoice":
        return CosyVoiceEngine()
    if name == "dev-tone":
        if os.environ.get("ICF_WORKER_DEV") != "1" or os.environ.get("ICF_ENV") == "production":
            raise SystemExit("dev-tone backend requires ICF_WORKER_DEV=1 and a non-production ICF_ENV")
        return DevToneEngine()
    # GPT-SoVITS and VoxCPM run as their own API servers; add a proxying
    # backend here once a checkpoint's licence has been reviewed.
    raise SystemExit(f"unknown or unsupported engine {name!r}")
