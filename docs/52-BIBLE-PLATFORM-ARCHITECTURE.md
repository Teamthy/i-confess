# iCONFESS Bible platform — repository impact audit and staged implementation

**Audit date:** 2026-09-26  
**Branch:** `arena/01a0dcc5-i-confess`  
**Status:** Repository audit completed. Phase 1 architecture and first reader/API slice are being added; this document does not claim the full master prompt is already delivered.

## Executive summary

The repository already has a meaningful Bible data foundation: `server/internal/bible` contains the 66-book OSIS canon, twelve local source editions, three source parsers, checksum validation, and a test fixture for the existing confession corpus. Eleven editions have explicit active grants in the current registry; Swahili remains pending review. Migration `0020_bible.sql` adds translation/verse tables plus private user highlights and bookmarks. `docs/51-BIBLE.md` records the initial audit before the reader slice.

The existing product is a modular Go HTTP API backed by PostgreSQL (forward-only embedded migrations), optional Redis (rate limits and cache invalidation), a durable PostgreSQL job queue, an established audio pipeline with voice-rights checks, a Next.js web client, and a Flutter/Riverpod app. Reuse those seams. Do not add a Bible microservice, a second database, or a client-to-provider integration.

The prior corpus is sourced from `seven1m/open-bibles`, not HelloAO. It must not be misrepresented as HelloAO data. The implementation therefore makes provider provenance explicit, introduces a HelloAO adapter behind the same provider interface, and retains the existing verified local corpus as a distinct provider/source. HelloAO license URLs are discovery metadata, not proof that every translation may be redistributed, downloaded, indexed, or synthesized. Translations remain unapproved until rights are reviewed.

## A. Architecture impact report

| Area | Existing capability | Bible impact / reuse |
|---|---|---|
| Backend | Go `net/http` with route recorder, session middleware, `httpx` response helpers | Add Bible domain handlers and provider contract inside the current modular API. Provider data is normalized before reaching handlers. |
| Database | PostgreSQL through `internal/db.DB`; migrations embedded, numbered, checksummed and immutable | Keep migration 0020 unchanged. Add additive migration 0021 for provenance/permissions, first-class languages and private reader records. Do not import translation text in a migration. |
| Redis/cache | Optional Redis limiter and pub/sub cache invalidation; in-process TTL caches for content | Phase 1 uses short-lived chapter/catalog cache in the Bible service; later share Redis through a small cache interface rather than adding a Redis client implementation. |
| Authentication | Server-validated user/admin middleware and route-auth tests | Public licensed catalog/chapter reads; user notes/bookmarks/preferences/history use existing session middleware; admin approval is super-admin-only. |
| Audio/storage | Existing voice pipeline, background jobs, storage abstraction, signed media, voice licensing | Bible audio must be a separate rights-gated asset type and job key. Reuse pipeline/storage only after translation audio/redistribution rights checks. No on-demand uncached chapter synthesis in this slice. |
| Web | Next.js 15 app routes, same-origin `/api/*` proxy, auth shell/design tokens | Add a responsive reader that calls only `/api/v1/bible/*`. No browser HelloAO calls. |
| Flutter | Native app with Riverpod, GoRouter, five-tab shell, shared Dart API client | Add a Bible feature route and typed API repository in a follow-up client slice; preserve established navigation, auth, theming and offline constraints. |
| Admin | Existing authenticated Go admin endpoints and separate admin SPA | Add translation review/status and provenance operations after ingestion/catalog persistence is implemented; do not expose provider credentials. |
| Product ties | Confession scripture references are normalized in `scripture_references` | Preserve authored citation text and canonical OSIS reference. Future “Reflect/Pray/Confess” actions should pass structured references into existing session creation, not duplicate Bible text into UGC. |

## B. Database migration plan

1. **Keep 0020 immutable.** It holds the existing verified corpus schema and user highlights/bookmarks.
2. **0021 additive foundation.** Add language registry, translation provenance/rights/status fields, and user notes/preferences/history/progress with account-cascade privacy and canonical references. Existing Open Bibles rows get explicit source identifiers; only exact translation IDs with reviewed registry grants receive permissions (never from license-string matching), and audio remains disabled pending separate review.
3. **Catalog/import stage.** Add provider mapping/import/sync job tables and `bible_translation_books` with canon order from actual translation book lists, rather than assuming all translations share 66 books. Never activate a translation automatically on discovery.
4. **Search stage.** Add a language-neutral PostgreSQL `simple` text-search vector/index initially, with translation/book filters. Benchmark before introducing Meilisearch/OpenSearch.
5. **Study/content stage.** Add passage, cross-reference, topic, keyword and reading-plan tables only when sourced/curated datasets and their licenses are identified.
6. **Offline/sync stage.** Add download grants and versioned sync cursor/device conflict policy; client stores content only when `offline_allowed` is true. Notes use merge-safe versioning and are never silently deleted.
7. **Audio stage.** Add immutable audio asset manifest and unique cache key over translation/content version/voice version/settings after rights approval.

## C. API contract (first slice and target)

All browser/mobile callers use the iCONFESS API (`/api/v1/...` at web origin; Go routes are `/v1/...`). The provider schema is never returned.

Implemented first-slice contract:

- `GET /v1/bible/languages`
- `GET /v1/bible/translations?language=&q=`
- `GET /v1/bible/translations/{id}`
- `GET /v1/bible/books?translation={id}`
- `GET /v1/bible/{translation}/{book}/{chapter}`
- `GET /v1/bible/{translation}/{book}/{chapter}/{verse}`
- `GET /v1/bible/passage?reference=&translation=`
- `GET /v1/bible/search?q=&translation=&book=&limit=`
- Authenticated `GET|POST /v1/me/bible/{bookmarks,highlights,notes}` and `DELETE /v1/me/bible/{bookmarks,highlights,notes}/{id}`; note `PATCH` supports row-version conflict protection.

Target additions: compare, sourced cross-reference/topic/verse-of-day/random, plans, history, progress, preferences, and sync. Authenticated records derive user identity exclusively from the validated session. Private content is never included in public responses or analytics.

Normalized chapter response is `{translation,book,chapter,verses}`; every verse carries an internal stable OSIS-derived ID such as `JHN.3.16`, with source/provider IDs confined to mappings.

## D. Provider implementation plan

`BibleProvider` is the only service-facing boundary. `HelloAOBibleProvider` calls the provider's documented JSON-file endpoints using a server-side HTTP client with timeouts, bounded response bodies, retries for transient GET failures, circuit breaking, response normalization and safe errors. Its endpoint paths and types live only in that adapter. `LocalBibleProvider` reads the existing PostgreSQL mirror and remains the outage/cache fallback where license permits. Provider selection comes from server configuration; there is no client credential or direct browser/mobile provider request.

Translation metadata from HelloAO is not a license grant. `status` begins `PENDING_REVIEW`; individual redistribution, commercial, audio, offline, copy/share and indexing flags are independently false until confirmed. A provider metadata/license URL and an import checksum/version are retained as provenance. Existing Open Bibles imports retain their separate source provenance.

## E. Frontend component map

Web: `BibleHome/Reader`, translation selector, book/chapter selectors, chapter body, verse action menu, search field/results, focus mode, canonical deep links, and explicit unavailable state. The first slice uses semantic HTML, keyboard-accessible controls, readable line width and responsive light/dark system palettes. Full study/compare/audio/offline modes remain later work.

Shared domain contract: canonical book IDs and references, translation metadata including availability rights, chapter/verse DTOs, errors with actionable retry state. Do not duplicate provider response parsing in TypeScript or Dart.

## F. Mobile screen map

Implemented reader flows include translation/book/chapter selection, rights-aware copy, verse actions, search/reference resolution, private notes/bookmarks/highlights/collections, history/progress/preferences, account-bound sync outbox, reviewed plans and verse-of-day, comparison, cross-references, audio playback, and licensed offline book packages with checksum/expiry checks. These broad Flutter changes still require `dart format`, `flutter analyze`, device builds, and interaction/accessibility testing; do not treat them as release-verified.

The repository uses GoRouter/Riverpod and the typed API client. Scripture direction follows translation metadata while UI locale remains independent; six interface locales have an initial in-reader dictionary and Arabic RTL direction. This is not complete localization, translated coverage, or formal RTL/accessibility QA.

## G. Admin screen map

`apps/web/app/admin/bible`: authenticated operations dashboard for catalog discovery/sync, provider/database health, aggregate metrics, per-use rights review, reading-plan status/day editing, verse-of-day review, and registered audio manifest approval/withdrawal. Translation rights are submitted independently and every decision requires HTTPS evidence and rationale; rejected/suspended decisions persist denied grants. The API, not the admin client, contacts HelloAO. Import execution/validation dashboards and search-index management remain incomplete.

## H. Search architecture

First release uses parameterized PostgreSQL search against permitted/active local corpus content, with language/translation/book filters, exact phrase support, normalized reference parsing, bounded query length/result count and safe highlighting on the client. Search indexes only translations whose `search_index_allowed` is true. At scale, preserve the same service contract and swap the index implementation for Meilisearch/OpenSearch. Never run an unbounded wildcard scan over the canonical relational store.

## I. Offline architecture

The API now builds a selected-book package only when the translation's independent offline grants and limits allow it, issues a per-user expiring license, and returns a signed storage URL plus content hash. Flutter verifies SHA-256 and expiry before use and removes the local package after an online revocation. Package files are currently stored in app documents without an additional application-level encryption layer; offline revocation cannot be learned until reconnect. Private notes/bookmarks/highlights/history/progress sync through an idempotent, bounded API outbox held in secure device storage and partitioned by account. Collection records remain server-side/private. Conflict and account-claim cases require end-to-end testing; no multi-device merge behavior is claimed as production-verified.

## J. Audio integration architecture

Audio playback and manifest registration/review are wired through the iCONFESS API. Publication rechecks translation audio/commercial/redistribution/API grants, immutable source hash, object existence/checksum, and voice playback/synthesis rights independently. The mobile player consumes only the API-issued signed URL and optional verse alignment. Audio generation, bulk upload/ingest, operational monitoring of playback failures, and end-to-end rights/expiry testing remain incomplete; readable text never implies audio rights.

## K. Security considerations

- Provider outbound calls are server-only, allowlisted HTTPS, timed and body-size bounded.
- Normalize and validate translation/book/chapter/reference path values; SQL remains parameterized.
- Rate-limit search and provider cache misses; cap request and result sizes.
- User records are always scoped to the validated `userID`; private notes never enter logs/analytics.
- Admin rights/status writes require admin middleware and audit events.
- Do not cache private data in shared caches. Public caching is rights/status-version keyed with bounded TTL and invalidation.
- API failures map to stable friendly errors; provider internals and secrets stay in server logs.

## L. Translation provenance model

Each translation records stable app ID, provider and provider translation ID, localized and English names, ISO language identifiers/BCP-47/locale/direction, country/dialect/publisher, description, copyright/license/URL, public-domain and commercial/redistribution/modification/audio/offline/copy/share/index/API flags, attribution text, source URL/version/hash, import version/time, status and review metadata. Rights are explicit per use and default-deny where evidence is missing. Revisions create a content version/hash; they do not silently rewrite previously cited text.

## M. Testing strategy

- Unit: alias/reference parser (John 3:16, Psalm 23, 1 Cor 13:4-7), canonical IDs, normalized DTOs, translation-right checks, HelloAO success/malformed/timeout/5xx/oversize/circuit behavior.
- DB/integration: migrations are additive/checksummed; imported chapters, missing books, Unicode/RTL, private row ownership, soft-delete/version behavior, search indexes.
- API: public/user/admin auth matrix, stable error DTO, path validation, bounds, fallback and API route/OpenAPI parity.
- Web/mobile: responsive reader, keyboard/screen reader semantics, dynamic type, RTL layout, translation switch preserving reference and scroll, offline error state, auth isolation.
- Perf/security: chapter cache, query plans/index use, provider timeouts, rate limits, no provider URL in browser bundle, no private note content in logs/events.

## Deployment and rollout

1. Apply the additive Bible migrations and configure the API-only HelloAO provider, outbound allowlist, timeouts and cache limits; HelloAO discovery requires no credential.
2. Keep discovered metadata pending review. Import/activate content only after provenance and each independent rights grant have been evidenced; never activate from a catalog sync alone.
3. Protect `/admin/bible` with the existing administrator role and review rights/editorial actions through the API audit trail.
4. Release reader, personal study, plans, offline and audio capabilities only after backend integration tests, mobile analyzer/device builds, localization/accessibility QA, rights review and production-storage checks pass.
5. Monitor sanitized Bible metrics and provider/database health; do not collect private notes, bookmarks, or history content in logs/analytics.
6. Roll back by disabling the Bible provider/feature route. Do not delete user study data or reverse a released migration.

## Explicitly not claimed complete

As of 2026-09-26 the repository contains an integrated Bible API/provider boundary, pending-by-default HelloAO catalog synchronization, explicit translation and audio rights review, private study/collections and sync endpoints, curated plan and verse-of-day editorial workflows, comparison/cross-references, rights-gated offline packages/audio, Flutter reader flows, an authenticated web admin dashboard, sanitized metrics, and refreshed route/OpenAPI contracts. This is implementation scope, not production sign-off.

Still incomplete or unverified: no Go toolchain/PostgreSQL was available in this workspace, so server formatting, compilation, migration application and database/API tests have not run; Flutter/Dart formatting, analysis, tests and device builds have not run; the browser build/typecheck and design/IA/contrast checks did pass. Import/validation job operations, search-index administration, full localization and formal RTL/accessibility QA, offline at-rest encryption, revocation while disconnected, and end-to-end rights/provider-failure/account-isolation tests remain outstanding. Do not claim the platform is complete or release-ready until those gates pass. Unsupported languages/translations remain unavailable rather than synthesized or implied.
