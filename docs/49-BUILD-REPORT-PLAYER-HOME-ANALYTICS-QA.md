# Ledger 49 — BUILD REPORT: persistent player, app home, analytics, QA

**Date:** 2026-09-22
**Branch:** `arena/01a0c6ca-i-confess`
**Depends on:** Ledger 48. Completes the web build slice of the brief.

---

## What was built

### 1. Persistent player (§31) — `lib/player-context.tsx`
One `<audio>` element mounted in the `/app` layout, above the shell: playback
survives client-side navigation. `PlayerClient` was refactored onto it —
session ownership, queue, progress narration, lifecycle and completion logic
are byte-for-byte identical; only the transport moved. The provider dedupes
loads by queue-item id, so returning to a playing session never restarts it,
and session audio ducks marketing voice samples via the existing pause
channel. `MiniPlayer` (bottom bar desktop, above-tab mobile) reflects the
same element with play/pause, progress, player deep-link, and dismiss.

### 2. App home (§29) — `components/AppHomeClient.tsx`
`/app` is now personalised from `GET /home` + `GET /recommendations`:
time-aware greeting, continue-listening card, today's experience, quick
actions, category rail, recommendations (labelled "Recommended for you" only
when `personalized: true`, else "Suggested starting points"), rhythm
shortcuts (routines/schedule/streaks), recent sessions, and an honest empty
state for new accounts.

### 3. Analytics (§51) — `lib/analytics.ts` + `ConsentBanner`
Consent-gated, authenticated-only, allowlisted-names-only batching client
that mirrors `analytics_batch.go` and the mobile prop blocklist. Events
wired: `app_opened`, `session_created`, `session_started`,
`session_completed`, `schedule_created`, `search_performed` (count only —
the query text never leaves the device), `category_viewed`. Marketing
funnel names are deliberately NOT sent: the endpoint cannot take them.
The banner appears once, for signed-in undecided listeners; the decision is
recorded via `POST /auth/consent` and mirrored locally. Fixture accepts
both endpoints for preview.

### 4. QA
- **Routes:** all 79 page routes return 200 (dynamic routes sampled with
  real ids/slugs).
- **Links:** 145 unique internal links crawled from 27 pages — 0 dead.
- **Responsive:** mobile-first audit of new motifs + shell: snap rails,
  drawer, wrapping control rows, 40px+ touch targets, safe-area-aware
  mini-player/banner, content clearance via `ia-shell--mini`; schedule
  form grid stacks under 40rem (new `.ic-form-grid-2`).
- **Build:** `tsc` clean, `next build` green (119 static pages, First Load
  JS unchanged at 103 kB).

## Out of scope for the PR (need humans/infra — brief phases 41–46)
Cross-browser pass on real Safari/Firefox/Edge, conversion audit with real
users, Vercel/staging deploy + staging QA, production deploy, final
certification. No P0/P1 remains in the web build itself.

---

## PHASE REPORT — WEB BUILD COMPLETE

**STATUS:** COMPLETE — ready for PR (to be created by the repo owner).

**FILES CREATED:** `lib/player-context.tsx`, `lib/analytics.ts`,
`components/MiniPlayer.tsx`, `components/ConsentBanner.tsx`,
`components/TrackView.tsx`, `components/AppHomeClient.tsx`,
`docs/49-…` (this file)

**FILES MODIFIED:** `components/PlayerClient.tsx` (transport refactor),
`components/AppShell.tsx` (mini-player + banner + clearance class),
`app/app/layout.tsx` (provider), `app/app/page.tsx` (rewrite),
`components/SessionBuilderClient.tsx`, `components/ScheduleClient.tsx`,
`components/SearchClient.tsx` (analytics wires),
`app/app/categories/[slug]/page.tsx` (view island), `styles/site.css`
(mini-player, consent, form grid), `scripts/dev-api.mjs` (analytics +
consent acceptance)

**TESTS:** route crawl 79/79 · link crawl 145/145 · `tsc` · `next build` ·
fixture round-trips (consent, batch, session build).

**NEXT:** owner creates the PR; post-merge: staging deploy, device/browser
pass, conversion audit, production.
