import base64
import json
import math
import subprocess

import numpy as np
import pytest

from icf_worker import dsp, encode, mastering, wav

RATE = 24000


def speechlike(seconds=3.0, rate=RATE, seed=1):
    """Amplitude-modulated harmonic signal with pauses: loud, peaky, speech-ish."""
    rng = np.random.default_rng(seed)
    t = np.arange(int(seconds * rate)) / rate
    f0 = 140 + 20 * np.sin(2 * np.pi * 0.7 * t)
    phase = 2 * np.pi * np.cumsum(f0) / rate
    x = sum(np.sin(k * phase) / k for k in range(1, 12))
    syll = (np.sin(2 * np.pi * 3.5 * t) > -0.2).astype(float)
    x = x * syll * 0.3 + 0.002 * rng.standard_normal(t.size)
    x[int(1.2 * rate):int(1.7 * rate)] = 0.0  # an intentional 500 ms pause
    return x


def test_chain_hits_loudness_and_guarantees_true_peak():
    x = speechlike() * 3.0  # clipping-hot input
    y, rep = dsp.process(x, RATE, dsp.ChainConfig(target_lufs=-16, ceiling_dbtp=-1))
    assert mastering.true_peak_dbtp(y) <= -1.0 + 1e-6
    # Limiting (not a whole-file gain cut) enforces the ceiling, so loudness
    # stays close to target even for peaky input.
    assert abs(mastering.integrated_loudness(y, RATE) - (-16)) < 1.5
    assert rep.stages == ["dc", "trim", "declick", "eq", "deess", "compress", "loudness", "limit"]


def test_declick_repairs_isolated_spikes_only():
    x = speechlike(1.0)
    clicked = x.copy()
    for i in (3000, 9000, 15000):
        clicked[i] += 0.9
    y, n = dsp.declick(clicked, 8.0)
    assert n >= 3
    assert np.max(np.abs(y - x)) < 0.2
    _, n_clean = dsp.declick(x, 8.0)
    assert n_clean <= 2  # speech itself is not "repaired"


def test_trim_keeps_air_and_internal_pauses():
    x = np.concatenate([np.zeros(RATE), speechlike(2.0), np.zeros(RATE)])
    y, trimmed = dsp.trim_edges(x, RATE, -50, 120)
    assert 1700 < trimmed < 2000
    # The 500 ms pause inside the speech survives.
    inner = y[int(0.12 * RATE):]
    silent = np.abs(inner) < 1e-9
    run = max(len(s) for s in "".join("1" if v else "0" for v in silent).split("0"))
    assert run >= int(0.45 * RATE)


def test_silence_is_never_amplified():
    x = np.zeros(RATE)
    y, _ = dsp.process(x, RATE)
    assert np.max(np.abs(y)) == 0.0


def test_deess_reduces_only_the_sibilant_band():
    t = np.arange(RATE) / RATE
    low = 0.3 * np.sin(2 * np.pi * 200 * t)
    hiss = 0.3 * np.sin(2 * np.pi * 7000 * t)
    y, red = dsp.deess(low + hiss, RATE, -28, 8)
    assert red > 3
    spec = np.abs(np.fft.rfft(y))
    f = np.fft.rfftfreq(y.size, 1 / RATE)
    lo_ratio = spec[np.argmin(abs(f - 200))] / np.abs(np.fft.rfft(low))[np.argmin(abs(f - 200))]
    hi_ratio = spec[np.argmin(abs(f - 7000))] / np.abs(np.fft.rfft(hiss))[np.argmin(abs(f - 7000))]
    assert lo_ratio > 0.9 and hi_ratio < 0.8


def test_long_render_is_fast_enough():
    import time

    x = np.tile(speechlike(10.0), 30)  # five minutes
    t0 = time.time()
    dsp.process(x, RATE)
    assert time.time() - t0 < 60


def test_chain_is_configurable_off():
    cfg = dsp.ChainConfig(trim=False, declick=False, deess=False, compress=False, highpass_hz=0)
    _, rep = dsp.process(speechlike(), RATE, cfg)
    assert rep.stages == ["dc", "eq", "loudness", "limit"]


needs_ffmpeg = pytest.mark.skipif(encode.ffmpeg_path() is None, reason="ffmpeg not available")


@needs_ffmpeg
def test_encode_bytes_produces_decodable_tagged_variants():
    master = wav.encode(speechlike(2.0) * 0.5, RATE)
    out = encode.encode_bytes(master, ["aac", "opus", "mp3"])
    assert set(out) == {"aac", "opus", "mp3"}
    for fmt, (data, ctype, ext) in out.items():
        assert len(data) > 1000 and len(data) < len(master), fmt
        p = f"/tmp/icf_enc_test{ext}"
        open(p, "wb").write(data)
        probe = subprocess.run([encode.ffmpeg_path(), "-hide_banner", "-i", p, "-f", "null", "-"],
                               capture_output=True, text=True)
        assert probe.returncode == 0, probe.stderr
        assert "synthetic_audio=true" in probe.stderr, fmt  # provenance tag survived


def test_encode_rejects_bad_input():
    with pytest.raises(Exception):
        encode.encode_bytes(b"not a wav", ["aac"])
    with pytest.raises(Exception):
        encode.encode_bytes(b"RIFF....", ["flac-lossless-9000"])
