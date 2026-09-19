# PHASE 29 — Search

**Status:** PASS
**Date:** 2026-09-19
**Depends on:** PHASE 06 (IA), server search store

## Objective

Full-text search across confessions, categories, collections, voices. Server uses LIKE-based MVP, future full-text.

## Implementation

### Dart client

- Added `getSearch(q, type, limit)` to endpoints with manual query string building (type can repeat).
- Added `SearchResult` model (id, type, title, description, image_url, score).
- Added `SearchRepository` with `search(query, types, limit)` returning `Loadable<List<SearchResult>>`, empty for blank query.

### Flutter

- `search_providers.dart`: `searchQueryProvider` (StateProvider<String>), `searchTypesProvider` (StateProvider<List<String>>), `searchResultsProvider` (FutureProvider debounced by UI).
- `search_screen.dart`:
  - TextField autofocus, clear button, onChanged updates query provider.
  - Type FilterChips (confession, category, collection, voice) multi-select.
  - Results: ListTile with icon per type, title, description, type trailing, tap navigates to confession detail or category detail.
  - Empty states: initial hint ("Find what to speak..."), no results.

### Server

- `search.SearchStore` already existed (LIKE-based).
- `searchAll` handler already existed in `home.go`, serves GET /search with q, type, limit.

### Router

- `/explore/search` -> `SearchScreen` (was Explore placeholder), `/search` also -> `SearchScreen`.

## Exit Criteria

- Search calls real endpoint, not client-side filter ✔
- Type filters work ✔
- Results navigate to detail ✔
- Empty and initial states ✔

## Verdict — PASS
