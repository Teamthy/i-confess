"""Delivery encodings (spec section 26).

The mastered WAV is the master. Web/mobile variants are derived from it
exactly once and stored next to it, keyed by the same content hash, so an
asset is never transcoded twice and never transcoded from a lossy copy.

    master : WAV / PCM                      (stored by the render)
    aac    : AAC-LC in MP4 (.m4a)           iOS/Android/Safari
    opus   : Opus in Ogg (.opus)            modern web, smallest
    mp3    : MP3 (.mp3)                     universal fallback

ffmpeg is located via ICF_FFMPEG, then PATH, then the imageio-ffmpeg wheel.
"""
from __future__ import annotations

import hashlib
import os
import shutil
import subprocess
import tempfile
from pathlib import Path

from . import storage
from .engines import EngineError

FORMATS: dict[str, dict] = {
    "aac": {"ext": ".m4a", "content_type": "audio/mp4",
            "args": ["-c:a", "aac", "-b:a", os.environ.get("ICF_AAC_BITRATE", "96k"), "-movflags", "+faststart"]},
    "opus": {"ext": ".opus", "content_type": "audio/ogg; codecs=opus",
             "args": ["-c:a", "libopus", "-b:a", os.environ.get("ICF_OPUS_BITRATE", "48k"), "-application", "voip"]},
    "mp3": {"ext": ".mp3", "content_type": "audio/mpeg",
            "args": ["-c:a", "libmp3lame", "-b:a", os.environ.get("ICF_MP3_BITRATE", "128k")]},
}


def ffmpeg_path() -> str | None:
    p = os.environ.get("ICF_FFMPEG")
    if p and Path(p).exists():
        return p
    p = shutil.which("ffmpeg")
    if p:
        return p
    try:  # pragma: no cover - depends on the optional wheel
        import imageio_ffmpeg

        return imageio_ffmpeg.get_ffmpeg_exe()
    except Exception:
        return None


def variant_key(master_key: str, fmt: str) -> str:
    base = master_key[:-4] if master_key.endswith(".wav") else master_key
    return base + FORMATS[fmt]["ext"]


def _check_formats(formats: list[str]) -> None:
    unknown = [f for f in formats if f not in FORMATS]
    if unknown or not formats:
        raise EngineError("permanent", f"unsupported formats: {unknown or formats}")


def encode_bytes(wav: bytes, formats: list[str]) -> dict[str, tuple[bytes, str, str]]:
    """Encode a WAV master held in memory. Returns {fmt: (data, content_type, ext)}."""
    _check_formats(formats)
    if not wav.startswith(b"RIFF"):
        raise EngineError("permanent", "master is not a WAV file")
    exe = ffmpeg_path()
    if not exe:
        raise EngineError("transient", "ffmpeg is not available on this worker")
    out: dict[str, tuple[bytes, str, str]] = {}
    with tempfile.TemporaryDirectory() as tmp:
        src = Path(tmp) / "master.wav"
        src.write_bytes(wav)
        for fmt in formats:
            spec = FORMATS[fmt]
            dst = Path(tmp) / ("out" + spec["ext"])
            # The comment tag marks the file itself as synthetic, in case it
            # is ever separated from our database (section 54).
            cmd = [exe, "-hide_banner", "-loglevel", "error", "-y", "-i", str(src), "-vn",
                   "-metadata", "comment=synthetic_audio=true; AI-generated using an authorized synthetic voice",
                   *spec["args"], str(dst)]
            proc = subprocess.run(cmd, capture_output=True, timeout=600)
            if proc.returncode != 0 or not dst.exists():
                raise EngineError("permanent", f"{fmt} encode failed: {proc.stderr.decode(errors='replace')[-300:]}")
            out[fmt] = (dst.read_bytes(), spec["content_type"], spec["ext"])
    return out


def encode(master_key: str, formats: list[str]) -> dict:
    """Encode a master already in the worker's store and store the variants
    beside it (shared-bucket deployments). Returns {fmt: {key, sha256, bytes,
    content_type}}."""
    if not storage.valid_key(master_key) or not master_key.endswith(".wav"):
        raise EngineError("permanent", "master must be a stored .wav under an allowed prefix")
    _check_formats(formats)
    store = storage.get_store()
    try:
        wav = store.local_path(master_key).read_bytes()
    except (storage.StorageError, OSError) as e:
        raise EngineError("storage", str(e)) from e
    result = {}
    for fmt, (data, ctype, _ext) in encode_bytes(wav, formats).items():
        key = variant_key(master_key, fmt)
        store.put(key, data)
        result[fmt] = {"key": key, "sha256": hashlib.sha256(data).hexdigest(), "bytes": len(data), "content_type": ctype}
    return result
