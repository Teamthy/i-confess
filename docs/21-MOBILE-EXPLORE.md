# PHASE 21 — Mobile Explore

## OBJECTIVE

Give a listener who does not yet know what to say a place to find it: search,
the category grid, a featured rail, and a per-category confession list — with
the horizontal card emphasis motion the brief calls for, done so it cannot hang
a test or read as a platform default.

## INPUTS

- The brief: search, categories, featured, popular, recommended, personalized;
  "implement the premium horizontal category/card motion system".
- The public content API that exists: `GET /categories`, `GET /collections`,
  `GET /categories/{id}/confessions`.

## DEPENDENCIES

- PHASE 18 (shell/tokens), PHASE 20 (home established the rail vocabulary).
- `clients/dart`: new `Confession` and `Collection` models and
  `ContentRepository.categoryConfessions()` / `.collections()`.

## IMPLEMENTATION

`apps/mobile/lib/src/features/explore/` adds providers, `ExploreScreen` and
`CategoryDetailScreen`; the router wires `/explore`, `/explore/search` and
`/explore/category/:id`.

- **Search is client-side over the categories the server already sent.** There
  is no search endpoint, and fetching all 39 categories' confessions to fake one
  would be thirty-nine calls per keystroke. Full-text confession search is a
  named condition (server feature).
- **Featured** uses published `GET /collections`. **Popular / recommended /
  personalized** have no server signal (no play counts, no recommender); they
  are deferred as conditions rather than faked, per §5.
- **Motion system:** `_EmphasisRail` scales the card nearest the rail's centre.
  It is scroll-driven — a `NotificationListener` rebuilds on scroll updates and
  nothing schedules a frame on its own — so `pumpAndSettle` settles the moment
  scrolling stops. A repeating ticker here would have hung every navigation
  test that taps the Explore tab.

## TESTING

119 tests in `apps/mobile` (116 before), 43 in `clients/dart`.

- `explore_test.dart`: search field, featured rail and grid render from real
  data; typing filters the grid live and an empty result says so; a category
  opens its confessions.
- Defect injection: replacing the live filter with the unfiltered list failed
  the search test; restored, all pass.

## SECURITY REVIEW

Read-only public endpoints; no writes, no tokens, no untrusted input beyond the
local search string, which is never sent to the server.

## DOCUMENTATION

This document; `docs/PROJECT-STATUS.md`.

## EXIT CRITERIA

- Explore renders search, featured rail and category grid from the real API. ✔
- Category detail lists confessions with title, lead line and intensity. ✔
- The motion system settles under `pumpAndSettle`. ✔
- `make verify` passes. ✔

## VERDICT — PASS WITH CONDITIONS

- **C-1** Full-text confession search deferred to a server endpoint.
- **C-2** Popular / recommended / personalized deferred: no play-count or
  recommender signal exists; faking them would put server-owned judgement in
  the client.
- **C-3** Confession rows do not open the player; that is PHASE 22/24.
