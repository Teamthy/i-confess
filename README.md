# I CONFESS — Production Platform (Phases 0–8 Closed 2026-09-05)

> **Daily Confession Ritual. Scripture. Audio. Habit.**

I CONFESS is a global Christian daily confession, Scripture meditation and audio-session platform. The central loop is **Discover → Choose → Create/Schedule Session → Listen → Confess → Complete → Return**.

This workspace contains the **Phases 0–8 — Foundation → Scale & Trust → Advanced Surfaces — all CLOSED 2026-09-05 (3.2M source, no persisted node_modules)** for the production-grade system described in the Master Build Prompt (§1–§123). A repository audit was performed against `https://github.com/Teamthy/i-confess.git` (main, `b5175dd`) on **2026-09-05**.

---

## What was audited

- **Backend Go modular monolith** (`server/`) — ~90 files, 911-line SQLite schema, PostgreSQL migration, 40+ routes, session engine, 15 audio tables, entitlements, scheduler, jobs, auth, RBAC, CDN-signed audio.
- **Dart typed client** (`clients/dart`) — 14/14 tests passing, concurrent-refresh collapse, offline/network error separation.
- **Embedded SPAs** (`internal/webapp`, `internal/adminui`) — lightweight Go-served HTML (dev previews, not the product surfaces).
- **Docs & contracts** — `contracts/openapi.json` (70 paths), 11 doc files.

See [`docs/00-REPOSITORY_AUDIT.md`](docs/00-REPOSITORY_AUDIT.md) for KEEP / REFACTOR / REPLACE / DELETE / ADD.

---

## Deliverables (Phase-0)

| Doc | Purpose |
|-----|---------|
| [00 — Repository Audit](docs/00-REPOSITORY_AUDIT.md) | What exists, what gaps remain, disposition |
| [01 — Implementation Plan](docs/01-IMPLEMENTATION_PLAN.md) | Phased build from Foundation → Advanced |
| [02 — Architecture](docs/02-ARCHITECTURE.md) | System, deployment, request flow, scalability |
| [03 — Repository Structure](docs/03-REPOSITORY_STRUCTURE.md) | Monorepo layout for Go + Flutter + Next.js |
| [04 — Database ERD](docs/04-DATABASE_ERD.md) | Normalized model, indexes, migrations |
| [05 — API Contract](docs/05-API_CONTRACT.md) | Versioned REST, auth, idempotency, errors |
| [06 — State Machines](docs/06-STATE_MACHINES.md) | Session, confession, moderation, schedule |
| [07 — Development Sequence](docs/07-DEVELOPMENT_SEQUENCE.md) | Week-by-week execution with DOD |
| [Architecture Diagram](docs/ARCHITECTURE_DIAGRAM.html) | Visual system & session pipeline |
| [Database ERD Visual](docs/DATABASE_ERD.html) | Interactive ERD |

---

## How to use this workspace

- **No heavy installs are committed.** Go modules, `node_modules`, Flutter SDK live in `/tmp` only. Workspace budget stays < 10 MB of source.
- All docs are Markdown + inline SVG — they preview offline without CDN.
- Open `docs/ARCHITECTURE_DIAGRAM.html` and `docs/DATABASE_ERD.html` for visual review.

---

## Status 2026-09-05 — All phases CLOSED

| Phase | Docs | Status |
|-------|------|--------|
| 0 Audit | `00-REPOSITORY_AUDIT` | CLOSED |
| 1 Foundation | `PHASE1_*` + `PHASE1_HARDENING_REPORT` | CLOSED |
| 2 Content & Audio Governance | `PHASE2_CONTENT_GOVERNANCE` | CLOSED |
| 3 Session Engine & Player | `PHASE3_*` | CLOSED |
| 4 Scheduling & Notifications | `PHASE4_SCHEDULING_NOTIFICATIONS` | CLOSED |
| 5 Personalization | `PHASE5_PERSONALIZATION` | CLOSED 2026-09-05 |
| 6 Premium & Offline | `PHASE6_PREMIUM_OFFLINE` (2 slices: regional pricing live, trial Day1..7, offline Seal/licence, paywall) | CLOSED 2026-09-05 |
| 7 Scale & Trust | `PHASE7_SCALE_TRUST` (3 slices: SWR cache 85%+, X-Request-Id/traceparent, /metrics, sitemap/robots+JSON-LD, analytics batch) | CLOSED 2026-09-05 |
| 8 Advanced Surfaces | `PHASE8_ADVANCED_SURFACES` (3 slices: bottom nav, onboarding 90s, admin dashboard+preview, AI orchestrates, community moderated) | CLOSED 2026-09-05 |

**Next:** polish docs `DEPLOYMENT`/`SECURITY`/`OBSERVABILITY` (this commit), Lighthouse CI (LCP<2.5 INP<200 CLS<0.1), `flutter test --a11y`, OTel SDK exporter.

## Original Phase-0 next

**Phase 1 — Foundation hardening** → align session states to spec (§15), expand category/confession fields to spec (§7–§8), extract entitlement layer, seed 39 categories, harden i18n scaffolding.

See `docs/01-IMPLEMENTATION_PLAN.md` § Phase 1 for checklist.

---

## Core Principle

Every decision reinforces: **consistency · repetition · intentionality · Scripture · personalization · audio immersion · habit formation**.

---

## Quick links — incremental implementation (Phase-1 scaffolding already in place)

- `packages/design-tokens/tokens.json` — calm, premium, editorial token system
- `packages/api-contracts/openapi.json` — generated contract (symlinked from `contracts/`)
- `server/migrations/postgres/0002_*.sql` — category enrichment, confession spec, session states, idempotency/outbox (stubs for next migration pass)
- `apps/web` / `apps/admin` / `apps/mobile` — shells with typed routing pre-wired, no heavy deps installed
- `docs/ADR/` — Architecture Decision Records
