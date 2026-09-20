# PHASE 27 — LIBRARY & COLLECTIONS

**Status:** PASS WITH CONDITIONS
**Date:** 2026-09-20
**Depends on:** PHASE 22 (favourites), PHASE 25 (Me / entry points), PHASE 31 (UGC submission)
**Supersedes:** the PHASE 27 entry dated 2026-09-19, which recorded PASS for a
surface that did not work. See *Reconciliation* below.

## OBJECTIVE

Deliver the library — the answer to "what is mine?" — as three tabs over one
screen: collections the listener built, confessions they favourited, and
confessions they wrote. Plus the collection detail behind them.

## RECONCILIATION — WHY THIS PHASE WAS REOPENED

`docs/PROJECT-STATUS.md` listed PHASE 27 as **PASS** with the exit criteria
"library shows real collections, favorites, personal confessions ✔". An audit
against a live PostgreSQL 17 and the real route table found the surface was a
212-line shim whose three tabs were, in order, wrong, unrenderable and empty.
Every defect below was reproduced by a test before it was fixed.

The rule this project follows — *nothing is listed as done unless a command
proves it* — was not applied to PHASE 27. It is applied here.

### D-1 — Favouriting the same thing three times wrote three rows

`AddFavorite` ended with `ON CONFLICT(id) DO NOTHING`, and `id` was generated
fresh on every call. The conflict target could therefore never be hit, so the
clause was decoration: the insert always succeeded. The `favorites` table has
carried no uniqueness constraint since the baseline, so nothing else caught it.

Reproduced before the fix:

```
favorites rows after 3 identical adds: 3
DEFECT: 3 rows for the same favourite
```

This was not cosmetic. Unfavouriting deletes by `(user, type, entity)` and
removed all three at once, so the count shown and the count deleted disagreed;
and `engine.FavoriteIDs` reads favourites as a preference signal, where a
triplicated entity silently weights that confession more heavily than the
listener ever asked for.

Fixed in migration `0013_favorites_unique.sql`, which collapses existing
duplicates — keeping the **oldest** of each group, because a favourite dates
from when it was first expressed, not from the last accidental re-tap — and
then adds `favorites_one_per_entity (user_id, entity_type, entity_id)`. The
store now conflicts on that natural key and `RETURNING`s the stored row, so a
repeat favourite returns the real id and the original `created_at` rather than
the ones it built and discarded.

### D-2 — The favourites tab could only render opaque ids

`favorites` is polymorphic by design: `(entity_type, entity_id)` and nothing
else. That is the right shape for writing and useless for rendering, and the
tab printed `f['entity_id']` — a UUID — as the row title.

`ListFavoritesDetailed` now resolves titles at read time, one query per entity
kind rather than one per favourite, and never denormalises: storing a title
would leave the library showing the old one after an editor renames a
confession. A confession also carries its category as a subtitle, which is what
makes a column of favourites readable.

A favourite whose target no longer resolves comes back with `missing: true`
rather than being dropped. The row is real, the user can see it in their data
export, and they need a way to clear it.

### D-3 — The "My Confessions" tab decoded the wrong model

`GET /me/confessions` returns `UserConfession` — `text`, `status`,
`visibility`. The client decoded it with `Confession.fromJson`, the editorial
model, which reads `short_text` and `description`. None of the keys line up, so
every row rendered blank with a blank subtitle. The tab had never displayed
anything.

A real `UserConfession` model now exists on the client, with the moderation
vocabulary the server actually persists.

### D-4 — `isFavorite` could light the heart on the wrong confession

The match was `entity_id == id || confession_id == id || item['id'] == id`.
That third clause compares the favourite **row's own id** against a confession
id. Both are UUIDs from the same generator, so it is a false positive waiting
for a collision, and it is checked on the confession screen where the
consequence is a heart shown filled on content the user never saved. Now
matched on `(entity_type, entity_id)` only, with the request narrowed
server-side via `?type=confession`.

### D-5 — Writes never invalidated the reads they invalidated

`LibraryRepository.collections()` is cached for 30 minutes. `createCollection`
wrote through `write()`, which does not touch the cache. A user created a
collection, the server accepted it, and the list kept serving the stale copy —
the single most common way a CRUD surface feels broken while being technically
correct. The same applied to the heart on the confession screen and the
favourites tab it feeds.

Every library mutation now drops the affected keys, **on success only**: a
failed write changed nothing, and discarding a good cache because the network
refused would make the offline case worse. `JsonCache` gained the `delete` it
never had.

### D-6 — The screen would have thrown on layout

`AppScaffold` defaults to `scrollable: true`, which wraps the body in a
`SingleChildScrollView`. The shim put a `TabBarView` inside an `Expanded`
inside that, giving the view unbounded height. Fixed with `scrollable: false`,
matching how the activity screen — the other tabbed surface — does it.

### D-7 — `GET /me/collections/{id}` and the detail route were undescribed

The app has shipped `/library/collection/:id` since the original PHASE 27, but
`design/ia.json` had no screen for it, so `test_ia.py` — which exists precisely
to catch a screen pointing at an endpoint the backend does not serve — was
never checking its four endpoints. Added as `library/collection`, distinct from
the editorial, read-only `explore/collection`. The IA is now 38 screens.

### D-8 — An unknown `?type=` answered 200 with an empty list

Reads as "you have no favourites" rather than "that is not a thing you can
favourite". Now a 400 against the same four-value vocabulary the write path
validates.

## IMPLEMENTATION

### Server

- `internal/db/migrations/0013_favorites_unique.sql` — de-duplicate, then
  constrain.
- `internal/store/engagement.go` — idempotent `AddFavorite`;
  `ListFavoritesDetailed` resolving confessions, categories, voices and
  sessions.
- `internal/models/models.go` — `Favorite` gains `Title`, `Subtitle`,
  `Missing`, all `omitempty` so the bare rows written by `AddFavorite`
  serialise unchanged.
- `internal/api/handlers.go` — `listFavorites` hydrates and validates `type`.

### Typed client (`clients/dart`)

- `models.dart` — `UserConfession` and `Favorite` added; `UserCollection`
  gains `coverUrl`, `items`, `updatedAt`; `CollectionItem` added with
  `resolved`.
- `repository.dart` — the full collection CRUD, `favorites({type})`,
  polymorphic `removeFavorite`, `myConfessions()`, `submitConfession()`, and
  cache invalidation on every write.
- `endpoints.dart` — `postMeConfessionsByIdSubmit` (a user-facing route that
  was missing from the typed client entirely) and a `type` filter on
  `getMeFavorites`.
- `cache.dart` — `JsonCache.delete`; `CacheKeys.favorites`,
  `CacheKeys.myConfessions`.

### Mobile (`apps/mobile`)

- `library_providers.dart` — three auto-disposing providers reading through
  the repository, so the library inherits its offline behaviour instead of
  calling the API client raw; `LibraryActions` for the writes.
- `library_screen.dart` — the three tabs, each with the four states a data
  surface owes (§58, §100) via a shared `_TabBody`; create/rename/delete,
  unfavourite, submit-for-review, and an offline notice (§102). Collection
  detail with items, removal, rename and a confirmed delete that states the
  confessions survive.

## VERIFICATION

Run against PostgreSQL 17.10 and Go 1.27.1 in this sandbox.

| Check | Command | Result |
|---|---|---|
| Full suite | `go test -count=1 ./...` | **32 packages ok, 0 failures** |
| Race | `go test -race ./internal/api ./internal/store ./internal/models` | **ok** |
| Format | `gofmt -l server` | **0 files** |
| Vet | `go vet ./...` | **0 findings** |
| Design tokens | `python3 design/generate.py --check` | 120 tokens, up to date |
| Design rules | `python3 design/test_design.py` | **PASS** |
| IA | `python3 design/test_ia.py` | **PASS**, 38 screens, 103 endpoints |
| Spec | `go run ./cmd/genspec ../contracts/openapi.json` | no diff |
| Route table | `EXPORT_ROUTES=1 … TestExportRouteTable` | no diff |
| Dart symbols | `python3 scripts/check_dart_symbols.py` | **49/49** |

New tests: 7 in `internal/store/engagement_test.go`, 9 in
`internal/api/favorites_test.go`, 20 across three new groups in
`clients/dart/test/repository_test.dart`, and 22 widget tests in
`apps/mobile/test/library_test.dart`.

Fault injection — each of these was introduced, observed to fail, and reverted:

- a model field the screen reads deleted → caught by the symbol check
- `postMeConfessionsByIdSubmit` removed → caught, twice over
- a widget key renamed in the test → caught
- the duplicate-favourites defect, before the fix → `3 rows, want 1`

## CONDITIONS

- **C-1 (inherited, PHASE 23).** The Dart and Flutter SDKs are unreachable from
  this sandbox — `pub.dev` and the Dart archive both fail to connect — so
  `flutter analyze`, `flutter test` and `dart test` did not run here and are
  owed to CI, which does run all four. To stop that being a blind spot,
  `scripts/check_dart_symbols.py` statically verifies every model field,
  endpoint, provider, route, design token, `AppSurfaces` field and widget key
  the new code and its tests reference. It is wired into `make mobile-check`
  and the CI `Dart symbols` step, and `mobile-check` no longer prints
  "SKIPPED" when Flutter is absent, because a check that silently skips is how
  fifteen test files once went unexecuted. It is not a substitute for the
  analyzer; both run.

- **C-2.** `PATCH /me/collections/{id}/reorder` and
  `POST /me/collections/{id}/items` are implemented, typed and reachable from
  the repository, but the library has no drag-to-reorder gesture and no
  "add to collection" affordance — adding is done from a confession's own
  page. Reordering a collection is therefore currently only possible through
  the API. Filed as **G-43**.

- **C-3.** `cover_url` is read, rendered and falls back to a monogram, but
  nothing in the product sets it: there is no upload path for collection
  artwork. Every collection will show the monogram until one exists. Filed as
  **G-44**.

## GAPS RAISED

- **G-43** — collection reordering and in-app "add to collection" have
  endpoints but no gesture.
- **G-44** — `cover_url` has no writer; collection artwork cannot be set.
- **G-45** — the favourites tab lists every entity type, but only confessions
  navigate anywhere. A favourited voice or session renders and can be removed,
  and does nothing when tapped, because no detail surface for them exists yet.

## VERDICT — PASS WITH CONDITIONS

The three tabs read real data through the repository, degrade independently,
state when they are serving cache, and every write refreshes what it changed.
Eight defects that made the previous PASS untrue are fixed, each with a test
that fails without the fix. The conditions are scope (C-2, C-3) and the
sandbox's missing Dart toolchain (C-1), not correctness of what shipped.
