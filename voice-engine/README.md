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
export ICF_CHECKPOINT_REVISION=cosyvoice2-2026-09   # optional; see below
python -m icf_worker.server --engine cosyvoice --port 8601
```

Then point the API at it: `VOICE_ENGINE_COSYVOICE_URL=http://gpu-1:8601`.

`engine_version` is reported per request (`X-Engine-Version`) and stored on every
generation, and it is part of the content hash — so it has to name the weights,
not just the code. Each backend derives it from its checkpoint (the directory
listing of `ICF_COSYVOICE_MODEL_DIR`, `ICF_VOXCPM_MODEL`, or the fine-tuned
GPT-SoVITS checkpoint once one is pushed), giving `0+<12 hex>`. That fingerprint
is cheap, not cryptographic: if two checkpoints ever had identical file names
and sizes it would not notice. Set `ICF_CHECKPOINT_REVISION` to the identifier
you actually track (a model registry tag, a release name) and it takes
precedence — which is what a deployment that cares about provenance should do.

For local plumbing without a GPU, `ICF_WORKER_DEV=1 python -m
icf_worker.server --engine dev-tone` emits a tone, never speech. It refuses to
start when `ICF_ENV=production`.

### Style is sent as words, not only as numbers

The Go adapter puts a natural-language instruction on every CosyVoice request
(`params.instruct`, alongside `params.instruct_style` naming the style that
produced it). CosyVoice renders it through `inference_instruct2`, which is the
only expressive control the model has — speed and pause scaling alone cannot
make "prayer" and "preaching" sound different.

A CosyVoice build whose `inference_instruct2` is missing or has a different
signature does not fail the request: the worker falls back to zero-shot, logs
one warning, and reports `style_instruction: false` in `/v1/capabilities`
thereafter. Check that flag when judging output quality — an engine quietly
ignoring the style is exactly the failure this reporting exists to prevent.

### Engine dependencies

`requirements.txt` is the worker's own dependency set: numpy and scipy are enough
to master and encode audio, not to synthesise it. The engines are supplied by the
deployment, so their dependency sets are declared separately and installed on the
GPU host, not in this image:

| Backend | Manifest | Notes |
|---|---|---|
| CosyVoice | `requirements-cosyvoice.txt` | The engine repository's own pins, verbatim, including its `--extra-index-url` lines. Install in the same environment as the engine checkout. |
| VoxCPM | `requirements-voxcpm.txt` | `voxcpm` plus a pinned torch pair. Its transitive set is unpinned upstream — freeze the resolved set once a GPU host has verified it. |
| GPT-SoVITS | none | Runs as a separate `api_v2` service; the worker only proxies to `ICF_GPTSOVITS_API`. |

None of these pins has been exercised on a GPU by CI — there is no GPU in CI and
no licence-cleared checkpoint yet — and the Dockerfile deliberately installs none
of them, so an uncleared engine cannot ride into the image. CosyVoice pins
torch 2.3.1 while voxcpm requires 2.5.0 or newer: the two backends need separate
environments, which is written down in the VoxCPM manifest because a resolver
error on a GPU host is an expensive way to learn it.

## Deploy

`Dockerfile` builds the worker; `k8s/deployment-gpu.yaml` runs it.

```bash
docker build -t iconfess-voice-engine voice-engine          # CPU base, for tests
docker build -t iconfess-voice-engine:cuda \
  --build-arg BASE_IMAGE=nvidia/cuda:12.4.1-runtime-ubuntu22.04 voice-engine
```

The image contains **no model and no engine repository**, deliberately:
`docs/model_licenses.json` blocks every engine until counsel has verified the
code *and* the exact checkpoint, and baking weights into a published image would
put them somewhere they cannot be withdrawn from. Both are supplied by the
deployment — weights mounted read-only at `ICF_CHECKPOINT_ROOT`, engine code on
`PYTHONPATH` (which must include `third_party/Matcha-TTS`, or upstream imports
fail in a way that looks like a missing model).

Two things the manifest gets right that are easy to miss:

- **Probes are `exec`, not `httpGet`.** `/v1/health` authenticates like every
  other endpoint, so an HTTP probe receives 401 and marks a healthy worker
  unready. `python -m icf_worker.healthcheck` sends the bearer token and treats
  a non-200 as a failure instead of an exception.
- **Both sides of the NetworkPolicy pair must agree.** The worker admits only
  `iconfess-backend` on 8601, and `server/k8s/deployment-prod.yaml` carries the
  matching egress rule. Adding one without the other yields a worker nothing can
  reach.

Egress is DNS-only: weights come from the volume, and a worker that cannot open
outbound connections cannot be used to exfiltrate reference audio.

## Health probe

`python -m icf_worker.healthcheck` exits 0 when the worker reports healthy and 1
otherwise, distinguishing three cases: 200 healthy, 503 engine cannot load its
model, 401 the probe's token does not match the worker's (an operator problem,
not a model problem).

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

CI runs this in the `voice-engine` job (`.github/workflows/ci.yml`) with
`ICF_WORKER_DEV=1 ICF_ENV=development`; it needs no GPU, no model and no
database. The same job checks the licence register:

```bash
python scripts/check_model_licenses.py            # report
python scripts/check_model_licenses.py --strict   # fail on incomplete evidence
python scripts/check_model_licenses.py --gate     # runtime gate still refuses
```

`--strict` is what makes flipping `production_allowed: true` a reviewed act
rather than a one-line edit: an engine marked cleared must record the verified
commit and a checkpoint with a name, source, `weights_sha256`, weights licence,
`commercial_use_verified: true`, `verified_by` and `verified_at`. `--gate`
asserts that `icf_worker.licenses.check` still refuses an uncleared engine under
`ICF_ENV=production`, so a future edit that makes the gate permissive fails the
build instead of quietly shipping an uncleared model.

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

## First real run on a GPU

No engine has produced a sample of speech in this repository yet, and nothing
below can be done from a laptop: it needs a GPU, a checkpoint, and a licence
decision. This is the shortest path from "plumbing works" to "we have heard it".

1. **Pick one engine.** CosyVoice is the primary candidate; GPT-SoVITS and
   VoxCPM have never been exercised against their real servers, so treat their
   first run as an integration test rather than a benchmark.
2. **Verify the licence before the weights move.** Read the code licence at the
   commit you pin *and* the model card of the exact checkpoint. Record both in
   `docs/model_licenses.json` with `verified_by`, `verified_at` and
   `weights_sha256`. Only then set `production_allowed: true`.
   `python scripts/check_model_licenses.py --strict` will refuse an incomplete
   entry, and `--gate` will fail CI if the runtime gate is ever weakened.
   **Do not set the flag to run a benchmark** — `ICF_ENV` unset or `development`
   already allows it, and the gate only bites in production.
3. **Build the GPU image and start the worker.**
   ```bash
   docker build -t iconfess-voice-engine:cuda \
     --build-arg BASE_IMAGE=nvidia/cuda:12.4.1-runtime-ubuntu22.04 voice-engine
   docker run --gpus all --rm -p 8601:8601 \
     -e ICF_WORKER_TOKEN=... -e ICF_ENV=development \
     -e ICF_COSYVOICE_MODEL_DIR=/models/cosyvoice/<checkpoint> \
     -e PYTHONPATH=/engine:/engine/third_party/Matcha-TTS \
     -v /models:/models:ro -v /engine:/engine:ro \
     iconfess-voice-engine:cuda
   ```
4. **Confirm it is actually serving before benchmarking.**
   `python -m icf_worker.healthcheck` with `ICF_WORKER_TOKEN` set — a 503 here
   means the model did not load, which is a different problem from a slow one.
5. **Run the golden set through the real adapter.**
   ```bash
   cd server && VOICE_ENGINE_TOKEN=... go run ./cmd/voice-bench \
     -engine cosyvoice=http://gpu-1:8601 -runs 3 -stream -out /tmp/bench
   ```
   Keep `blind_key.json` away from the raters. Without a `-scorer`, quality is
   reported as **not measured** — that is the correct answer until someone
   listens.
6. **Record the result against the commit and the checkpoint hash**, so a later
   "the voice sounds worse" can be traced to a specific change.

Until step 5 has run, every statement about voice quality — naturalness,
similarity, accent, long-form stability — is speculation, including any number
that looks like a score.

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
