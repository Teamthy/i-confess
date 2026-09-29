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
| `ICF_MASTER_CEILING_DBTP` | `-1` | true-peak ceiling; takes priority over the target |
| `ICF_WORKER_TOKEN` | — | bearer token, required in production |

The response headers include `X-Loudness-LUFS`, `X-True-Peak-dBTP` and
`X-Peak-Limited`, which report the loudness actually achieved, and
`X-Synthetic: true`.

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

- **Tested:** the dev-tone engine, ingestion (energy VAD, a speaker-consistency
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
