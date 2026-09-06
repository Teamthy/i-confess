# Project Status

**Last verified:** 2026-09-06, at PHASE 05.

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
| 23 test packages pass against PostgreSQL 17 | `make test` |
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
| **6 handlers** | Still return 501: recommendations, subscription, entitlements, confession QA, moderation queue, user confession review. |
| **Database constraints** | 22 `status` columns, 5 CHECK constraints. |
| **Cache** | Per-process only; no cross-instance invalidation. |
| **Design system** | 120 tokens, contrast-verified, but not yet consumed by any real surface. |
| **Observability** | No cache hit-rate metric; runtime dependency failure untested. |

## Phase progress

PHASE 00 Product Source of Truth — **PASS**
PHASE 01 Domain Model — **PASS**
PHASE 02 System Architecture — **PASS**
PHASE 03 Technology Decisions — **PASS**
PHASE 04 Repository Bootstrap — **PASS**
PHASE 05 Design System — **PASS**
PHASE 06 UX / Information Architecture — not started

Open gaps carried forward: G-2, G-3, G-4, G-5, G-6, G-7, G-9, G-10, G-12, G-13,
G-14, G-15, G-16.
Each is described in the phase document that raised it.

## Reproducing any of this

```
make verify      # fmt-check, design-check, build, vet, lint, test
```

Requires Go 1.25+ and a PostgreSQL 17 reachable at `TEST_DATABASE_URL`.
Tests fail rather than skip without a database — a green suite always means a
real database was exercised.
