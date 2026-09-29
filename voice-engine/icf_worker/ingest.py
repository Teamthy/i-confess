"""Recording intake: clean -> VAD -> segment -> speaker check -> ASR -> quality.

Every stage records in the report what actually ran, so a reviewer can tell a
measurement from a missing one:

- VAD: an adaptive energy detector with hysteresis. It is dependable on
  clean studio and pulpit microphones but not on music beds. Treat it as a
  segmenter, not a judge.
- Speaker check: with a verified reference clip, each segment is compared to
  it. Without one, each segment is compared to the recording's majority
  voice. By default the features are standardized log-mel statistics. This
  is a heuristic consistency score, NOT a speaker-verification model; set
  ICF_SPEAKER_MODEL=pyannote to use a pyannote embedding model if its licence
  has been reviewed.
- ASR: optional (ICF_ASR=faster_whisper). Its output is only a draft for the
  human reviewer. Nothing enters a dataset without a verified transcript.
- Quality: a documented heuristic composite of SNR, clipping, duration and
  level. It is used for screening and sorting, not as a claim of fitness.

No stage denoises or otherwise alters the voice beyond DC removal and a gentle
high-pass. Aggressive denoising puts artifacts into training data.
"""

from __future__ import annotations

import io
import math
import os
from dataclasses import dataclass, field

import numpy as np
from scipy import signal

from . import wav
from .storage import get_store

ANALYSIS_RATE = 16000


# ----------------------------------------------------------------- loading

def load_audio(path) -> tuple[np.ndarray, int]:
    data = open(path, "rb").read()
    if str(path).lower().endswith(".flac"):
        try:
            import soundfile as sf  # type: ignore
        except ImportError as e:  # pragma: no cover
            raise ValueError(f"FLAC needs the soundfile package: {e}")
        x, rate = sf.read(io.BytesIO(data), dtype="float64", always_2d=True)
        return x.mean(axis=1), rate
    return wav.decode(data)


def clean(x: np.ndarray, rate: int) -> np.ndarray:
    x = x - np.mean(x)
    sos = signal.butter(4, 70, btype="highpass", fs=rate, output="sos")
    return signal.sosfiltfilt(sos, x)


# ----------------------------------------------------------------- VAD

@dataclass
class VADConfig:
    frame_ms: int = 30
    hop_ms: int = 10
    on_db: float = 12.0     # above noise floor to enter speech
    off_db: float = 6.0     # above noise floor to stay in speech
    min_speech_ms: int = 250
    merge_gap_ms: int = 350
    max_segment_ms: int = 15000
    target_split_ms: int = 9000


def frame_db(x: np.ndarray, rate: int, frame_ms: int, hop_ms: int) -> np.ndarray:
    f, h = int(rate * frame_ms / 1000), int(rate * hop_ms / 1000)
    if x.size < f:
        return np.array([-120.0])
    n = 1 + (x.size - f) // h
    idx = np.arange(f)[None, :] + h * np.arange(n)[:, None]
    p = np.mean(x[idx] ** 2, axis=1)
    return 10 * np.log10(p + 1e-12)


def energy_vad(x: np.ndarray, rate: int, cfg: VADConfig) -> tuple[list[tuple[int, int]], float, np.ndarray]:
    """Return speech regions in ms, the noise floor in dB, and per-frame dB."""
    db = frame_db(x, rate, cfg.frame_ms, cfg.hop_ms)
    floor = float(np.percentile(db, 10))
    on, off = max(floor + cfg.on_db, -55.0), max(floor + cfg.off_db, -60.0)
    regions, active, start = [], False, 0
    for i, v in enumerate(db):
        if not active and v >= on:
            active, start = True, i
        elif active and v < off:
            active = False
            regions.append((start, i))
    if active:
        regions.append((start, len(db)))
    ms = [(s * cfg.hop_ms, e * cfg.hop_ms + cfg.frame_ms) for s, e in regions]
    merged: list[list[int]] = []
    for s, e in ms:
        if merged and s - merged[-1][1] < cfg.merge_gap_ms:
            merged[-1][1] = e
        else:
            merged.append([s, e])
    out = []
    for s, e in merged:
        if e - s >= cfg.min_speech_ms:
            out.extend(split_long(s, e, db, cfg))
    return out, floor, db


def split_long(s: int, e: int, db: np.ndarray, cfg: VADConfig) -> list[tuple[int, int]]:
    """Split a long region at its quietest point near the target length."""
    out = []
    while e - s > cfg.max_segment_ms:
        lo = (s + cfg.target_split_ms // 2) // cfg.hop_ms
        hi = min((s + cfg.max_segment_ms) // cfg.hop_ms, len(db) - 1)
        cut = lo + int(np.argmin(db[lo:hi])) if hi > lo else hi
        out.append((s, cut * cfg.hop_ms))
        s = cut * cfg.hop_ms
    out.append((s, e))
    return out


# ----------------------------------------------------------------- speaker

def mel_filterbank(rate: int, n_fft: int, n_mels: int = 40) -> np.ndarray:
    def hz2mel(f):
        return 2595 * np.log10(1 + f / 700)

    def mel2hz(m):
        return 700 * (10 ** (m / 2595) - 1)

    pts = mel2hz(np.linspace(hz2mel(60), hz2mel(rate / 2 - 200), n_mels + 2))
    bins = np.floor((n_fft + 1) * pts / rate).astype(int)
    fb = np.zeros((n_mels, n_fft // 2 + 1))
    for m in range(1, n_mels + 1):
        a, b, c = bins[m - 1], bins[m], bins[m + 1]
        for k in range(a, b):
            fb[m - 1, k] = (k - a) / max(b - a, 1)
        for k in range(b, c):
            fb[m - 1, k] = (c - k) / max(c - b, 1)
    return fb


_FB = mel_filterbank(ANALYSIS_RATE, 512)


def voice_features(x16: np.ndarray) -> np.ndarray:
    """Mean and std of log-mel energies over voiced frames (80 dims)."""
    if x16.size < 512:
        x16 = np.pad(x16, (0, 512 - x16.size))
    _, _, z = signal.stft(x16, fs=ANALYSIS_RATE, nperseg=400, nfft=512, noverlap=240)
    lm = np.log(_FB @ (np.abs(z) ** 2) + 1e-9)
    energy = lm.mean(axis=0)
    voiced = lm[:, energy >= np.percentile(energy, 40)]
    if voiced.shape[1] == 0:
        voiced = lm
    return np.concatenate([voiced.mean(axis=1), voiced.std(axis=1)])


def speaker_confidences(feats: list[np.ndarray], ref: np.ndarray | None) -> list[float]:
    """Cosine similarity (mapped to 0..1) of each segment to the reference, or
    to the median segment when there is no reference. Features are
    standardized across the set so that shared channel traits do not dominate.
    """
    if not feats:
        return []
    allf = np.stack(feats + ([ref] if ref is not None else []))
    mu, sd = allf.mean(axis=0), allf.std(axis=0) + 1e-6
    z = (allf - mu) / sd
    target = z[-1] if ref is not None else np.median(z[: len(feats)], axis=0)
    out = []
    for v in z[: len(feats)]:
        denom = np.linalg.norm(v) * np.linalg.norm(target)
        cos = float(v @ target / denom) if denom > 0 else 0.0
        out.append(round((cos + 1) / 2, 4))
    return out


# ----------------------------------------------------------------- ASR

class ASR:
    name = "not_run"

    def transcribe(self, x16: np.ndarray, language: str) -> tuple[str, float | None]:
        return "", None


class FasterWhisperASR(ASR):  # pragma: no cover - needs the model
    name = "faster_whisper"

    def __init__(self):
        from faster_whisper import WhisperModel  # type: ignore

        self.model = WhisperModel(os.environ.get("ICF_ASR_MODEL", "large-v3"),
                                  device=os.environ.get("ICF_ASR_DEVICE", "auto"))
        self.name = f"faster_whisper:{os.environ.get('ICF_ASR_MODEL', 'large-v3')}"

    def transcribe(self, x16, language):
        segs, _ = self.model.transcribe(x16.astype(np.float32), language=language.split("-")[0], beam_size=5,
                                        vad_filter=False, condition_on_previous_text=False)
        segs = list(segs)
        text = " ".join(s.text.strip() for s in segs).strip()
        if not segs:
            return "", None
        return text, round(float(math.exp(np.mean([s.avg_logprob for s in segs]))), 4)


def load_asr() -> ASR:
    if os.environ.get("ICF_ASR") == "faster_whisper":
        return FasterWhisperASR()
    return ASR()


# ----------------------------------------------------------------- quality

def quality_score(snr_db: float, clip_ratio: float, dur_ms: int, rms_db: float) -> float:
    """Heuristic 0..1 composite (weights: SNR .4, clipping .2, duration .2, level .2)."""
    snr_s = min(max((snr_db - 10) / 30, 0.0), 1.0)
    clip_s = 1.0 - min(clip_ratio * 1000, 1.0)
    if 2000 <= dur_ms <= 12000:
        dur_s = 1.0
    elif dur_ms < 2000:
        dur_s = max(dur_ms / 2000, 0.0)
    else:
        dur_s = max(1 - (dur_ms - 12000) / 8000, 0.0)
    if -30 <= rms_db <= -12:
        lvl_s = 1.0
    else:
        lvl_s = max(1 - min(abs(rms_db + 30), abs(rms_db + 12)) / 15, 0.0)
    return round(0.4 * snr_s + 0.2 * clip_s + 0.2 * dur_s + 0.2 * lvl_s, 4)


# ----------------------------------------------------------------- pipeline

@dataclass
class IngestResult:
    duration_ms: int
    segments: list[dict] = field(default_factory=list)
    report: dict = field(default_factory=dict)


def ingest(req: dict, asr: ASR | None = None, vad_cfg: VADConfig | None = None) -> IngestResult:
    store = get_store()
    vad_cfg = vad_cfg or VADConfig()
    asr = asr or load_asr()
    raw, rate = load_audio(store.local_path(req["audio_key"]))
    if raw.size == 0:
        raise ValueError("recording is empty")
    x = clean(raw, rate)
    x16 = signal.resample_poly(x, ANALYSIS_RATE, rate) if rate != ANALYSIS_RATE else x
    regions, floor_db, _ = energy_vad(x16, ANALYSIS_RATE, vad_cfg)

    ref_feat, speaker_mode = None, "majority_voice_logmel_heuristic"
    if req.get("reference_key"):
        try:
            r, rr = load_audio(store.local_path(req["reference_key"]))
            r = clean(r, rr)
            ref_feat = voice_features(signal.resample_poly(r, ANALYSIS_RATE, rr) if rr != ANALYSIS_RATE else r)
            speaker_mode = "reference_similarity_logmel_heuristic"
        except Exception as e:  # noqa: BLE001 - a bad reference must not kill intake
            speaker_mode = f"majority_voice_logmel_heuristic (reference unusable: {e})"

    prefix = req["segment_prefix"].rstrip("/")
    segs, feats = [], []
    for i, (s, e) in enumerate(regions):
        a16, b16 = s * ANALYSIS_RATE // 1000, e * ANALYSIS_RATE // 1000
        a, b = s * rate // 1000, min(e * rate // 1000, x.size)
        if b <= a:
            continue
        seg, seg16, seg_raw = x[a:b], x16[a16:b16], raw[a:b]
        p = float(np.mean(seg ** 2)) + 1e-12
        rms_db = 10 * math.log10(p)
        snr = round(rms_db - floor_db, 2)
        clip = round(float(np.mean(np.abs(seg_raw) >= 0.999)), 6)
        key = f"{prefix}/{i:04d}.wav"
        store.put(key, wav.encode(seg, rate))
        text, conf = asr.transcribe(seg16, req.get("language", "en"))
        feats.append(voice_features(seg16))
        segs.append({"start_ms": s, "end_ms": e, "audio_key": key, "raw_transcript": text, "asr_confidence": conf,
                     "snr_db": snr, "clipping_ratio": clip, "quality": quality_score(snr, clip, e - s, rms_db)})
    for sg, c in zip(segs, speaker_confidences(feats, ref_feat)):
        sg["speaker_confidence"] = c

    report = {"vad": "energy_hysteresis", "vad_config": vad_cfg.__dict__, "noise_floor_db": round(floor_db, 2),
              "speaker_check": speaker_mode, "asr": asr.name, "cleaning": "dc_removal+highpass_70hz",
              "quality": "heuristic: snr .4, clipping .2, duration .2, level .2",
              "source_sample_rate": rate, "segments_found": len(segs)}
    return IngestResult(duration_ms=int(1000 * raw.size / rate), segments=segs, report=report)
