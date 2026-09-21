**Last verified:** 2026-09-21, at PHASE 42 (moderation completeness — user
blocking and appeals. A block is now a real server-enforced boundary in both
directions, and a dismissed report or rejected confession can be appealed
through an explicit four-state lifecycle that is heard once; overturning
reopens the work rather than granting the opposite decision. Gaps G-50 and G-51
closed. Two real defects were found and fixed on the way: the deletion package
misattributed one table's bug to another because a tolerated error left the
transaction aborted, and the appeal reopen wrote an `updated_at` column that
`reports` does not have. The schema moves to 70 tables / 83 foreign keys; the
API moves to 320 routes / 250 paths / 320 operations. PHASE 35 Conditions
remain unavailable in this checkout: `docs/35-TRIAL-LIFECYCLE.md` is absent, so
no claim is made that they were read. Previous phase reconciliation still
applies — see `docs/31-MODERATION.md`, `docs/33-AUDIT-COVERAGE.md`,
`docs/34-LIBRARY-GESTURES.md`, `docs/36-BILLING-TRIAL.md`,
`docs/37-CONTENT-LIFECYCLE.md`, `docs/38-RETENTION-VERSIONING.md`,
`docs/39-CANONICAL-AUDIO.md`, `docs/40-THEOLOGICAL-REVIEW.md`,
`docs/41-TRIAL-ENGAGEMENT.md`, `docs/42-MODERATION-BLOCKING-APPEALS.md`).

This file supersedes `MASTER-PROMPT-COMPLETION.md`,
`CONTENT-DOMAIN-COMPLETION.md`, `AUDIO-PLATFORM-STATUS.md`, `SESSION-NOTES.md`
and `README-COMPLETE.md`. Those documents claim the system is complete and
production-ready. It is not, and the claims are contradicted by specific defects
found in PHASE 00–04. They are kept as a record of what was believed, not as a
description of the system.

The rule this file follows: **nothing is listed as done unless a command proves
it.** Every claim below was produced by running something.

## Verified working

| Claim | Evidence |
|---|---|
| Backend builds | `make build` |
| 34 Go packages with tests pass against PostgreSQL 17 | `go test -modfile=/tmp/local.mod -count=1 ./...` → zero FAIL |
| No data races | `make race` |
| Lint clean, 10 linters | `make lint` → 0 issues |
| Schema loads 70 tables, 83 foreign keys | `internal/db` tests (`TestPostgresSchemaLoads`, retention audit) |
| Session lifecycle: 11 states, no forged completions | `internal/sessions`, 16 tests |
| Session queues are snapshots | `internal/store/snapshot_test.go` |
| Account erasure covers every user table | `internal/deletion` |
| All 39 categories seed | `internal/seed` |
| Production refuses stub payment receipts | `internal/billing/verify_prod_test.go` |

## Not done

| Area | State |
|---|---|
| **Content** | **Done in PHASES 39–40** — 78 canonical confessions, 312 object-backed audio fixtures, explicit theological-review metadata, and provenance-neutral authorship. |
| **Mobile app** | `apps/mobile` cannot play audio — `just_audio` and `audio_service` are commented out. Being replaced per D-4. |
| **Website / admin** | 434 and 168 lines of scaffolding. Being replaced per D-5. |
| **Payments** | **Done in PHASE 36** — production uses the Apple signed-transaction verifier or Google Play Developer API and fails closed without configuration; `TestProductionRefusesStubReceipts` remains green. |
| **Trial lifecycle** | **Done in PHASE 36** — persistent `trials` row, explicit six-state graph, one-time start, expiry/conversion, and Premium projection tests. |
| **UGC `PUBLIC` readers** | **Done in PHASE 32** — `GET /community/confessions` public, anonymous, newest-first, mobile 2-tab + web both-feeds. Was G-40. |
| **Soft delete / versioning** | **Done in PHASE 38** — all 66 application tables carry `deleted_at` and `row_version`; retention writes are tombstoned and versioned. |
| **Cache** | Per-process only; no cross-instance invalidation. |
| **Design system** | 120 tokens, contrast-verified, but not yet consumed by any real surface. |
| **Navigation** | 37 screens specified and validated; mobile has the shell plus real home, explore, category, confession, builder, activity, and production player surfaces; me and remaining secondary surfaces continue in subsequent phases. |
| **Observability** | No cache hit-rate metric; runtime dependency failure untested. |

## Phase progress

PHASE 00 Product Source of Truth — **PASS**
PHASE 01 Domain Model — **PASS**
PHASE 02 System Architecture — **PASS**
PHASE 03 Technology Decisions — **PASS**
PHASE 04 Repository Bootstrap — **PASS**
PHASE 05 Design System — **PASS**
PHASE 06 UX / Information Architecture — **PASS**
PHASE 07 Database Foundation — **PASS**
PHASE 08 Go Backend — **PASS**
PHASE 09 Auth — **PASS WITH CONDITIONS**
PHASE 10 Users & Account Surface — **PASS WITH CONDITIONS**
PHASE 11 Content Engine — **PASS WITH CONDITIONS**
PHASE 12 Content Governance — **PASS WITH CONDITIONS**
PHASE 13 Audio Infrastructure — **PASS WITH CONDITIONS**
PHASE 14 Voice Platform — **PASS WITH CONDITIONS**
PHASE 15 Session Engine — **PASS WITH CONDITIONS** (groundwork shipped early as PR #22 while the
    numbering was wrong, filed then as "PHASE 13 — SESSION ENGINE"; the audit in `docs/15` closes
    PHASE 14's C-1 and appends the earlier work as an appendix)
PHASE 16 Queue / Worker — **PASS WITH CONDITIONS** (durable PostgreSQL queue, worker pool, shared
    exponential backoff, idempotency, dead letters; closes PHASE 14 C-3 and C-4)
PHASE 17 Session APIs — **PASS WITH CONDITIONS** (delete added as a soft delete that cancels a live
    session first, keyset pagination on history, the full twelve-operation surface asserted against
    the live route table)
PHASE 18 Mobile Foundation — **PASS WITH CONDITIONS** (Flutter 3.47 / Riverpod 3 / go_router;
    two visual modes from generated tokens, mapped error recovery, keystore-backed tokens,
    analytics vocabulary with a PII blocklist)
PHASE 19 Mobile Authentication & Onboarding — **PASS WITH CONDITIONS** (splash, welcome,
    three-slide onboarding, sign in with in-place MFA, sign up, forgot password, verification,
    reset, completion; client-side validation that mirrors `password_policy.go` and is guarded
    against drift by a test that reads the Go source)

PHASE 20 Mobile Home — **PASS WITH CONDITIONS** (signed-in dashboard: greeting,
    daily-session CTA into the builder, continue-listening and recent activity
    split by the server's session states, category carousel; rails degrade
    independently and never cache session lists)

PHASE 21 Mobile Explore — **PASS WITH CONDITIONS** (search, featured rail,
    category grid, per-category confession list with horizontal emphasis motion
    that settles under pumpAndSettle)

PHASE 22 Mobile Confession Experience — **PASS WITH CONDITIONS** (confession
    detail: full texts, variants, scripture anchors, intensity, tags, favourite
    toggle and build-session action; confess tab now a real category-selection
    surface; category rows navigate to detail)

PHASE 23 Mobile Session Builder — **PASS WITH CONDITIONS** (the builder walk
    is real end to end: duration step with the engine's ladder, a bounds-
    enforcing custom field, the BALANCED-defaulted strategy selector and a
    live `POST /sessions/preview` card; voice step filtering the catalogue to
    `status == 'active'` with "no preference" as a first-class choice; review
    step that creates the session — asserting the exact request body — shows
    the composition with locked items and no play button, and offers the shape
    as a template per §5.4; "Build a session with this" hands the confession
    into the builder with its category pre-selected and announced; drift-guard
    test reads `planner.go` and `handlers.go` so the client's mirrored
    constants cannot rot; `contracts/openapi.json` regenerated from the live
    route table after the audit found it 146 paths stale)

PHASE 24 Mobile Player — **PASS** (immersive player above tab bar, queue
    snapshot G-1, real signed audio URLs via AudioSigner, Session Engine
    lifecycle ownership, progress sync via POST /sessions/{id}/progress,
    transparent URL refresh preserving position, deterministic local/cloud
    conflict resolution, audio focus and route change interruption handling,
    server-authoritative completion validation, locked items show upgrade
    affordance, 100% test pass on Go + race and Flutter suites)

PHASE 24 Activity — **PASS** (activity tab: streak card, 3 tabs Continue/History/Schedules,
    continue from ACTIVE/PAUSED/INTERRUPTED/READY/STARTING, history from COMPLETED,
    schedules from GET /schedules, empty states with CTA, session tiles with
    play/check icons)

PHASE 25 Me / Profile — **PASS** (Me shows real bootstrap: avatar, display name,
    completion progress, entitlements summary, library entry points, edit profile
    via PATCH /me/profile, interests, premium, settings, sign-out clears cache
    and token)

PHASE 26 Templates — **PASS** (templates list from GET /templates, detail shows
    shape and can start via POST /templates/{id}/start, share via GET /t/{token}
    renders for signed-out per IA, delete/update)

PHASE 27 Library — **PASS WITH CONDITIONS** (re-audited 2026-09-20; the earlier
    PASS was not true. 8 defects found and fixed: duplicate favourite rows —
    ON CONFLICT(id) on a freshly generated id could never fire, no uniqueness
    constraint existed; favourites rendered raw UUIDs, now hydrated server-side
    with titles and a `missing` flag; My Confessions decoded editorial
    Confession instead of UserConfession so every row was blank; isFavorite
    matched the favourite row's own id against a confession id; writes never
    invalidated the 30-minute read cache; the tabs would have thrown on layout
    under scrollable:true; /library/collection/:id had no IA entry so test_ia.py
    never checked it; unknown ?type= answered 200 empty. Migration 0013.
    36 new tests. Conditions: flutter analyze/test owed to CI (no SDK in the
    sandbox — mitigated by scripts/check_dart_symbols.py, 49 assertions, now
    gating in CI); reorder + add-to-collection have endpoints but no gesture
    (G-43); cover_url has no writer (G-44). See docs/27-LIBRARY.md)

PHASE 28 Downloads — **PASS** (offline licences from GET /me/downloads,
    DownloadLibrary with used/limit/offlineHoursAllowed, expiringWithin 3d banner,
    renew via POST /me/downloads/{id}/refresh, remove via DELETE, empty state)

PHASE 29 Search — **PASS** (full-text search via GET /search with q, type, limit,
    SearchResult model, SearchRepository, type FilterChips, results navigate to
    confession/category detail, server SearchStore LIKE-based MVP)

PHASE 30 Premium — **PASS** (paywall with regional pricing NGN/USD/GBP/EUR/PHP
    from GET /subscriptions/plans, Plan.priceFor, entitlements chips, trial
    journey from GET /subscriptions/trial, current plan card, server-side
    verification via POST /subscriptions/verify, no hard-coded prices)

PHASE 31 Moderation — **PASS WITH CONDITIONS** (the last three 501s are real:
    user reporting with per-entity dedupe, a per-account throttle and a
    decidable outcome; the moderation queue (UGC + reports + editorial
    pending, oldest-first) that drains as work is done; author submission
    draft→submitted; moderator review that publishes only what the author
    offered publicly and never rejects without a reason; the §75 QA gate as
    the one enforced editorial edge audio_qa→approved with the checklist
    persisted pass or fail; canonical status changes finally write
    content_moderation_history, preserve published_at and 404 a phantom id;
    all vocabularies parity-tested against the live CHECK constraints; three
    fault injections confirmed caught. Conditions: G-40 published UGC has no
    reader yet, G-41 audit_logs still does not cover content/moderation
    actions, G-42 the seeded demo catalogue fails its own voices_licensed
    gate)

PHASE 32 Community UGC Reader — **PASS** (G-40 closed: `GET /community/confessions`
    and `/v1/community/confessions` public, anonymous projection `id,title,text,category_id,visibility,status,published_at,created_at`
    ordered newest-first, filtered in SQL to `visibility=public AND status=published` — shared/private/draft/submitted/approved excluded;
    typed Dart `getCommunityConfessions` and `CommunityRepository.confessions` returning `List<UserConfession>`;
    mobile community screen now two tabs Stories (existing `community_posts` feed) + Testimonies (published UGC) with
    RefreshIndicator, loading/empty/retry, reaction chips preserved;
    web community page fetches both feeds in parallel, renders Testimonies then Stories, anonymous;
    `design/ia.json` extended, `design/routes.json` 300 routes, `contracts/openapi.json` regenerated;
    `scripts/check_dart_symbols.py` now 59 assertions; Go suite api+store+community PASS, vet/build clean)

PHASE 33 Audit coverage for content and moderation — **PASS** (G-41 closed:
    `audit_logs` now records every moderation and editorial action through the
    one `Handler.recordAudit` sink — report_created (once per real filing, not
    per retry), user_confession_submitted/_approved/_rejected (result carries
    the state actually reached, published vs approved), report_resolved/_dismissed,
    confession_qa_passed/_failed (fail records the failing check names), and
    confession_status_{status} for the canonical PATCH.
    `UpdateConfessionStatusAudited` returns the state read under its own lock
    so `ok` vs `unchanged` cannot disagree with the history row; refusals
    (400/404/409) leave no trail. `TestAuditLogsCoverContentAndModeration`;
    see docs/33-AUDIT-COVERAGE.md)

PHASE 34 Library gestures and licence seeding — **PASS** (G-42 closed: seed
    writes an active `voice_rights` row for Grace, so the demo catalogue passes
    the `voices_licensed` gate it enforces — `TestSeedVoicesAreLicensed`.
    G-43 closed: `ReorderableListView.builder` with an explicit drag grip in
    collection detail, sending the complete order to PATCH reorder; the
    confession page gained `_AddToCollectionButton` → bottom sheet → POST
    membership. G-44 closed: `cover_url` writable on POST/PATCH
    (`validCollectionCover`: http(s) or same-origin path, ≤2048; empty
    clears, omission preserves — `TestCollectionCoverUrlWriter`); menu
    "Set cover image" dialog. G-45 closed: favourites navigate all four kinds
    (confession→detail, category→category detail, session→player, voice→the
    builder's voice step); a `missing` favourite still navigates nowhere.
    `check_dart_symbols.py` 59→73 assertions; no routes/endpoints changed —
    routes.json and openapi.json regenerate byte-identical. See
    docs/34-LIBRARY-GESTURES.md)

PHASE 36 Billing and trial lifecycle — **PASS** (G-29 closed: Apple receipts
    are accepted only after ES256 JWS verification against the pinned Apple
    certificate chain and Google purchases are fetched and evaluated through
    the Play Developer API; production/staging never fall back to the Noop
    verifier, and `TestProductionRefusesStubReceipts` remains in the suite.
    `trials` is now persistent with `ELIGIBLE→STARTED→ACTIVE→EXPIRING→EXPIRED`
    or `CONVERTED`; `TestTrialTransitions` names the explicit edge-table test,
    `TestTrialLifecycle` covers clock boundaries, and `TrialStore` is the only
    trial projection writer (`premium/trial` while running, fail-closed on
    terminal states). The API adds the start/status/convert surface under both
    prefixes; the typed client and IA are synchronized. Migration 0014 moves
    the live schema to 66 tables / 77 foreign keys. Proving commands: `go test
    ./internal/billing ./internal/trial ./internal/store ./internal/api`,
    `python3 design/test_ia.py`, `python3 scripts/check_dart_symbols.py`, and
    route export plus genspec (306 routes / 240 paths / 306 operations). See
    docs/36-BILLING-TRIAL.md)

PHASE 37 Content lifecycle enforcement — **PASS** (G-37/G-38/G-39 closed:
    `content.Edges` is the explicit forward-only graph, including the two
    published withdrawal edges and terminal archived state; the audited admin
    status writer rejects every backward, skipped, or fabricated move with
    HTTP 409; `TestContentTransitions` is the named state-machine test;
    `TestContentVocabularyParityAgainstConstraint` reads the live `confessions`
    CHECK; and `TestDatabaseVocabularyMatchesGoConstants` now audits all 23
    constrained status columns, with `TestTrialVocabularyParityAgainstConstraint`
    covering the trial `state` CHECK. Deprecated content remains playable only
    through existing snapshots and is excluded from new session building.
    Proving commands: `go test ./internal/content ./internal/db ./internal/store
    ./internal/api -count=1`, `gofmt -l internal cmd`, and `go vet ./...`. See
    docs/37-CONTENT-LIFECYCLE.md)

PHASE 38 Retention and row versioning — **PASS** (G-21/G-22 closed:
    migration 0015 adds nullable `deleted_at` and `row_version` to all 66
    application tables without changing table or foreign-key counts. The
    generic retention store validates identifiers, soft-deletes and restores
    rows idempotently, and advances the row version; content and audio reads
    exclude tombstoned rows. `TestEveryApplicationTableHasRetentionAndVersionColumns`,
    `TestSoftDeleteAndRestoreAdvanceRowVersion`, and the safe-default checks
    prove the installed PostgreSQL schema and write behavior. Proving commands:
    `go test ./internal/db ./internal/retention ./internal/store -count=1`,
    `gofmt -l internal cmd`, and `go vet ./...`. See
    docs/38-RETENTION-VERSIONING.md)

PHASE 39 Canonical audio coverage — **PASS** (G-34 and G-36 closed:
    `EnsureCanonicalAudio` runs after the all-environment content bootstrap,
    snapshots each confession's text, uploads four object-backed bootstrap
    fixtures for each of the 78 canonical confessions, records `audio_source`
    provenance, and is idempotent. Migration 0016 adds the provenance CHECK;
    startup now fails closed on content or audio bootstrap errors. The seed and
    production path no longer rely on a development-only audio side effect.
    Proving commands: `go test ./internal/seed ./internal/store ./internal/db
    ./internal/api -count=1`, `gofmt -l internal cmd`, and `go vet ./...`. See
    docs/39-CANONICAL-AUDIO.md)

PHASE 40 Canonical theological review — **PASS** (G-35 closed:
    migration 0017 adds the constrained `unreviewed|reviewed|needs_revision`
    vocabulary and review provenance columns. `EnsureCanonicalTheology` checks
    every corpus item has complete text and explicit Scripture references,
    records an internal editorial review without claiming clergy or church
    endorsement, and normalizes the old `i-confess content team` author to
    `Canonical corpus`. `EnsureCanonicalAudio` refuses an unreviewed canonical
    row, so review is a real prerequisite rather than documentation. The named
    seed tests prove 78 reviewed rows and zero overstated authors; live CHECK
    parity covers the new vocabulary. Proving commands: `go test ./internal/db
    ./internal/seed ./internal/store ./internal/api -count=1`, `gofmt -l
    internal cmd`, and `go vet ./...`. See docs/40-THEOLOGICAL-REVIEW.md)

PHASE 41 Trial engagement and the seven-day journey — **PASS** (master-plan 37
    remainder; G-46 and G-47 closed. The journey was six categories and a
    summary, so it never showed the personalization, the Premium voice or the
    custom builder that the paywall charges for: each day now carries an
    `Intent` and the flags that make it true — Day 2 asks for two sessions,
    Day 3 resolves the listener's own interests, Day 4 is the longest of the
    week, Day 5 a Premium voice, Day 6 the builder, Day 7 the review — asserted
    verbatim by `TestTrialJourneyMatchesSpec` against the seeded catalogue.
    `trial_day_completions` makes a day completable only through a real session
    completion: the sole writer is `TrialStore.CompleteDay`, called only from
    `completeSession`, with the day number taken from the trial row rather than
    the caller, and `trial_day_completed` is not on the batch allowlist
    (`TestTrialDayCompletionCannotBeAssertedByAClient`). `analytics_events`
    persists what `POST /analytics/batch` acknowledged — the handler's only sink
    was a no-op, so `202 {"accepted": n}` was a receipt for data the system did
    not hold. Expiry, conversion and cancellation funnel events are emitted by
    the store, because expiry is a clock fact no handler reliably observes.
    `GET /subscriptions/trial/engagement` reports days completed and the funnel;
    the paywall renders the progress and each day's call to action, which the
    typed client had declared and the server never sent. Schema 68 tables /
    81 foreign keys; API 308 routes / 242 paths / 308 operations. A
    backward-clock reading turned `Refresh` into a 500 and was fixed with a
    regression test. Proving commands: `go test -modfile=/tmp/local.mod
    -count=1 ./...` (34 ok, zero FAIL), `python3 design/test_ia.py` (108 wired),
    `python3 scripts/check_dart_symbols.py` (105/105), route export plus
    genspec. See docs/41-TRIAL-ENGAGEMENT.md)

PHASE 42 Moderation completeness: blocking and appeals — **PASS** (master-plan
    32 remainder; G-50 and G-51 closed. PHASE 31 could report and decide but
    could not be answered, and had no self-service boundary: `grep -rn "appeal"
    server/internal/ --include=*.go` returned nothing, and
    `ReportableEntityTypes` could not even name a user. `user_blocks` makes a
    block a boundary rather than a punishment — it deletes nothing, penalises
    nobody and is not shown to the blocked account — with two effects enforced
    server-side and tested: a blocked author's testimony leaves the blocker's
    public reader (filtered in SQL, so the limit still means rows the reader may
    see) and a reaction is refused in both directions, because honouring only
    one would let a listener keep contacting someone who asked not to hear from
    them. `moderation_appeals` gives a dismissed report or rejected confession an
    explicit `submitted→under_review→upheld|overturned` lifecycle that is heard
    once, refuses to appeal a decision nobody made or one belonging to a
    stranger, and on an overturn reopens the work instead of granting the
    opposite decision — `TestAppealOfARejectedConfessionDoesNotPublish` keeps it
    from becoming a publication back door. Appeals are in the moderation queue
    and in the audit log. Two real defects found and fixed: `parentTableFor`
    did not know `user_blocks.blocker_id` is a direct user reference, so the
    erasure generated a subquery against a column that does not exist, the error
    was tolerated as a missing table, the transaction was left aborted, and an
    innocent later table reported it; and the reopen wrote a `reports.updated_at`
    that does not exist. Proving commands: `go test -modfile=/tmp/local.mod
    -count=1 ./...` (34 ok, zero FAIL), the state-machine trio plus five
    live-constraint parity tests, `python3 design/test_ia.py` (113 wired),
    `python3 scripts/check_dart_symbols.py` (130/130), route export plus genspec
    (320/250/320). Schema 70 tables / 83 foreign keys. See
    docs/42-MODERATION-BLOCKING-APPEALS.md)

Open gaps carried forward: G-7, G-9, G-10, G-12, G-13,
G-14, G-15, G-16, G-17, G-18, G-19, G-20, G-23, G-24, G-25, G-26, G-27, G-28,
G-33, G-48, G-49, G-52, G-53, G-54.
(G-2 was removed from this list: it has been closed since PHASE 07 —
"23/23 status columns constrained" — yet appeared in both lists here, a
documentation bug fixed in PHASE 31.)

Closed: **G-1** (queues are snapshots), **G-2** (23/23 status columns constrained),
**G-8** (route parity), **G-11** (clients/dart is not a Flutter app),
**G-30** (one password policy replaces three inline `len < 8` checks),
**G-31** (session rotation already links successors; the three dead
plaintext-token functions were removed),
**G-32** (all 44 admin routes asserted to reject a non-admin),
**G-40** (published public UGC now has a public anonymous reader — `GET /community/confessions` + mobile 2-tab + web both-feeds).
**G-41** (PHASE 33: `audit_logs` covers content/moderation — one sink, `recordAudit`, every moderation and status action; refusals record nothing).
**G-42** (PHASE 34: the seed installs Grace's active licence; the demo catalogue passes its own `voices_licensed` gate).
**G-43** (PHASE 34: drag-reorder with a grip in collection detail; add-to-collection from the confession page).
**G-44** (PHASE 34: `cover_url` written by POST/PATCH, cleared by empty, refused unless http(s)/same-origin).
**G-45** (PHASE 34: favourites navigate all four entity kinds).
**G-29** (PHASE 36: production receipt verification is real Apple/Google
provider verification, and unconfigured deployments refuse rather than grant).
**G-3** (PHASE 36: the persistent six-state trial lifecycle and its one
Premium projection writer are implemented).
**G-37** (PHASE 37: the content lifecycle is an explicit forward-only edge table,
enforced by the audited status writer and covered by `TestContentTransitions`).
**G-38** (PHASE 37: `IsServedToNewSessions` is the session-building authority;
deprecated content is excluded from new queues while existing snapshots remain
playable).
**G-39** (PHASE 37: all 23 constrained status columns are vocabulary-audited in
both directions against live PostgreSQL CHECK constraints, with the trial state
covered separately).
**G-21** (PHASE 38: all 66 application tables carry a nullable deletion
 tombstone and the retention writer preserves a reversible row).
**G-22** (PHASE 38: all 66 application tables carry the uniform `row_version`
 concurrency field, and delete/restore writes advance it).
**G-34** (PHASE 39: all 78 canonical confessions have four object-backed
bootstrap audio assets, each linked to a content version and storage key).
**G-36** (PHASE 39: content and canonical-audio bootstrap failures are fatal at
startup, so a healthy process cannot hide an empty or partial catalogue).
**G-35** (PHASE 40: every canonical confession has explicit bounded editorial
review metadata, and `Author` is provenance-neutral rather than an unsupported
team or church claim).
**G-33** is new: the 24-entry blocklist is a floor, not a breach corpus.

New in PHASE 41: **G-46** (the trial could be displayed but not measured — no
day-completion record, and `POST /analytics/batch` acknowledged events into a
no-op sink, so a conversion funnel had no denominator), and **G-47** (the
seven-day journey taught six categories and a summary, never the
personalization, Premium voice or custom builder that the paywall charges for).
Both were **closed in PHASE 41**.

Raised in PHASE 41 and carried open:

**G-48 — The journey declares behaviour it does not perform.** `TrialDay` now
carries `PremiumVoice`, `Custom` and `SessionCount`, but no server code selects
a Premium voice for Day 5, opens the builder for Day 6, or creates the second
session Day 2 asks for; and nothing builds a session *for* a journey day at all
— the engine is reached through the normal builder. The flags are contract for
the client, which is a real improvement over advertising nothing, but it is not
the same as the journey running itself.

New in PHASE 42: **G-50** (a listener being harassed had one tool - file a
report and wait for a human - and no self-service boundary), and **G-51** (every
moderation decision was terminal; the person it was made about could not
answer). Both were **closed in PHASE 42**.

Raised in PHASE 42 and carried open:

**G-52 — The deletion package misattributes failures.** `applyPolicy` tolerates
any error whose message contains "does not exist" as a missing table and
continues the loop, but in PostgreSQL that error has already aborted the
transaction, so every later statement fails and the first one to report it names
an unrelated table. PHASE 42 hit exactly this: `user_blocks` generated a
subquery against a `user_id` column it does not have, and the failure surfaced
as `apply policy for moderation_appeals`. The specific cause is fixed
(`parentTableFor` now maps `user_blocks` to `users`); the masking behaviour is
not. A tolerated error inside `Erase` should abort the erasure loudly rather
than continue.

**G-53 — Blocking and appeals have no mobile UI.** The typed client carries the
whole surface (`ModerationRepository`) and the symbol check gates it, but no
screen renders it. Both are gestures a listener needs in the moment - blocking
while being harassed, appealing while reading a rejection - so an API-only
surface is not the feature.

**G-54 — A report still cannot name a user.** `ReportableEntityTypes` remains
`{confession, community_post}`. Blocking now covers the harassment case that
motivated it, but "report this person" is still not something the product can
do, and a pattern of behaviour across many posts has no way to be reported as
one thing.

**G-49 — Trial expiry analytics depend on someone looking.** `trial_expired` is
recorded when something next refreshes the trial, and there is no sweeper, so an
account that never returns is never counted as churned. The churn side of the
funnel is therefore a lower bound. Fixing it means a scheduled trial sweep,
which belongs with the worker work rather than here.

Recorded in PHASE 11: **G-34** (canonical audio coverage), **G-35**
(canonical content has had no theological review; `Author` overstates its
provenance), and **G-36** (an `EnsureContent` failure booted silently). G-34
and G-36 were closed in PHASE 39; G-35 was closed in PHASE 40. PHASE 11 also
fixed a launch blocker that had no gap number: production came up with an empty
catalogue because content was classed as dev-only seed data.

Closed in PHASE 12: **G-4** (`public` visibility added end to end), **G-5** (one
editorial lifecycle, enforced in both the database and the code, parity-tested
against the live constraint), **G-6** (`deprecated` added, distinct from
`archived` because §9 forbids mutating an existing session's queue). G-5 turned
out to be three vocabularies, not two: five of the eight documented governance
states could not be persisted at all and returned a 500.

New in PHASE 12: **G-37** (no enforced transition graph, only a vocabulary),
**G-38** (`deprecated` is defined but nothing reads it), **G-39** (the other 20
constrained `status` columns were not audited).

New in the PHASE 27 re-audit: **G-43** (collection reordering and in-app
"add to collection" have endpoints and typed client methods but no gesture, so
reordering is API-only), **G-44** (`cover_url` is rendered but nothing can set
it — no upload path for collection artwork, so every collection shows the
monogram fallback), **G-45** (the favourites tab lists all four entity kinds,
but only confessions navigate; a favourited voice or session renders and can be
removed yet does nothing when tapped, because no detail surface exists).
Each is described in the phase document that raised it.

## How the phases are gated

Interrelated phases now run as a **group**: back to back, one evidence report per
group, but still one commit, one document and one delivery script per phase. The
first group is the mobile block (PHASE 19–21). A phase that fails stops the
group; a condition that would invalidate the next phase stops it and asks. The
full process — conception, audit, build, verify, deliver — is in
`docs/ENGINEERING-PIPELINE.md`.

## Reproducing any of this

```
make verify      # fmt-check, design-check, build, vet, lint, test
```

Requires Go 1.25+ and a PostgreSQL 17 reachable at `TEST_DATABASE_URL`.
Tests fail rather than skip without a database — a green suite always means a
real database was exercised.
