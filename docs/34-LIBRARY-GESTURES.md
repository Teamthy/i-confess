# PHASE 34 — Library gestures and licence seeding (G-42, G-43, G-44, G-45)

**Status:** PASS
**Date:** 2026-09-21
**Depends on:** PHASE 27 (library re-audit), PHASE 31 (QA gate, G-42 context)
**Closes:** G-42, G-43, G-44, G-45

## OBJECTIVE

Four gaps, one theme: the library's machinery existed and nothing reachable
by hand drove it.

- **G-42** (from PHASE 31 C-3): the seeded demo catalogue failed the platform's
  own `voices_licensed` QA gate — no `voice_rights` row for the seeded voice —
  so a development database could never walk a confession through
  `audio_qa → approved` without hand-inserting a licence.
- **G-43**: collection reorder and "add to collection" had endpoints and typed
  client methods but **no gesture**; curating a collection was an API-only
  activity.
- **G-44**: `cover_url` was rendered by every collection row and written by
  nothing — the monogram was permanent.
- **G-45**: the favourites tab listed four entity kinds but only confessions
  navigated; a favourited session, category or voice was a dead row.

## IMPLEMENTATION

### Server

- `internal/seed/seed.go` — after creating Grace, the seed writes an active
  `voice_rights` row (holder `i-confess studio`, allowed use `tts`, GLOBAL).
  Grace is an in-house voice, so the honest licence is exactly that.
- `internal/store/library.go` — `UpdateCollection` takes `coverURL *string`;
  nil means "not mentioned", the empty string clears to NULL (G-44's whole
  complaint was conflating those).
- `internal/api/library.go` — POST and PATCH accept `cover_url`.
  `validCollectionCover` admits only `https://`, `http://` or a same-origin
  `/` path, max 2048 characters: no `javascript:`, no `data:`. The store is
  unchanged on read; the reader has always been able to show a cover.

### Typed client (`clients/dart`)

- `LibraryRepository.createCollection` / `updateCollection` carry `coverUrl`;
  on update, a non-null empty string is sent deliberately (clear), a null is
  omitted (untouched). `addToCollection` and `reorderCollection` already
  existed and were already typed — no new client surface, only plumbing.

### Mobile (`apps/mobile`)

- `library_providers.dart` — `LibraryActions` gains `addToCollection`,
  `reorderCollection`, `updateCover`, each invalidating the detail and list
  providers on success (the PHASE 27 lesson: a write that does not refresh is
  a broken write).
- `library_screen.dart` —
  - Collection items render in a `ReorderableListView.builder` with an
    explicit `ReorderableDragStartListener` grip per row: the row itself
    navigates, so whole-row dragging (the framework default) is switched off
    rather than made ambiguous. The reorder payload is the **complete order**,
    because the server rewrites every position from the array.
  - The collection menu gains "Set cover image" → `_CoverUrlDialog`; empty
    input means "remove the cover" and says so; the client mirrors the
    server's URL rule so an invalid link is refused inline rather than with a
    400 round trip.
  - `_FavoriteRow._openFavorite` switches on all four entity kinds:
    confession → detail, category → category detail, session → player,
    voice → the builder's voice step (there is no voice detail surface, and
    the voice step is where a voice is actually chosen and auditioned).
- `confession_detail_screen.dart` — `_AddToCollectionButton` on the
  confession page itself opens a modal sheet listing the listener's
  collections; choosing one POSTs the membership. Filing happens where the
  item is, not on the screen items are filed into. The sheet distinguishes
  "no session" (sign in to organise) from "no collections yet" (create one in
  the Library) instead of spinning or vanishing.

### Design

- No new routes, endpoints or screens: IA (`38 screens, 104 endpoints`),
  `design/routes.json` and `contracts/openapi.json` regenerate identical.

## TESTS

- Go: `TestSeedVoicesAreLicensed` (no ready seeded render fails the licence
  check; Grace has exactly one active `tts` licence) and
  `TestCollectionCoverUrlWriter` (POST persists, PATCH replaces, omission
  preserves, empty clears, four hostile values refused on both paths).
- Dart (CI runs `flutter test`; the sandbox has no SDK): drag-past test
  asserts the full new order reaches `PATCH /me/collections/{id}/reorder`;
  cover test asserts `PATCH … {"cover_url": …}` and the clear; the three
  favourite-kind tests assert navigation happens and a `missing` favourite
  still does not; the confession test asserts the sheet lists collections and
  the membership POST carries `confession_id`.
- `scripts/check_dart_symbols.py` extended with 14 assertions for the above
  (73 total), including that the repository literally sends
  `'cover_url': coverUrl`.

## VERIFICATION

- `go test ./...` green against PostgreSQL 17; the PHASE 33 audit test still
  green with the seed change (the seeded licence must not confuse
  gate-behaviour tests — it does not; they create their own voices).
- gofmt/vet clean; `python3 design/generate.py --check`, `test_design.py`,
  `test_ia.py`, `check_dart_symbols.py` all PASS.

## Verdict — PASS

`flutter analyze/test` are owed to CI (PHASE 23 C-1 unchanged); the condition
on PHASE 27 ("reorder + add-to-collection have endpoints but no gesture") is
retired here.
