"""Engine backends. Each wraps one self-hosted TTS model.

Backends import their heavy ML dependencies lazily so the worker can start,
report health, and be tested without a GPU. No backend downloads weights at
runtime: checkpoints are resolved from ICF_CHECKPOINT_ROOT (populated from the
private bucket by deployment), and are never returned to callers.

Every engine passes the licence gate (licenses.check) at load time.

VoiceStudio is deliberately absent: it is AGPL and is used only as an offline
benchmarking tool (see docs/VOICESTUDIO_LICENSE.md), never linked in here.
"""

from __future__ import annotations

import hashlib
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
    # base_version is the upstream generation ("0", "1", "v2"); `version` adds a
    # fingerprint of the checkpoint actually in use. See checkpoint_revision.
    base_version = "0"
    sample_rate = 24000
    capabilities: dict = {}

    def checkpoint_paths(self) -> tuple[str, ...]:
        """Where this backend's weights live, for version fingerprinting."""
        return ()

    @property
    def version(self) -> str:
        rev = checkpoint_revision(*self.checkpoint_paths())
        return f"{self.base_version}+{rev}" if rev else self.base_version

    def synthesize_chunk(self, text: str, *, reference: dict | None, checkpoint: Path | None,
                         params: dict, chunk: dict) -> np.ndarray:
        raise NotImplementedError

    def healthy(self) -> bool:
        return True


def checkpoint_revision(*paths: str) -> str:
    """A short fingerprint of the checkpoint a backend will load.

    ``engine_version`` used to be a hardcoded string per backend ("0" for
    CosyVoice, "1" for VoxCPM). Two different checkpoints published under the
    same version were therefore indistinguishable in the asset record, and the
    content hash - which includes engine_version - would treat a render from the
    old checkpoint and a render from the new one as the same audio (audit
    VE-012).

    This fingerprints the deployment's *layout*, not the weights: the path plus
    the name and size of each file in the checkpoint directory. It is not a
    checksum - two checkpoints with identical file names and sizes collide - but
    it costs a directory listing instead of reading gigabytes, and it changes
    whenever a deployment swaps in a different build. Operators who need
    certainty set ICF_CHECKPOINT_REVISION, which wins over the derived value.

    An empty result means "no checkpoint configured", and the version stays the
    plain upstream generation rather than gaining a meaningless suffix.
    """
    explicit = os.environ.get("ICF_CHECKPOINT_REVISION", "").strip()
    if explicit:
        return explicit
    h = hashlib.sha256()
    found = False
    for raw in paths:
        if not raw:
            continue
        found = True
        p = Path(raw)
        h.update(str(p).encode())
        try:
            entries = sorted(p.iterdir()) if p.is_dir() else [p]
        except OSError:
            continue
        for e in entries[:200]:  # a checkpoint directory is not a corpus
            try:
                st = e.stat()
            except OSError:
                continue
            h.update(f"{e.name}:{st.st_size}".encode())
    if not found:
        return ""
    return h.hexdigest()[:12]


def resolve_checkpoint(key: str) -> Path | None:
    """Map a private object key to a local file via shared storage."""
    if not key:
        return None
    from .storage import StorageError, get_store, valid_key

    if not valid_key(key):
        raise EngineError("permanent", "invalid or disallowed object key")
    try:
        return get_store().local_path(key)
    except StorageError as e:
        raise EngineError("model", f"object not available on this worker: {e}")


class CosyVoiceEngine(Engine):
    """CosyVoice zero-shot / instruct inference.

    Requires the CosyVoice repository on PYTHONPATH and a pretrained model
    directory at ICF_COSYVOICE_MODEL_DIR. Verify the model licence in
    docs/MODEL_LICENSES.md before production use.
    """

    name = "cosyvoice"
    base_version = "0"
    capabilities = {"zero_shot": True, "fine_tune": True, "streaming": True, "languages": ["en", "zh"],
                    "max_chunk_chars": 300, "self_hosted": True, "license": "see MODEL_LICENSES.md"}

    def __init__(self):
        self._model = None

    def checkpoint_paths(self) -> tuple[str, ...]:
        return (os.environ.get("ICF_COSYVOICE_MODEL_DIR") or "",)

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
        transcript = reference_transcript(reference)
        ref_path = resolve_checkpoint(reference["uri"])
        speed = float(params.get("speed") or 1.0)
        out = []
        for piece in model.inference_zero_shot(text, transcript, str(ref_path),
                                               stream=False, speed=speed):
            out.append(piece["tts_speech"].squeeze().cpu().numpy())
        return np.concatenate(out) if out else np.zeros(0)


def reference_transcript(reference: dict) -> str:
    """The verified transcript a zero-shot clone needs.

    A reference clip recorded without one is a data fault, not a crash. Before
    this guard each backend read the transcript key directly, so the request
    died with a KeyError that the server could only classify as an unexpected
    fault: a retryable 500 for something that can never succeed, and a
    traceback instead of a reason (audit VE-016).

    Every backend that clones from a reference goes through here, so the failure
    is a classified "content" error (422) that names what is missing.
    """
    text = (reference or {}).get("transcript") or ""
    if not text.strip():
        raise EngineError("content", "reference clip has no verified transcript")
    return text


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


class GPTSoVITSEngine(Engine):
    """Proxy to a GPT-SoVITS api_v2 server (ICF_GPTSOVITS_API, e.g.
    http://127.0.0.1:9880) running on the same GPU host. The upstream server
    loads the weights; fine-tuned checkpoints are switched in through its
    /set_gpt_weights and /set_sovits_weights endpoints.

    This backend has NOT been exercised against a live GPT-SoVITS instance in
    CI; verify the api_v2 parameter names against the pinned upstream commit.
    """

    name = "gpt_sovits"
    # The upstream service owns the base weights; once a fine-tuned checkpoint
    # is pushed to it, the version names that checkpoint too.
    base_version = "v2"
    sample_rate = 32000
    capabilities = {"zero_shot": True, "fine_tune": True, "streaming": True, "languages": ["en", "zh", "ja", "ko"],
                    "max_chunk_chars": 200, "self_hosted": True, "license": "see MODEL_LICENSES.md"}

    def __init__(self):
        self.base = os.environ.get("ICF_GPTSOVITS_API", "http://127.0.0.1:9880").rstrip("/")
        self._loaded_ckpt: str | None = None

    def checkpoint_paths(self) -> tuple[str, ...]:
        return (self._loaded_ckpt or "",)

    def _get(self, path: str, params: dict) -> bytes:
        import urllib.parse
        import urllib.request

        url = f"{self.base}{path}?{urllib.parse.urlencode(params)}"
        try:
            with urllib.request.urlopen(url, timeout=300) as r:
                return r.read()
        except Exception as e:  # noqa: BLE001
            raise EngineError("transient", f"GPT-SoVITS API: {e}")

    def healthy(self) -> bool:
        import urllib.request

        try:
            urllib.request.urlopen(self.base + "/docs", timeout=3)
            return True
        except Exception:  # noqa: BLE001
            return False

    def _ensure_weights(self, checkpoint: Path | None):
        if checkpoint is None or str(checkpoint) == self._loaded_ckpt:
            return
        d = checkpoint.parent
        gpt = next(iter(sorted(d.glob("*.ckpt"))), None)
        sov = next(iter(sorted(d.glob("*.pth"))), None)
        if not gpt or not sov:
            raise EngineError("model", "fine-tuned GPT-SoVITS checkpoint needs a .ckpt (GPT) and a .pth (SoVITS)")
        self._get("/set_gpt_weights", {"weights_path": str(gpt)})
        self._get("/set_sovits_weights", {"weights_path": str(sov)})
        self._loaded_ckpt = str(checkpoint)

    def synthesize_chunk(self, text, *, reference, checkpoint, params, chunk):
        self._ensure_weights(checkpoint)
        if not reference:
            raise EngineError("model", "GPT-SoVITS needs a reference clip and transcript")
        transcript = reference_transcript(reference)
        ref = resolve_checkpoint(reference["uri"])
        lang = "en"
        audio = self._get("/tts", {
            "text": text, "text_lang": lang, "ref_audio_path": str(ref), "prompt_text": transcript,
            "prompt_lang": lang, "speed_factor": params.get("speed_factor", 1.0),
            "temperature": params.get("temperature", 1.0), "media_type": "wav", "streaming_mode": "false",
        })
        from . import wav as _wav

        x, rate = _wav.decode(audio)
        self.sample_rate = rate
        return x


class VoxCPMEngine(Engine):
    """In-process VoxCPM (pip package `voxcpm`), model at ICF_VOXCPM_MODEL.

    Not exercised against the real model in CI; confirm the generate()
    signature against the pinned package version.
    """

    name = "voxcpm"
    base_version = "1"
    sample_rate = 16000
    capabilities = {"zero_shot": True, "fine_tune": False, "streaming": False, "languages": ["en", "zh"],
                    "max_chunk_chars": 300, "self_hosted": True, "license": "see MODEL_LICENSES.md"}

    def __init__(self):
        self._model = None

    def checkpoint_paths(self) -> tuple[str, ...]:
        return (os.environ.get("ICF_VOXCPM_MODEL") or "",)

    def _load(self):
        if self._model is None:
            try:
                from voxcpm import VoxCPM  # type: ignore
            except ImportError as e:  # pragma: no cover
                raise EngineError("model", f"voxcpm is not installed on this worker: {e}")
            src = os.environ.get("ICF_VOXCPM_MODEL")
            if not src:
                raise EngineError("model", "ICF_VOXCPM_MODEL is not set")
            self._model = VoxCPM.from_pretrained(src)
            tts = getattr(self._model, "tts_model", None)
            self.sample_rate = int(getattr(tts, "sample_rate", self.sample_rate))
        return self._model

    def healthy(self) -> bool:
        try:
            self._load()
            return True
        except EngineError:
            return False

    def synthesize_chunk(self, text, *, reference, checkpoint, params, chunk):  # pragma: no cover
        model = self._load()
        kw = {"text": text, "cfg_value": float(params.get("cfg_value", 2.0)),
              "inference_timesteps": int(params.get("inference_timesteps", 10))}
        if reference:
            # Validated before the path is resolved: a reference with no
            # transcript is a content fault whatever storage would say.
            kw["prompt_text"] = reference_transcript(reference)
            kw["prompt_wav_path"] = str(resolve_checkpoint(reference["uri"]))
        return np.asarray(model.generate(**kw), dtype=np.float64)


def load_engine(name: str) -> Engine:
    from . import licenses

    if name == "dev-tone":
        if os.environ.get("ICF_WORKER_DEV") != "1" or os.environ.get("ICF_ENV") == "production":
            raise SystemExit("dev-tone backend requires ICF_WORKER_DEV=1 and a non-production ICF_ENV")
        return DevToneEngine()
    engines = {"cosyvoice": CosyVoiceEngine, "gpt_sovits": GPTSoVITSEngine, "voxcpm": VoxCPMEngine}
    if name not in engines:
        raise SystemExit(f"unknown or unsupported engine {name!r}")
    licenses.check(name, "inference")
    return engines[name]()
