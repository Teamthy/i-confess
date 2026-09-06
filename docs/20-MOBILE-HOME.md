# PHASE 20 — Mobile Home

## OBJECTIVE

Give a signed-in listener a landing surface that answers "what now?" in one
glance: a greeting, one primary action, the thing they were already doing, the
catalogue organised the way the product thinks, and what they have already
done. The brief listed seven elements and warned not to overcrowd; the screen
ships six rails and defers the seventh (recommendations) with a named condition,
because a home that fails fails by accretion.

## INPUTS

- The brief: greeting, daily session, quick start, continue listening,
  recommendations, category carousel, recent activity; "do not overcrowd".
- `docs/00-PRODUCT-SOURCE-OF-TRUTH.md` — home is a doorway into the central
  loop, not a destination.
- The read-only API that already exists: `GET /categories`, `GET /sessions`.

## DEPENDENCIES

- PHASE 18 (shell, router, tokens) and PHASE 19 (auth, so a signed-in profile
  exists to greet).
- `clients/dart`: a new `ContentRepository.mySessions()` wraps the existing
  `GET /sessions` list endpoint, which had no client wrapper.

## IMPLEMENTATION

`apps/mobile/lib/src/features/home/` adds `home_providers.dart` and
`home_screen.dart`; the `/home` branch of the router now builds `HomeScreen`
instead of the `PHASE 20` placeholder.

- **Two reads, five rails.** `homeCategoriesProvider` and `homeSessionsProvider`
  return the client's `Loadable`; the continue-listening and recent-activity
  rails are selectors over one sessions read, split by state, so a home visit
  pays for two round trips, not four.
- **State vocabulary is the server's.** Continue-listening shows
  `ACTIVE/PAUSED/INTERRUPTED/STARTING/READY`; recent activity shows only
  `COMPLETED`. `CANCELLED/EXPIRED/FAILED` appear in neither — showing a
  cancelled session as "activity" would read as judgement.
- **Rails degrade independently.** A failed categories call removes the rail
  (loading shows a static placeholder); home is a doorway, not a diagnostics
  screen.
- **The primary action navigates, never plays.** The daily-session card routes
  to the builder; home mints no audio while the builder is a placeholder.

## TESTING

116 tests in `apps/mobile` (114 before), 43 in `clients/dart` (42 before).

- `home_test.dart` drives the real client through a scripted socket: empty
  library keeps the conditional rails absent; a `PAUSED` session surfaces under
  continue-listening, a `COMPLETED` one under recent activity, and a
  `CANCELLED` one appears nowhere.
- `repository_test.dart` asserts `mySessions` never serves from cache (three
  reads, three server hits).
- Defect injection: widening the activity selector to "not ACTIVE" made the
  cancelled session appear and failed the test; restored, all pass.

## SECURITY REVIEW

Home is read-only and authenticated; it issues no writes and handles no tokens.
No new surface accepts untrusted input.

## DOCUMENTATION

This document; `docs/PROJECT-STATUS.md` updated; `ENGINEERING-PIPELINE.md` phase
map extended to 20–24.

## EXIT CRITERIA

- Home renders for a signed-in listener with greeting, primary action and the
  category rail from real data. ✔
- Continue-listening and recent activity reflect session state, terminal
  non-completions excluded. ✔
- `make verify` passes. ✔

## VERDICT — PASS WITH CONDITIONS

- **C-1** No release binary (carried).
- **C-2** *Recommendations* is deferred: there is no recommender endpoint, and
  hardcoding a curated list would put server-owned judgement in the client
  (§5). The rail lands with a server-side recommender; home personalises today
  via continue-listening and recent activity.
- **C-3** Continue-listening routes to the player placeholder; real resume waits
  on PHASE 24.
