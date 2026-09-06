# PHASE 01 — Domain Model

**Status:** PASS WITH CONDITIONS
**Date:** 2026-09-06
**Branch base:** `main` @ `cbdee9d`
**Depends on:** PHASE 00 (PASS)

---

## OBJECTIVE

Name the domain once, in one place: its aggregates, its invariants, and the
vocabulary every later phase must use. Then state, with evidence, which
invariants are actually enforced and which are merely intended.

A domain model that only exists in prose is a wish. This one is written against
the code that exists, and every claim about enforcement points at the test or
constraint that provides it.

## INPUTS

- The Master Build Directive, §3–§10, §19, §22, §25, §35–§36
- `docs/00-PRODUCT-SOURCE-OF-TRUTH.md`
- 37 packages under `server/internal/`, 64 tables, inspected directly

## DEPENDENCIES

PHASE 00, for the canonical category list and the resolved decisions D1–D3.

---

## 1. UBIQUITOUS LANGUAGE

These terms have one meaning each. Where the code uses a different word, the
divergence is noted, because a domain model that disagrees with the codebase is
a description of a different system.

| Term | Meaning | Code |
|---|---|---|
| **Confession** | A speakable declaration grounded in Scripture. The atomic unit of content. | `models.Confession` |
| **Category** | One area of life a user confesses over. 39 canonical. | `models.Category`, `CanonicalCategories` |
| **Session** | A bounded act of listening and speaking. Not a player, not a queue. | `models.Session`, `internal/sessions` |
| **Queue** | The ordered, frozen list of items a session will play. | `models.SessionItem` |
| **Snapshot** | The property that a built session's queue does not mutate when content changes. | see §4, I-3 |
| **Voice** | A narrator identity, human or synthetic, with rights attached. | `models.Voice`, `models.VoiceRights` |
| **Rights gate** | The check that must pass before any audio is generated. | `internal/rights` |
| **Audio asset** | A rendered file for one confession × voice × variant. | `models.AudioAsset` |
| **Entitlement** | What a user may do, decided by the server. Never a client flag. | `internal/entitlements` |
| **Schedule** | A recurring intent to hold a session at a local time. | `models.Schedule` |
| **Moderation case** | A review opened on user-submitted content. | `models` + `moderation_cases` |

**Divergence worth naming:** the directive calls the audio-playing thing a
"player" and the orchestrator a "Session Engine". The code agrees. But the
database has both `sessions` and `audio_playback_sessions`, which are *not* the
same entity — the first is the domain session, the second a playback telemetry
record. Conflating them has already caused one bug in this project.

## 2. BOUNDED CONTEXTS

The directive names 16 modules. The code has 37 packages, which is not a
contradiction: several packages are infrastructure, not domain. Grouped:

| Context | Packages | Owns |
|---|---|---|
| **Identity** | `auth`, `mfa`, `oauth`, `security` | who is calling |
| **Content** | `store` (content), `seed`, `search` | categories, confessions, Scripture |
| **Governance** | `community`, `moderation` (in `api`) | UGC lifecycle, review |
| **Voice & Rights** | `voice`, `rights`, `media` | who may speak, and where |
| **Audio** | `storage`, `jobs` | rendering, assets, delivery |
| **Session** | `sessions`, `engine` | lifecycle, planning, queue |
| **Scheduling** | `scheduler`, `push`, `email` | when, and reminding |
| **Commerce** | `billing`, `entitlements` | what a user may do |
| **Platform** | `db`, `config`, `httpx`, `log`, `metrics`, `tracing`, `cache`, `ratelimit`, `health` | not domain |
| **Surface** | `api`, `adminui`, `webapp` | transport, not domain |

The rule this establishes: **`api` is transport and holds no invariants.** Every
rule below lives in a domain package or in the database. A handler that
re-implements a rule is a bug waiting to disagree with the original.

## 3. AGGREGATES

Four aggregates carry identity and invariants. Everything else is either inside
one of them or a read model.

### Session (root: `Session`)

Members: `SessionItem` (queue), playback progress.
Invariant owner: `internal/sessions`.

### Content (root: `Confession`)

Members: `ScriptureRef`, `ConfessionVariant`, `ContentVersion`.
`Category` is a separate aggregate — a confession references a category by id
and must not mutate it.

### Voice (root: `Voice`)

Members: `VoiceRights`. Rights are *inside* the aggregate because a voice
without resolvable rights must not generate audio; splitting them would allow
the two to disagree.

### User (root: `User`)

Members: profile, preferences, devices, favourites, collections.
Deliberately **not** a member: `Session`. A session references a user but has
its own lifecycle; nesting it would make every session write contend on the
user aggregate.

## 4. INVARIANTS

This is the substance of the phase. Each row states the invariant, what enforces
it, and how that is proven.

| # | Invariant | Enforced by | Proof |
|---|---|---|---|
| **I-1** | A session cannot reach `COMPLETED` without passing through a state where audio actually ran | `internal/sessions` transition graph | `TestCompletionRequiresPlaybackOnEveryPath` walks the whole graph; `TestPlaybackStatesAreTheOnlyOnesThatMayComplete` |
| **I-2** | Terminal states admit no exit | `terminal` map | `TestTerminalStatesHaveNoExit` |
| **I-3** | A built session's queue is a snapshot; later content edits do not mutate it | queue materialised at build time | **not covered by a test — see §6, G-1** |
| **I-4** | Audio is never generated without valid rights | `internal/rights` gate | `rights_test.go`, 6 tests |
| **I-5** | A free plan cannot play premium voice or content | `internal/entitlements` | `TestFreePlanCannotPlayPremiumVoiceOrContent` |
| **I-6** | Unpublished content is refused even for premium | `internal/entitlements` | `TestUnpublishedBeatsPremiumRefusal` — ordering matters, premium must not override publication |
| **I-7** | No account erasure leaves personal data behind | `internal/deletion` policy list | `TestEveryUserTableHasAPolicy`, `TestEveryPolicyGeneratesRunnableSQL` |
| **I-8** | Every queried table exists in the schema | `schema.postgres.sql` | `TestEveryTableReferencedInGoExistsInTheSchema` |
| **I-9** | All 39 canonical categories exist | `CanonicalCategories` + seed | `TestSeedInstallsEveryCanonicalCategory` |
| **I-10** | Public UGC is never auto-published | `community.CanPublish` | `TestCreateAndFeedOnlyShowsApprovedSharedPosts` |
| **I-11** | Session state values on the wire match the database constraint | `sessions.status` CHECK | `TestDomainVocabularyMatchesTheDatabase` (added this phase) |

I-1 deserves emphasis because it is the strongest invariant in the system and it
is *structural*: there is no edge into `COMPLETED` except from `Active`,
`Paused` or `Interrupted`, and the test proves it by graph traversal rather than
by asserting one input. Before this package existed, status was a flat whitelist
in the HTTP layer and a client could forge a completion — which matters because
completion is the product's primary retention metric.

I-6 is subtle and easy to break. The naive implementation checks entitlement
first and returns 403 for free users, which accidentally grants premium users
access to unpublished drafts. The test exists because the ordering is the rule.

## 5. LIFECYCLES — WHAT EXISTS AND WHAT DOES NOT

| Lifecycle | Directive | Implemented | State |
|---|---|---|---|
| **Session** | 11 states (§7) | 11 states, transition table, 13 tests | **complete** |
| **Session item** | 5 statuses (§9) | `QUEUED PLAYING COMPLETED SKIPPED FAILED` + legacy normalisation | **complete** |
| **Content** | DRAFT→REVIEW→APPROVED→PUBLISHED→DEPRECATED (§19) | `draft approved published rejected archived` | **partial** — no `REVIEW`, no `DEPRECATED` |
| **UGC** | PRIVATE/SHARED/PUBLIC (§22) | `private shared` only; 7 statuses | **partial** — no `PUBLIC` visibility |
| **Trial** | ELIGIBLE→STARTED→ACTIVE→EXPIRING→EXPIRED→CONVERTED (§36) | none of these names exist | **absent** |
| **Voice rights** | 11 fields (§5) | `models.VoiceRights`, 14 fields | **complete, superset** |
| **Audio job** | 7 statuses (§20) | `queued running completed failed dead_letter` | **partial** — no `QA_REQUIRED`, `APPROVED` |

The session engine is genuinely production-quality. The commerce and content
lifecycles are not, and PHASE 35–37 will find that out. Recording it here is the
point: a phase that discovers an absent lifecycle later has to redesign around
it, and a phase that is told up front can plan.

## 6. GAPS

**G-1 — Snapshot immutability (I-3) is untested.**
The directive is explicit (§9): "Do not let later content changes unexpectedly
mutate an already-created session." The queue is materialised at build time, so
the behaviour is probably correct, but nothing proves it. A test must edit a
confession after a session is built and assert the queue is unchanged. This is
the single most important missing test in the domain.

**G-2 — 22 `status` columns, 5 CHECK constraints.**
Only `sessions.status` and the four added in PHASE 00 are constrained. The other
18 accept any string. The domain vocabularies above are real; the database does
not know them.

**G-3 — The trial lifecycle does not exist.**
§36 specifies six states. The code has `trial`, `active`, `expiring`, `expired`,
`cancelled` on subscriptions but no trial entity and no state machine. PHASE 37
cannot be a small phase.

**G-4 — `PUBLIC` UGC visibility is missing.**
`community/policy.go` defines `private` and `shared`. The directive's third
level, `PUBLIC`, has no representation, so the public moderation pipeline (§22)
has nothing to publish *to*.

**G-5 — No `DEPRECATED` content state.**
Content can be `archived`, which may be the same idea under another name. If it
is, the directive's vocabulary should be adopted; if it is not, the distinction
needs defining. Either way the ambiguity should not survive into PHASE 12.

## TESTING

Tests added this phase:

| Test | Protects |
|---|---|
| `TestSeedInstallsEveryCanonicalCategory` | runs the real `Seed` against PostgreSQL; asserts 39 categories by slug, not just by count |
| `TestSeedConfessionsAllResolveToACategory` | no confession resolves to a missing category — the D2 rename guard |
| `TestDomainVocabularyMatchesTheDatabase` | the Go `State` constants and the `sessions.status` CHECK constraint agree |

The seed tests exercise the real `Seed` function rather than re-implementing it.
An earlier draft of the category coverage test asserted against a hand-copied
list; once the seed derived from `CanonicalCategories`, that test would have
become tautological, so it was deleted rather than kept as decoration.

Full suite: `go test ./... -count=1` → **21/21 packages, 0 failures** against
PostgreSQL 17. `gofmt -l .` empty, `go vet ./...` clean, `go build ./...` clean.

## SECURITY REVIEW

The domain model is where authorisation semantics are decided, so two points
belong on the record:

**Entitlement is server-side (I-5, I-6).** `internal/entitlements` is the only
place that answers "may this user play this". No handler re-derives it from a
plan string. This satisfies §35's rule that the server determines entitlement.

**The rights gate is not bypassable from a handler (I-4).** Generation routes
through `internal/rights`; there is no code path from an HTTP handler to a voice
provider that skips it. That is the §5 hard gate.

One residual risk: because 18 status columns are unconstrained (G-2), an admin
endpoint that writes a status string could persist a value no domain code
recognises. That is not directly exploitable, but an unrecognised status is an
unhandled code path, and unhandled paths are where authorisation checks get
skipped.

## PERFORMANCE REVIEW

No hot paths changed. The seed now inserts 39 categories instead of 16, at
startup only, in development or when `SEED=1`. Not a concern.

One note for later: `Session` deliberately excludes the user aggregate (§3), so
session writes do not contend on a user row. That decision should be preserved
when PHASE 15 adds session persistence under load.

## DOCUMENTATION

- This file.
- Package-level comments already carry the reasoning in `internal/sessions` and
  `internal/db`; they were read, not rewritten, and the model above is
  consistent with them.

## EXIT CRITERIA

| Criterion | State |
|---|---|
| Ubiquitous language defined | Done |
| Bounded contexts mapped to real packages | Done |
| Aggregates and their boundaries named | Done |
| Invariants listed with their enforcement and proof | 11 listed, 10 proven |
| Lifecycles compared against the directive | Done, 5 divergences recorded |
| Gaps identified and owned by a later phase | 5 gaps, all mapped |
| Tests pass | 21/21 packages |

---

## PHASE REPORT

1. **Built:** the domain model document; the D2 category rename; three tests tying the seed and the state vocabulary to the database.
2. **Files changed:** `docs/01-DOMAIN-MODEL.md` (new), `server/internal/seed/seed.go` (derives from canonical), `server/internal/seed/categories_canonical_test.go`, `server/internal/seed/seed_integration_test.go` (new), `server/internal/sessions/vocabulary_test.go` (new).
3. **Architecture decisions:** `api` is transport and owns no invariants; `Session` is not nested inside `User`; `VoiceRights` is inside the `Voice` aggregate.
4. **Database/API changes:** none. The seed now writes 39 categories instead of 16.
5. **UI/UX changes:** none.
6. **Tests added:** 3.
7. **Tests executed:** `go test ./... -count=1` → 21/21 packages, 0 failures.
8. **Security considerations:** entitlement and rights gate confirmed single-path; G-2 recorded as residual risk.
9. **Performance considerations:** none material.
10. **Known issues:** G-1 through G-5.
11. **Remaining work:** G-1 (the snapshot test) should be closed before PHASE 15 builds on the queue.
12. **Phase score:** 8/10. Docked for G-1 — the most important invariant in the content domain is unproven — and for the five lifecycle divergences that later phases will have to absorb.
13. **Decision:** **PASS WITH CONDITIONS** — condition is G-1.
14. **Recommended next phase:** **PHASE 02 — System Architecture**, with G-1 closed first since it is a one-test fix.
