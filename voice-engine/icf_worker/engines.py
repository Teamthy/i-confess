"""Engine backends. Each wraps one self-hosted TTS model.

Backends import their heavy ML dependencies lazily so the worker can start,
report health, and be tested without a GPU. No backend downloads weights at
runtime: each loads from a configured local, immutable bundle, populated by
deployment. Weights are never returned to callers.

Every engine passes the licence gate (licenses.check) at load time.

VoiceStudio is deliberately absent: it is AGPL and is used only as an offline
benchmarking tool (see docs/VOICESTUDIO_LICENSE.md), never linked in here.
"""

from __future__ import annotations

import logging
import os
from functools import cached_property
from pathlib import Path

import numpy as np

from .checkpoint import bundle_sha256, revision_for, wire_engine

log = logging.getLogger("icf_worker.engines")


class EngineError(Exception):
    """Carries a worker error class understood by the Go adapter."""

    def __init__(self, cls: str, message: str):
        super().__init__(message)
        self.cls = cls


class Engine:
    name = "base"
    sample_rate = 24000
    capabilities: dict = {}
    model_env = ""

    def __init__(self):
        src = os.environ.get(self.model_env) if self.model_env else None
        self.model_dir = Path(src).resolve() if src else None

    @property
    def wire_name(self) -> str:
        return wire_engine(self.name)

    @cached_property
    def version(self) -> str:
        if self.model_dir is None:
            raise EngineError("identity", "a locally mounted checkpoint bundle is required")
        try:
            return revision_for(self.name, bundle_sha256(self.model_dir))
        except (OSError, ValueError) as e:
            raise EngineError("identity", "cannot identify the checkpoint bundle") from e

    def prepare_render(self, checkpoint: Path | None):
        # A request cannot select weights outside the measured deployment.
        if checkpoint is not None and (self.model_dir is None or
                                       not checkpoint.resolve().is_relative_to(self.model_dir)):
            raise EngineError("identity", "requested checkpoint is outside the pinned bundle")

    def synthesize_chunk(self, text: str, *, reference: dict | None, checkpoint: Path | None,
                         params: dict, chunk: dict) -> np.ndarray:
        raise NotImplementedError

    def healthy(self) -> bool:
        return True

    def runtime_capabilities(self) -> dict:
        """Capabilities that are only known once the engine is in use."""
        return {}


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
    model_env = "ICF_COSYVOICE_MODEL_DIR"
    capabilities = {"zero_shot": True, "fine_tune": True, "streaming": True, "languages": ["en", "zh"],
                    "max_chunk_chars": 300, "self_hosted": True, "license": "see MODEL_LICENSES.md"}

    def __init__(self):
        super().__init__()
        self._model = None
        # VE-006: the Go adapter sends `instruct`, a natural-language style
        # instruction, alongside the numeric prosody. Inference uses
        # inference_instruct2 when this build has it and zero-shot when it does
        # not - the render must not fail because a build lacks the richer API,
        # but the fact that the instruction was dropped has to be visible, so
        # the flag is reported in /v1/capabilities as style_instruction.
        self._instruct_supported = True

    def runtime_capabilities(self) -> dict:
        return {"style_instruction": self._instruct_supported}

    def _load(self):
        if self._model is None:
            try:
                from cosyvoice.cli.cosyvoice import AutoModel  # type: ignore
            except ImportError as e:  # pragma: no cover - needs the real engine
                raise EngineError("model", f"CosyVoice is not installed on this worker: {e}")
            model_dir = self.model_dir
            if not model_dir:
                raise EngineError("model", "ICF_COSYVOICE_MODEL_DIR is not set")
            self._model = AutoModel(model_dir=str(model_dir))
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
        instruction = str(params.get("instruct") or "").strip()

        out = []
        if instruction and self._instruct_supported:
            pieces = None
            if hasattr(model, "inference_instruct2"):
                try:
                    pieces = model.inference_instruct2(text, instruction, str(ref_path),
                                                       stream=False, speed=speed)
                except TypeError as e:
                    self._drop_instruct(f"signature mismatch: {e}")
            else:
                self._drop_instruct("this CosyVoice build has no inference_instruct2")
            if pieces is not None:
                for piece in pieces:
                    out.append(piece["tts_speech"].squeeze().cpu().numpy())
                return np.concatenate(out) if out else np.zeros(0)

        for piece in model.inference_zero_shot(text, transcript, str(ref_path),
                                               stream=False, speed=speed):
            out.append(piece["tts_speech"].squeeze().cpu().numpy())
        return np.concatenate(out) if out else np.zeros(0)

    def _drop_instruct(self, reason: str) -> None:
        """Stop asking for style instructions, once, and say why.

        The render continues zero-shot - a build without the richer API is still
        usable - but the loss is recorded rather than hidden: one warning per
        process, and style_instruction=false in /v1/capabilities for anyone who
        asks instead of watching logs.
        """
        if self._instruct_supported:
            self._instruct_supported = False
            log.warning("cosyvoice: style instructions cannot be used (%s); "
                        "rendering zero-shot from here on", reason)


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
    must load the pinned weight pair and report its loaded identity on the
    final setter and each /tts response. Stock uninstrumented api_v2 cannot
    make that guarantee and is refused. See README.md.

    This backend has NOT been exercised against a live GPT-SoVITS instance in
    CI; verify the api_v2 parameter names against the pinned upstream commit.
    """

    name = "gpt_sovits"
    model_env = "ICF_GPTSOVITS_MODEL_DIR"
    sample_rate = 32000
    capabilities = {"zero_shot": True, "fine_tune": True, "streaming": True, "languages": ["en", "zh", "ja", "ko"],
                    "max_chunk_chars": 200, "self_hosted": True, "license": "see MODEL_LICENSES.md"}

    def __init__(self):
        super().__init__()
        self.base = os.environ.get("ICF_GPTSOVITS_API", "http://127.0.0.1:9880").rstrip("/")

    def _get(self, path: str, params: dict) -> bytes:
        import urllib.parse
        import urllib.request

        url = f"{self.base}{path}?{urllib.parse.urlencode(params)}"
        try:
            with urllib.request.urlopen(url, timeout=300) as r:
                if path in ("/set_sovits_weights", "/tts"):
                    # A 200 from a setter does not identify the weights on the
                    # OTHER process. Require its loaded-bundle readback before
                    # sending text, then verify the replica that returned WAV.
                    if (r.headers.get_all("X-Engine") != [self.wire_name] or
                            r.headers.get_all("X-Engine-Version") != [self.version]):
                        raise EngineError("identity", "GPT-SoVITS upstream did not confirm the pinned bundle")
                return r.read()
        except EngineError:
            raise
        except Exception as e:  # noqa: BLE001 - never log query params (text/reference)
            raise EngineError("transient", "GPT-SoVITS upstream unavailable") from e

    def healthy(self) -> bool:
        import urllib.request

        try:
            urllib.request.urlopen(self.base + "/docs", timeout=3)
            return True
        except Exception:  # noqa: BLE001
            return False

    def prepare_render(self, checkpoint: Path | None):
        super().prepare_render(checkpoint)
        # api_v2 has mutable process-wide weights.
        # Require a dedicated upstream with the SAME read-only bundle mounted
        # at the same absolute path. Select both files for EVERY request (also
        # the base case and after a partial switch/restart), before first audio.
        if self.model_dir is None:
            raise EngineError("identity", "no pinned GPT-SoVITS bundle")
        gpt = sorted(self.model_dir.glob("*.ckpt"))
        sov = sorted(self.model_dir.glob("*.pth"))
        if len(gpt) != 1 or len(sov) != 1:
            raise EngineError("identity", "pinned GPT-SoVITS bundle needs exactly one .ckpt and one .pth")
        self._get("/set_gpt_weights", {"weights_path": str(gpt[0])})
        self._get("/set_sovits_weights", {"weights_path": str(sov[0])})

    def synthesize_chunk(self, text, *, reference, checkpoint, params, chunk):
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
    model_env = "ICF_VOXCPM_MODEL"
    sample_rate = 16000
    capabilities = {"zero_shot": True, "fine_tune": False, "streaming": False, "languages": ["en", "zh"],
                    "max_chunk_chars": 300, "self_hosted": True, "license": "see MODEL_LICENSES.md"}

    def __init__(self):
        super().__init__()
        self._model = None

    def _load(self):
        if self._model is None:
            try:
                from voxcpm import VoxCPM  # type: ignore
            except ImportError as e:  # pragma: no cover
                raise EngineError("model", f"voxcpm is not installed on this worker: {e}")
            src = self.model_dir
            if not src:
                raise EngineError("model", "ICF_VOXCPM_MODEL is not set")
            self._model = VoxCPM.from_pretrained(str(src))
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
