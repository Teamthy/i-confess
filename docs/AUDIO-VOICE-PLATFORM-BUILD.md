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

The refreshed route export records 608 registered operations. A Go integration test checks all exported fields against the live router so a stale file cannot silently pass the mobile IA check. Settings now links to the existing authenticated Minister Voices screen with back navigation; the IA records the library, style, generation, job and cancellation calls it actually makes. A widget test covers the entry and return journey; Flutter execution is still owed to CI when the SDK is unavailable.

The mobile IA is not the entire web/admin architecture. Its coverage exceptions name `POST /voices/stream` as a web-admin preview (implemented in `web/components/voice-studio.tsx`) and three still-API-only operations: `GET /voices/{id}`, `POST /voice-sessions` and `GET /voice-sessions/{id}`. They remain explicit integration gaps, not verified mobile journeys. The IA test validates each exception is registered, currently unclaimed by a mobile screen, unique and accompanied by a reason. New unaccounted user-facing operations still fail; integrating these three requires product design and real client calls before their exceptions are removed. The tests do not establish that real licensed speech or a deployed GPU service is available.
