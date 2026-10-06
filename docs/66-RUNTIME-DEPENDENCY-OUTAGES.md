# PHASE 66 — Runtime dependency outages (G-7)

## Finding

G-7 asked what happens when Redis, object storage, or the voice provider becomes
unavailable after startup while requests are being served. Inspection found that
several of the lower-level behaviors already had tests: the Redis limiter's
local fallback and recovery, Redis pub/sub reconnection, S3 operation failure
classification, and retryable voice-provider failures. The material missing
coverage was at the API/request boundary: an invalidation publish failure during
a committed write, and object-storage URL-signing failure during session
creation.

The signing paths are not interchangeable: `audio.URLGenerator` fell back to
local URL signing when its storage presigner errored, while `PlaybackResolver`
returns a signing error. Both were initialized at startup, but neither was
called by an API route. The live session response path instead signs through the
handler's `ObjectStorage` dependency; its outage behavior is exercised directly
below. This phase avoids claiming API-level coverage for the two unwired paths
or conflating a locally signed URL with proof that an object can currently be
fetched.

> Update (VE-008): `audio.URLGenerator` has since been deleted. Nothing read the
> handler field it was installed into, so its fallback was unreachable — the
> presigner it fell back *from* is the only signing path that ever ran.

## Changes

- Added `TestRuntimeRedisPublishOutageKeepsRequestsServing`. The API's Redis bus
  subscription is healthy at setup, then invalidation publishing fails at
  request time. The admin write still returns 201, the writer's local cache is
  fresh, and an isolated reader remains stale (the documented TTL fallback).
- Added `TestSessionContinuesSafelyDuringStorageSigningOutageAndRecovers`. The
  same API serves a healthy request, a storage signing outage, and recovery. The
  request remains 201 during the outage, session items remain present but have
  no playable URL, and URL signing works again after recovery. This protects
  against leaking raw object keys while preserving the rest of the session.
- No production behavior needed changing: best-effort invalidation, safe
  omission of un-signable audio, local rate-limit fallback, and queued provider
  retry were already implemented.

## Existing coverage reviewed

- `internal/ratelimit.TestFallsBackWhenStoreIsDown`: runtime Redis failure uses
  a local limiter, reports degraded state, still enforces a local limit, and
  automatically recovers.
- `internal/cache.TestRedisBusReconnectsAfterTheSubscriptionDrops` and
  `TestRedisBusPublishReportsAnUnreachableServer`: pub/sub reconnect and
  unreachable-publish behavior.
- `internal/api.TestRetryableProviderFaultQueuesARetry`,
  `TestRepeatedFailureDoesNotQueueTwice`, and
  `TestQueuedGenerationRunsThroughTheWorker`: retryable provider failure is
  queued once and the queued render completes after provider recovery.
- `internal/storage` tests cover storage operation error classification and
  retryability. These are provider-boundary tests; the new API test covers
  request behavior when signing fails at runtime.

## Degraded-mode boundaries

A Redis publish failure cannot invalidate other replicas' local cache copies;
those may serve stale data until their TTL expires. The originating API still
invalidates its own cache and does not fail a write that has already committed.
During storage signing failure, the session is served without playable audio
URLs; no raw storage key is returned. Provider outages are queued only when the
voice error is classified retryable, and idempotency prevents repeated requests
from filling the queue with duplicate jobs.

## Verification

The focused outage tests passed with PostgreSQL and Redis configured. The full
Go suite passed with the race detector (`go test -modfile=/tmp/local.mod -race
-count=1 ./...`), including the live Redis assertions. `go build
-modfile=/tmp/local.mod ./...` and `go vet -modfile=/tmp/local.mod ./...` also
passed.

## Result

**G-7 closed** for the in-use Redis, object-storage signing, and voice-provider
request paths. This does not claim that every dependency operation or the
currently unwired playback resolver has been exercised end to end.

G-52 remains an independent blocker: its intended files are unavailable in this
checkout and it is not claimed tested. Do not open a pull request until G-52 is
restored and its tests plus the full suite are green.
