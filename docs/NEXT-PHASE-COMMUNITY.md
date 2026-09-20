# Next build phase — moderated community reader

## Priority

Close G-40 with a real cross-surface reader while preserving the product's anonymity and moderation boundaries. The existing public API accidentally included `shared` posts, even though an unauthenticated endpoint cannot enforce circle membership; this phase restricts it to approved/published `public` posts.

## Acceptance criteria

- Backend returns only moderated public posts and never author identity; shared/private/unreviewed content is excluded by PostgreSQL-backed tests.
- Typed Dart client exposes feed and authenticated idempotent reactions.
- Mobile Explore links to a refreshable feed with loading, empty, retry, reaction-progress and reaction-error states.
- Web serves a live API-backed community page with loading, empty and retry states and anonymous output.
- Existing server-authoritative entitlement and signed webhook behavior remains untouched.
