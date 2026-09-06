# Project Status

**Last verified:** 2026-09-06, at PHASE 21 (Mobile Explore; live client-side search over the fetched catalogue, featured collections rail, category grid and per-category confession lists with a scroll-driven card-emphasis motion — 119 app tests, 43 client tests, 28 Go packages, `make verify` clean).

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
| 24 test packages pass against PostgreSQL 17 | `make test` |
| No data races | `make race` |
| Lint clean, 10 linters | `make lint` → 0 issues |
| Schema loads 64 tables, 76 foreign keys | `internal/db` tests |
| Session lifecycle: 11 states, no forged completions | `internal/sessions`, 16 tests |
| Session queues are snapshots | `internal/store/snapshot_test.go` |
| Account erasure covers every user table | `internal/deletion` |
| All 39 categories seed | `internal/seed` |
| Production refuses stub payment receipts | `internal/billing/verify_prod_test.go` |

## Not done

| Area | State |
|---|---|
| **Content** | 16 confessions exist. 39 categories need content before launch (D-3). |
| **Mobile app** | `apps/mobile` cannot play audio — `just_audio` and `audio_service` are commented out. Being replaced per D-4. |
| **Website / admin** | 434 and 168 lines of scaffolding. Being replaced per D-5. |
| **Payments** | Every store verifier is a stub. Real App Store / Play verification is PHASE 36. |
| **Trial lifecycle** | The six states in §36 do not exist. |
| **UGC `PUBLIC` visibility** | Not represented; the public moderation pipeline has nothing to publish to. |
| **4 handlers** | Still return 501: recommendations, confession QA, moderation queue, user confession review. |
| **Soft delete / versioning** | Present on 2 of 64 tables each. Section 25 asks for both generally. |
| **Cache** | Per-process only; no cross-instance invalidation. |
| **Design system** | 120 tokens, contrast-verified, but not yet consumed by any real surface. |
| **Navigation** | 37 screens specified and validated; none are built. Mobile has 2 real screens and 3 placeholders. |
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

PHASE 21 Mobile Explore — **PASS WITH CONDITIONS** (live search over the fetched
    catalogue, featured collections rail, category grid, category-detail
    confession lists, scroll-driven card emphasis that settles under
    pumpAndSettle; full-text search and popular/recommended deferred as
    conditions)

Open gaps carried forward: G-2, G-3, G-4, G-5, G-6, G-7, G-9, G-10, G-12, G-13,
G-14, G-15, G-16, G-17, G-18, G-19, G-20, G-21, G-22, G-23, G-24, G-25, G-26, G-27, G-28,
G-29, G-33, G-34, G-35, G-36, G-37, G-38, G-39.

Closed: **G-1** (queues are snapshots), **G-2** (23/23 status columns constrained),
**G-8** (route parity), **G-11** (clients/dart is not a Flutter app),
**G-30** (one password policy replaces three inline `len < 8` checks),
**G-31** (session rotation already links successors; the three dead
plaintext-token functions were removed),
**G-32** (all 44 admin routes asserted to reject a non-admin).
**G-33** is new: the 24-entry blocklist is a floor, not a breach corpus.

New in PHASE 11: **G-34** (no audio exists for any of the 78 confessions),
**G-35** (canonical content has had no theological review; `Author` overstates
its provenance), **G-36** (an `EnsureContent` failure boots silently).
PHASE 11 also fixed a launch blocker that had no gap number: production came
up with an empty catalogue because content was classed as dev-only seed data.

Closed in PHASE 12: **G-4** (`public` visibility added end to end), **G-5** (one
editorial lifecycle, enforced in both the database and the code, parity-tested
against the live constraint), **G-6** (`deprecated` added, distinct from
`archived` because §9 forbids mutating an existing session's queue). G-5 turned
out to be three vocabularies, not two: five of the eight documented governance
states could not be persisted at all and returned a 500.

New in PHASE 12: **G-37** (no enforced transition graph, only a vocabulary),
**G-38** (`deprecated` is defined but nothing reads it), **G-39** (the other 20
constrained `status` columns were not audited).
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
