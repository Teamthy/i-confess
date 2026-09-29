# voice-engine: GPU inference worker

Minister-voice synthesis runs here, **never in the Go API or the Flutter
app**. Here is the request flow:

```
client → POST /v1/voices/generate (Go API)
           rights check (voicegov) → model + reference selection → content hash
           cache hit? → signed URL           miss → enqueue job "voice.generate"
Go queue worker → re-checks rights (revocation wins) → HTTP → this worker
this worker → engine inference → BS.1770 mastering → WAV + provenance headers
Go → object storage (private key) → generation COMPLETED → signed CDN URL
```

## Run

```bash
pip install -r requirements.txt
export ICF_WORKER_TOKEN=...            # shared with the API's VOICE_ENGINE_TOKEN
export ICF_CHECKPOINT_ROOT=/models     # synced from the private bucket
export ICF_COSYVOICE_MODEL_DIR=/models/cosyvoice/<checkpoint>
python -m icf_worker.server --engine cosyvoice --port 8601
```

Then point the API at it: `VOICE_ENGINE_COSYVOICE_URL=http://gpu-1:8601`.

For local plumbing without a GPU, `ICF_WORKER_DEV=1 python -m
icf_worker.server --engine dev-tone` emits a tone, never speech. It refuses to
start when `ICF_ENV=production`.

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `ICF_MASTER_TARGET_LUFS` | `-16` | integrated loudness target |
| `ICF_MASTER_CEILING_DBTP` | `-1` | true-peak ceiling, enforced by the limiter (guaranteed) |
| `ICF_DSP_TRIM` / `_THRESHOLD_DB` / `_PAD_MS` | on / `-50` / `120` | trim leading and trailing silence only; pauses inside the script are never shortened |
| `ICF_DSP_DECLICK` / `_THRESHOLD` | on / `8` | repair isolated vocoder clicks (spike vs local deviation) |
| `ICF_DSP_HIGHPASS_HZ` | `70` | rumble filter; `0` disables |
| `ICF_DSP_LOW_SHELF_DB`, `ICF_DSP_PRESENCE_DB` | `0`, `0` | optional warmth (200 Hz) and presence (3 kHz) EQ |
| `ICF_DSP_DEESS` / `_THRESHOLD_DB` / `_MAX_DB` | on / `-28` / `8` | dynamic reduction of the 5–9 kHz band only |
| `ICF_DSP_COMPRESS` / `_THRESHOLD_DB` / `_RATIO` | on / `-24` / `2` | gentle compression before loudness normalisation |
| `ICF_FFMPEG` | PATH, then the `imageio-ffmpeg` wheel | ffmpeg used for delivery encodings |
| `ICF_AAC_BITRATE`, `ICF_OPUS_BITRATE`, `ICF_MP3_BITRATE` | `96k`, `48k`, `128k` | delivery encoding bitrates |
| `ICF_WORKER_TOKEN` | — | bearer token, required in production |

### Post-processing chain (`icf_worker/dsp.py`)

DC removal → edge-silence trim → de-click → high-pass/EQ → de-ess →
compression → loudness normalisation → look-ahead true-peak limiter (4×
oversampled). Loudness is normalised first and the limiter then enforces the
ceiling. A few peaky milliseconds cost a little limiting, and the whole file
stays on target. A final exact check guarantees the ceiling holds.

Response headers report what happened: `X-Loudness-LUFS`, `X-True-Peak-dBTP`,
`X-Peak-Limited`, `X-DSP-Chain`, `X-Limiter-Max-dB`, `X-Trimmed-Ms`, and
`X-Synthetic: true`.

### Delivery encodings (`POST /v1/encode`)

The WAV master is the canonical asset. After the master is stored, the Go
generation job posts it (`audio_b64`) and receives AAC (`.m4a`), Opus and MP3
variants, each with a checksum it verifies. It uploads them next to the
master under the same content-hash path, so an identical render is never
encoded twice. Each encoded file carries a `synthetic_audio=true` comment tag,
so the file still identifies itself if it is ever separated from the
database. Encoding failures are non-fatal: clients get the WAV. Deployments
that share one bucket can send `{"key": ...}` instead, and the worker reads
and writes the store directly. The rights sweep deletes variants together with
the master under the `delete` post-termination policy.

## Engine benchmark (`server/cmd/voice-bench`)

```
cd server
VOICE_ENGINE_TOKEN=... go run ./cmd/voice-bench \
  -engine cosyvoice=http://gpu-1:8000 -engine gptsovits=http://gpu-2:8000 \
  -runs 3 -stream -out /tmp/bench \
  # to clone a licensed voice (requires an approved grant):
  -voice voice_ab12 -rights-confirmed \
  -reference-uri voice-private/voices/voice_ab12/refs/r1.wav -reference-transcript "..." \
  # optional: quality scorer printing {"speaker_similarity":0.83,...} in [0,1]
  -scorer "python3 my_scorer.py"
```

The benchmark runs `docs/voice/golden_set.json` through the production
provider adapters and markup chunking. It writes `report.md`/`report.json`
(p50/p95 latency, time to first audio, RTF, failures by class, licence
status), every render, and a blind listening pack (`blind/`, with
`blind_key.json` kept separate for unblinding). Without a scorer, quality is
reported as **not measured** and no ICF_VOICE_SCORE is produced. Scorer values
outside [0,1] are rejected, not clamped. The tool never picks the winning
engine; people decide.

## Tests

```bash
pip install -r requirements.txt -r requirements-dev.txt
python -m pytest -q tests
```

### Go ↔ worker interop

This test drives the real worker from the Go client (`server/internal/voiceengine/interop_test.go`).
It covers synthesis, streaming, ingestion and, optionally, a training run. It
is skipped unless the URL is set.

```bash
# terminal 1: worker with the dependency-free dev engine
ICF_WORKER_TOKEN=interop ICF_STORAGE_ROOT=/tmp/interop/objects ICF_TRAIN_DIR=/tmp/interop \
ICF_TRAIN_CMD_GPT_SOVITS='python /path/to/stub_trainer.py {manifest} {out}' \
  python -m icf_worker.server --engine dev-tone --host 127.0.0.1 --port 8601

# terminal 2
cd server
VOICE_WORKER_INTEROP_URL=http://127.0.0.1:8601 VOICE_WORKER_INTEROP_TOKEN=interop \
VOICE_WORKER_INTEROP_TRAIN=1 go test ./internal/voiceengine/ -run Interop -v
```

The trainer only has to print `ICF_PROGRESS` lines and write a checkpoint into
`{out}`, so any stub will do.

## Status and limits

- **Tested:** the post-processing chain and ffmpeg encodings (real ffmpeg
  7), the dev-tone engine, ingestion (energy VAD, a speaker-consistency
  heuristic, quality scoring, and optional faster-whisper ASR via `ICF_ASR`),
  streaming, and training orchestration with an external trainer command.
- **Written but not yet run against real models:** the GPT-SoVITS backend
  (proxies GPT-SoVITS `api_v2`) and the VoxCPM backend. Run the benchmark
  harness on a GPU host before relying on either.
- **Licence gate:** `docs/model_licenses.json` sets `production_allowed: false`
  for every engine. With `ICF_ENV=production`, the worker refuses to load or
  train an engine until counsel has verified its licence and flipped that
  flag.
- **Speaker check:** the log-mel speaker check is a heuristic, not a
  diarization model. Treat its confidence as a triage signal. Human segment
  review in the admin Voice Studio is still required.
