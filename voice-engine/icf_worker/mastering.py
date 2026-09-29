"""Loudness mastering for synthetic minister speech.

Integrated loudness follows ITU-R BS.1770-4 (K-weighting, 400 ms blocks with
75% overlap, -70 LUFS absolute gate, -10 LU relative gate). True peak is
estimated by 4x polyphase oversampling, as BS.1770 Annex 2 recommends.

Targets are configuration, not constants: ICF_MASTER_TARGET_LUFS (default
-16) and ICF_MASTER_CEILING_DBTP (default -1). Gain is applied toward the
target but never past the true-peak ceiling; when the ceiling wins, the
achieved loudness is reported honestly rather than claimed as the target.
"""

from __future__ import annotations

import math
import os
from dataclasses import dataclass

import numpy as np
from scipy import signal


def _k_weighting(rate: int):
    """Return (b, a) pairs for the two K-weighting biquads at `rate`.

    Coefficients are derived from the analogue prototype via the bilinear
    transform so any sample rate works, not only the 48 kHz table values.
    """
    # Stage 1: high-shelf (head effects).
    f0, g, q = 1681.974450955533, 3.999843853973347, 0.7071752369554196
    k = math.tan(math.pi * f0 / rate)
    vh = 10 ** (g / 20)
    vb = vh ** 0.4996667741545416
    a0 = 1 + k / q + k * k
    b1 = [(vh + vb * k / q + k * k) / a0, 2 * (k * k - vh) / a0, (vh - vb * k / q + k * k) / a0]
    a1 = [1.0, 2 * (k * k - 1) / a0, (1 - k / q + k * k) / a0]
    # Stage 2: high-pass (RLB).
    f0, q = 38.13547087602444, 0.5003270373238773
    k = math.tan(math.pi * f0 / rate)
    a0 = 1 + k / q + k * k
    b2 = [1.0, -2.0, 1.0]
    a2 = [1.0, 2 * (k * k - 1) / a0, (1 - k / q + k * k) / a0]
    return (b1, a1), (b2, a2)


def integrated_loudness(x: np.ndarray, rate: int) -> float:
    """Integrated loudness in LUFS of mono float audio in [-1, 1]."""
    if x.size < int(0.4 * rate):
        return float("-inf")
    (b1, a1), (b2, a2) = _k_weighting(rate)
    y = signal.lfilter(b2, a2, signal.lfilter(b1, a1, x))
    block, hop = int(0.4 * rate), int(0.1 * rate)
    n = 1 + (y.size - block) // hop
    ms = np.array([np.mean(y[i * hop : i * hop + block] ** 2) for i in range(n)])
    with np.errstate(divide="ignore"):
        lk = -0.691 + 10 * np.log10(ms)
    ms = ms[lk > -70.0]
    if ms.size == 0:
        return float("-inf")
    rel = -0.691 + 10 * math.log10(ms.mean()) - 10.0
    with np.errstate(divide="ignore"):
        ms = ms[-0.691 + 10 * np.log10(ms) > rel]
    if ms.size == 0:
        return float("-inf")
    return -0.691 + 10 * math.log10(ms.mean())


def true_peak_dbtp(x: np.ndarray) -> float:
    if x.size == 0:
        return float("-inf")
    peak = float(np.max(np.abs(signal.resample_poly(x, 4, 1))))
    return 20 * math.log10(peak) if peak > 0 else float("-inf")


@dataclass
class MasterReport:
    input_lufs: float
    output_lufs: float
    output_true_peak_dbtp: float
    gain_db: float
    target_lufs: float
    ceiling_dbtp: float
    peak_limited: bool


def config_from_env() -> tuple[float, float]:
    return (
        float(os.environ.get("ICF_MASTER_TARGET_LUFS", "-16")),
        float(os.environ.get("ICF_MASTER_CEILING_DBTP", "-1")),
    )


def master(x: np.ndarray, rate: int, target_lufs: float | None = None,
           ceiling_dbtp: float | None = None) -> tuple[np.ndarray, MasterReport]:
    """Normalize toward target loudness without exceeding the peak ceiling."""
    env_target, env_ceiling = config_from_env()
    target = env_target if target_lufs is None else target_lufs
    ceiling = env_ceiling if ceiling_dbtp is None else ceiling_dbtp

    x = x.astype(np.float64)
    x = x - x.mean() if x.size else x  # remove DC before measuring
    lin = integrated_loudness(x, rate)
    if not math.isfinite(lin):
        # Silence or too short to measure: never amplify noise into speech.
        tp = true_peak_dbtp(x)
        return x, MasterReport(lin, lin, tp, 0.0, target, ceiling, False)

    gain = target - lin
    tp = true_peak_dbtp(x)
    limited = False
    if math.isfinite(tp) and tp + gain > ceiling:
        gain, limited = ceiling - tp, True
    y = x * (10 ** (gain / 20))
    return y, MasterReport(lin, integrated_loudness(y, rate), true_peak_dbtp(y), gain, target, ceiling, limited)
