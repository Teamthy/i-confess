# PHASE 13 — AUDIO INFRASTRUCTURE

Supersedes nothing: `AUDIO-INFRASTRUCTURE.md` (1 583 lines, "Architecture &
Schema Complete") specified this layer. The schema and the key scheme were
built to it. **The implementations were not.**

---

## OBJECTIVE

Make the audio layer real against the directive's nine items: asset model,
storage abstraction, upload, retrieval, metadata, CDN integration, signed and
private assets, **audio validation**, **duration detection**.

## INPUTS

`internal/storage/` (1 178 L), `internal/voice/pipeline.go`, `audio_assets`
(`0001_baseline.sql:162`), `AUDIO-INFRASTRUCTURE.md`.

## DEPENDENCIES

PHASE 07 (schema), PHASE 08 (bootstrap), PHASE 11 (content), PHASE 12
(lifecycle). Adds one dependency: `aws-sdk-go-v2` (config, credentials, s3).

---

## Findings

### 1. Production was wired to the local filesystem

`main.go:42` hardcoded `Provider: "local"`, and `internal/config` parsed **no**
S3, GCS or Azure variables at all — only `MEDIA_DIR` and `MEDIA_BASE_URL`.

Every method on all three cloud providers returned
`"not yet implemented"`. I first suspected this was a "boots healthy, fails at
runtime" bug. It is worse in a quieter way: the app boots *successfully* and
serves audio from local disk indefinitely, so nothing ever reports that the
cloud path was never built. On a container host the bytes die on restart and
are invisible to every other replica — audio generated on one instance is
unreachable from the next, which is what §6 forbids.

### 2. Duration was never measured

`duration_seconds` is what the session planner schedules every queue item
against. It arrived from two sources, neither of them a measurement:

- `admin.go:216` — `req.DurationSeconds`, a number typed into an admin form.
- `pipeline.go:217` — `out.DurationSeconds`, the provider's self-report.

`voice/provider.go` documents that field as *"may be zero … the pipeline then
derives it during post-processing"*. **No derivation existed in the pipeline.**
The only derivation anywhere is `elevenlabs.go`'s `size × 8 ÷ bitrate`
estimate — a bitrate assumption, not a measurement, and only in one adapter.

The existing test suite made this concrete: `pipeline_test.go:38` returned
`Audio: []byte("fake-mp3-bytes")` with `DurationSeconds: 30`. A **14-byte
payload claiming thirty seconds** passed the "happy path", was stored, and was
eligible for scheduling.

### 3. No audio validation at all

Grep for `ffprobe`, `ffmpeg`, `ValidateAudio`, duration detection: nothing. A
truncated upload, a file that says `m4a` but is `mp3`, or arbitrary bytes were
all accepted.

---

## IMPLEMENTATION

### `internal/audio` — new package (measure, then trust)

`Inspect(payload, declaredFormat) (Report, error)` sniffs the format from
magic bytes, refuses content that contradicts its declared format, measures
duration from the container header, and bounds-checks the result.

| Format | Method | Exact |
|---|---|---|
| `wav` | RIFF chunks; `data ÷ byteRate`, clamped to bytes actually present | yes |
| `flac` | STREAMINFO; `totalSamples ÷ sampleRate` (20/36-bit fields) | yes |
| `m4a` | box walk → `mdhd` preferred over `mvhd`; `duration ÷ timescale` | yes |
| `ogg` | page walk → max `granulepos ÷ sampleRate` | yes |
| `mp3` | Xing/Info frame count when present, else bitrate | Xing only |

No ffmpeg, no external binary, no network (§47). `DurationSeconds` is the
**ceiling** of the millisecond measurement, so a session never under-plans.
Bounds: 1 s to 4 h.

A truncated WAV is measured from the bytes present, not from what its header
claims — otherwise a partial upload would be scheduled as a full-length item.

### Real S3

`aws-sdk-go-v2`, with `S3API` and `S3Presigner` as **interfaces** so the
provider is tested against a fake rather than a live bucket.

- `Upload` — sets `audio/mp4` for `.m4a` (mime's table does not know it) and
  `Cache-Control: public, max-age=31536000, immutable`. Safe because the key
  encodes content, version and voice, so a key never changes meaning.
- `GenerateSignedURL` — SigV4 presign, honours the requested TTL, falls back to
  a positive default rather than minting an already-expired URL.
- `Exists` — `NotFound` is an **answer**, not a failure; a 403 is a failure and
  propagates. Getting this backwards makes the caller regenerate audio that is
  already stored and pay the provider twice.
- `Download` — a `Body.Close()` failure fails the call. Silencing it would
  return a truncated file the inspector might still parse as shorter audio.
- `List` — paginates. `Delete` — idempotent, so cleanup jobs retry safely.
- `StorageError` carries `Retryable`: 429/5xx/network yes; 403/404 no.
- Credentials: explicit keys when both are set, otherwise the SDK default chain
  — which is what production should use, since an instance role keeps no secret
  on disk. `S3Endpoint` selects path-style for MinIO/R2/Ceph.

**CDN note, deliberately not faked:** `CDNDomain` is *not* applied to S3 URLs.
SigV4 signs the host, so rewriting an S3 presigned URL to a CDN hostname
produces a URL that fails signature validation. CloudFront needs its own
key-pair signing, which is a separate mechanism and is not implemented.
`CDNDomain` remains meaningful for `LocalStorage`, which signs in-process.

### Unimplemented providers fail at boot, not at first upload

GCS and Azure had full method sets that each returned "not yet implemented".
That is worse than no code: a stub satisfies the interface, so the provider
constructs successfully. Both constructors now return `ErrProviderUnavailable`
immediately, `New()` refuses them, and their method bodies are gone.

### Boot guard

`storage.ValidateProvider(cfg, isProduction)`, called from `config.Validate()`:

- `local` in production → refused, citing §6.
- `s3` without `S3_BUCKET`/`S3_REGION` → refused.
- `gcs`, `azure`, unset, unknown → refused, naming what is available.
- Case-insensitive: `STORAGE_PROVIDER=LOCAL` must not slip past.

New env: `STORAGE_PROVIDER`, `S3_BUCKET`, `S3_REGION` (falls back to
`AWS_REGION`), `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_ENDPOINT`. `main.go` now
reads the provider from config.

### Duration measured at both entry points

`voice/pipeline.go` inspects the bytes **before** uploading: unverifiable audio
is rejected and audited as `rejected`, never stored and never billed for. The
measured value is returned and written to object metadata alongside
`sample_rate` and `format`. A provider whose self-report disagrees with the
container by more than a second is noted in the audit trail.

`api/adminUpsertAudio` measures when the URL is a key in our own storage and
discards the caller's figure; when the URL is external and unreadable from
here, it range-checks instead. Validation runs before the database lookups.

---

## TESTING

| Suite | Tests |
|---|---|
| `internal/audio` | 8 formats measured against independently computed truth; Xing vs CBR; ID3 skipped; format mismatch; not-audio; truncated; implausible; `SupportedFormat` |
| `internal/storage/s3_test.go` | immutable caching + content type; traversal key refused; TTL on signed URL; NotFound vs 403; retry classification (4 cases); pagination; idempotent delete; `ValidateProvider` (production-local, half-configured S3, case-insensitivity) |
| `internal/voice` | measured duration beats the provider's claim; non-audio rejected and audited |
| `internal/api` | absurd, zero and negative durations → 422; plausible → past the guard |
| `internal/config` | production refuses local / half-configured S3 / gcs / azure; development allows local |

Format durations are asserted against values the test computes itself from the
container it builds, so a parser that merely echoed its input would not pass.

**Defect injection, both verified:**

- Restoring `DurationSeconds: out.DurationSeconds` →
  `TestDurationIsMeasuredNotTakenFromTheProvider` fails: `30, want 3`.
- Disabling the production guard → `TestProductionRefusesLocalStorage` fails:
  `production booted with STORAGE_PROVIDER=local`.

Full suite **26 packages, 0 failures**. `make verify`: all checks passed.

---

## SECURITY REVIEW

- No credential reaches a client; signing is server-side (§14).
- `ValidKey` rejects traversal, absolute paths, `//`, `?#` and NUL before any
  S3 call — upload and sign both check it.
- Signed URLs are time-limited; a zero TTL cannot mint an expired-or-forever URL.
- Presigned S3 URLs expire; nothing is served by the API (§6).
- Production cannot boot on local storage or an unimplemented provider.

## DOCUMENTATION

`docs/13-AUDIO-INFRASTRUCTURE.md` (this file). `docs/13-SESSION-ENGINE.md`
renamed to `docs/15-SESSION-ENGINE.md` — that work is PHASE 15 content and was
mislabeled when the repo named no PHASE 13.

---

## EXIT CRITERIA

| Criterion | Status |
|---|---|
| audio asset model | PASS — unchanged, was already correct |
| storage abstraction | PASS — interface + S3 + local |
| upload / retrieval / metadata | PASS — S3 real, tested against a fake |
| CDN integration | **CONDITIONAL** — `immutable` caching set; CDN token-auth not implemented and not faked |
| signed / private assets | PASS — SigV4 presign + in-process HMAC for local |
| audio validation | PASS — new |
| duration detection | PASS — new, five formats |
| Unimplemented providers fail loudly | PASS — at boot |
| `make verify` clean | PASS |

## CONDITIONS

- **C-1** CDN token-auth (CloudFront key pairs or Cloudflare signed URLs) is
  not implemented. Audio is served from presigned S3 URLs, which is correct but
  not CDN-fronted. Needs a decision before launch.
- **C-2** GCS and Azure are still absent, now honestly so.
- **C-3** `mp3` without a Xing header yields a bitrate-derived duration,
  accurate to within a frame. Our generation path produces m4a, so this is a
  fallback rather than the common case.
- **C-4** Adding `aws-sdk-go-v2` takes the module past PHASE 03's four direct
  dependencies. Justified: §6 requires object storage, and there is no way to
  talk to S3 without an SDK or hand-rolled SigV4.
