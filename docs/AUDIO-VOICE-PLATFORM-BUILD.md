# Audio + voice platform: architecture and build ledger

**Specification baseline:** iCONFESS Audio + Voice Engine Master Production Build Specification, supplied 2026-11-17. **Repository baseline:** `c8be472` plus current working-tree changes. This is an initial source review, not an exhaustive implementation audit or production certification. Older reports and the 2026-10-06 voice audit describe their own points in time; consult their appendices for subsequent fixes. No real voice, license, delivery service, physical device, or deployment was verified in this review.

## Product boundary and target data flow

`reviewed content + version + locale` → `voice identity + voice/model version + live rights` → `validated generation request` → `durable idempotent job` → `rights recheck` → `provider/worker` → `master + QA + renditions + transcript/waveform` → `private object storage + authorized delivery` → `shared player + session/queue` → `progress and privacy-safe telemetry`.

Voice selection and audio playback are separate from generation. Listener DSP is non-destructive and must not create new mastered assets. An unavailable render must not silently become another voice. Voice identity, rights, model identity, and provider implementation are separate concepts; styles and region are descriptive metadata, not personality claims. No real person's voice may be cloned or imitated without the required authorization.

## Domain model and invariants

| Concept | Responsibility / relationship |
|---|---|
| Content and content version | Reviewed, immutable source text and locale/translation revision; a session refers to a version/snapshot, not mutable text. Private user text stays private through every stage. |
| Voice identity and catalog | Stable public identity and approved descriptive metadata; provider IDs and confidential licensing details remain server-side. Catalog presence is not itself authorization to generate or distribute. |
| Rights grant | Per-voice capability, purpose, language, territory and expiry. **New generation requires APPROVED**; expiration and revocation must be evaluated live, at enqueue, execution, and delivery as appropriate. RESTRICTED may permit separately licensed non-generation uses, subject to policy. Existing assets require the applicable post-termination policy; a new rights gate alone does not retract issued URLs. |
| Voice/model version | Immutable model/provider/checkpoint provenance and reviewed release state. A voice can have multiple historical versions; never silently replace an existing render. |
| Generation request and job | References content/voice versions, locale, style, generation parameters and idempotency identity. Enqueue must be bounded; worker rechecks rights before inference. Failures have classified, bounded retries. Request-time versus execution-time access and revocation are distinct checks. |
| Audio asset and rendition | An immutable generation result with provenance, quality state, master and delivery encodings; publishing/visibility is separate from successful generation. Playback delivery must verify entitlement, content privacy and rights. |
| Transcript and waveform | Derived artifacts belong to an exact audio version. Timestamped segments must come from actual provider timings or validated alignment, never estimated as though accurate. Access follows the associated asset/content. |
| Session, queue and progress | Session is an ordered snapshot; queue/player state is durable where applicable; progress belongs to the authenticated user and resolves multi-device conflicts. Device-local playback and a server-authorized session are not the same authority. |
| Listening profile and mix | User-scoped playback settings and optional ambience with its own license. EQ and volume are client-side and capability-dependent; platform-specific limitations must be visible, not simulated. |
| Observability | Correlation IDs, state, timings and aggregated usage without private text, secrets, raw provider credentials or sensitive inferences. Costs and audio quality must not be inferred from missing measurements. |

## Existing paths and initial assessment

| Area | Evidence in repository | Status against brief |
|---|---|---|
| Voice governance | `server/internal/voicegov`, `server/internal/rights`, `server/internal/api/voice_platform.go` | Granular grant and lifecycle machinery exists; generation's RESTRICTED behavior required tightening to match the specification. Full asset revocation, public visibility, and issued-URL behavior still need end-to-end review. |
| Generation and workers | `server/internal/voice`, `server/internal/voiceengine`, `server/internal/jobs`, `voice-engine/icf_worker` | Provider abstractions, queues, inference worker and retry machinery exist. They are multiple paths that must be reconciled and tested as one experience; do not assume uniform caching, policy or fallback. |
| Mastering and storage | `voice-engine/icf_worker/dsp.py`, `mastering.py`, `encode.py`; `server/internal/storage` | Implementations and historical tests exist. A live R2/CDN range, signing, privacy and cache integration test is still required. Real speech quality has not been certified. |
| Player and DSP | `web/lib/player.tsx`, `audio-playback.ts`, `sound-engine.ts`; `apps/mobile/lib/src/features/audio` | Existing global web transport, EQ presets and ambience; mobile audio code exists. This is not proof of full custom EQ, ducking, identical mobile DSP, or physical-device behavior. |
| Catalog/admin | `web/components/voices.tsx`, `voice-studio.tsx`, API routes `/voices` and `/v1/minister-voices` | Existing public and admin surfaces. Mobile Settings links to the existing minister-voice screen; that screen uses the library, styles and generation/job APIs. These are not automatically a unified paginated 1,000-voice catalog; metadata, safe preview, filters and rights-controlled visibility need an audit and contract. No invented inventory or accents. |
| Transcript | `web/components/session-player.tsx` displays available item text without progress-based highlighting, disclosing that it is not synchronized and may differ from the recording. | No verified sentence/word timings are established by this path. Alignment pipeline, version binding, authorization and synchronized captions remain to be built. |
| Resume, settings and analytics | `web/lib/session-playback.ts`, `web/lib/preferences.ts`, mobile audio, backend sessions/analytics | Some functionality exists; cross-device conflicts, all clients, queue persistence and privacy require end-to-end tests. Web sound settings explicitly describe browser-local persistence. |
| Certification | `AUDIT-VOICE-ENGINE-2026-10-06.md` Appendix C, `docs/model_licenses.json` | No approved production checkpoint or verified real speech sample recorded. Load, browser/device, signed-delivery and product journeys are not established by this review. **Not production-ready.** |

## Initial implementation sequence

1. **Rights and safety:** enforce APPROVED-only generation centrally, including already-queued work; verify cloud and GPU paths, and separately audit distribution/revocation of existing assets and any signed URL lifetime. This is an intentionally stricter behavior change for previously RESTRICTED grants.
2. **Contract and provenance:** trace both generation paths to versions, request hashes, deduplication and durable jobs; define capability and failure contracts and test retry/revocation races. Do not add duplicate tables until existing schema is reconciled.
3. **Real audio artifacts:** version-bound transcript/waveform architecture and safe access; produce timings only with measured, reviewed alignment. Validate mastering/encoding against real licensed speech and human review.
4. **Delivery and player:** live non-production R2/CDN access and range tests; unify catalog and session/player contracts; verify player recovery and resume on actual browsers and devices.
5. **DSP and discovery:** implement testable custom controls and accurate capability disclosure, cross-platform settings and licensed previews; enforce rights and locale in search, not labels or fabricated voices.
6. **Operational gates:** real credentials and licensing approvals, dashboards, cost/quota controls, threat and privacy review, scale/load experiments, accessible UX, mobile interruption testing and complete product journey. Document measured results and outstanding risks before any production claim.

## First slice in this branch

`voicegov.Authorize` rejects `ActionGenerate` for RESTRICTED grants even when the purpose is not explicitly carved out. Existing non-generation rights remain independently scoped; this is not a blanket deletion or denial of previously generated assets. Tests cover the decision, cloud adapter refusal, and a queued minister render restricted before execution. This does not eliminate races after a worker's final rights check or invalidate previously issued signed URLs; those require further design and verification.

## Transcript design gate (next implementation slice)

A text snapshot is not automatically a transcript of a recording. The web player now presents available item text as unsynchronized, potentially different from the recording, rather than using total progress to pretend that sentences have timings. It preserves the existing browser-local accessibility preference. No captions, alignment or cross-device sync are claimed by that change.

Before exposing timed transcripts, implement and verify these boundaries:

1. **Identity:** associate each alignment with the exact immutable audio generation/asset version and a cryptographic checksum of the actual audio bytes, not just the voice, content ID, URL, or mutable session item. Snapshot the source text/version, locale, alignment producer/version and provenance. A replaced or re-encoded file requires an explicit equivalence policy or a new alignment.
2. **Data:** store validated, ordered sentence intervals in integer milliseconds, with optional word intervals only when actually produced. Reject negative, inverted, overlapping or out-of-duration intervals, missing text and malformed records. Gaps represent silence; do not stretch text into them. Record whether the data is machine-generated, reviewed, or unavailable; never manufacture precision from duration or sentence count.
3. **Access:** a transcript is at least as sensitive as its audio and source content. Reuse the existing session/audio access, privacy, moderation, rights and deletion rules rather than creating an anonymous lookup by asset ID. Test cross-user access, withdrawn/revoked assets, downloads and signed URL expiry. Do not log private transcript contents.
4. **Playback:** only highlight when the fetched alignment matches the currently playing asset/version and checksum and the player's measured media position falls within an interval. Clear stale data when switching, seeking, or retrying a track; allow auto-scroll to be disabled and respect reduced motion. Unavailable or unverified alignment falls back to clearly labelled static text, not guessed synchronization. Speech-synthesis events are browser-dependent and need separate capability handling.
5. **Evidence:** test interval boundaries and silence, malformed and mismatched versions, seeking/repeats/track changes, privacy and authorization. Test screen reader behavior and real browser playback; validate actual alignment accuracy against authorized real speech before claiming synchronized captions.

## Web custom EQ slice

The existing web sound panel now offers a Custom EQ mode with bass (200 Hz low shelf), mid (1 kHz peaking) and treble (6 kHz high shelf), each bounded to −12…+12 dB. Adjustments update the existing Web Audio filter parameters locally, with smoothed transitions and conservative attenuation for combined boosts. They never regenerate source audio. Settings remain browser-local; prior stored presets hydrate with zeroed custom bands, and reset clears them. The existing presets and optional listening compressor remain separate choices. Headroom is not a measured true-peak limiter and does not guarantee clipping-free output for every signal/device.

`web/tests/sound-engine.test.cjs` tests settings migration, bounds, constructed filter parameters, non-rebuilding slider updates, attenuation and graph cleanup with a mocked AudioContext. It does **not** verify audible behavior, CORS delivery, clipping, latency or browser/device compatibility. Device speech does not expose audio to this graph; mobile DSP, saved/synced profiles and physical-device listening remain open.

## Route inventory and mobile navigation

The refreshed route export records 608 registered operations. A Go integration test checks all exported fields against the live router so a stale file cannot silently pass the mobile IA check. Settings now links to the existing authenticated Minister Voices screen with back navigation; the IA records the library, style, generation, job and cancellation calls it actually makes. A widget test covers the entry and return journey; The navigation widget test passed in CI.

The mobile IA is not the entire web/admin architecture. Its coverage exceptions name `POST /voices/stream` as a web-admin preview (implemented in `web/components/voice-studio.tsx`) and three still-API-only operations: `GET /voices/{id}`, `POST /voice-sessions` and `GET /voice-sessions/{id}`. They remain explicit integration gaps, not verified mobile journeys. The IA test validates each exception is registered, currently unclaimed by a mobile screen, unique and accompanied by a reason. New unaccounted user-facing operations still fail; integrating these three requires product design and real client calls before their exceptions are removed. The tests do not establish that real licensed speech or a deployed GPU service is available.

## CosyVoice security candidate

The CosyVoice manifest upgrades hydra-core to 1.3.8, ONNX to 1.22.0, protobuf to 4.25.9 and the matched torch/torchaudio pair to 2.10.0 in response to new dependency advisories. These are candidate pins, not upstream-verified or GPU-verified. The existing engine baseline still includes known unresolved vulnerabilities; this change does not make the engine safe or production-ready. The declared-pin audit does not audit transitive dependencies. A PyPI-only dry-run found that ONNX 1.22.0 conflicts with the old protobuf 4.25 pin; the protobuf update addresses that specific conflict, but a subsequent dry-run was killed while fetching torch metadata, so complete dependency resolution remains unverified. Resolve and freeze the complete environment, assess remaining advisories, confirm CUDA and real inference against the exact engine commit and licensed checkpoint, and complete licence approval before deployment. The production licence gate remains closed.

## Governed worker checkpoint verification

The minister-voice path now binds a render to the reviewed model’s engine family and measured checkpoint bundle. It checks discovery at planning and compares the receipt from the actual master/preview response, which may come from a different replica. A mismatch fails before storage or stream handoff, with no automatic substitution. Approved fallbacks on eligible failures still record their own provenance and no longer occupy the primary cache key. See [worker pin and rollout contract](../voice-engine/README.md#register-and-verify-a-checkpoint) and `server/internal/api/voice_identity_test.go` for the DB-backed publication and cache regressions.

This concerns new governed renders, not the separate cloud TTS pipeline or retroactive withdrawal of historical assets. No speech checkpoint, legal rights, or GPU quality result is approved by these checks.

## Rights changes during a governed render

Starting inference does not reserve the licence. The Go API checks the actual renderer’s use (purpose, language, requested territory, user-authored text and third-party processing) against live grant terms before sending text, including to a fallback. It checks again after inference and after the private upload. A grant edit invalidates the in-flight decision even when the new terms would allow a fresh render. Expiry is evaluated at handoff; it does not depend on the expiry sweep.

- **Publication:** the master pointer, completed cache row and allowed audit commit together. That short PostgreSQL transaction checks the job owner/phase and serializes its final rights decision with grant updates. No grant lock is held across GPU, storage, encoder or listener I/O. A rights refusal is terminal and audited; a database or storage fault can retry.
- **Storage:** the content hash still deduplicates submissions. Physical attempts write separate private keys. A refused attempt deletes only its own unpublished objects. New AAC, Opus and MP3 variants have their own publication check; refusing them does not retract an already-committed master. If cleanup fails, a process dies after a put, or commit acknowledgement is lost, an operator may need to reconcile private objects against the master and variant pointers **before deleting anything**. The object store and PostgreSQL are not a single transaction.
- **Preview:** worker headers alone do not start playback. The API waits for the first body bytes, rechecks both generation and streaming permission, and records the authorized start before sending audio headers or bytes to the client. Withdrawal before that check returns `403 voice_unavailable`; an empty worker response is a temporary failure, not a successful stream.

Before enabling this feature, rehearse publication with the intended runtime role. PostgreSQL `SELECT … FOR SHARE` on the grant requires row-lock privileges (including `UPDATE` on at least one column), not just read access. The [startup smoke grants](MIGRATION-ROLE-SEPARATION.md) deliberately do not cover this feature; do not grant blanket writes to rights tables to make a test pass.

These are checks on **new** output, not continuous authorization of an already-started stream. They do not recall issued URLs or change the licence’s retention, archive, unpublish or delete policy for completed recordings. Regression coverage: `server/internal/api/voice_handoff_test.go` and `server/internal/store/voice_publication_test.go` (real PostgreSQL, fixture inference and private storage).

## Private minister-voice jobs and playlists

| Surface | Who can use the result? |
| --- | --- |
| Generate, live preview and voiced session sections | Text typed here is user-submitted, **including in Voice Studio**. The personal safety floor and user-text grant apply. Mastered jobs and their cache hits belong to that account; a preview does not create a shareable job. |
| Job status, signed master/encodings and cancellation | For a private job, only the owner. Other accounts, including super admins, get the same `404` as for a missing job. Unauthenticated requests still get `401`. Cancellation checks the audience in the database update, not just on a preceding read. |
| Session playlist | Only the session owner. Each section must also refer to a readable generation for the same voice. A missing or inaccessible section is `UNAVAILABLE`, without a generation ID, duration or URL. |
| Explicit editorial batch / pregeneration | An authenticated listener can read a catalog job; only a batch-management role can cancel it. Catalog access requires an unowned catalog row **and** a live batch item for that voice. Confession-purpose jobs remain private even in a batch. A team batch view is not permission to see another account’s private job. |

The render hash includes purpose and whether the text was user-submitted, plus the owner for private jobs. A conflicting hash cannot return or reset another owner’s row. Rights and the existing asset policy are separate from ownership: an owner is not entitled to a new URL for a withdrawn, unpublished asset.

`audio/voices/` holds governed output, not importable source material. General media signers, catalog imports and the worker’s source-recording access cannot use those keys. A blind listening test can use a governed **catalog** clip only when the stored master or attached encoding belongs to the specified voice/model and remains servable. This is checked again when reading a saved test. Studio source recordings are a separate workflow; a storage prefix is not a licence.

**Rollout:** coordinate all API/worker replicas and drain or cancel old jobs as for the checkpoint rollout above. No schema change or automatic backfill. A historic `catalog` label may have come from a staff member typing personal text. Unlinked rows are withheld, not assigned to an owner by email or silently published. Re-submit reviewed editorial material through a batch. Review legacy raw-key links and noncanonical reference keys before enabling the feature. The runtime needs feature-scoped read access to batch evidence as well as the relevant job, session and rights rows.

Job/playlist responses, new governed object uploads, direct S3 signed GETs and the local media origin use `private, no-store`. The checked-in CloudFront stack disables edge caching for `audio/voices/*`, `voice-private/*` and `bible/*` and overrides viewer `Cache-Control` even for old objects; ordinary catalog objects retain signed, optimized delivery. **Verify the deployed CDN** with the [synthetic staging check](MEDIA-EDGE-VERIFICATION.md) before enabling playback. Already-issued URLs remain bearer credentials until expiry. This change does not rewrite old object metadata, recall downloads, grant consent for external processing, or set a retention schedule.

Regression coverage: `server/internal/api/voice_privacy_test.go` (PostgreSQL, real account roles and a counting signer), `server/internal/storage/voice_scope_test.go`, and `voice-engine/tests/test_intake.py` (rendered audio is not a worker source).
