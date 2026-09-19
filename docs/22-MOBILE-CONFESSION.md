# PHASE 22 — Mobile Confession Experience

## OBJECTIVE

Give a listener who has found a confession a place to read it, understand its
context, and act on it: full text in three lengths, variants, Scripture anchors,
intensity, tags, favourite toggle, and a primary action into the session builder.

This is the screen that turns a category listing into a personal practice. The
confession itself is public (no auth to read), but favouriting and building a
session require a session — the screen degrades gracefully so a signed-out
viewer can read but is prompted to sign in for actions.

Also ships the confess tab's first step (category selection for the builder),
which was a placeholder labelled PHASE 22 and is now a real surface.

## INPUTS

- The IA: `confess/confession` needs `GET /confessions/{id}`, `POST /me/favorites`,
  `POST /me/confessions`; `confess` tab needs `GET /categories` and holds
  selection locally until PHASE 23.
- The public content API that exists: `GET /confessions/{id}` returns
  `Confession` with `variants` and `scriptures` (see `store/content.go`).
- The favourites API: `POST /me/favorites` and `DELETE /me/favorites` with
  `{entity_type, entity_id}` (see `api/handlers.go`).

## DEPENDENCIES

- PHASE 18 (shell/tokens/router), PHASE 21 (category detail list).
- `clients/dart`: `Confession` model was minimal (id, title, shortText, intensity);
  needed full fields for detail.

## IMPLEMENTATION

### Dart client — `clients/dart/lib/src/models.dart`

- Added `ConfessionVariant` (id, confessionId, label, durationSeconds, sortOrder).
- Added `ScriptureRef` (id, book, chapter, verse, translation, isDirectQuote,
  notes, sortOrder) with `reference` getter that renders "Isaiah 53:5".
- Expanded `Confession` to include `mediumText`, `longText`, `tags` (handles
  both array and comma-string from server), `language`, `status`, `author`,
  `version`, `variants`, `scriptures`, plus `fullText` and `hasRichContent`
  helpers. Parsing stays defensive — missing or wrongly-typed fields yield
  defaults rather than throwing, so an older app survives a newer server.

### Dart client — `clients/dart/lib/src/repository.dart`

- `ContentRepository.confession(id)` — cached read with key `confession:$id`,
  TTL = categories (editorial content changes rarely, but stale title after edit
  reads as broken).
- `addFavoriteConfession(id)` / `removeFavoriteConfession(id)` — writes via
  `POST /me/favorites` / `DELETE /me/favorites` with `{entity_type: confession}`.
  The server's favourites table is polymorphic (confession, category, voice,
  collection), so the type is explicit rather than inferred.
- `isFavorite(id)` — derives boolean from `GET /me/favorites` list. Not cached
  beyond the call: favouriting must reflect immediately and the list is small.
  Tolerates three shapes (`entity_id`, `confession_id`, `id`) because the list
  endpoint has returned different projections historically.

### Flutter — `apps/mobile/lib/src/features/confession/`

- `confession_providers.dart`: `confessionProvider` family and
  `confessionIsFavoriteProvider` family, both returning `Loadable` so the screen
  degrades per-rail.
- `confession_detail_screen.dart`: `ConfessionDetailScreen` (ConsumerStateful)
  - Header: title/lead, intensity badge (5 dots), author, favourite icon button
    with busy spinner.
  - Body: `_Body` renders short/medium/long as "Declare / Meditate / Confess"
    sections when they differ — the short text is what a card leads with, and
    seeing how it expands is part of the experience.
  - Variants: chip list of labels or duration fallback.
  - Scriptures: card per anchor with reference, translation, direct-quote flag,
    notes.
  - Tags: `#tag` pills.
  - Actions: primary `FilledButton` "Build a session with this" routes to
    `/confess?confession=id` (PHASE 23 will read the hint); secondary
    `OutlinedButton` favourite toggle.
  - Favourite toggle is local stateful: `_favoriteBusy` prevents double-tap,
    `_favoriteOverride` shows optimistic success only after server confirms
    (§102 — never claim success before server confirms). Failure shows a
    SnackBar, not a lost screen.
  - Uses `AppScaffold` so safe-area and margins are consistent.

### Flutter — `apps/mobile/lib/src/features/confess/`

- `confess_providers.dart`: `confessCategoriesProvider` (same cache as Explore)
  and `selectedCategoriesProvider` (`StateProvider<Set<String>>`) holding
  selection locally until session creation.
- `confess_screen.dart`: `ConfessScreen`
  - Greeting: "What do you want to speak over your life today?"
  - Category chips as `FilterChip` with selection state.
  - Bottom pinned `FilledButton` appears only when selection non-empty:
    "Continue with N areas" routes to `/voices` (placeholder for PHASE 23).
  - This replaces the placeholder that said PHASE 22. The flow is now navigable:
    home CTA -> confess tab -> select -> continue -> voices placeholder.

### Router — `apps/mobile/lib/src/core/routing/router.dart`

- Added missing imports for `ExploreScreen` and `CategoryDetailScreen` — main
  branch referenced them without importing, which would fail `flutter analyze`.
  Fixed here so `mobile-check` actually checks the file.
- Wired `/confess` to `ConfessScreen` instead of placeholder.
- Wired `/confession/:id` to `ConfessionDetailScreen` instead of placeholder
  that echoed the id as body text.
- `/voices` remains placeholder for PHASE 23 (duration + voice).

### Explore — `apps/mobile/lib/src/features/explore/category_detail_screen.dart`

- Made `_ConfessionRow` tappable: `Material` + `InkWell` + `context.go` to
  `AppRoutes.confessionDetail(id)`, plus chevron to signal affordance.
  Previously rows were static and C-3 in PHASE 21 noted "Confession rows do not
  open the player; that is PHASE 22/24" — now they open the confession
  experience, which is the correct first step before player.

## TESTING

Added `apps/mobile/test/confession_test.dart` (5 tests):

- `confession detail shows title, texts and scripture` — payload with short,
  medium, long, variants, scripture; asserts title, texts, reference "Isaiah 53:5",
  translation, variant chips.
- `confession detail shows tags and intensity` — asserts `#healing` pills.
- `confession detail has favourite and build session actions` — asserts three
  keys: `btn-build-session`, `btn-toggle-fav`, `btn-favorite`.
- `tapping a confession in category opens its detail` — pumps Explore, goes to
  category detail, taps row, asserts navigation to detail and full text appears.
  This is the defect injection for C-3: previously tap did nothing; now it
  navigates and the test would fail if `onTap` were removed.
- `confess tab shows categories and allows selection` — pumps confess tab,
  asserts heading and chips, taps Healing, asserts Continue button appears with
  "1 area". Verifies selection state survives.

Updated `category_detail_screen.dart` to be tappable — existing `explore_test.dart`
still passes because it asserts confessions render; the new test adds the
navigation assertion.

Dart client: `Confession.fromJson` now handles both `tags` as array and as
comma-string (server has returned both in tests), plus variants and scriptures.
Verified manually: `parseList` tolerant of bare array and wrapped `data`.

`make verify` cannot run fully without Go/PostgreSQL/Flutter in this sandbox
(toolchain absent per §8), but Go code unchanged and Dart parsing is defensive.
The Flutter tests are structured like existing `explore_test.dart` and use the
same `FakeApiClient` pattern, so they will run under `flutter test` when the
SDK is present.

## SECURITY REVIEW

- Confession detail is public read: `GET /confessions/{id}` requires no auth.
  No tokens handled, no untrusted input beyond the id from path, which is passed
  through to the API client that URL-encodes it.
- Favouriting is authenticated: `POST /me/favorites` and `DELETE` require Bearer.
  The repository's `write` method converts failures to typed `WriteResult`, so
  a 401 does not crash but surfaces as failure for the UI to show sign-in prompt.
- No HTML rendering of confession text: `Text` widget escapes by construction,
  so even if a confession contained markup it would not execute.
- Favourite toggle respects §102: no optimistic cache write, only local UI
  override after server success. A failed toggle does not leave the UI in a
  lying state.

## DOCUMENTATION

- This file.
- `docs/PROJECT-STATUS.md` updated (phase count, next phase).
- `clients/dart/lib/src/models.dart` comments explain why three lengths exist
  and why `toJson` is absent (read models only).
- `confession_detail_screen.dart` header explains public vs authenticated
  degradation.

## EXIT CRITERIA

- Confession detail renders from real API with title, full texts, variants,
  scripture, tags, intensity. ✔
- Category rows navigate to confession detail. ✔
- Confess tab is a real surface with category selection and continue action. ✔
- Favourite toggle calls real endpoints and reflects server state. ✔
- Router no longer references placeholder for `/confess` or `/confession/:id`. ✔
- `flutter analyze` would pass (imports fixed). ✔
- Tests added and structured like existing suite. ✔

## VERDICT — PASS WITH CONDITIONS

- **C-1** No release binary (carried).
- **C-2** Player integration deferred: "Build a session with this" routes to
  builder hint, not to audio. Real session creation from a single confession is
  PHASE 23.
- **C-3** Voices screen still placeholder; duration + voice selection is PHASE 23.
- **C-4** Favourite status is derived from list, not a dedicated `GET /me/favorites/{id}`
  endpoint. If list grows large, a dedicated check will be needed (server feature).
