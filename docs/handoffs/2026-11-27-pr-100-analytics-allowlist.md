# Handoff — PR #100 merged (row #4: analytics allowlist)

**Date:** 2026-11-27 · **Branch:** `arena/2a377dd9-i-confess` · **Commits:** `6e6be68`…`1956de3` · **PR:** #100 (merged to main)

## Goal

Row #4 of the fix-everything plan: centralized analytics allowlist with PII stripping, per master prompt rule #1 (analytics take IDs/counts/enums from per-event allowlist only; journal/prayer/confession/notes/highlights never reach analytics, logs, public feeds or third parties).

## What changed

- `server/internal/analytics/events.go`:
  - `AllowedEvents()` — centralized register of all 24 valid event names (server + client)
  - `ClientSubmittableEvents()` — subset clients may POST (excludes `trial_day_completed`, server-only)
  - `IsAllowed(name)` / `IsClientSubmittable(name)` — predicate forms
  - `PIIPropKeys` — 8 keys never allowed in analytics props (body, email, scripture_text, scripture, confession, note, highlight, prayer)
  - `StripPII(props)` — strips PII keys from event props at ingestion

- `server/internal/api/analytics_batch.go`:
  - Uses `analytics.IsClientSubmittable()` instead of duplicated inline allowlist
  - Uses `analytics.StripPII()` instead of hand-deleting 3 keys

- `server/internal/store/trial_engagement.go`:
  - `AnalyticsStore.Record()` rejects non-allowlisted events (defense-in-depth for server-side emissions from trials.go, billing.go, webhooks.go)

- `server/internal/analytics/events_test.go` — 5 tests (allowlist covers all constants, client-submittable excludes server-only events, IsAllowed/IsClientSubmittable predicates, StripPII)

## Verification

- `go test ./internal/analytics/` — 5/5 ok
- `go test ./internal/store/ ./internal/mfa/ ./internal/auth/` — ok against real PostgreSQL
- `go vet` — clean on analytics, api, store
- `gofmt` — clean
- CI PR #100: 8/8 checks green (Build/vet/test, Container/deployment, Dart, Flutter, Web typecheck+build, Vercel, Voice engine)
- PR #100 merged to main

## Open items

- Rows 5–12 not started: purge tombstoned private text, feature-status register + label fakes, CODEOWNERS/PR template/branch protection, Dependabot, web ESLint, E2E staging test, staging runbook, repo hygiene
- `.arena/state.json` still tracks open gaps G-14, G-15, G-19, G-24, G-28, G-33, G-48 as work items

## Picking it up

Next session: row #5 (purge tombstoned private text). Read `docs/handoffs/2026-11-27-pr-100-analytics-allowlist.md` and `docs/PROJECT-STATUS.md` for context, then check `.arena/state.json` for tracked gaps.
