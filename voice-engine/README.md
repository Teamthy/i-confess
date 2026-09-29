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

## Not implemented yet

- GPT-SoVITS and VoxCPM backends. Their Go adapters exist, but each Python
  backend waits until that engine's licence is verified in
  `docs/model_licenses.json`.
- The ingestion pipeline (VAD, diarization, ASR, quality scoring) and the
  training/dataset APIs.
- Streaming synthesis.
