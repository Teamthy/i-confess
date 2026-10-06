# iCONFESS VOICE ENGINE AUDIT

**Audit date:** 2026-10-06
**Repository:** `Teamthy/i-confess`
**Commit audited:** `5ffca64` (merge of PR #88), branch `arena/513ada3a-i-confess`
**Auditor stance:** adversarial. Nothing below is asserted without a command that
produced it, a `file:line`, or an explicitly labelled unchecked assumption.

### What I executed (not just read)

| Check | Command | Result |
|---|---|---|
| Voice-engine worker test suite | `python -m pytest -q tests` (`ICF_WORKER_DEV=1`) | **27 passed** in 10.77s |
| DSP chain, loudness + ceiling | `dsp.process()` on a synthetic clipped 3 s signal | 8 stages ran; **−16.00 LUFS**; true peak **−1.47 dBTP** ≤ −1 ceiling; no NaN; duration preserved |
| Limiter under forced binding | same, `ICF_MASTER_TARGET_LUFS=-6` | `peak_limited=True`, `limiter_max_db=7.90`, achieved **−11.00 LUFS** (reported honestly, not as −6) |
| Delivery encodings | `encode.encode_bytes()` via real ffmpeg 7.0.2 | aac 19 537 B, opus 13 813 B, mp3 33 542 B — **all three carry `synthetic_audio=true`** |
| Web typecheck | `npx tsc --noEmit` | **exit 0** |
| Golden set | parsed `docs/voice/golden_set.json` | 10 prompts, `en-NG` + `en-NG-PIDGIN` |
| CosyVoice API claim | compared to upstream HF model cards + `vllm_example.py` | `AutoModel` **is correct** (my initial suspicion was wrong) |

### What I could not execute — and why

- **Go test suite (391 Go files — most of the codebase): NOT RUN.** No Go toolchain
  in this sandbox and none obtainable: `go.dev`, `dl.google.com`, `deb.debian.org`,
  `proxy.golang.org`, `raw.githubusercontent.com` all return `000`/SSL failure.
  Egress is limited to `pypi.org`, `files.pythonhosted.org`, `github.com`; `golang/go`
  publishes no GitHub release assets; PyPI's `golang` is a 0.0.3 stub. Every Go
  conclusion below is therefore **static evidence (file:line), not runtime proof.**
- **Flutter tests: NOT RUN** (no Flutter SDK).
- **Real model inference: NOT POSSIBLE** (no GPU, no checkpoints, engines not installed).
- **Load tests, real streaming, concurrent DB behaviour: NOT RUN** (no Postgres/Redis here).

Where a Go claim below is load-bearing for a verdict, it is marked **[static]**.

### Prior audit

`.arena/AUDIT-2026-09-20-PRODUCTION-CERTIFICATION.md` (181 KB) exists. I checked it:
`grep -c 'voice-engine\|icf_worker'` → **0**. That audit never examined the Python GPU
worker, so the worker findings here are new. I also re-verified one of its findings:
it reported `PurgeExpired` was never called; it **now is** wired
(`main.go:459` → `RunIdempotencySweep` → `PurgeExpired`). That finding is closed.

---

## 1. Executive Summary

| | |
|---|---|
| **Overall Score** | **60 / 100** |
| **Production Readiness** | **DEVELOPMENT READY** (band 60–74) — see §3 |
| **Biggest Strength** | The **rights model**. 24 independently-licensed capabilities (`voicegov`), re-evaluated at *execution* time, not just at enqueue. Revocation genuinely wins. This is better than the "consent=true" pattern the brief assumed. |
| **Biggest Weakness** | **Not a single sample of synthetic minister speech has ever been produced by this system.** Every engine is licence-blocked, no engine is installed, and no checkpoint exists. |
| **Biggest Security Risk** | The GPU worker has **no container, no network policy, no orchestration**. It authenticates by one shared bearer token and "must not be exposed publicly" — a comment, not an enforced boundary. |
| **Biggest Voice Quality Risk** | Unmeasurable. No model has run; no speaker-similarity baseline; `ICF_VOICE_SCORE` has never been produced from real audio. |
| **Biggest Architecture Risk** | **Two live generation paths with two different rights models.** The 24-capability model protects the GPU path; the cloud-TTS path is guarded only by a 4-boolean `License`. |
| **Biggest Scalability Risk** | One worker, no batching, no autoscaling, and **stuck jobs are never reclaimed** (`ReclaimStale` has zero callers). A single GPU OOM orphans a user's generation forever. |
| **Biggest Legal/Rights Risk** | `docs/model_licenses.json` sets `production_allowed: false` for **all four engines**, with every checkpoint field `null`. There is no legally cleared path to production speech. |
| **Most Important Fix** | Wire `ReclaimStale` into a sweeper **and** containerise the worker. Both are days, not weeks, and both are currently unmitigated. |
| **Estimated Work Remaining** | ~10–14 engineering weeks to a defensible closed beta; the licence work is on the critical path and is not an engineering task. |

**The honest headline:** this is a well-engineered *platform* wrapped around an
*unproven voice*. The plumbing — rights, queue, DSP, provenance, mastering — is
genuinely good and much of it I verified by running it. The thing that makes it a
"Voice Engine", the synthesis of a real minister's voice, has never happened.

---

## 2. Overall Score

Mean of the 26 §82 categories: **156 / 26 = 6.0 → 60 / 100**

| Category | Score | Basis |
|---|---|---|
| Architecture | 6 | Clean `voiceengine` layering; undermined by 3 abstractions, 2 rights models |
| Backend | 7 | Idiomatic Go, unusually good comments, strong config guards |
| Provider abstraction | 6 | `voiceengine.VoiceProvider` is genuine; duplicated twice |
| Voice cloning | 4 | Zero-shot plumbing complete; never executed against a model |
| Dataset pipeline | 6 | Real VAD/segment/quality + mandatory human review; no real diarization |
| **Audio processing** | **9** | **Verified empirically.** Best component in the repo |
| **TTS quality** | **2** | **Zero samples produced. Unmeasurable** |
| Pronunciation | 5 | Real locale-aware dictionary; **ships empty**; no normalisation |
| Prosody | 6 | Real markup + `ProsodyFor`; engine-side effect unproven |
| Model management | 8 | Immutable versions, promote/rollback, dataset pinning, eval gate |
| **Rights management** | **8** | Best-in-class for a project this size; one rollback gap |
| Security | 7 | Strong validation, key allowlist, constant-time HMAC; dead signing code |
| Privacy | 7 | Owner-scoped caching, private visibility, termination purge |
| Storage | 7 | Verified signing, traversal defence, range requests |
| CDN | 5 | CloudFront CFN exists; R2 explicitly "not tested"; no hit-rate metrics |
| Streaming | 5 | Implemented (streamed WAV header); unproven at scale |
| Web | 7 | Typechecks clean; Media Session implemented |
| Flutter | 6 | Real `just_audio_background` + `AudioSession.speech()`; CI runs it, I could not |
| Queue | 8 | Durable Postgres, idempotent, DLQ, correct index — **but no stale reclaim** |
| **GPU infrastructure** | **2** | **No Dockerfile, no k8s, no CI, no scaling** |
| Scalability | 4 | Single worker, no batching, no autoscaling |
| Performance | 4 | Unmeasured; no baseline anywhere |
| Observability | 7 | Metrics + Prometheus + rights audit log; per-process only |
| Testing | 6 | Go strong; **Python worker absent from CI**; no golden audio |
| CI/CD | 6 | Excellent for Go (real PG17 + Redis7 + `-race`); blind to the worker |
| Documentation | 8 | Unusually honest — repeatedly marks its own gaps |

**TECHNICAL SCORE: 6.0 / 10**
**SECURITY SCORE: 7.2 / 10**
**VOICE QUALITY SCORE: 4.3 / 10**
**PRODUCTION READINESS SCORE: 60 / 100**

---

## 3. Production Readiness

**Band: 60–74 → DEVELOPMENT READY.**

Two P0 blockers make production impossible regardless of score (§35). I am *not*
applying the "PRODUCTION READY WITH CONDITIONS" band, because one of the blockers
(licence clearance) is not an engineering task the team can complete alone.

**Verdict: READY FOR INTERNAL TESTING** — with the precise caveat that "internal
testing" today means **plumbing only** (`--engine dev-tone`, which emits a tone and
never speech, and correctly refuses to start under `ICF_ENV=production`).

---

## 4. Architecture Audit

### Repository map

```
server/            Go 1.25 modular monolith. 431 tracked files. THE backend.
  cmd/server/      HTTP API + queue worker + sweepers (one binary)
  cmd/voice-bench/ Engine benchmark harness (golden set, blind listening pack)
  internal/voiceengine/  ← the real engine abstraction (orchestrator, markup, safety)
  internal/voicegov/     ← 24-capability rights model
  internal/voice/        ← cloud TTS adapters (Google/Azure/ElevenLabs) + Pipeline
  internal/audio/        ← signed URLs, processor, lifecycle  (generator.go = DEAD)
  internal/jobs/         ← queue contract + worker pool
  internal/store/        ← Postgres persistence incl. JobQueue
  internal/rights/       ← coarse 4-boolean licence model (legacy)
voice-engine/      Python GPU inference worker. 17 files, 2 157 LOC. NO container.
web/               Next.js 15.5.26 / React 19. 246 files. Typechecks clean.
apps/mobile/       Flutter, Dart ^3.13.2. 191 files. 3 166 LOC of audio code.
clients/dart/      Generated Dart API client.
docs/              110 files, incl. model_licenses.json + voice/golden_set.json
infrastructure/    media-cloudfront.yaml (S3 + CloudFront signed delivery)
server/k8s/        ONE deployment: iconfess-backend. No GPU workload.
```

### Technology versions (from manifests, not assumed)

| Component | Version | Source |
|---|---|---|
| Go | **1.25.0** | `server/go.mod:3` |
| Python | numpy ≥1.26, scipy ≥1.11 **only** | `voice-engine/requirements.txt` |
| Next.js / React | 15.5.26 / 19.0.0 | `web/package.json` |
| Dart SDK | ^3.13.2 | `apps/mobile/pubspec.yaml` |
| just_audio / _background | ^0.10.5 / ^0.0.1-beta.17 | same |
| PostgreSQL | 17 | `.github/workflows/ci.yml` service |
| Redis | 7 | same |
| ffmpeg | 7.0.2 (via `imageio-ffmpeg`) | verified at runtime |
| CUDA / torch | **absent** | no dependency declares them |

**Dependency observations:**
- **`requirements.txt` has no ML framework at all.** CosyVoice/VoxCPM are imported
  lazily from the host environment; GPT-SoVITS is proxied over HTTP. Consequence:
  the worker's declared dependencies cannot produce speech. Any deployment must
  hand-build an environment the repo does not describe. → **VE-014**
- Go dependencies are minimal and current (aws-sdk-go-v2 v1.46, jwt v5.3.1, lib/pq v1.12.3).
- **No Redis client library.** `cache/redis_bus.go` and `ratelimit/redis.go` hand-roll
  RESP. Deliberate and documented; it is a maintenance surface, not a bug.
- `NewGCSStorage` / `NewAzureStorage` always return `ErrProviderUnavailable`
  (`providers.go:334,339`) — honest stubs, but dead surface.

### Actual vs intended architecture

**Intended** (from `voice-engine/README.md`):
```
client → POST /voices/generate → rights check (voicegov) → content hash
       → cache hit? signed URL : enqueue "voice.generate"
Go worker → RE-CHECKS rights → HTTP → Python worker → inference → BS.1770
          → WAV + provenance → object storage (private) → signed CDN URL
```

**Actual** — I traced it and it matches, with two deviations:

1. **There is a second, parallel live path.** `POST /admin/audio/generate` →
   `adminGenerateAudio` (`admin_voice.go:243`) → `voice.Pipeline.Generate`
   (`pipeline.go:117`) → **cloud TTS** (Google/Azure/ElevenLabs) → `rights.Evaluate`
   (4 booleans). This path never touches `voicegov`.
2. **The queue is PostgreSQL, not Redis.** `jobs/queue.go:36` documents
   `FOR UPDATE SKIP LOCKED`; `store/jobqueue.go` implements it; index
   `idx_jobs_claim ON jobs(status, available_at, created_at)` (`0007_job_queue.sql:29`).
   Redis is used only for rate limiting and cache-invalidation pub/sub.
   The README's "Redis" label is **documentation drift**, not a design flaw —
   Postgres-`SKIP LOCKED` is a defensible choice at this scale.

**Architectural drift:** the README describes a Redis queue that does not exist,
and omits the cloud-TTS path entirely.

---

## 5. Voice Provider Audit

**Is there a genuine `VoiceProvider`? Yes — and two impostors beside it.**

| # | Interface | File | Methods | Wired? |
|---|---|---|---|---|
| 1 | `voiceengine.VoiceProvider` | `voiceengine/provider.go:41` | `Generate`, `Clone`, `Health`, `Capabilities` | ✅ `cmd/server/main.go:508-542` |
| 2 | `voice.Provider` | `voice/provider.go:24` | `Name`, `Synthesize` | ✅ `main.go:238`, `568-594` |
| 3 | `audio.TTSProvider` | `audio/generator.go:27` | `Generate`, `GetVoiceInfo`, `ListVoices`, `Name` | ❌ **zero callers** |

Abstraction #1 is real and well-designed: engine-specific markup never leaks
(`Render` passes phonemes only to engines declaring support), inference never runs
in the Go process, and `NewRegistry(production)` refuses the `fake` engine.

**Finding VE-001** — three abstractions, two rights models (§36).
**Finding VE-002** — `audio.Generator` is dead code (§34).

### Provider capability matrix

Verified from `engines.py` source and each engine's own status comment — **not**
from the mere existence of an adapter.

| Provider | Cloning | Fine-tune | Languages | Streaming | GPU | Licence | **Has it ever run here?** |
|---|---|---|---|---|---|---|---|
| CosyVoice | zero-shot ✅ | claimed | en, zh | claimed | required | `production_allowed: false` | **NO** |
| GPT-SoVITS | zero-shot ✅ | via `/set_*_weights` | en,zh,ja,ko | proxy | required | `production_allowed: false` | **NO** — source says *"NOT been exercised against a live GPT-SoVITS instance in CI"* |
| VoxCPM | zero-shot ✅ | `False` | en, zh | `False` | required | `production_allowed: false`, `code_license_reported: null` | **NO** — *"Not exercised against the real model in CI"* |
| VoiceStudio | — | — | — | — | — | **AGPL**, benchmark-only, never linked | N/A by design |
| Google / Azure / ElevenLabs | provider-side | n/a | many | provider | no | commercial SaaS | adapters exist; **no key configured** |
| dev-tone | tone only | no | en | yes | no | internal test | ✅ **the only engine that has ever run** |

**The matrix is the finding.** Three of four engines are marked "written but not yet
run against real models" *by their own source*. No capability above the dev-tone row
has been demonstrated.

**CosyVoice import — verified correct.** I initially suspected
`from cosyvoice.cli.cosyvoice import AutoModel` (`engines.py:76`) was wrong. It is
**not**: `AutoModel` appears in the official `CosyVoice2-0.5B` and `CosyVoice-300M`
model cards and in upstream `vllm_example.py`. I withdraw that suspicion.

What *is* a real gap: upstream requires
`sys.path.append('third_party/Matcha-TTS')`. Neither `engines.py` nor the README's
configuration table adds it. → **VE-015** (P3, first-run ImportError risk).

---

## 6. Voice Cloning Audit

| Capability | State | Evidence |
|---|---|---|
| Zero-shot | Plumbing complete | `CosyVoiceEngine.synthesize_chunk` → `inference_zero_shot(text, transcript, ref_path, stream=False, speed=…)` |
| Reference-based | Yes, required | both CosyVoice and GPT-SoVITS raise `EngineError("model", …)` when `reference` is absent |
| Speaker adaptation | Not distinguished from fine-tune | — |
| Fine-tuning | Orchestration only | `train.py` shells out to `ICF_TRAIN_CMD_*` and parses `ICF_PROGRESS`; no trainer in-repo |

- **Reference length:** not enforced anywhere I found. No minimum-duration check.
- **Transcript:** mandatory (`reference["transcript"]` is read unguarded — a
  `KeyError` on a missing transcript, not a classified error). → **VE-016** (P3)
- **Sample rate / purity:** ingestion analyses at 16 kHz; purity via a log-mel
  heuristic explicitly labelled *"NOT a speaker-verification model"*.
- **Cloning quality measured?** **No.** `voiceeval` computes `ICF_VOICE_SCORE` from
  metrics supplied by the caller. Nothing in-repo measures similarity; the bench
  tool requires an external `-scorer` and otherwise reports *"not measured"*.

**Verdict: cloning is designed, never demonstrated.**

---

## 7. Minister Voice Audit

**Can the system safely represent a licensed minister voice? Yes — this is its best work.**

`voicegov` (`voicegov.go`) defines **24 independent capabilities**, not one consent flag:

```
can_record, can_store_recordings, can_process_recordings, can_extract_voice,
can_create_embedding, can_clone, can_train, can_fine_tune, can_generate,
can_commercialize, can_distribute, can_stream, can_download,
can_use_in_bible_audio, can_use_in_confessions, can_use_in_prayers,
can_use_in_reflections, can_use_in_marketing, can_use_for_research,
can_use_third_party_infrastructure, can_store_model_checkpoints,
can_create_derivative_models, can_use_user_submitted_text,
can_retain_after_termination
```

Design rules that hold up under reading:
- **Zero values authorise nothing.** An unknown status is not approved.
- Only `APPROVED` and `RESTRICTED` authorise anything; `RESTRICTED` can carve out purposes.
- **Expiry is evaluated at call time, not trusted from the stored status** — tested at
  `voicegov_test.go:107 TestExpiryIsEvaluatedAtCallTime`.
- Callers ask "may I do X?" and never assemble capability lists, so a new call site
  cannot forget one. `ActionGenerate` → the full set, including `ThirdParty` and
  `UserSubmittedText`.
- **Post-termination is modelled** (`can_retain_after_termination`) and enforced by a
  sweeper that deletes renders.

The brief warned "do not accept a single `consent=true`". **This system does not.**

**Gap VE-003:** `adminRollbackVoiceModel` (`voice_platform.go:889`) does **not** call
`voicegov.Authorize`, while `adminPromoteVoiceModel` (`:864`) does. Rollback moves a
model into production without re-checking the voice's licence.

---

## 8. Dataset Audit

Pipeline (`voice-engine/icf_worker/ingest.py`), each stage honestly labelled:

| Stage | Implemented | Honest caveat in source |
|---|---|---|
| Load (WAV/FLAC) | ✅ | FLAC needs optional `soundfile` |
| Clean | DC removal + 70 Hz high-pass **only** | *"No stage denoises… Aggressive denoising puts artifacts into training data"* — **correct call** |
| VAD | ✅ adaptive energy + hysteresis | *"dependable on clean studio and pulpit mics but not on music beds. Treat it as a segmenter, not a judge"* |
| Speaker check | ✅ log-mel heuristic | *"NOT a speaker-verification model"* |
| ASR | optional `ICF_ASR=faster_whisper` | *"only a draft for the human reviewer. Nothing enters a dataset without a verified transcript"* |
| Quality score | ✅ SNR/clipping/duration/level | *"screening and sorting, not as a claim of fitness"* |
| Human verification | ✅ `POST /admin/segments/{id}/review` | requires a verified transcript |
| Dataset freeze | ✅ immutable version | `POST /admin/voices/{id}/datasets` |

**Missing stages vs the brief's §14 list:** music detection, dereverb, explicit
noise *detection* reporting. Music detection matters — VAD is documented as
unreliable on music beds, and sermons are often recorded over organ or choir.

**Transcript accuracy risk:** mitigated by design. ASR output lands in
`raw_transcript`; only `verified_transcript` enters a frozen dataset
(`store/voice_intake.go:410`). The brief's "CRITICAL DATASET QUALITY RISK" is
**structurally prevented** — provided reviewers actually do their job.

**Dataset versioning: real.** `voice_datasets` + `voice_dataset_segments`; every
`voice_models` row carries `dataset_version` and `training_run_id` (§25).

---

## 9. Audio Processing Audit

**The strongest component. I ran it.**

Chain (`icf_worker/dsp.py`), verified executing all 8 stages:
```
dc → trim → declick → eq → deess → compress → loudness → limit
```

Runtime results:

| Input | Result |
|---|---|
| 3 s synthetic speech-like signal, peak −0.16 dBFS, hard-clipped at 0.98 | output **−16.00 LUFS** exactly; true peak **−1.47 dBTP**; `peak_limited=False`; no NaN; duration preserved at 3.00 s |
| Same with `TARGET_LUFS=-6` (ceiling must bind) | `peak_limited=True`, `limiter_max_db=7.90`, achieved **−11.00 LUFS**, peak **−1.056 dBFS** |

- **BS.1770-4 integrated loudness** with correct K-weighting derived via bilinear
  transform (works at any sample rate, not just 48 kHz), −70 LUFS absolute gate,
  −10 LU relative gate.
- **True peak by 4× polyphase oversampling**, per BS.1770 Annex 2.
- **Loudness normalised first, limiter enforces the ceiling after** — and when the
  ceiling wins, the achieved loudness is **reported honestly** (−11, not the −6 target).
  This is the correct engineering choice and many systems get it wrong.
- **Over-processing risk: LOW.** Trim affects edges only (*"pauses inside the script
  are never shortened"*); de-ess touches 5–9 kHz only; EQ shelves default to **0 dB**.
  Every stage is env-configurable. Defaults are conservative.

**DSP runs server-side only.** Responsibilities are unambiguous — clients get mastered
audio and do no processing.

**Mastering targets are configurable** (`ICF_MASTER_TARGET_LUFS=-16`,
`ICF_MASTER_CEILING_DBTP=-1`) and documented in a table. Not hardcoded.

---

## 10. TTS Pipeline Audit

```
TEXT → [NORMALIZATION ✗] → PRONUNCIATION (dict, empty) → SpeechMarkup → chunking
     → provider → raw audio → post-processing → QUALITY CHECK (partial)
     → ENCODING → STORAGE
```

| Stage | State |
|---|---|
| Text normalisation | **ABSENT** — see VE-004 |
| Pronunciation | Dictionary engine real; **no data** — VE-005 |
| Speech markup | ✅ `{pause}`, `{emph}`, `{say as}`, `{speed}`, `{pitch}`; malformed known tags **error** rather than silently drop |
| Chunking | ✅ sentence-boundary split honouring `MaxChunkChars` |
| Provider | ✅ out-of-process HTTP |
| Post-processing | ✅ verified |
| Quality check | ⚠️ loudness/true-peak asserted; **no** intelligibility, similarity or artifact check |
| Encoding | ✅ verified, provenance-tagged |
| Storage | ✅ private key `audio/voices/{voice}/{purpose}/{modelVersion}/{contentHash}.wav` |

**Bypass check:** I looked for a path that reaches a provider without rights.
`voice.Pipeline` documents itself as *"the only sanctioned route to a Provider"* and
enforces `rights gate → dedupe → synthesize → verify → store` in that order.
`voiceengine.Orchestrator` calls `preflightRights` before any model lookup, then
re-checks per candidate (because `ThirdParty` depends on engine capability).
**No bypass found [static].**

---

## 11. Pronunciation Audit

**Architecture is right. Content is missing.**

- `PronunciationEntry` supports `Term`, `Aliases`, `Respelling`, **`IPA`**, and
  **`ProviderOverrides map[Engine]string`** — so one term can carry an engine-specific
  correction. That is the correct design.
- `Dictionary.Lookup` ranks **exact locale → language → universal**, and deliberately
  keeps `en-NG` and `en-NG-PIDGIN` separate (`isPidgin` guard, `markup.go:272`).
- Centralised, not duplicated per provider — `Apply` runs once in the orchestrator.
- Admin-updatable: `PUT /admin/pronunciations`, with `DictVer` folded into the content
  hash so a dictionary edit invalidates the cache.

**VE-005 — the table ships empty.** `pronunciation_dictionary` is created at
`0027_voice_platform.sql:307`; the only writer is the admin upsert
(`store/voice_platform.go:647`). There is **no seed migration and no seed data**.
A fresh production deployment has zero entries, so *Nebuchadnezzar*, *Chukwuemeka*,
*Ogbomosho* and *Oshogbo* get no help at all — despite the golden set testing exactly
those words.

**No phoneme/IPA support is claimed by any engine capability**, so `Phonemes` is
computed and then dropped for engines that don't declare support. Correct behaviour;
means the IPA field is currently decorative.

---

## 12. Prosody & Style Audit

- `ProsodyProfile` carries speed, pitch, pause multiplier, energy — engine-neutral,
  translated per adapter.
- `ProsodyFor(style, speed)` maps a style to a profile; `Render` scales
  `SilenceAfterMS` by the profile's pause multiplier.
- `{speed}` clamped 0.5–2.0 in markup, and again 0.5–1.5 at the API
  (`decodeVoiceRequest`); `{pitch}` ±12 in markup, ±6 at the API. Layered bounds.
- **Styles are validated** (`ValidStyle`) — not free-form UI labels.

**Do styles actually change output?** Partially, and provably so at the markup layer:
a style changes the pause multiplier, speed and pitch passed to the worker, so chunk
timing and prosody parameters **do** differ. What is **not** demonstrated is that
CosyVoice/GPT-SoVITS respond perceptibly — none has run. CosyVoice's real
expressiveness control is `inference_instruct2` with a natural-language instruction,
which the adapter **does not use** (it calls `inference_zero_shot` only).

**VE-006 (P2):** the style system's most powerful lever is unused. A "prayer" vs
"preaching" difference currently reduces to speed/pitch/pause scaling.

---

## 13. Audio Storage / CDN Audit

**Storage — good:**
- `allowedPrefixes = {audio/, avatars/, bible/offline/, bible/audio/, voice-private/}`
  — an **allowlist**, so a new caller must opt in (`signing.go`).
- `ValidKey` rejects `..`, leading `/`, `//`, `\ ? # NUL`, and >512 chars.
- `voice-private/` documented as *"never linked publicly; reviewers get short-lived
  signed URLs only"*. Checkpoints and source recordings live there.
- Object metadata carries `synthetic_audio=true`, `model_id`, `engine`,
  `engine_version`, `audio_sha256`, `fell_back` — **full provenance per asset**.

**Signing — verified:**
`storage.Sign`/`Verify` use `hmac.Equal` (constant time), a newline separator to stop
key-boundary shifting, expiry checks, and are actually called:
`providers.go:559` in `LocalStorage.Handler`, which also serves range requests via
`http.ServeContent` and sets `Cache-Control: private, no-store` (bible) /
`private, max-age=300`. The handler is mounted **only** when `!cfg.IsProduction()`
(`main.go:143-146`).

**CDN:** `infrastructure/media-cloudfront.yaml` provisions a private bucket (all four
`PublicAccessBlock` flags true), AES256 SSE, versioning, OAC with `SigningBehavior: always`,
and a CloudFront trusted key group — real signed-URL delivery. Config **enforces** it:
production fails without *"CloudFront signed HTTPS media or an HTTPS Cloudflare R2
endpoint with private presigned URLs"*.

**VE-007 (P2):** `docs/52-…:125` states plainly *"Actual R2 integration has not been
tested here."* And there are **no cache-hit-rate metrics** anywhere.

**VE-008 (P4, dead code):** `audio/urls.go` contains a **second, parallel** HMAC
scheme. `ValidateSignedURL` (`:166`) and `ValidateToken` (`:230`) have **zero callers**
outside their own tests. The `SigningSecret == ""` branch generates a random dev
secret — unreachable in production because config rejects the default
`dev-only-audio-secret`. So: dead code, **not** a security hole. I checked before calling it one.

---

## 14. Streaming & Player Audit

- **Server streaming:** `POST /voices/stream` (`voice_intake.go:688`), orchestrator
  `Stream()` requires `can_stream` **in addition to** generation rights, uses the
  production model only, and **never falls back** — *"switching engines mid-utterance
  would be an audible, silent voice substitution."* Correct reasoning.
- Streamed WAV uses a `0xFFFFFFFF` length header ("length unknown"), which players accept.
- Gain for a stream is set from the first chunk (clamped ±20 dB) with per-chunk
  ceiling protection — whole-file mastering is impossible before the audio exists,
  and the code says so.
- **Range requests** verified present (`http.ServeContent`).
- **Not tested:** startup latency, seeking, slow networks, interruption/resume,
  10 s → 60 min durations. No such test exists in the repo and I could not run one.

---

## 15. Web Audit

- `npx tsc --noEmit` → **exit 0** (verified).
- **Media Session API is implemented** — `web/lib/player.tsx:246-247`. (I first grepped
  only `audio-playback.ts` and wrongly concluded it was missing; widening the grep corrected this.)
- Security headers set globally in `next.config.mjs`: `nosniff`, `X-Frame-Options: DENY`,
  `Referrer-Policy`, `Permissions-Policy`.
- API reached same-origin through `/api/*` rewrites — no cross-origin calls from the browser.
- `allowedDevOrigins` includes `*.e2b.app` for sandbox previews, correctly scoped to dev.
- Components: `session-player`, `synthetic-player`, `voice-preferences`, `voice-studio`, `voices`.
- **Transcript synchronisation: absent** (§34 detail). No word-level highlighting.
- **Not verified:** actual playback in Safari/Chrome/Firefox; no browser available here.

---

## 16. Flutter Audit

- **Background playback is real:** `just_audio_background` initialised at startup
  (`background_playback_service.dart`), `AudioSession.instance.configure(AudioSessionConfiguration.speech())`
  at `player/audio_playback_service.dart:104-105`.
- **iOS:** `UIBackgroundModes: [audio, remote-notification]` — `Info.plist:31-35` ✅
- **Android:** `WAKE_LOCK`, `FOREGROUND_SERVICE`, **`FOREGROUND_SERVICE_MEDIA_PLAYBACK`**,
  and `android:foregroundServiceType="mediaPlayback"` — the exact set Android 14+ requires ✅
- Offline: `offline_audio_service.dart` (700 lines) + `queue_persistence_service.dart` (346).
- Entitlement is server-authoritative: *"the store is where money changes hands; this
  app never decides entitlement from it"* (pubspec comment) — correct.
- **CI runs `flutter analyze` + `flutter test`** (`.github/workflows/ci.yml:188-218`).
- **I could not run them** (no Flutter SDK). Not verified locally.

---

## 17. Bible Audio Audit

- Rights are checked **per use, separately**: `bible_audio_admin.go:178` evaluates
  `UsePlayback` and *then* `UseSynthesis` — text rights and audio rights are genuinely
  distinct evaluations, which is the distinction the brief demanded.
- `GET /v1/bible/audio` returns a signed, rights-gated URL (route summary says so;
  handler `bibleAudio`).
- Bible objects get `Cache-Control: private, no-store` (verified in `providers.go:591`).
- Translation/audio rights are separately reviewable (`bible_rights_reviews` table;
  `adminReviewBibleAudio` approve/withdraw).
- `docs/52-…` is explicit that *"no upstream metadata alone licenses redistribution."*
- Scripture in the golden set is **KJV (public domain)** — deliberate and stated.
- **Voice rights for Bible audio** are a separate capability: `can_use_in_bible_audio`.

**Not verified:** actual byte-level translation licensing evidence. That is a legal
artefact, not a code artefact, and no such document is in the repo.

---

## 18. Confession Audio Audit

Confession audio is the most sensitive asset class. What I found:

- **Per-owner cache isolation.** `queueVoiceGeneration` (`voice_platform.go`): for
  `userText` or `PurposeConfession`, visibility becomes `"private"`, `owner` is set to
  the user, and the cache key is `sha256(contentHash + "|" + userID)`. The code comment
  is explicit: *"two users never share — or discover — each other's renders."*
  **This is the correct defence against cross-user leakage via a shared cache.**
- **`voice_generations.owner_user_id`** is a real column, indexed partially:
  `idx_voice_generations_owner ON voice_generations(owner_user_id) WHERE owner_user_id IS NOT NULL`.
- `visibility CHECK (visibility IN ('private','catalog'))` — a DB-level constraint.
- Text is **not stored**, only `text_sha256` — minimises personal-data retention.
- `synthetic INTEGER NOT NULL CHECK (synthetic = 1)` — the table cannot hold non-synthetic audio.
- Storage keys are private; URLs are signed and short-lived.
- Deletion sweep exists (`RunDeletionSweep`, `main.go:439`).

**VE-009 (P2):** I could not execute a cross-user access test (no DB). The isolation is
**correct by construction** in the cache key, but I did not observe a request from
user B being refused user A's asset. Marked **unchecked**.

---

## 19. Rights & Licensing Audit

**Enforcement points — 11 non-test `voicegov.Authorize` call sites [static]:**

| Site | Action |
|---|---|
| `orchestrator.go:419` | `ActionGenerate` per candidate (incl. `ThirdParty`) |
| `orchestrator.go:459` | preflight generate |
| `orchestrator.go:482` | `ActionStream` |
| `voice_intake.go:87` | training run |
| `voice_intake.go:242` | `ActionIngestRecording` + per-recording permission |
| `voice_intake.go:463` | `ActionFineTune` |
| `voice_platform.go:589,593` | per-action + per-purpose evaluation |
| `voice_platform.go:733` | `ActionCreateReference` |
| `voice_platform.go:779` | model registration |
| `voice_platform.go:822` | model evaluation |
| `voice_platform.go:872` | model promotion |

**Revocation actually blocks generation — and I can show why [static]:**
`runVoiceGeneration` (`voice_platform.go`) does **not** reuse the enqueue-time snapshot:
> *"Fresh rights, models and references: never the enqueue-time snapshot."*

It re-loads `Grant`, `Models`, `References` at execution, passes them to
`Orchestrator.Generate`, and an `ErrRightsDenied` is recorded as
`GENERATION_REFUSED_RIGHTS` and classified `ClassRights` (non-retryable). Tests exist:
`TestOrchestratorRefusesRevokedVoice` (`voiceengine_test.go:335`),
`TestResolveChecksRightsWithoutCallingProvider` (`:465`),
`TestRevokedIsTerminal` (`voicegov_test.go:180`),
`TestExpiryIsEvaluatedAtCallTime` (`:107`).

**Post-termination:** `StartVoiceRightsSweeper` (`voice_ops.go`) runs every 10 minutes,
expires lapsed grants and purges renders. Wired at `main.go:300`. ✅

**Covered:** generation ✅, regeneration ✅ (same path), batch ✅ (`voice_ops.go:64`
stops the whole batch on a rights refusal), training ✅, fine-tune ✅, model
registration ✅, promotion ✅, download/stream ✅ (separate capabilities).

**Not covered:** rollback (**VE-003**).

**Model licensing — the hard blocker.** `docs/model_licenses.json`:

| Engine | code licence | verified commit | checkpoint verified | `production_allowed` |
|---|---|---|---|---|
| cosyvoice | Apache-2.0 (reported) | `null` | `null` | **false** |
| gpt_sovits | MIT (reported) | `null` | `null` | **false** |
| voxcpm | `null` | `null` | `null` | **false** |
| voicestudio | AGPL-3.0 | `null` | — | **false** (never linked) |

`licenses.py:46` raises `LicenseError(SystemExit)` for any of these when
`ICF_ENV=production`. **The worker will refuse to start.** This is correct,
disciplined behaviour — and it means there is no licensed engine.

---

## 20. Security Audit

| Area | Finding |
|---|---|
| Authentication | JWT; production **fails fast** if `JWT_SECRET` is empty or `dev-only-change-me` |
| Authorisation | Role lists per route in a declarative table; `RouteTable()` generates the spec so a route cannot be undocumented |
| RBAC | Layered voice roles: `lvlVoiceRead`, `lvlVoiceML`, `lvlVoiceAudio`, `lvlVoiceBatch`, `lvlVoiceRater`, `lvlVoiceRights`, `voice_manager`, `audio_producer`. A rater cannot promote a model. **Least privilege is genuinely modelled** |
| Secrets | Config validation rejects defaults; `VOICE_ENGINE_TOKEN` → `log.Fatalf` in production (`main.go:304,539`) |
| Model files | `voice-private/` prefix, never public; `resolve_checkpoint` validates keys and never returns them to callers |
| Signed URLs | Constant-time HMAC, verified; production requires CloudFront/R2 HTTPS |
| Rate limits | `VoiceGenerationRule{Burst: 20, Window: 1h}` per user on generation; Redis-backed so limits hold across replicas (required in production) |
| Input validation | Language, style, speed, pitch, purpose all bounded at the API |
| Path traversal | `ValidKey` rejects `..`, absolute, `//`, NUL |
| Command injection | `encode.py` uses `subprocess.run([...])` with **no** `shell=True`; `train.py` uses a configured command template |
| Malicious media | `MAX_BODY = 2 MB`; `/v1/encode` 160 MB (needs a whole WAV); WAV magic checked (`if not wav.startswith(b"RIFF")`) |
| Worker auth | `hmac.compare_digest` on the bearer token; required in production |
| Dependency CVEs | **Not run** — no `govulncheck`/`pip-audit` in CI → **VE-013** |

**VE-010 (P1):** the worker's isolation is a **docstring**: *"must not be exposed
publicly."* There is no `NetworkPolicy` for it (the k8s manifest has one only for
`iconfess-backend`), no container, and one shared static token. Anyone who reaches
port 8601 can synthesise with any licensed voice and read `voice-private/` keys.

---

## 21. Privacy Audit

| Question | Answer |
|---|---|
| What personal data? | User ID + email on `voice_generations.requested_by`/`owner_user_id`; profile locale/timezone |
| What voice data? | Licensed source recordings + intake segments in `voice-private/`; reference clips; checkpoints |
| Confession text? | **Not stored** — only `text_sha256` |
| Retention? | Deletion sweep wired (`main.go:439`); `deleted_at` soft-delete column; `retention` package present |
| Deletable? | Yes — `internal/deletion` with tests |
| Auditable? | Yes — `voice_rights_audit_logs` records actor, action, decision, reason, `grant_version`, `remote_addr`, including **refusals** |
| Cross-user access? | Prevented by per-owner cache key + `visibility` constraint (unchecked at runtime) |

`PRIVACY.md` exists and names Cloudflare/AWS S3 with encryption at rest and in transit.

**Good:** refused attempts are audited, not only successes — *"so a rights dispute can
be reconstructed after the fact."*

---

## 22. Queue & Worker Audit

**Design is genuinely solid:**
- `Queue` interface with two interchangeable implementations (`MemoryQueue` for tests,
  `store.JobQueue` for Postgres). The reason is documented: *"A queue that lives in a
  process's memory loses every pending job when the process restarts, and a restart is
  not an unusual event - it is a deploy."*
- `Claim` is atomic — `FOR UPDATE SKIP LOCKED`; *"two workers must never be handed the same job."*
- **Idempotency:** `Enqueue` returns the existing job ID + `ErrDuplicateJob`.
  Generation uses `IdempotencyKey = gen.ID + ":" + gen.UpdatedAt`, `MaxAttempts: 4`.
- **Dead letter** with a recorded reason; `RequeueDead` puts them back — *"parking a job
  is only useful if a human can put it back once the cause is fixed."*
- `ErrPermanent` distinguishes non-retryable faults so a bad payload isn't retried 4×.
- Correct index: `idx_jobs_claim ON jobs(status, available_at, created_at)`.
- Timestamps are RFC3339 UTC text — *"fixed-width and lexicographically ordered"* —
  with `''` defaulting to "available immediately". Deliberate and internally consistent.

**Duplicate-request behaviour:** same voice + same text + same model → same
`ContentHash` → `CreateOrGetGeneration` returns the existing row and **no job is
enqueued**. Deterministic caching works as designed [static].

### VE-011 (P1) — stuck jobs are never reclaimed

```
ID:        VE-011
CATEGORY:  Reliability / Queue
SEVERITY:  P1 — CRITICAL
TITLE:     ReclaimStale is implemented and unit-tested but has zero production callers

LOCATION:  server/internal/store/jobqueue.go
FUNCTION:  JobQueue.ReclaimStale
LINE:      288

WHAT EXISTS:
  A correct implementation:
    UPDATE jobs SET status=?, worker_id='', claimed_at=NULL, updated_at=?
    WHERE status=? AND claimed_at IS NOT NULL AND claimed_at <> '' AND claimed_at <= ?
  A purpose-built index (idx_jobs_running ON jobs(status, claimed_at),
  0007_job_queue.sql:32) whose own comment says "Stale-claim recovery scans for
  running jobs whose claim has gone quiet."
  Two unit tests (jobqueue_test.go:354,359).

WHAT SHOULD EXIST:
  A periodic sweeper calling it, alongside the five that already exist
  (StartVoiceRightsSweeper, RunScheduleSweep, RunTrialExpirySweep,
  RunDeletionSweep, RunIdempotencySweep).

WHY IT MATTERS:
  grep for callers returns only the definition and its tests. Nothing invokes it.

TECHNICAL IMPACT:
  A worker killed mid-job leaves the row in 'running' permanently.

SECURITY IMPACT: none direct.

PERFORMANCE IMPACT:
  The job is never retried and never dead-lettered, so it is invisible to the
  dead-letter view that ops is told to watch.

PRODUCT IMPACT:
  GPU OOM is the single most common TTS worker failure. Every occurrence leaves one
  user's generation stuck in PROCESSING forever, with no timeout, no retry, and no
  operator signal. At scale this is a steady drip of permanently-broken sessions.

RECOMMENDED FIX:
  Add a goroutine in cmd/server/main.go next to the existing sweepers (main.go:429-470)
  calling q.ReclaimStale(ctx, 15*time.Minute) on a 5-minute ticker, guarded by the
  same advisory lock the other sweepers use at >1 replica.

IMPLEMENTATION APPROACH: ~20 lines, copy the RunIdempotencySweep pattern verbatim.

TEST REQUIRED:
  Claim a job, backdate claimed_at past the threshold, run the sweeper, assert the job
  is 'queued' again and is claimable by a second worker.

PRIORITY: P1 — highest value-per-hour fix in this audit.
```

**Note:** this is the mirror image of a finding from the 2026-09-20 audit
(`PurgeExpired` never called), which **has since been fixed**. The same class of bug
survives one function over.

---

## 23. GPU Infrastructure Audit

**This is the weakest area, and it is a blocker.**

| Requirement | State |
|---|---|
| GPU type / VRAM | **Not specified anywhere in the repo** |
| Container image | **None.** `find -iname 'Dockerfile*'` → `server/Dockerfile` only |
| Orchestration | **None.** `server/k8s/deployment-prod.yaml` has one Deployment (`iconfess-backend`, 3 replicas), no GPU resource, no nvidia runtime class |
| CI | **None.** Zero of four workflows mention `voice-engine`, `icf_worker`, `pytest`, or `requirements` |
| Model loading | **Once per worker process** — `_load()` memoises into `self._model`. **Not per-request.** ✅ |
| Model unloading | Not implemented |
| Multiple models coexisting | **One engine per process** (`--engine` flag). Multiple models require multiple processes; no scheduler |
| Batching | **None.** Chunks are synthesised in a serial `for` loop (`server.py:render`) |
| Concurrency | `ThreadingHTTPServer` — thread-per-request, unbounded |
| Health checks | `GET /v1/health` calls `engine.healthy()`; **no k8s probe consumes it** |
| Cold start | Not measured; model load happens on first synthesis |

**The good news:** model loading is **once per worker**, which is the thing the brief
rightly flags as unacceptable if done per request. That is correct.

**The bad news:** the entire component that justifies the name "Voice Engine" is an
un-containerised script you must run by hand, with no scaling story and no CI.

---

## 24. Model Versioning Audit

**Genuinely reproducible — this is well done.**

`voice_generations` stores, for every asset: `model_id`, `engine`, `engine_version`,
`audio_sha256`, `storage_key`, `grant_version`, `fell_back`, `fallback_reason`.

`voice_models` rows are **immutable** — *"model version already exists or is invalid;
versions are immutable"* (`voice_platform.go:791`) — and carry `dataset_version` and
`training_run_id`.

**Can you identify exactly which model generated yesterday's asset? Yes [static]** —
from `voice_generations` you get model + engine + engine version + dataset version
(via the model) + the grant version in force + whether a fallback occurred.

The content hash includes `ModelVersion`, `EngineVersion`, `ReferenceID` **and
`DictVer`** — so a pronunciation-dictionary edit correctly invalidates cached renders.

**VE-012 (P3):** `engine_version` for CosyVoice/VoxCPM is hardcoded (`version = "0"`,
`"1"`), not derived from the loaded checkpoint. Two different checkpoints under the
same `EngineVersion` would be indistinguishable in the asset record.

---

## 25. Dataset Versioning Audit

**Real.** `voice_datasets` with immutable versions; `voice_dataset_segments` rows
carry `dataset_id`; freezing is an explicit admin action at `lvlVoiceML`
(*"Freeze approved segments into an immutable dataset version"*). Every
`voice_models` row references `dataset_version` + `training_run_id`.
**No anonymous training datasets.** ✅

---

## 26. Performance Audit

**Nothing was measured, and nothing in the repo measures it.**

No latency baseline, no p50/p95 record, no GPU utilisation data, no DB timing. The
instrumentation *exists* (`voiceMetrics.generatedIn`, `.queued`, `.streamStarted`)
and the bench harness computes p50/p95 + RTF + time-to-first-audio — but it has never
been run, because no engine has run.

One design note that is correct: the dedupe check runs **before** synthesis
(*"so the platform never pays twice for identical audio"*), and `Store.Exists` is
consulted first in `voice.Pipeline.Generate`.

**I am not going to speculate about bottlenecks.** The first one to appear will be GPU
throughput, because there is exactly one worker and no batching — but that is an
inference from the architecture, not a measurement, and I label it as such.

---

## 27. Scalability Audit

| Load | Expected behaviour [inferred from architecture, not measured] |
|---|---|
| 1 request | Fine. Cache miss → queue → worker → ~RTF-bound |
| 10 | Queue absorbs it; single worker serialises |
| 100 | Queue depth grows at (arrival − 1/RTF); latency dominated by queue wait |
| 1 000 | Saturation. No batching, one engine per process, no autoscaler |
| 10 000 / 100 000 | Not addressable without a GPU fleet, batching and a scheduler |

**Bottleneck:** GPU worker throughput, unmitigated by batching or horizontal scaling.

**Mitigations that do exist and are real:**
- Deterministic content-hash caching (a repeated request never re-renders)
- Per-user rate limit 20/hour
- Priority queues (`PriorityInteractive` for users, separate queue for batch)
- Batch runs up to 10 000 items at a lower priority

**Missing:** autoscaling, batching, model multiplexing, backpressure signalling to clients.

---

## 28. Observability Audit

**Metrics (`api/voice_metrics.go`):** generation latency per engine, queue wait,
stream TTFB, cache hit/miss, failures **by error class**, content refusals, rights
refusals, fallbacks. Rendered as JSON **and** Prometheus text exposition
(`GET /admin/voice-metrics?format=prometheus`).

Honestly documented: *"Per-process counters since start. Scrape each instance
(format=prometheus) and aggregate."* — that is accurate and not oversold.

**Audit log:** `voice_rights_audit_logs` captures every decision **including refusals**,
with actor, `grant_version`, reason and `remote_addr`. Exposed at
`GET /admin/voices/{id}/audit`.

**Error classification is real, not generic** — `voiceengine.Classify` yields
`ClassContent`, `ClassRights`, `ClassPermanent`, `ClassTransient`, `ClassModel`,
`ClassStorage`, and `Class.Retryable()` drives retry policy. This is exactly the
taxonomy the brief asked for.

**Missing:** GPU utilisation/memory metrics (nothing exports them), distributed
tracing across API→queue→worker→GPU, and alerting rules.

---

## 29. Testing Audit

| Area | Test files | Assessment |
|---|---|---|
| `voiceengine` | 2 (+ interop) | 2 456 LOC; `TestOrchestratorRefusesRevokedVoice`, rights-without-provider, markup, interop (skipped unless URL set) |
| `voicegov` | 1 | 718 LOC; expiry-at-call-time, revoked-is-terminal |
| `voiceeval` | 1 | 515 LOC |
| `voice` | 4 | 2 127 LOC; includes `regionalLocale["yo"] == "yo-NG"` — *"this is a Nigerian product"* |
| `audio` | 3 | 3 588 LOC across 9 source files |
| `rights` | 2 | 805 LOC |
| `jobs` | 1 | 774 LOC |
| `voice-engine` (Python) | 3 | **27 tests — I ran them, all pass** |
| Flutter audio | 4 | `audio_player_test`, `audio_services_test`, `background_playback_test`, `queue_test` — run in CI, **not by me** |

**Critical untested paths:**
1. **The entire Python worker is absent from CI.** All 27 tests pass when I run them
   manually; nothing runs them automatically. A regression in `dsp.py` or `ingest.py`
   would ship silently.
2. `interop_test.go` is **skipped unless** `VOICE_WORKER_INTEROP_URL` is set — so the
   Go↔worker contract is never exercised in CI either.
3. No test produces or compares real audio (**no golden audio test**, §64).
4. No load test, no failure-injection test (Redis/Postgres/R2/GPU failure).

### VE-013 (P2) — the voice worker is invisible to CI

```
ID:        VE-013
CATEGORY:  CI/CD
SEVERITY:  P2 — HIGH
TITLE:     No CI workflow executes the voice-engine Python test suite

LOCATION:  .github/workflows/
FILE:      ci.yml, deploy.yml, golden-update.yml, publish.yml
FUNCTION:  n/a
LINE:      n/a

WHAT EXISTS:
  Four workflows. grep for 'pytest|requirements|voice-engine|icf_worker|pip install'
  returns zero matches in all four. CI runs Go (PG17 + Redis7 + -race + vet + lint +
  gofmt), Flutter analyze/test, Dart client checks, design tokens, Docker build,
  k8s/CloudFormation validation.

WHAT SHOULD EXIST:
  A job: pip install -r requirements.txt -r requirements-dev.txt && pytest -q tests,
  with ICF_WORKER_DEV=1 ICF_ENV=development.

WHY IT MATTERS:
  The component that synthesises minister speech has no automated regression gate.
  I ran the suite by hand: 27 passed in 10.77s. It is cheap — it needs no GPU,
  no model, no database.

TECHNICAL IMPACT:  Silent regressions in DSP, ingestion and the HTTP contract.
SECURITY IMPACT:   A broken auth or key-validation change would not be caught.
PERFORMANCE IMPACT: none.
PRODUCT IMPACT:    The one component with the least test infrastructure is the one
                   with the most safety-critical behaviour (loudness ceiling,
                   provenance tagging, licence gating).

RECOMMENDED FIX:  Add a ~15-line job. Optionally add pip-audit.
IMPLEMENTATION APPROACH: Copy the existing 'mobile' job shape.
TEST REQUIRED:    The job itself.
PRIORITY:         P2 — trivial effort, closes a real blind spot.
```

---

## 30. CI/CD Audit

**What CI verifies (verified by reading `ci.yml`):**
- `go mod verify`, `go build`, `go vet`, `go test -race` against **real PostgreSQL 17
  and Redis 7 services**
- `dbtest.New` *"fails rather than skips when TEST_DATABASE_URL is unset, so a green
  run always means a real database was exercised"* — a genuinely good property
- golangci-lint with `new-from-merge-base: main` and `fetch-depth: 0` (so it reports
  only new findings)
- `gofmt` check, design-token regeneration check, Dart symbol check
- Flutter analyze + test; production Docker build; **non-root runtime assertion**;
  k8s manifest validation; CloudFormation lint; filesystem scan
- A route-parity test keeps `contracts/openapi.json` honest against live routes
- Separate `deploy.yml`, `publish.yml`, `golden-update.yml`

**What CI does not verify:** the Python voice worker (§29), Go↔worker interop,
dependency CVEs, real model behaviour, migration rollback, API backward compatibility.

**Would production deploy with failed critical tests?** The Go suite is a hard gate.
The voice worker has no gate at all — which is worse than a weak one.

---

## 31. Deployment Audit

| Step | State |
|---|---|
| Build | ✅ `server/Dockerfile`, CI-verified, non-root asserted |
| Migration | ✅ checksum-verified forward migrations applied at startup |
| API deployment | ✅ k8s Deployment + HPA + PDB + NetworkPolicy |
| **Worker deployment** | ❌ **does not exist** |
| **GPU deployment** | ❌ **does not exist** |
| Model deployment | ⚠️ *"synced from the private bucket by deployment"* — no mechanism in repo |
| Storage | ✅ CloudFormation, private by default, versioned, encrypted |
| DNS / CDN | ⚠️ CloudFront distribution partially specified; DNS not in repo |
| Secrets | ✅ k8s Secret; production config refuses defaults |
| Rollback | ⚠️ Model rollback exists (`adminRollbackVoiceModel`); **image rollback not documented** |

**Reproducible? For the API, yes. For the voice engine, no.**

The API image is tagged `ghcr.io/teamthy/i-confess:IMAGE_DIGEST_REQUIRED` — a
deliberate placeholder that fails the deploy rather than shipping `latest`. Good.

---

## 32. Cost Audit

**There is no cost tracking for voice.** `grep -iE 'cost|price|spend|budget'` across
`api/voice*.go`, `voiceengine/*.go`, `store/voice*.go` returns nothing but comments.

No cost-per-generated-minute, no GPU-hour accounting, no per-voice or per-user budget.

**What controls cost today:**
- **Deterministic caching** — the single biggest saving. `ContentHash` covers voice,
  model, model version, engine, engine version, text, language, locale, style, speed,
  pitch, reference ID and dictionary version. An identical render is never paid for twice.
- **Dedupe before synthesis** in `voice.Pipeline.Generate`.
- **Per-user 20/hour rate limit.**
- **Delivery variants are content-hash addressed** — *"an identical render is never
  encoded twice."*
- **`ClassPermanent` errors are not retried** — *"Retrying only wastes money."*

**Highest-cost operation:** fine-tuning (unmeasured), then long-form batch generation
(up to 10 000 items per batch).

**VE-017 (P3):** no cost telemetry means no cost ceiling. A 10 000-item batch at
`lvlVoiceBatch` is an unbounded GPU spend with no budget check.

---

## 33. Technical Debt

1. **Three provider abstractions, two live** (§5). The dead one must go; the two live
   ones must converge on one rights model.
2. **Two rights models over three tables.** `voice_licenses` has **zero Go references**
   — a dead table. `voice_rights` (11 refs) and `voice_rights_grants` (2) coexist.
3. **134 `TEXT` timestamp columns** vs `TIMESTAMPTZ` in the bible platform. The
   `0007_job_queue.sql` comment defends the choice as *"fixed-width and lexicographically
   ordered"* — true for consistent RFC3339-UTC, but `time.RFC3339Nano` **trims trailing
   zeros**, so variable fractional-digit values do not sort lexicographically. Worth
   an audit of every writer.
4. **Duplicate HMAC URL-signing scheme** in `audio/urls.go` (§13, VE-008).
5. **Two `web` roots** — `web/` and `apps/web` are both referenced in docs
   (`docs/52-…:123` mentions `apps/web`); only `web/` exists here.
6. **15 top-level markdown "PHASE/COMPLETE/STATUS" documents** that read as progress
   journals rather than reference docs — a real onboarding tax.

---

## 34. Dead Code

| Item | Location | Evidence |
|---|---|---|
| `audio.Generator` + `audio.TTSProvider` | `audio/generator.go:73` | `NewGenerator` has **zero callers** repo-wide (the `fakeGenerator` in `workers_test.go` is a different local type). ~495 lines |
| `URLGenerator.ValidateSignedURL` | `audio/urls.go:166` | zero callers outside its own test |
| `URLGenerator.ValidateToken` | `audio/urls.go:230` | zero callers outside its own test |
| `voice_licenses` table | `0001_baseline.sql:119` | **zero Go references** |
| `NewGCSStorage` / `NewAzureStorage` | `providers.go:334,339` | always return `ErrProviderUnavailable` |
| `GenerateShortURL` / `ResolveShortURL` | `audio/urls.go:274,289` | no callers found |
| `EngineVoiceStudio` provider | `voiceengine/provider.go:35` | registered in `buildVoiceOrchestrator` but licence-blocked by design and never linked |

**Not dead (I checked before listing):** `ReclaimStale` — it is *unwired*, not unused,
and the distinction matters for the fix.

---

## 35. Production Blockers

# PRODUCTION BLOCKERS

### P0-1 — No engine is licensed for production

**Problem.** `docs/model_licenses.json` sets `production_allowed: false` for
cosyvoice, gpt_sovits, voxcpm and voicestudio. Every checkpoint field —
`weights_sha256`, `weights_license`, `commercial_use_verified`, `verified_by` — is `null`.

**Evidence.** `licenses.py:46-51`:
```python
if not entry.get("production_allowed"):
    msg = f"engine {engine!r} is not cleared for production {purpose}"
    if production:
        raise LicenseError(msg)   # LicenseError subclasses SystemExit
```
`load_engine` calls `licenses.check(name, "inference")` before constructing any engine.

**Impact.** With `ICF_ENV=production` the worker **exits at startup**. There is no
configuration that produces licensed minister speech. This is the gate working exactly
as designed — the system correctly refuses to ship something nobody has cleared.

**Required fix.** Counsel verifies, for the **exact checkpoint** in use, both the code
licence and the weights licence; record `verified_by`, `verified_at` and
`weights_sha256`; then flip `production_allowed`. Note the register's own warning:
*"Weights may carry a different licence from the code."* VoxCPM's code licence is not
even recorded (`null`).

---

### P0-2 — No model has ever produced a sample of speech

**Problem.** Every real engine is unexecuted. `engines.py` says so about two of three:
GPT-SoVITS *"has NOT been exercised against a live GPT-SoVITS instance in CI"*;
VoxCPM *"Not exercised against the real model in CI"*. The README's own status section
lists them under *"Written but not yet run against real models."*

**Evidence.** `requirements.txt` contains only `numpy` and `scipy` — no torch, no
CUDA, no engine package. No checkpoint exists in the repo. The only engine that has
ever run is `dev-tone`, which *"emits a quiet tone, never speech."*

**Impact.** **Voice quality is not merely unverified — it is unknown.** No
speaker-similarity score, no naturalness assessment, no pronunciation benchmark, no
long-form stability test. `ICF_VOICE_SCORE` has never been computed from real audio.
Everything downstream of "the voice sounds like the minister" is unproven.

**Required fix.** Stand up one GPU host, install one engine plus its checkpoint, run
`server/cmd/voice-bench` against `docs/voice/golden_set.json`, and record the results.
The harness already exists and already produces a blind listening pack with
`blind_key.json` kept separate.

---

### P1-1 — The GPU worker has no deployment, isolation or CI

**Problem.** No Dockerfile, no k8s workload, no NetworkPolicy, no CI job.

**Evidence.** `find -iname 'Dockerfile*'` → `server/Dockerfile` only.
`grep -E 'nvidia|gpu'` on `deployment-prod.yaml` → no matches. Zero of four workflows
mention the worker.

**Impact.** The worker is protected by one shared bearer token and a docstring saying
it *"must not be exposed publicly."* Anyone reaching port 8601 can synthesise speech
in any licensed voice and resolve `voice-private/` object keys. There is also no way
to scale it, roll it back, or prevent a regression shipping.

**Required fix.** Dockerfile (non-root, pinned base, CUDA runtime), k8s Deployment with
`nvidia.com/gpu`, a NetworkPolicy restricting ingress to the API worker, liveness and
readiness probes against `/v1/health`, and the CI job from VE-013.

---

### P1-2 — Stuck jobs are never reclaimed (VE-011)

Full finding in §22. A worker crash — GPU OOM being the most likely cause — permanently
orphans the job and the user's session. `ReclaimStale` is written, indexed and tested;
nothing calls it.

---

## 36. Critical Findings

```
ID:        VE-001
CATEGORY:  Architecture / Rights
SEVERITY:  P1 — CRITICAL
TITLE:     Two live generation paths enforce two different rights models

LOCATION:  server/internal/
FILE:      api/admin_voice.go, api/generation_job.go, voice/pipeline.go
FUNCTION:  adminGenerateAudio (admin_voice.go:243), Pipeline.Generate (pipeline.go:117)
LINE:      243, 117

WHAT EXISTS:
  Path A (GPU / minister voices): POST /voices/generate →
    voiceengine.Orchestrator → voicegov.Authorize → 24 capabilities, 7 statuses.
  Path B (cloud TTS): POST /admin/audio/generate → voice.Pipeline →
    rights.Evaluate → 4 booleans (CommercialUse, AIGenerationAllowed,
    MarketingAllowed, UserContentAllowed) + territories + languages.
  rights/adapter.go bridges models.VoiceRights → rights.License. Nothing bridges
  to voicegov.Grant.

WHAT SHOULD EXIST:
  One rights authority. Either voicegov subsumes rights, or Pipeline takes a
  voicegov.Grant.

WHY IT MATTERS:
  A voice_manager can render a confession through a cloud provider under a 4-boolean
  licence that knows nothing about can_clone, can_create_embedding,
  can_use_third_party_infrastructure or can_retain_after_termination. Sending
  recordings or prompts to ElevenLabs/Google/Azure is exactly the
  third-party-infrastructure question the 24-capability model exists to answer —
  and Path B never asks it.

TECHNICAL IMPACT: Two code paths to maintain, two caches, two audit shapes.
SECURITY IMPACT: A capability revoked in voicegov does not affect Path B.
PERFORMANCE IMPACT: none.
PRODUCT IMPACT: A rights dispute could turn on which endpoint produced the asset.

RECOMMENDED FIX:
  Make voicegov the single authority. Convert rights.License consumers to a Grant,
  or have Pipeline accept a Grant and delegate to voicegov.Authorize.

IMPLEMENTATION APPROACH:
  Add voicegov.Grant → rights.License narrowing for the transition, then delete
  rights.Evaluate's call sites. voicegov is a superset; the mapping is mechanical.

TEST REQUIRED:
  Revoke can_use_third_party_infrastructure, then assert POST /admin/audio/generate
  is refused with 451 when TTS_PROVIDER is a cloud provider.

PRIORITY: P1
```

```
ID:        VE-003
CATEGORY:  Rights enforcement
SEVERITY:  P2 — HIGH
TITLE:     Model rollback promotes to production without a rights check

LOCATION:  server/internal/api/voice_platform.go
FUNCTION:  adminRollbackVoiceModel
LINE:      889

WHAT EXISTS:
  adminPromoteVoiceModel (:864) loads the Grant and calls
  voicegov.Authorize(g, {Action: ActionGenerate}), returning 403 on refusal.
  adminRollbackVoiceModel (:889) loads Models, calls voiceengine.Rollback, and
  applies the changes. No Grant is loaded. No Authorize call.

WHAT SHOULD EXIST: The same gate, verbatim.

WHY IT MATTERS:
  Rollback makes a different model the production model. It is a promotion by
  another name.

TECHNICAL IMPACT: Small, local.
SECURITY IMPACT: A voice_manager can put a model into production for a voice whose
  rights were suspended or revoked after that model was last live.
PERFORMANCE IMPACT: none.
PRODUCT IMPACT: Unlicensed audio could be served following an operational rollback —
  the exact moment nobody is thinking about rights.

RECOMMENDED FIX: Copy the five-line gate from adminPromoteVoiceModel.
IMPLEMENTATION APPROACH: Paste; both handlers already load from the same store.
TEST REQUIRED: Revoke a grant, attempt rollback, assert 403 and an audit entry.
PRIORITY: P2 — five lines, closes a real hole.
```

```
ID:        VE-004
CATEGORY:  TTS pipeline / Pronunciation
SEVERITY:  P2 — HIGH
TITLE:     No text-normalisation stage exists; Bible references are read literally

LOCATION:  server/internal/voiceengine/
FILE:      markup.go, orchestrator.go
FUNCTION:  ParseMarkup, Dictionary.Apply, Orchestrator.parse
LINE:      markup.go:57, 309

WHAT EXISTS:
  ParseMarkup handles {pause}, {emph}, {say as}, {speed}, {pitch}.
  Dictionary.Apply rewrites known terms to respellings.
  The only "normalisation" in the package is whitespace folding for the content hash
  (orchestrator.go:206). grep for normalise|verse|chapter|numeral|abbrev across
  voiceengine/*.go returns only that comment.

WHAT SHOULD EXIST:
  A normalisation stage before the dictionary: Bible references
  ("John 3:16" → "John chapter three verse sixteen"), ordinals, dates, times,
  thousands separators, currency, acronyms, URLs.

WHY IT MATTERS:
  The product's core content is Scripture. The golden set already contains the exact
  case — prompt numbers-refs-01 reads:
    "Read Psalm 23 verses 1 to 6, then John 3:16, and Romans 8:28. The meeting is on
     the 14th of March, 2026, at 7:30 in the evening, and about 1,250 people are
     expected."
  Nothing in the repo transforms it, and the golden set carries no `expect` field, so
  there is no oracle to score the result against either.

TECHNICAL IMPACT: Every engine's own text frontend decides, differently per engine —
  so the same verse reads differently across a fallback.
SECURITY IMPACT: none.
PERFORMANCE IMPACT: none.
PRODUCT IMPACT: "John three colon sixteen" in a minister's voice is the single most
  likely way this product sounds broken to a Nigerian congregation.

RECOMMENDED FIX:
  Add a normaliser in voiceengine, applied before Dictionary.Apply, with a
  Bible-reference rule (book + chapter:verse), ordinals, dates, times and grouped
  numbers. Make it locale-aware so en-NG uses "the 14th of March".

IMPLEMENTATION APPROACH: Pure function over Segments; table-driven; ~150 lines plus
  a table test. Runs before the dictionary so respellings still win.

TEST REQUIRED: Table test: "John 3:16", "Romans 8:28", "1,250", "14th of March, 2026",
  "7:30", "₦5,000", "Psalm 23". Assert exact expected expansions.

PRIORITY: P2 — high product impact, contained effort, no model required to test.
```

```
ID:        VE-005
CATEGORY:  Pronunciation
SEVERITY:  P2 — HIGH
TITLE:     The pronunciation dictionary ships empty

LOCATION:  server/internal/db/migrations/0027_voice_platform.sql
FILE:      0027_voice_platform.sql, store/voice_platform.go
FUNCTION:  n/a (no seed)
LINE:      307 (table), store/voice_platform.go:647 (only writer)

WHAT EXISTS:
  A correct engine — locale-ranked lookup, aliases, IPA, per-engine overrides,
  DictVer folded into the content hash, admin upsert endpoint.
  No seed migration. No seed data file. grep 'INSERT INTO.*pronunciation' returns
  only the runtime upsert.

WHAT SHOULD EXIST:
  A seeded dictionary of biblical, Hebrew, Greek and Nigerian names and places.
  The golden set already names the vocabulary: Chukwuemeka, Onitsha, Ile-Ife,
  Oluwaseun, Ngozi, Abeokuta, Adaeze, Babatunde, Oshogbo, Ogbomosho.

WHY IT MATTERS: Infrastructure without content. A fresh production deployment
  pronounces every one of those words with no guidance.

TECHNICAL IMPACT: The dictionary code path is exercised only by admin action.
SECURITY IMPACT: none.
PERFORMANCE IMPACT: none.
PRODUCT IMPACT: Mispronounced Nigerian place names in a Nigerian product, in a
  minister's voice, is a credibility failure the target users will notice immediately.

RECOMMENDED FIX:
  Seed ~150 entries (biblical + Nigerian names/places + church terminology) with a
  migration or a startup loader, each with locale 'en-NG'.

IMPLEMENTATION APPROACH: New migration inserting into pronunciation_dictionary, or a
  JSON fixture loaded by orchestratorWithDictionary when the table is empty.

TEST REQUIRED: Lookup test asserting "Ogbomosho" resolves under en-NG and that
  en-NG-PIDGIN does not silently inherit it (the isPidgin rule).

PRIORITY: P2
```

---

## 37. High-Priority Findings

| ID | Sev | Title |
|---|---|---|
| VE-002 | P3 | `audio.Generator` + `TTSProvider` dead (~495 lines) |
| VE-006 | P2 | `inference_instruct2` unused — the strongest style lever is not connected |
| VE-007 | P2 | R2 integration untested; no cache-hit metrics |
| VE-008 | P4 | Duplicate unverified HMAC URL scheme in `audio/urls.go` |
| VE-009 | P2 | Cross-user audio isolation correct by construction but **not runtime-tested** |
| VE-010 | P1 | GPU worker isolation is a docstring, not an enforced boundary |
| VE-011 | **P1** | `ReclaimStale` never called — stuck jobs never recovered |
| VE-012 | P3 | `engine_version` hardcoded, not derived from the checkpoint |
| VE-013 | P2 | Voice worker absent from CI |
| VE-014 | P2 | `requirements.txt` cannot produce speech (numpy + scipy only) |
| VE-015 | P3 | `third_party/Matcha-TTS` never added to `PYTHONPATH` |
| VE-016 | P3 | `reference["transcript"]` read unguarded → `KeyError`, not a classified error |
| VE-017 | P3 | No cost telemetry; a 10 000-item batch has no budget ceiling |
| VE-018 | P3 | No dependency vulnerability scanning (`govulncheck` / `pip-audit`) |
| VE-019 | P3 | No transcript synchronisation — `voice_generations` stores `text_sha256` only, no word timestamps |
| VE-020 | P3 | No golden-audio regression set; benchmarks require an external scorer |

**On the "95–96% similarity" claim the brief asked about (§67): it does not exist.**
`grep -riE '[0-9]{2}%.*(similar|replica|match)'` across all docs, Go, Python, TS and
Dart returns nothing. The README instead says: *"Without a scorer, quality is reported
as **not measured** and no ICF_VOICE_SCORE is produced. Scorer values outside [0,1]
are rejected, not clamped. The tool never picks the winning engine; people decide."*
That is the opposite of an unsupported marketing claim, and it should be preserved.

---

## 38. Quick Wins

# QUICK WINS

1. **Wire `ReclaimStale`** (~20 lines, copy `RunIdempotencySweep`). Fixes VE-011, the
   most likely source of permanently-broken sessions.
2. **Add the rights gate to rollback** (~5 lines, copy from promote). Fixes VE-003.
3. **Add the Python CI job** (~15 lines). 27 tests already pass; they need no GPU.
4. **Delete `audio/generator.go`** (~495 lines) and `ValidateSignedURL`/`ValidateToken`.
   Pure subtraction; removes a third abstraction nobody calls.
5. **Drop the `voice_licenses` table** (zero Go references).
6. **Seed the pronunciation dictionary** with the ~10 names the golden set already uses.
7. **Add `pip-audit` + `govulncheck`** to CI.
8. **Add `expect` fields to `docs/voice/golden_set.json`** so the benchmark has an oracle.
9. **Guard `reference["transcript"]`** and raise `EngineError("content", …)`.
10. **Add `Matcha-TTS` to `PYTHONPATH`** in the worker startup docs.

Items 1–5 are all under an hour each and remove real risk or real confusion.

---

## 39. Top 10 Fixes

# TOP 10 ENGINEERING FIXES

Ranked by impact ÷ (risk × effort).

1. **Wire `ReclaimStale` into a sweeper.** Highest impact-per-hour in the audit.
2. **Licence one engine and run one benchmark.** Unblocks every quality question at
   once. Engineering effort is small; the blocker is legal, so start it now.
3. **Containerise the GPU worker** (Dockerfile + k8s Deployment + NetworkPolicy +
   probes). Converts a docstring boundary into an enforced one.
4. **Add the Python worker to CI.** Prevents silent regression in DSP, licensing and
   the HTTP contract.
5. **Implement text normalisation** with a Bible-reference rule. Biggest product-audible
   improvement, and fully testable without a GPU.
6. **Unify the rights models** on `voicegov`. Removes the only path where a revoked
   capability does not apply.
7. **Seed the pronunciation dictionary.**
8. **Connect `inference_instruct2`** for CosyVoice so styles do more than scale speed
   and pitch.
9. **Add batching + a GPU autoscaling signal** (queue depth → replica count). The
   bottleneck is throughput and nothing currently responds to it.
10. **Record `engine_version` from the loaded checkpoint** and add cost telemetry per
    generation. Both are cheap and both make the system auditable.

---

## 40. Target Architecture

### Current

```
Web / Flutter
   ↓
Go API  ──(A)──→ voiceengine.Orchestrator ──→ voicegov (24 caps) ──→ Postgres queue
   │                                                                    ↓
   │                                                          Python worker (no container)
   │                                                                    ↓
   │                                                          S3/R2 → CloudFront
   └──(B)──→ voice.Pipeline ──→ rights.Evaluate (4 bools) ──→ cloud TTS ──→ S3/R2

Third abstraction (audio.TTSProvider) — dead
```

### Target

```
Web / Flutter
   ↓
Go API
   ↓
Voice Service ──────────────┐
   ↓                        │
Rights Service (voicegov) ←─┘   ← ONE authority, on BOTH paths
   ↓
VoiceProvider abstraction  ← ONE interface
   ↓
Postgres queue (SKIP LOCKED) + ReclaimStale sweeper
   ↓
GPU worker pool (containerised, NetworkPolicy-isolated, batched, autoscaled)
   ↓
TTS model (licence-cleared, version pinned, checkpoint hash recorded)
   ↓
Audio DSP → quality validation
   ↓
R2 (private, versioned)
   ↓
Cloudflare / CloudFront CDN (signed, range, cache metrics)
```

### Gap

| Gap | Required change |
|---|---|
| Two rights models | Collapse `rights` into `voicegov` |
| Three abstractions | Delete `audio.TTSProvider`; keep `voiceengine.VoiceProvider`; make `voice.Pipeline` a thin adapter over it |
| Worker not deployable | Dockerfile + k8s + NetworkPolicy + probes |
| No stale reclaim | Sweeper |
| No normalisation | New pipeline stage |
| No licensing | Counsel + register |
| No scaling | Batching + queue-depth autoscaling |

**What I am deliberately *not* recommending:** Kafka, Kubernetes service mesh,
microservice decomposition, or a separate rights service as its own process. The
evidence does not support any of them. The Postgres-`SKIP LOCKED` queue is the right
choice at this scale, and the modular monolith is the right shape. The brief's warning
against premature architecture applies here, and the current design already respects it.

---

## 41. Remediation Roadmap

### PHASE 0 — Critical blockers *(~2–3 weeks, plus legal in parallel)*

| Task | Reason | Files | Deps | Complexity | Risk | Test | Done when |
|---|---|---|---|---|---|---|---|
| Licence one engine | P0-1 | `docs/model_licenses.json` | Counsel | Low (code), **High (legal)** | High | Worker starts under `ICF_ENV=production` | `production_allowed: true` with a recorded checkpoint hash |
| Run one real benchmark | P0-2 | `cmd/voice-bench` | GPU host, checkpoint | Low | Low | Golden set renders | `report.md` + blind pack exist |
| Containerise the worker | P1-1 | new `voice-engine/Dockerfile`, `server/k8s/` | CUDA base image | Medium | Medium | Pod passes `/v1/health` | Non-root pod serving synthesis |
| Wire `ReclaimStale` | VE-011 | `cmd/server/main.go`, `store/jobqueue.go` | none | **Low** | Low | Stale job reclaimed | Killed worker's job retries |

### PHASE 1 — Security + rights *(~2 weeks)*
- Unify rights on `voicegov` (VE-001) — test: revoke `can_use_third_party_infrastructure`, assert cloud path refuses
- Rights gate on rollback (VE-003)
- NetworkPolicy for the worker; rotate `ICF_WORKER_TOKEN` (VE-010)
- `pip-audit` + `govulncheck` in CI (VE-018)
- Runtime test: user B requests user A's confession audio → 403/404 (VE-009)

### PHASE 2 — Voice quality *(~3 weeks, needs Phase 0)*
- Text normalisation (VE-004) — table test, no GPU needed, **can start now**
- Seed the dictionary (VE-005)
- Connect `inference_instruct2` (VE-006)
- Golden audio set with committed reference renders (VE-020)
- Blind human evaluation using the existing `blind-tests` API

### PHASE 3 — Provider architecture *(~1 week)*
- Delete `audio.Generator` (VE-002), drop `voice_licenses`
- Collapse `voice.Pipeline` onto `voiceengine.VoiceProvider`
- Remove the duplicate HMAC scheme (VE-008)

### PHASE 4 — Dataset / training *(~2 weeks)*
- Music detection before VAD
- Minimum reference duration + transcript validation (VE-016)
- Checkpoint-derived `engine_version` (VE-012)

### PHASE 5 — Audio infrastructure *(~1 week)*
- R2 verified end-to-end (VE-007)
- Cache-hit and CDN metrics
- Transcode-once audit (master WAV → variants is already single-pass; keep it)

### PHASE 6 — Web / mobile *(~1 week)*
- Transcript synchronisation: emit word timestamps server-side (VE-019)
- Verify background playback on a physical iOS and Android device
- Accessibility pass on the player (keyboard, screen reader, focus, touch targets)

### PHASE 7 — Scaling *(~2 weeks)*
- Batching in the worker
- Queue-depth-driven autoscaling
- Load test at 10 / 100 / 1 000 concurrent

### PHASE 8 — Observability *(~1 week)*
- GPU utilisation + VRAM export
- Distributed trace API → queue → worker
- Alert rules on queue depth, failure-by-class, rights refusals

### PHASE 9 — Production certification *(~1 week)*
- Failure injection: Redis, Postgres, R2, GPU, worker kill
- Documented RPO/RTO with a rehearsed restore
- Cost telemetry with a batch budget ceiling (VE-017)

---

## 42. Production Certification Checklist

| # | Gate | Status |
|---|---|---|
| 1 | At least one engine licence-cleared with a recorded checkpoint hash | ❌ **P0** |
| 2 | Real audio produced and reviewed by a human | ❌ **P0** |
| 3 | Speaker-similarity measured against a real reference | ❌ |
| 4 | Blind evaluation completed with ≥3 raters on a closed test | ❌ (API exists, no data) |
| 5 | Pronunciation benchmark scored on biblical + Nigerian names | ❌ |
| 6 | Golden-audio regression set committed and passing | ❌ |
| 7 | Long-form stability verified to 30 min without drift | ❌ |
| 8 | Rights revocation blocks generation — verified at runtime | ⚠️ tested in Go unit tests; not observed live |
| 9 | Expired grant blocks generation | ⚠️ same |
| 10 | Rollback respects rights | ❌ VE-003 |
| 11 | Cross-user audio access refused — runtime test | ❌ VE-009 |
| 12 | Worker containerised, non-root, network-isolated | ❌ |
| 13 | Worker in CI | ❌ VE-013 |
| 14 | Stuck jobs reclaimed | ❌ VE-011 |
| 15 | Dependency vulnerability scan clean | ❌ |
| 16 | Storage buckets verified private; signed URL expiry/tamper tested | ⚠️ verified for LocalStorage; R2 untested |
| 17 | Range requests + seeking verified on a real client | ⚠️ implemented, not exercised |
| 18 | iOS + Android background playback verified on devices | ❌ |
| 19 | Load test at 1 000 concurrent with recorded p95 | ❌ |
| 20 | Failure injection for Postgres / Redis / R2 / GPU | ❌ |
| 21 | Backups + rehearsed restore; documented RPO/RTO | ❌ not in repo |
| 22 | Cost telemetry with a batch ceiling | ❌ VE-017 |
| 23 | Text normalisation for Bible references | ❌ VE-004 |
| 24 | Pronunciation dictionary seeded | ❌ VE-005 |
| 25 | Accessibility audit of the player | ❌ |

**0 of 25 gates pass outright. 4 pass conditionally.**

---

## 43. Final Verdict

# **READY FOR INTERNAL TESTING**

**Not** "Production Ready", and I want to be precise about why — and about what the
phrase does and does not cover here.

**Why not lower.** The engineering is real, and I verified much of it by running it.
The DSP chain hit **−16.00 LUFS** with a true peak of **−1.47 dBTP** on a clipped
input; forced into limiting it delivered 7.90 dB of gain reduction and still held
**−1.056 dBFS**, reporting the achieved loudness honestly rather than claiming an
unreachable target. All three delivery encodings produced real files through ffmpeg
7.0.2, each carrying `synthetic_audio=true`. 27 worker tests pass. The web app
typechecks clean. The queue is durable, idempotent and dead-lettered with the correct
claim index. The rights model has 24 independently-licensed capabilities re-evaluated
at execution time, with a post-termination sweeper. Provenance is recorded per asset
down to the grant version in force. The documentation is unusually honest — it marks
its own gaps, and the one quality claim the brief expected to find (95–96% similarity)
**does not exist**; the README says quality is "not measured" without a scorer.

**Why not higher.** Two P0 blockers make production impossible:

1. **No engine is licensed.** `production_allowed: false` for all four, every checkpoint
   field `null`. The worker exits at startup under `ICF_ENV=production`. That is the
   gate working correctly — and it means there is no legal path to speech today.
2. **No model has ever produced a sample.** Three of four engines are marked
   "not yet run against real models" by their own source. `requirements.txt` holds
   `numpy` and `scipy` only. The only engine that has ever executed is `dev-tone`,
   which emits a tone. **Voice quality is not unverified — it is unknown.**

And the component that makes this a *Voice Engine* is the least supported thing in the
repository: no Dockerfile, no k8s workload, no NetworkPolicy, no CI job, no autoscaling.
It is a script you run by hand, protected by one static bearer token and a docstring
saying it must not be public.

**What "internal testing" means today.** Plumbing only. You can exercise the full
request lifecycle — auth, rights, caching, queue, worker dispatch, DSP, mastering,
encoding, storage, signed URLs, playback — against `dev-tone`, which is precisely what
that backend is for, and it correctly refuses to start in production. That is a
meaningful thing to test, and the platform will hold up. What you cannot do is hear
the minister.

**The single most useful next action** is not a code change: put one licensed engine on
one GPU host and run `server/cmd/voice-bench` against the golden set that already
exists. Until that happens, every statement about voice quality — including any I
could make — is speculation, and this audit declines to speculate.

**Estimated remaining effort:** ~10–14 engineering weeks to a defensible closed beta,
with legal clearance running in parallel on the critical path.

---

*Audit performed 2026-10-06 against `5ffca64`. Every executed check is listed in the
preamble with its command and result. Go conclusions are static (file:line) because no
Go toolchain was obtainable in this sandbox; that limitation is stated rather than
papered over, and it is the first thing to close when this audit is repeated.*

---

# APPENDIX A — Remediation applied 2026-10-06 (same session)

Blockers from §35 were worked immediately after the audit. Status below, with the
command that proves each claim.

| Blocker | Status | Evidence |
|---|---|---|
| **P1-2 / VE-011** — stuck jobs never reclaimed | **FIXED** | `cmd/server/main.go`: `jobQueue := store.NewJobQueue(conn)` retained; sweeper goroutine calls `jobQueue.ReclaimStale(staleCtx, staleAfter)` every `JOB_STALE_SWEEP_MINUTES` (default 5 m) with a `JOB_STALE_AFTER_MINUTES` threshold (default 15 m, `0` disables) |
| **P1-1 / VE-010** — worker not deployable or isolated | **FIXED** | `voice-engine/Dockerfile` (non-root 10001, tini, exec healthcheck, **no model baked in**); `voice-engine/k8s/deployment-gpu.yaml` (Deployment + `nvidia.com/gpu` + Secret + ConfigMap + **NetworkPolicy** + PDB); `icf_worker/healthcheck.py` |
| **VE-013** — worker absent from CI | **FIXED** | `ci.yml` gains a `voice-engine` job; `CI jobs: test, mobile, voice-engine, deployment, web, dart-client` |
| **VE-003** — rollback skips rights | **FIXED** | `api/voice_platform.go:889` now mirrors `adminPromoteVoiceModel`: loads the Grant and calls `voicegov.Authorize(g, {Action: ActionGenerate})` |
| **P0-1** — no engine licensed | **PARTIALLY ADDRESSED** | Cannot be completed by engineering: it is a counsel decision. What was added is the enforcement around it — `scripts/check_model_licenses.py` (`--strict` refuses a cleared engine without commit + checkpoint evidence; `--gate` fails CI if the runtime gate is ever weakened). **`production_allowed` remains `false` for all four engines and must stay that way until a human verifies it.** |
| **P0-2** — no model has produced a sample | **NOT FIXED** | Requires a GPU, a checkpoint and a licence decision. What was added is the runbook: *First real run on a GPU* in `voice-engine/README.md`. |

## Two bugs found by running the new code

Both were in code written during this remediation, and neither would have been
caught by reading it:

1. **The container healthcheck could never pass.** The first version probed
   `/v1/health` without a bearer token and got **401**, and used
   `urlopen(...).status == 200` — which never evaluates for non-2xx because
   urllib *raises* `HTTPError`. The container would have been permanently
   unhealthy and pulled from rotation while working correctly. Replaced with
   `icf_worker/healthcheck.py`, which sends the token and distinguishes 200 /
   503 (model did not load) / 401 (probe misconfigured).
   Verified against a live worker: correct exit code in all four cases.
2. **The NetworkPolicy pair was half-built.** The new worker policy admits the
   API on 8601, but `server/k8s/deployment-prod.yaml`'s backend egress allowed
   only Postgres, Redis and DNS — so the API could never have reached the
   worker. The matching egress rule was added. Adding either side alone produces
   a worker nothing can talk to.

## Verification performed

| Check | Command | Result |
|---|---|---|
| Worker suite (27 → 48 tests) | `pytest -q tests` | **48 passed** in 12.6 s |
| Licence checker, 3 modes | `check_model_licenses.py [--strict|--gate]` | all exit 0 on the shipped register |
| Licence checker **catches bad input** | 6 hand-built bad registers | all exit 1 with the right error |
| Licence checker **catches a neutered gate** | monkeypatched `licenses.check` to a no-op | 4 errors, exit 1 |
| YAML validity | `yaml.safe_load_all` × 3 files | OK (5 + 8 + 1 docs) |
| Web typecheck | `npx tsc --noEmit` | exit 0 |
| Go edits | brace/paren balance + pattern-match against `adminPromoteVoiceModel` | balanced; **not compile-verified** (no Go toolchain) |

### Live worker exercised end to end

The worker was started (`--engine dev-tone`) and every endpoint the Go client
calls was driven over HTTP:

| Endpoint | Result |
|---|---|
| `/v1/health` | 401 without token, 401 wrong token, **200** with token |
| `/v1/capabilities` | 200 — reports `streaming_granularity`, `self_hosted`, `engine_version` |
| `/v1/synthesize` | **200**, 160 682-byte WAV, 5 019 ms, `X-Loudness-LUFS: -16.00`, `X-DSP-Chain: dc,trim,declick,eq,deess,compress,loudness,limit`, `X-Synthetic: true`, `X-Audio-SHA256` present |
| `/v1/synthesize/stream` | 200, streamed WAV |
| `/v1/clone` | 200 — returns a `speaker_handle`, **not** a raw embedding |
| `/v1/encode` | 200 — aac/opus/mp3 with checksums; MP3 ID3 contains `synthetic_audio=` (decoded and confirmed) |
| `/v1/ingest` | 200 — found **3 of 3** placed utterances (170–940, 1470–2240, 2770–3540 ms vs. 200–900, 1500–2200, 2800–3500 ms ground truth); noise floor −54.29 dB; report honestly labels `speaker_check` a heuristic and `asr: not_run` |
| `/v1/train` | 400 with a classified `permanent` error, no crash |
| Malformed JSON | 400, not 500 |
| 3 MB body (> 2 MB limit) | **413** |

After that abuse the worker still reported healthy.

## What remains open

- **P0-1 and P0-2 are still open and are not engineering tasks.** The licence
  decision needs counsel; the first sample needs a GPU. Everything around them
  is now enforced rather than trusted.
- **The Go changes are not compile-verified.** No Go toolchain was obtainable in
  this sandbox. They are mechanical copies of adjacent verified patterns
  (`adminPromoteVoiceModel`, the five existing sweepers) and the files balance,
  but `go build ./...` and `go test ./...` must be run before merge.
- VE-001 (two rights models), VE-004 (text normalisation), VE-005 (dictionary
  seed) were P1/P2 findings, not §35 blockers, and were **not** addressed here.
