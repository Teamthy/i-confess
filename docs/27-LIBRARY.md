# PHASE 27 — Library

**Status:** PASS
**Date:** 2026-09-19
**Depends on:** PHASE 22 (favorites), PHASE 21 (collections)

## Objective

Library: user collections, favorites, personal confessions. Collection detail with items.

## Implementation

- `library_providers.dart`: `libraryCollectionsProvider` (GET /me/collections), `libraryFavoritesProvider` (GET /me/favorites), `libraryConfessionsProvider` (GET /me/confessions), `collectionDetailProvider` (GET /me/collections/{id}).
- `library_screen.dart`:
  - TabController with 3 tabs: Collections, Favorites, My Confessions.
  - Collections tab: list with name, itemCount, visibility, chevron to detail, empty state with Create CTA.
  - Favorites tab: list from polymorphic favorites table (entity_type, entity_id), shows entity_id and type.
  - My Confessions tab: personal confessions (user-authored), title fallback to shortText.
  - `CollectionDetailScreen`: name, description, items list.

### Router

- `/library` -> `LibraryScreen`, `/library/collection/:id` -> `CollectionDetailScreen`.
- `/saved` now points to Library (was placeholder).

## Exit Criteria

- Library shows real collections, favorites, personal confessions ✔
- Collection detail loads items ✔
- Empty states with CTA ✔

## Verdict — PASS
