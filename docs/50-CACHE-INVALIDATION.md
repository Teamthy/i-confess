# Ledger 50 — Cache invalidation across instances (closes G-10)

**Date:** 2026-09-25
**Branch:** `arena/01a0d8ae-i-confess`
**Depends on:** Ledger 45 (`apps/web`, unchanged here), PHASE 07 §7.1 (the caches
themselves), G-10 in `docs/03-TECHNOLOGY-DECISIONS.md` (the finding this closes).
**Scope:** backend only. No route, schema, contract or response-shape change.

---

## 1. What was wrong

The three read caches — `ListCategories` (5m/10m), `CategoryConfessions`
(2m/5m), `ListVoices` (5m/10m) — are per-process copies of rows that live in one
shared database. `cache.Cache` has had `Invalidate` and
`InvalidatePrefix` since PHASE 07. **Nothing called either of them.** There were
zero non-test call sites.

Two consequences, and the second was worse than the one on file:

1. **G-10, as recorded:** with two API instances, an admin edit on one stays
   invisible on the other for up to `ttl+swr` — fifteen minutes on the category
   and voice caches.
2. **Not recorded:** the instance that *handled* the write was stale too. The
   admin who publishes a confession, reloads the catalogue, and still sees the
   old list is looking at their own machine's cache. That is the failure a
   human notices first, and it exists at one replica.

## 2. What was built

**A key space with one definition.** `cacheKeyCategories`,
`cacheKeyCategoryConfessionsPrefix` and `cacheKeyVoices` now live in
`internal/api/cache_invalidation.go`, and `content_cache.go` reads and writes
through them. Before, the reader built `"catconf:" + id` inline while the
invalidator would have had to rebuild the same string; a test that publishes
through the constants and then asserts a fresh read is asserting the same
strings the read path uses.

**Invalidation on write, locally first.** Four admin writes now invalidate:
category create, confession create, confession status change (the publish and
unpublish path), and voice create. The local delete happens synchronously before
the response is written, so the writing instance is correct on its next request.
If a write path is ever added that does not call `h.invalidateCache`, the TTL
still covers it — this is an improvement on the previous behaviour, not a new
invariant that hides a missing call.

**`internal/cache.Bus` — a transport with two implementations.**

| Implementation | Used when | Delivers |
|---|---|---|
| `MemoryBus` | Single instance, and every test | Synchronously, in the publisher's goroutine, to each subscriber |
| `RedisBus` | `REDIS_ADDR` is set — which production requires | Redis pub/sub on `iconfess:cache:invalidate` |

`Message` carries a key (or a prefix), the origin instance, and a timestamp.
Nothing else — no values travel between instances, so a lost message degrades to
exactly the previous behaviour: the entry expires on its own.

`RedisBus` is a deliberately small RESP client (AUTH, SUBSCRIBE, PUBLISH) for
the same reason `internal/ratelimit`'s is: three commands against a stable wire
protocol are cheaper than a dependency, and what it does not do — cluster,
sentinel, resuming missed messages — is stated rather than implied. Pub/sub is
at-most-once by design, so the TTL is the backstop. A dropped subscription
reconnects on `internal/backoff` (250ms doubling to 30s), and `main.go` logs
"cross-instance cache invalidation enabled" only after Redis has confirmed the
subscription.

**A publish failure never fails the write.** The write has committed; logging
and carrying on leaves the other instances on their TTL, which is where they
already were. Failing the request would turn a staleness window into a
correctness error in the opposite direction.

**Observability.** `cache.Stats` gained `Invalidations`, exposed at
`GET /metrics` as `cache.invalidations` next to `hit_rate`. A hit rate alone
cannot distinguish "the catalogue never changes" from "the catalogue changed and
nothing noticed".

## 3. Two bugs found while building it

- **A throwaway `bufio.Reader` on the subscribe path.** `dialSubscription`
  read the SUBSCRIBE confirmation through a reader it then discarded, and the
  message loop built a new one. `bufio` reads ahead: a message that arrived in
  the same TCP segment as the confirmation would have been buffered in the
  discarded reader and silently lost, while the subscription looked healthy. The
  reader is now created once per connection and travels with it.
- **`Close` blocked on a live subscription.** The reader is parked in `Read`,
  and cancelling a context does not interrupt a blocked read, so `Close` waited
  for Redis to send something. On `SIGTERM` that is a hung shutdown. `Close` now
  closes the subscribed connection too, and the reconnect test asserts that
  `Close` returns while the server is deliberately idle.

Neither would have failed a test that only exercised the happy path, which is
why the reconnect test drops the connection on purpose.

## 4. Proving commands

| Claim | Command | Result |
|---|---|---|
| Builds, formatted, vet-clean | `gofmt -l .`, `go build ./...`, `go vet ./...` | clean |
| Whole suite, race detector, PostgreSQL 17 + Redis 7.4 | `TEST_DATABASE_URL=… REDIS_ADDR=127.0.0.1:6379 go test -race ./... -count=1` | all `ok`, zero FAIL |
| Cache package, including live Redis pub/sub | `go test ./internal/cache/ -count=1 -v` | 12/12 pass (5 skip without `REDIS_ADDR`, none silently) |
| Cross-instance behaviour at the HTTP layer | `go test ./internal/api/ -run 'Cache\|Invalidation' -count=1` | 5 tests pass |
| Reconnect after a dropped subscription | `TestRedisBusReconnectsAfterTheSubscriptionDrops` | passes against a stand-in server that closes the connection after one message |

**Live two-instance verification** — two copies of the real server binary over
one PostgreSQL, one Redis 7.4 built in this sandbox:

```
instance A :8091  REDIS_ADDR=127.0.0.1:6379   (bus enabled)
instance B :8092  REDIS_ADDR=127.0.0.1:6379   (bus enabled)

B GET /categories                     → 39
A POST /admin/categories              → 201
B GET /categories                     → 40, includes the new slug      ← G-10 closed
B POST /admin/categories              → 201
A GET /categories                     → includes the new slug          ← symmetric

instance C :8093  no REDIS_ADDR       (control)
C GET /categories                     → 41 (warmed)
A POST /admin/categories              → 201
C GET /categories                     → still 41, does not see it      ← control holds
C POST /admin/categories              → 201
C GET /categories                     → sees its own write at once     ← local fix, no bus needed

A GET /metrics → {"cache":{"hit_rate":0,"hits":0,"misses":1,"invalidations":3,...}}
C GET /metrics → {"cache":{"hit_rate":33.3,"hits":1,"misses":2,"invalidations":1,...}}
```

**Not run here:** `make lint`. The golangci-lint release CDN
(`release-assets.githubusercontent.com`) is unreachable from this sandbox, so
the binary could not be installed. `gofmt`, `go vet` (which shares most of the
`govet` findings), the full `-race` suite and manual review against the ten
enabled linters stand in; CI runs the real gate. This is a gap in the evidence,
not a claim that lint passes.

## 5. Findings

- **G-10 — closed.** The per-process cache now has an invalidation path, and
  production cannot start without `REDIS_ADDR`, which is the same setting the
  bus uses. The `docs/03` finding text is kept as written; it was accurate.
- **New — G-56: invalidation coverage is by call site, not by construction.**
  A new admin write that changes categories, confessions or voices will not
  invalidate until someone adds the call. A store-level hook would remove the
  possibility, and would also remove the ability to invalidate precisely — the
  prefix delete exists because a confession write does not cheaply name its
  category. Recorded rather than fixed: today's four writes are the complete set
  (`grep -rn "CreateCategory\|CreateConfession\|UpdateConfessionStatus\|CreateVoice"
  internal/ --include='*.go' | grep -v _test` lists only these plus `internal/seed`,
  which runs before any cache exists).
- **New — G-57: the audio-QA and voice-rights writes invalidate nothing.** They
  change whether a render or a voice may be used, and neither is held in the
  three cached projections (`models.Confession` carries no audio field,
  `ListVoices` carries no rights field), so no stale content is served from
  these caches today. The next cache added over audio metadata will need those
  two call sites, and nothing will remind anyone.
- **Observability row corrected, not changed.** `PROJECT-STATUS.md` claimed
  "No cache hit-rate metric". `/metrics` has exposed
  `cache.hit_rate` since PHASE 07; this ledger adds the missing
  `cache.invalidations` and a test that asserts both keys are present. The row
  was stale, and the table now says what a command proves.

## 6. Conditions

- **C-1:** `make lint` was not runnable in this sandbox (see §4). Owed to CI.
- **C-2:** `MemoryBus` is the default when `REDIS_ADDR` is unset, which is a
  single-instance deployment. There is no test that runs two *processes* against
  a memory bus, because a memory bus cannot cross a process boundary by
  construction; the live verification above used two processes over Redis, which
  is the configuration that matters.
- **C-3:** the live two-instance run reused one PostgreSQL server and the
  `postgres` database, seeded with the development demo catalogue. It proves
  the invalidation path, not a production-shaped deployment.

## 7. Decision

**PASS WITH CONDITIONS.** The gap that was closed was recorded in three
documents; two of them understated it. The fix is in the write path, the
transport is optional by configuration and impossible to miss in production,
and the behaviour is asserted at the HTTP layer over a real database and a real
Redis rather than by unit-testing a cache wrapper.
