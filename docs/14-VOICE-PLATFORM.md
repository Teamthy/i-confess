# PHASE 14 — VOICE PLATFORM

## OBJECTIVE

Voice registry, provider abstraction, provider adapters, voice rights,
generation requests, generation status, audio versioning, QA workflow.
**No unauthorized voice generation.**

## INPUTS

`internal/voice/` (provider, pipeline, ElevenLabs adapter), `internal/rights/`,
`internal/store/voice_rights.go`, `internal/api/admin_voice.go`, and eleven
schema tables that existed but had never been written to.

## DEPENDENCIES

PHASE 07 (schema), PHASE 09 (roles), PHASE 11–12 (content and lifecycle),
PHASE 13 (audio inspection and object storage).

---

## Findings

The registry, the provider interface, the rights evaluator and the generation
chokepoint were all in good shape. **What was missing was everything that had to
persist**, and one of those gaps made the rights gate unsatisfiable.

### 1. Voice rights could not store the flags the gate reads

`models.VoiceRights` has carried `CommercialUse`, `AIGenerationAllowed`,
`MarketingAllowed`, `ProviderVoiceID`, `RevocationTerms` and friends since it
was written. `rights.Evaluate` reads them:

```go
case UseSynthesis:
    if !lic.AIGenerationAllowed {
        return deny(ReasonNoAIGeneration, "...")
```

`voice_rights` had **twelve columns and none of those fields were among them.**
`Create` and `Update` wrote twelve columns and dropped the rest; `ByVoiceID`
read back zero values.

The chain, end to end: `PUT /admin/voices/{id}/rights` accepted
`ai_generation_allowed: true`, verified the signed-agreement attestation,
returned **200** and wrote an audit entry — and the grant did not exist. Every
licence read as forbidding AI generation and forbidding commercial use, and no
sequence of writes could make it otherwise.

The gate was not too permissive. **It was unsatisfiable.**

### 2. Generated audio could never leave QA

Generated audio has always entered `processing`, with a comment saying so (§31).
But the codebase contained **no `UPDATE audio_assets` statement at all** — status
was only ever set at INSERT. So a render sat in QA forever and could never be
served. The only escape was re-posting the whole asset through
`POST /admin/audio` with `status: "ready"`, which recorded neither reviewer nor
reason. "Who approved this voice?" was unanswerable.

### 3. `audio_generation_jobs` had never been written to

The table has existed since the baseline schema with `status`, `attempt_count`,
`max_attempts`, `error_code`, `error_message` and a `UNIQUE idempotency_key`.
**Zero code references.** A generation was a synchronous provider call inside an
HTTP request: no record of what was asked for, no status to poll, nothing to
retry, and a double-submitted form billed the provider twice.

`voice.Backoff()` — exponential backoff with a cap — was defined at
`provider.go:82` and **never called.**

### 4. `UpsertAsset`'s conflict target could never fire

The table's real uniqueness is
`UNIQUE (content_id, content_version_id, voice_id, asset_type, quality_tier)`.
The upsert said `ON CONFLICT(id)`, and `id` is a fresh UUID on every call — so
the branch never fired. A second render for the same confession, version and
voice hit the real constraint and returned **500** instead of replacing the
asset. Regenerating audio was impossible.

### 5. Pulled audio kept playing

`audio_access.go` passed the literal `"ready"` to the entitlement check, with a
comment arguing the engine only selects ready assets so the gate was harmless.
The engine does filter at selection — but **a session queue is a snapshot**, so
an asset pulled afterwards stays referenced forever, and asserting a constant
means the gate cannot fail however the catalogue changes.

Proven before fixing: archiving every asset left **2 of 2 items serving signed
audio.**

### 6. `content_versions` was unwired

`audio_generation_jobs.content_version_id` is `NOT NULL` with a foreign key onto
`content_versions`, and nothing wrote that table — so recording a generation
request was structurally impossible.

### 7. A client disconnect threw paid audio away

`pipeline.Generate(r.Context(), …)`: bound to the request context, a closed
laptop or a proxy timeout on a long render cancelled both the provider call and
the upload. The provider was billed and the audio was discarded, with no record
of either.

---

## IMPLEMENTATION

### One authority for the asset lifecycle

`internal/audio/lifecycle.go` — seven statuses, `ServedStatuses()`, and a
deliberately narrow transition graph. Both the selection query and the
entitlement gate now read it, so they cannot disagree. That also closes a
vocabulary split: the schema's CHECK permitted `published`, but selection and
entitlement both demanded exactly `ready`, so a `published` asset could never be
selected or played and nothing compared the two lists.

The graph is narrow on purpose — **reaching a listener requires a human step**:

```
uploading  → processing | failed
processing → ready | qa_rejected | failed
ready      → published | archived | failed
published  → archived
failed     → processing | archived      (regenerate)
qa_rejected→ processing | archived      (re-render after a fix)
archived   → (terminal)
```

`processing → published` does not exist: it would skip approval.

### Voice rights, actually persisted

Migration `0004` adds the nine missing columns; `Create`, `Update` and
`ByVoiceID` write and read all of them. `boolInt` follows the schema's existing
`INTEGER 0/1` convention.

### Content versioning

`ContentStore.EnsureVersion` snapshots the confession's text before synthesis and
**reuses the version when the text is unchanged**, so regenerating does not
inflate history. A reviewer approves a render against the words that were spoken,
not against whatever the confession says later. `audio_assets.content_version_id`
records which text each render speaks.

### Generation requests and status

`AudioStore` gains `CreateJob` / `JobByID` / `Jobs` / `StartJob` / `RequeueJob` /
`CompleteJob` / `FailJob`. Every status change goes through one
`transitionJob`, so the lifecycle cannot be bypassed by writing the column.

- **Idempotency:** `sha256(confession ‖ variant ‖ voice ‖ language ‖ version)` in
  the `UNIQUE idempotency_key` column. A repeated request returns the existing
  job and does not reach the provider. `force` produces a distinct key on
  purpose.
- **Retries:** a failed job is requeued (`failed → queued → processing`) while
  attempts remain, then refused at `max_attempts` with 409.
- **Failures are explained:** `error_code` plus `error_message`, and a refusal on
  rights grounds is recorded on the job rather than only returned.

### QA workflow

`admin_audio_qa.go`: approve, reject, publish, archive — each with the transition
validated, an illegal move answered **409** naming the current state and what is
allowed, and the reviewer and note recorded on the asset. **A rejection requires
a note**, because a rejection with no reason cannot be acted on.

### Execution detached from the request

`context.WithoutCancel(r.Context())` keeps the request's values but drops its
cancellation, so a client disconnect no longer destroys a render the provider was
paid for. The job record makes any loss visible.

### Twelve new routes

`GET /admin/audio/jobs`, `GET /admin/audio/jobs/{id}`, and
`POST /admin/audio/{id}/{qa/approve,qa/reject,publish,archive}` — each registered
under both `/` and `/v1/`, as the route-parity test requires.

---

## TESTING

21 new test functions across four files.

| Area | Coverage |
|---|---|
| `internal/audio/lifecycle_test.go` | vocabulary matches the migration; only ready/published served, including the empty-status case; every legal and forbidden transition; `qa_rejected → ready` refused; job lifecycle |
| `internal/store/audio_jobs_test.go` | job recorded and observable; idempotency prevents a second job; failure records why; a job cannot exist without a content version; `EnsureVersion` idempotent per text; QA approve/reject/re-render/archive; archived is terminal |
| `internal/api/audio_lifecycle_test.go` | pulled audio stops serving, for archived / qa_rejected / failed / processing; `published` is served |
| `internal/api/audio_generation_test.go` | generation records a job, enters QA, and is invisible to session building; unlicensed voice → 451 and never reaches the provider; QA transitions enforced over HTTP including 409 on overriding a rejection; repeat request is not billed twice while `force` is; **rights grant survives a round trip**; an update can withdraw the grant |

**Defect injection, all three verified:**

- Restoring `AssetStatus: "ready"` → `TestPulledAudioStopsServingInExistingSessions` fails.
- Forcing `vr.AIGenerationAllowed = false` → `TestGenerationRecordsAJobAndEntersQA` fails.
- Restoring `ON CONFLICT(id)` → `TestRepeatGenerationIsNotBilledTwice` fails.

Full suite **26 packages, 0 failures.** `make verify`: all checks passed, 0 issues.

---

## SECURITY REVIEW

- **No unauthorized voice generation:** `rights.Evaluate` is the single chokepoint
  above every adapter, `nil` licence denies, and an unlicensed voice never reaches
  a provider — asserted with `synth.calls == 0`.
- Rights writes are `voice_manager` only; generation and QA are
  `audio_producer,voice_manager`. AI grants still require a recorded attestation.
- A human rejection cannot be overridden; archived is terminal.
- Provider credentials remain server-side (§14).

## DOCUMENTATION

This file. `docs/PROJECT-STATUS.md` updated.

---

## EXIT CRITERIA

| Directive item | Status |
|---|---|
| voice registry | PASS — existed, verified |
| provider abstraction | PASS — existed, verified |
| provider adapters | **CONDITIONAL** — ElevenLabs only |
| voice rights | PASS — now actually persisted |
| generation requests | PASS — new |
| generation status | PASS — new, pollable |
| audio versioning | PASS — new |
| QA workflow | PASS — new |
| No unauthorized voice generation | PASS — gate verified, and now satisfiable |

## CONDITIONS

- **C-1** **The engine does not prefer the asset matching a confession's current
  version.** Once versions exist, a session can still select a render made from
  superseded text. Both renders are approved, so nothing is unauthorized — but
  version-aware selection belongs in the session engine (PHASE 15).
- **C-2** Only the ElevenLabs adapter exists. The schema comment names
  `google-tts | azure-tts | openai | aws-polly | human-recording`.
- **C-3** Execution is still synchronous. The job record now exists, so moving it
  to a worker is mechanical — but that move is PHASE 16 (queue/workers).
- **C-4** `voice.Backoff()` is still uncalled; the retry loop arrives with the
  worker.
- **C-5** `commercial_use` and `marketing_allowed` are now storable, but no
  surface sets them except the rights API. Admin UI is PHASE 40.
