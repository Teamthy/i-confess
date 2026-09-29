# 63 — Mobile safety actions: block and appeal (G-53)

**Status: implemented; Go/design gates passed, mobile SDK checks owed to CI.**

The mobile client now exposes listener-owned blocking and moderation appeals in
context, without undoing the community feed's anonymity boundary.

## User-facing changes

- **Settings → Blocked accounts & appeals** opens a Safety screen with separate
  Blocked accounts and Appeals tabs. List states support retry/refresh; block
  entries can be unblocked after confirmation; appeal rows show their decision
  type, status, statement, and moderator response when present.
- A listener can submit an appeal from the Safety screen. Rejected entries in
  **My Confessions** also have an in-context **Appeal this decision** action,
  which sends the confession decision ID and the listener's statement.
- Community story cards expose **Block author** in their options menu. The
  listener confirms first; on success the feed refreshes and the blocked
  author's stories disappear from that listener's view.

## Anonymity and storage boundary

A community-feed post intentionally does not return `author_id`. Rather than
leaking account IDs to enable blocking, the authenticated client sends only the
post ID to `POST /community/posts/{id}/block-author`. The API resolves its
owner through `internal/community.Store.PostAuthor`, scoped to public,
approved/published feed posts, then records the existing reversible boundary
through `internal/store.BlockStore`. The endpoint response contains only
success/idempotency state. This keeps the feed projection anonymous and keeps
community-post lookups in the separate `internal/community` store, not in the
moderation/block store.

`internal/community.Store.FeedFor` applies the signed-in viewer's block list in
SQL; anonymous readers remain unfiltered. The existing reaction path now uses
the community store's `author_id` lookup (correcting the old `user_id` column
lookup) before checking blocks in both directions. A listener cannot block
themselves through their own post.

## Verification

- PostgreSQL-backed API/race test covers anonymous feed output, unauthenticated
  and self-block rejection, idempotent contextual blocking, per-viewer feed
  filtering, private block-list visibility, and rejection of a reaction across
  a block boundary.
- Full Go race suite passed: `go test -modfile=/tmp/local.mod -race -count=1 ./...`
  with PostgreSQL and Redis configured.
- `go build -modfile=/tmp/local.mod ./...` and
  `go vet -modfile=/tmp/local.mod ./...` passed.
- `design/test_ia.py` passed (40 screens, 8 entry points, 163 endpoints);
  `scripts/check_dart_symbols.py` passed 137/137; design generation and design
  checks passed.
- Flutter/Dart SDKs are unavailable in this sandbox. The new Safety widget tests,
  Flutter analysis, and Dart repository tests are therefore CI gates, not claimed
  as locally passed.

## Remaining gate

G-53 is implemented. G-52 remains absent from this checkout and unverified, so
the full handoff gate is still not satisfied and no pull request should be
opened until G-52 is restored and both requested suites pass.
