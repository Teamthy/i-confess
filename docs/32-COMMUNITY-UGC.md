# PHASE 32 — COMMUNITY UGC READER (G-40)

**Status:** PASS
**Date:** 2026-09-20
**Depends on:** PHASE 31 (moderation pipeline), PHASE 27 (library), community feed
**Closes:** G-40

## OBJECTIVE

Close G-40: approved/published UGC has no reader. The moderation pipeline in PHASE 31 now ends at `published` for public-intent user confessions, but no public surface served them. This phase adds a real cross-surface reader while preserving anonymity and moderation boundaries.

Per `docs/NEXT-PHASE-COMMUNITY.md`:
- Backend returns only moderated public posts and never author identity; shared/private/unreviewed excluded by PostgreSQL-backed tests.
- Typed Dart client exposes feed and authenticated idempotent reactions (already done) plus the new public confessions feed.
- Mobile Explore links to a refreshable feed with loading, empty, retry, reaction-progress and reaction-error states, now with two tabs.
- Web serves a live API-backed community page with loading, empty and retry states and anonymous output, now with both stories and testimonies.

## INPUTS

- `docs/PROJECT-STATUS.md` — G-40 definition: "The pipeline publishes (visibility=public + approval → published), but no public surface serves published UGC yet".
- `docs/31-MODERATION.md` — approval honours author's asked audience: public → published, private/shared → approved. Only published public should be public.
- `server/internal/store/engagement.go` — existing `ListUserConfessions` (own only) and `CreateUserConfession`.
- `server/internal/api/handlers_community.go` — existing `feedCommunity` for `community_posts` (public, anonymous, approved/published only).
- `server/internal/community/store.go` — `Feed` already excludes shared posts from public endpoint, tested by `TestPublicFeedExcludesApprovedSharedPosts`.
- `design/ia.json` — community screen already declared but only wired to `/community/feed`.

## DEPENDENCIES

- Go 1.27.1 via go-bin wheel, PostgreSQL 17.10 via embedded-postgres npm tarball (same as PHASE 31).
- Migration chain unchanged; no new migration needed — `user_confessions` already has `visibility`, `status`, `published_at`.
- Dart/Flutter SDK unreachable in sandbox, so `flutter analyze`/`flutter test` owed to CI; mitigated by `scripts/check_dart_symbols.py` (now 59 assertions).

## IMPLEMENTATION

### Server

- `internal/store/engagement.go` — new `ListPublishedUserConfessions(ctx, limit)`:
  ```sql
  SELECT ... FROM user_confessions
  WHERE visibility='public' AND status='published'
  ORDER BY COALESCE(published_at, created_at) DESC LIMIT ?
  ```
  Limit clamped 1..100, default 20. No post-filtering — exclusion is in SQL.

- `internal/api/handlers_community.go` — new `feedUserConfessions` handler:
  - Public, no auth required.
  - Parses optional `?limit=1..100`.
  - Calls `ListPublishedUserConfessions`.
  - Projects to anonymous shape: `id, title, text, category_id, visibility, status, published_at, created_at` — deliberately omits `user_id`, `reviewed_by`, `review_notes`, `rejection_reason`.
  - Returns `{"confessions": [...]}`.

- `internal/api/router.go` — two new routes:
  - `GET /community/confessions` (public)
  - `GET /v1/community/confessions` (public)
  - Both wired with same handler, so parity test passes.

- `internal/store/engagement_test.go` — `TestListPublishedUserConfessions`:
  - Inserts public+published, private+published, shared+published, public+draft, public+submitted, public+approved.
  - Asserts only 1 row returned, correct id, correct visibility/status.
  - Inserts second newer published confession, asserts ordering newest first.

- `internal/api/community_feed_test.go` (new) — `TestPublicUGCFeed` over HTTP:
  - Seeds same six cases via SQL.
  - `GET /community/confessions` without auth → 200, 1 confession, no `user_id`/`reviewed_by` leaked.
  - `GET /v1/community/confessions` alias → 200.
  - Inserts second newer, asserts ordering.

### Typed client (`clients/dart`)

- `endpoints.dart` — `getCommunityConfessions({int? limit})` building `?limit=` query.
- `repository.dart` — new `CommunityRepository`:
  - `feed()` → existing community posts feed.
  - `confessions({limit})` → `getCommunityConfessions` decoded as `List<UserConfession>`.
  - `react(postId, reaction)` → existing reaction endpoint.
  - This repository is the single place mobile should read community, rather than calling `apiClient` raw.

### Mobile (`apps/mobile`)

- `community/community_screen.dart`:
  - Previously one `FutureProvider` and one list. Now two providers: `communityFeedProvider` (posts) and `communityConfessionsProvider` (published UGC).
  - `CommunityScreen` now `DefaultTabController(length:2)` with `AppScaffold(scrollable:false)`:
    - TabBar: Stories | Testimonies.
    - TabBarView: `_StoriesTab` and `_TestimoniesTab`, each with `RefreshIndicator`, loading, error with retry, empty states.
  - `_StoriesTab` unchanged behaviour (cloud_off, forum icons, reaction chips with busy/error states).
  - `_TestimoniesTab` new: shows title (if present, semibold), text, published date (split T), empty state "No published testimonies yet. Be the first to share." with auto_stories icon.

### Web (`apps/web`)

- `app/community/page.tsx`:
  - Previously fetched only `/v1/community/feed`. Now fetches both `/v1/community/feed` and `/v1/community/confessions` in parallel via `Promise.all`.
  - State: `posts` and `confessions` separate.
  - Loading/error/empty cover both.
  - Renders two sections when non-empty: Testimonies (title, text, published date) and Stories (body, created_at).
  - Copy updated: "Published user confessions (G-40) are now readable anonymously."

### Design

- `design/ia.json` — community screen endpoints extended from 3 to 4, adding `GET /community/confessions`. Surgical edit, no round-trip.
- `design/routes.json` — regenerated from live route table: 300 routes (was 298), purely additive.
- `contracts/openapi.json` — regenerated via `go run ./cmd/genspec`.

## TESTING

Run against PostgreSQL 17.10 and Go 1.27.1 in this sandbox.

| Check | Command | Result |
|---|---|---|
| Store UGC feed | `go test -modfile=/tmp/local.mod -count=1 ./internal/store -run TestListPublishedUserConfessions -v` | **PASS** |
| API UGC feed | `go test -modfile=/tmp/local.mod -count=1 ./internal/api -run TestPublicUGCFeed -v` | **PASS** |
| Community package | `go test -modfile=/tmp/local.mod -count=1 ./internal/community -v` | **PASS** (5 tests, includes shared-exclusion) |
| Full api+store+community | `go test -modfile=/tmp/local.mod -count=1 ./internal/api ./internal/store ./internal/community` | **PASS** |
| Vet | `go vet -modfile=/tmp/local.mod ./...` | **0 findings** |
| Build | `go build -modfile=/tmp/local.mod ./...` | **ok** |
| Design tokens | `python3 design/generate.py --check` | 120 tokens, up to date |
| Design rules | `python3 design/test_design.py` | **PASS** |
| IA | `python3 design/test_ia.py` | **PASS**, 38 screens, 104 endpoints wired |
| Dart symbols | `python3 scripts/check_dart_symbols.py` | **59/59** (was 51, +8 for G-40) |

Fault injection — each introduced, observed to fail, reverted:

- Changed store query to `visibility IN ('public','shared')` → `TestListPublishedUserConfessions` FAIL (2 rows, want 1) and `TestPublicFeedExcludesApprovedSharedPosts` still PASS for community_posts but new UGC test catches shared leak.
- Removed projection stripping `user_id` → API test FAIL ("leaked user_id").
- Changed `status='published'` to `status IN ('approved','published')` → UGC feed test FAIL (approved public appears).

## SECURITY REVIEW

- **Anonymity.** Public UGC projection omits `user_id`, `reviewed_by`, `review_notes`, `rejection_reason`. Tested explicitly in `TestPublicUGCFeed` — presence of `user_id` or `reviewed_by` fails the test. Same pattern as `community.FeedPost` which omits `author_id`.
- **Moderation boundary.** Only `visibility='public'` AND `status='published'` rows are returned. Shared/private are excluded by SQL, not by UI filter. Draft/submitted/approved are excluded. Tests insert each disallowed combination and assert 0 or 1 row accordingly.
- **No auth bypass.** Endpoint is public by design (anonymous reader), same as `GET /community/feed`. No privileged data returned, so 401 is not required. `POST /community/posts` and reaction remain authenticated.
- **Rate limiting.** Public feed is read-only, no per-account throttle needed; write path already throttled in PHASE 31.
- **No new secrets.** No env vars, no storage keys.

## DOCUMENTATION

- This document: `docs/32-COMMUNITY-UGC.md`.
- `docs/NEXT-PHASE-COMMUNITY.md` — acceptance criteria now satisfied; can be retired or kept as reference.
- `docs/PROJECT-STATUS.md` — to be updated: PHASE 32 PASS, G-40 closed.
- `design/ia.json` — surgical edit, community endpoints now 4.
- `design/routes.json` and `contracts/openapi.json` — regenerated from live route table, never hand-edited.
- `scripts/check_dart_symbols.py` — 59 assertions, +8 for community.

## EXIT CRITERIA

| Criterion | Evidence |
|---|---|
| Published public UGC has a public reader, anonymous, ordered newest first | `TestListPublishedUserConfessions` and `TestPublicUGCFeed` |
| Shared/private or non-published UGC excluded | Same tests, 5 disallowed cases inserted |
| Author identity never leaked | API test checks absence of `user_id`/`reviewed_by` |
| Mobile shows refreshable feed with loading/empty/retry and two tabs | `community_screen.dart` with TabBar Stories/Testimonies, RefreshIndicator, error retry |
| Web shows live API-backed page with loading/empty/retry and anonymous output | `app/community/page.tsx` fetches both feeds, anonymous rendering |
| Route table and OpenAPI regenerated | `design/routes.json` 300 routes, `contracts/openapi.json` regenerated |
| Dart symbols gating passes | `python3 scripts/check_dart_symbols.py` 59/59 |
| No format/vet/build regressions | `gofmt -l` 0, `go vet` 0, `go build` ok |

## VERDICT — PASS

G-40 closed: the pipeline now ends at a reader. A published public confession appears in `GET /community/confessions` and in both mobile and web community surfaces, without ever exposing who wrote it. The existing community_posts feed continues to exclude shared posts, verified by its own PostgreSQL-backed test.
