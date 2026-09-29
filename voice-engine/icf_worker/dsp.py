"""Post-TTS processing chain (spec section 25).

    generated audio -> edge-silence trim -> de-click -> high-pass / EQ -> de-ess
    -> compression -> loudness normalisation -> true-peak limiting -> encoding

Every stage is gentle by default and individually configurable (ICF_DSP_*),
because the goal is clean speech that keeps the speaker's timbre. Stages
report what they did so the render headers can show the chain that ran.

Deliberately NOT here: intentional pauses are never shortened. Only
leading/trailing silence is trimmed; pauses inserted by the markup are part
of the script.
"""
from __future__ import annotations

import math
import os
from dataclasses import dataclass, field

import numpy as np
from scipy import ndimage, signal

from . import mastering


def _env_f(name: str, default: float) -> float:
    try:
        return float(os.environ.get(name, default))
    except ValueError:
        return default


def _env_on(name: str, default: bool = True) -> bool:
    v = os.environ.get(name)
    if v is None:
        return default
    return v.strip().lower() not in ("0", "false", "off", "no")


@dataclass
class ChainConfig:
    trim: bool = True
    trim_threshold_db: float = -50.0
    trim_pad_ms: float = 120.0
    declick: bool = True
    declick_threshold: float = 8.0      # spike / local MAD ratio
    highpass_hz: float = 70.0
    low_shelf_db: float = 0.0           # at 200 Hz ("warmth")
    presence_db: float = 0.0            # peak at 3 kHz ("presence")
    deess: bool = True
    deess_threshold_db: float = -28.0   # sibilant-band level that triggers reduction
    deess_max_reduction_db: float = 8.0
    compress: bool = True
    comp_threshold_db: float = -24.0
    comp_ratio: float = 2.0
    comp_attack_ms: float = 10.0
    comp_release_ms: float = 150.0
    target_lufs: float = -16.0
    ceiling_dbtp: float = -1.0

    @classmethod
    def from_env(cls) -> "ChainConfig":
        t, c = mastering.config_from_env()
        return cls(
            trim=_env_on("ICF_DSP_TRIM"),
            trim_threshold_db=_env_f("ICF_DSP_TRIM_THRESHOLD_DB", -50),
            trim_pad_ms=_env_f("ICF_DSP_TRIM_PAD_MS", 120),
            declick=_env_on("ICF_DSP_DECLICK"),
            declick_threshold=_env_f("ICF_DSP_DECLICK_THRESHOLD", 8),
            highpass_hz=_env_f("ICF_DSP_HIGHPASS_HZ", 70),
            low_shelf_db=_env_f("ICF_DSP_LOW_SHELF_DB", 0),
            presence_db=_env_f("ICF_DSP_PRESENCE_DB", 0),
            deess=_env_on("ICF_DSP_DEESS"),
            deess_threshold_db=_env_f("ICF_DSP_DEESS_THRESHOLD_DB", -28),
            deess_max_reduction_db=_env_f("ICF_DSP_DEESS_MAX_DB", 8),
            compress=_env_on("ICF_DSP_COMPRESS"),
            comp_threshold_db=_env_f("ICF_DSP_COMP_THRESHOLD_DB", -24),
            comp_ratio=max(1.0, _env_f("ICF_DSP_COMP_RATIO", 2)),
            target_lufs=t,
            ceiling_dbtp=c,
        )


@dataclass
class ChainReport:
    stages: list[str] = field(default_factory=list)
    trimmed_ms: float = 0.0
    clicks_repaired: int = 0
    deess_max_db: float = 0.0
    comp_max_db: float = 0.0
    master: mastering.MasterReport | None = None
    limiter_max_db: float = 0.0


# ------------------------------------------------------------------ stages

def trim_edges(x: np.ndarray, rate: int, threshold_db: float, pad_ms: float) -> tuple[np.ndarray, float]:
    """Trim leading/trailing silence, keeping pad_ms of air on each side."""
    if x.size == 0:
        return x, 0.0
    win = max(1, int(rate * 0.01))
    frames = x[: x.size - x.size % win].reshape(-1, win) if x.size >= win else x.reshape(1, -1)
    rms_db = 20 * np.log10(np.sqrt(np.mean(frames ** 2, axis=1)) + 1e-12)
    loud = np.flatnonzero(rms_db > threshold_db)
    if loud.size == 0:
        return x, 0.0
    pad = int(rate * pad_ms / 1000)
    start = max(0, loud[0] * win - pad)
    end = min(x.size, (loud[-1] + 1) * win + pad)
    return x[start:end], 1000.0 * (x.size - (end - start)) / rate


def declick(x: np.ndarray, threshold: float) -> tuple[np.ndarray, int]:
    """Repair isolated impulsive spikes (vocoder clicks) with a local median.

    A sample is a click when it deviates from its 5-sample median by more than
    `threshold` times the local median absolute deviation. Speech transients
    span many samples and are left alone.
    """
    if x.size < 16:
        return x, 0
    med = signal.medfilt(x, 5)
    dev = np.abs(x - med)
    mad = signal.medfilt(dev, 31) + 1e-6
    clicks = (dev > threshold * mad) & (dev > 0.05)
    n = int(clicks.sum())
    if n:
        x = x.copy()
        x[clicks] = med[clicks]
    return x, n


def _biquad_peak(f0: float, gain_db: float, q: float, rate: int):
    a = 10 ** (gain_db / 40)
    w = 2 * math.pi * f0 / rate
    alpha = math.sin(w) / (2 * q)
    b = [1 + alpha * a, -2 * math.cos(w), 1 - alpha * a]
    den = [1 + alpha / a, -2 * math.cos(w), 1 - alpha / a]
    return np.array(b) / den[0], np.array(den) / den[0]


def _biquad_lowshelf(f0: float, gain_db: float, rate: int):
    a = 10 ** (gain_db / 40)
    w = 2 * math.pi * f0 / rate
    alpha = math.sin(w) / 2 * math.sqrt(2)
    cw = math.cos(w)
    b = [a * ((a + 1) - (a - 1) * cw + 2 * math.sqrt(a) * alpha),
         2 * a * ((a - 1) - (a + 1) * cw),
         a * ((a + 1) - (a - 1) * cw - 2 * math.sqrt(a) * alpha)]
    den = [(a + 1) + (a - 1) * cw + 2 * math.sqrt(a) * alpha,
           -2 * ((a - 1) + (a + 1) * cw),
           (a + 1) + (a - 1) * cw - 2 * math.sqrt(a) * alpha]
    return np.array(b) / den[0], np.array(den) / den[0]


def equalize(x: np.ndarray, rate: int, highpass_hz: float, low_shelf_db: float, presence_db: float) -> np.ndarray:
    if highpass_hz > 0 and highpass_hz < rate / 2:
        sos = signal.butter(2, highpass_hz, "highpass", fs=rate, output="sos")
        x = signal.sosfilt(sos, x)
    if abs(low_shelf_db) > 0.01:
        b, a = _biquad_lowshelf(200.0, low_shelf_db, rate)
        x = signal.lfilter(b, a, x)
    if abs(presence_db) > 0.01 and 3000 < rate / 2:
        b, a = _biquad_peak(3000.0, presence_db, 1.0, rate)
        x = signal.lfilter(b, a, x)
    return x


def _envelope(x: np.ndarray, rate: int, attack_ms: float, release_ms: float) -> np.ndarray:
    """Peak envelope follower with separate attack and release.

    Runs at 1 ms block rate (per-block peak) and is interpolated back to the
    sample rate, so a 30-minute render costs ~1.8M loop steps, not 43M.
    """
    if x.size == 0:
        return x.copy()
    blk = max(1, rate // 1000)
    n = -(-x.size // blk)
    padded = np.pad(np.abs(x), (0, n * blk - x.size))
    peaks = padded.reshape(n, blk).max(axis=1)
    a = math.exp(-1.0 / max(1.0, attack_ms))    # per 1 ms block
    r = math.exp(-1.0 / max(1.0, release_ms))
    env = np.empty(n)
    level = 0.0
    for i in range(n):
        v = peaks[i]
        coef = a if v > level else r
        level = coef * level + (1 - coef) * v
        env[i] = level
    centers = np.arange(n) * blk + blk / 2
    return np.interp(np.arange(x.size), centers, env)


def deess(x: np.ndarray, rate: int, threshold_db: float, max_red_db: float) -> tuple[np.ndarray, float]:
    """Dynamic sibilance reduction on a 5-9 kHz band only.

    The band is split off, attenuated where its envelope exceeds the
    threshold, and recombined, so vowels and body are untouched.
    """
    hi = min(9000.0, rate / 2 * 0.95)
    if hi <= 5000:
        return x, 0.0
    sos = signal.butter(2, [5000.0, hi], "bandpass", fs=rate, output="sos")
    band = signal.sosfiltfilt(sos, x)
    env_db = 20 * np.log10(_envelope(band, rate, 2, 60) + 1e-9)
    over = np.clip(env_db - threshold_db, 0, None)
    red_db = np.minimum(over * 0.75, max_red_db)
    g = 10 ** (-red_db / 20)
    return x - band + band * g, float(red_db.max()) if red_db.size else 0.0


def compress(x: np.ndarray, rate: int, threshold_db: float, ratio: float,
             attack_ms: float, release_ms: float) -> tuple[np.ndarray, float]:
    """Gentle downward compression; loudness is restored by normalisation."""
    if ratio <= 1.0:
        return x, 0.0
    env_db = 20 * np.log10(_envelope(x, rate, attack_ms, release_ms) + 1e-9)
    over = np.clip(env_db - threshold_db, 0, None)
    red_db = over * (1 - 1 / ratio)
    return x * 10 ** (-red_db / 20), float(red_db.max()) if red_db.size else 0.0


def true_peak_limit(x: np.ndarray, rate: int, ceiling_dbtp: float, lookahead_ms: float = 5.0,
                    release_ms: float = 80.0) -> tuple[np.ndarray, float]:
    """Look-ahead limiter driven by 4x-oversampled (true) peaks.

    Gain reduction is computed from the oversampled peak envelope, smoothed so
    it ramps down *before* the peak (look-ahead) and recovers slowly. A final
    exact check scales the whole file if any residual overshoot remains, so
    the ceiling is a guarantee rather than an approximation.
    """
    ceiling = 10 ** (ceiling_dbtp / 20)
    if x.size == 0:
        return x, 0.0
    over = np.abs(signal.resample_poly(x, 4, 1))
    peaks = over[: (over.size // 4) * 4].reshape(-1, 4).max(axis=1)
    if peaks.size < x.size:
        peaks = np.pad(peaks, (0, x.size - peaks.size), mode="edge")
    need = np.minimum(1.0, ceiling / np.maximum(peaks, 1e-12))
    la = max(1, int(rate * lookahead_ms / 1000))
    # Minimum over the upcoming window: gain starts falling before the peak.
    ahead = ndimage.minimum_filter1d(need, size=la, origin=-(la // 2), mode="nearest")
    # Release smoothing at 1 ms blocks: instant attack, exponential recovery.
    blk = max(1, rate // 1000)
    nb = -(-ahead.size // blk)
    blocks = np.pad(ahead, (0, nb * blk - ahead.size), constant_values=1.0).reshape(nb, blk).min(axis=1)
    rel = math.exp(-1.0 / max(1.0, release_ms))
    gb = np.empty(nb)
    level = 1.0
    for i in range(nb):
        v = blocks[i]
        level = v if v < level else rel * level + (1 - rel) * v
        gb[i] = level
    g = np.minimum(np.repeat(gb, blk)[: x.size], ahead)
    y = x * g
    tp = mastering.true_peak_dbtp(y)
    if math.isfinite(tp) and tp > ceiling_dbtp:
        y = y * 10 ** ((ceiling_dbtp - tp - 0.05) / 20)
    red = float(-20 * np.log10(max(g.min(), 1e-9)))
    return y, red


# ------------------------------------------------------------------ chain

def process(x: np.ndarray, rate: int, cfg: ChainConfig | None = None) -> tuple[np.ndarray, ChainReport]:
    """Run the full chain. Silence is never amplified."""
    cfg = cfg or ChainConfig.from_env()
    rep = ChainReport()
    x = np.asarray(x, dtype=np.float64)
    if x.size:
        x = x - x.mean()
        rep.stages.append("dc")
    if cfg.trim:
        x, rep.trimmed_ms = trim_edges(x, rate, cfg.trim_threshold_db, cfg.trim_pad_ms)
        rep.stages.append("trim")
    if cfg.declick:
        x, rep.clicks_repaired = declick(x, cfg.declick_threshold)
        rep.stages.append("declick")
    x = equalize(x, rate, cfg.highpass_hz, cfg.low_shelf_db, cfg.presence_db)
    rep.stages.append("eq")
    if cfg.deess:
        x, rep.deess_max_db = deess(x, rate, cfg.deess_threshold_db, cfg.deess_max_reduction_db)
        rep.stages.append("deess")
    if cfg.compress:
        x, rep.comp_max_db = compress(x, rate, cfg.comp_threshold_db, cfg.comp_ratio,
                                      cfg.comp_attack_ms, cfg.comp_release_ms)
        rep.stages.append("compress")
    # Normalise with the ceiling lifted, then let the true-peak limiter (not
    # a gain cut) enforce it - so loud peaks cost a few ms of limiting rather
    # than the whole file missing the loudness target.
    lufs = mastering.integrated_loudness(x, rate)
    if math.isfinite(lufs):
        x = x * 10 ** ((cfg.target_lufs - lufs) / 20)
    rep.stages.append("loudness")
    x, rep.limiter_max_db = true_peak_limit(x, rate, cfg.ceiling_dbtp)
    rep.stages.append("limit")
    out_lufs = mastering.integrated_loudness(x, rate)
    tp = mastering.true_peak_dbtp(x)
    rep.master = mastering.MasterReport(lufs, out_lufs, tp, (cfg.target_lufs - lufs) if math.isfinite(lufs) else 0.0,
                                        cfg.target_lufs, cfg.ceiling_dbtp, rep.limiter_max_db > 0.01)
    return x, rep
