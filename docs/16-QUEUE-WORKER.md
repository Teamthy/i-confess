# PHASE 16 — QUEUE / WORKER INFRASTRUCTURE

## OBJECTIVE

Build the background work infrastructure the directive asks for: a job queue, a
worker, retries, exponential backoff, idempotency, a dead-letter strategy, and
the three job kinds — generation, notification, processing.

## INPUTS

`docs/00`–`15`, the directive's PHASE 16 definition, and the existing
`internal/jobs`, `internal/email`, `internal/voice`, `internal/push` and the
`jobs` table from the baseline schema.

## DEPENDENCIES

PHASE 13 (object storage and audio inspection) and PHASE 14 (the voice pipeline,
rights gate and generation job records). The generation handler drives
`voice.Pipeline` directly, so this phase could not have been built before them.
It also closes PHASE 14's **C-3** (generation was synchronous with no retry
path) and **C-4** (`voice.Backoff()` was dead code).

---

## Findings

### 1. The queue existed in shape and was hollow

`internal/jobs` had a queue, a worker and a job table. Nothing used them:

| Thing | State |
|---|---|
| `jobs` table | Present since the baseline |
| `store.JobStore` (Save / ByID / ListByStatus / Delete) | **Zero callers, repo-wide** |
| The queue the server actually ran | `jobs.MemoryQueue`, in-process |
| Callers of `queue.Enqueue` | **None.** The only `.Enqueue(` calls in the codebase were `h.mail.Enqueue`, a different type entirely |
| The four registered handlers | `log.Printf` plus a `TODO`, returning `nil` |

So `main.go` started N worker goroutines that polled a queue nothing ever wrote
to, and the four handlers — `audio_generate`, `analytics_event`,
`notification_send`, `recommendation_compute` — each logged a line and returned
success. **A generation job would have been recorded as completed with no audio
generated and no provider called.** That is worse than an unimplemented feature,
because it reports itself as working.

### 2. No exponential backoff

`MemoryQueue` retried after a **fixed** `RetryDelay`. A provider that is
rate-limiting got the same interval on attempt 5 as on attempt 1.

### 3. Idempotency did not survive a restart

Deduplication was `map[string]struct{}` inside the process. The `jobs` table
already had a `UNIQUE` constraint on `idempotency_key` that nothing used.

### 4. Three separate backoff implementations

`internal/email` had a 5 s doubling capped at 5 minutes. `internal/voice` had an
identical 30 s doubling capped at 30 minutes, called by nothing. `internal/jobs`
had neither. One question, three answers.

### 5. The claim had no deterministic order

`ORDER BY created_at` over a column with second precision ties for every job
enqueued in the same second, so PostgreSQL was free to return them in any order.
A caught it: a test asserting oldest-first failed non-deterministically.

---

## IMPLEMENTATION

### 1. `internal/backoff` — one retry policy

A leaf package with `Policy{Base, Max, Jitter}` and `Next(attempt)`.
`email.defaultBackoff` and `voice.Backoff` now delegate to it, so the mailer,
the voice pipeline and the queue cannot drift apart.

The shift is capped and the product is range-checked **before** multiplying: an
overflowed `time.Duration` is negative, and a negative delay reads as "retry
immediately" — a retry storm against whatever just failed.

### 2. `0007_job_queue.sql`

Adds `available_at`, `worker_id`, `claimed_at` and `dead_letter_reason`, plus
`idx_jobs_claim (status, available_at, created_at)` and `idx_jobs_running`.
`available_at` defaults to `''`, which sorts before every RFC3339 timestamp and
therefore means "available immediately" — existing rows need no backfill.

### 3. `jobs.Queue` — the contract

`Register`, `HandlerFor`, `Enqueue`, `Claim`, `Complete`, `Fail`,
`FailPermanent`, `RequeueDead`, `Stats`. Two implementations sit behind it:
`MemoryQueue` for tests and tooling, `store.JobQueue` for the server. The
worker, the handlers and the producers do not care which is in use.

`Enqueue` on a duplicate idempotency key returns **the existing job's id
alongside `ErrDuplicateJob`**, so a caller that retried its own request gets the
job it already has rather than only a refusal to interpret.

`jobs.Permanent(err)` marks a cause that retrying cannot fix; `Fail` parks those
immediately instead of spending the whole schedule first.

### 4. `store.JobQueue` — the durable queue

Claiming is one statement:

```sql
UPDATE jobs SET status='running', attempts=attempts+1, worker_id=?, claimed_at=?, updated_at=?
WHERE id = (SELECT id FROM jobs
            WHERE status='queued' AND (available_at='' OR available_at <= ?)
            ORDER BY created_at ASC, id ASC
            LIMIT 1 FOR UPDATE SKIP LOCKED)
RETURNING ...
```

`FOR UPDATE SKIP LOCKED` is what lets several workers poll one table without
sharing a job and without a lock manager of our own. The `id` tiebreaker makes
the order deterministic. Ordering is by `created_at` with `id` breaking ties;
strict FIFO within one second would need a sequence column, and a work queue
does not need it.

The retry decision happens in a transaction holding the row, so two workers
reporting on the same job cannot both conclude an attempt remains.

`ReclaimStale` returns jobs a worker claimed and never finished — because the
worker died — to the queue. Without it those rows sit in `running` forever: not
due, not failed, not finished.

`RequeueDead` is the operational half of the dead-letter strategy. Parking a job
rather than deleting it is only useful if a human can release it once the cause
is fixed.

### 5. `internal/workers` — the handlers

Separate from `internal/jobs` so the queue has no opinion about voices or
notifications, and the handlers can depend on the store and the push transport
without pulling them into the queue.

`Register` installs **only the types whose collaborator is configured**. A type
with nothing behind it is left unregistered, so the queue parks such a job with
"no handler registered" — which names the real problem — instead of accepting
work it cannot do.

Classification is the substance here:

| Outcome | Treatment |
|---|---|
| Missing payload field | Permanent — retrying does not supply it |
| No collaborator configured | Permanent — configuring one is not a retry |
| Rights refusal | Permanent — a legal answer will not change |
| Provider rejected the request | Permanent — retrying only wastes money |
| Provider timed out / 429 / 5xx | Retryable, with backoff |
| Push token not registered | Permanent |
| Push transport fault | Retryable |
| Asset could not be recorded after a paid render | Retryable — the audio is already paid for |

### 6. Wiring

`main.go` now runs the durable queue and registers the handlers with the real
collaborators. The admin API gains `GET /admin/queue` (counts by status, the job
types this server runs, and whether the queue is durable) and
`POST /admin/queue/requeue`. Both have `/v1/` twins.

**The producer:** a retryable provider fault during synchronous generation now
enqueues an `audio.generate` retry, keyed on the render so repeated failures
queue one job rather than several. This is the case a queue exists for — the
request was well-formed, the rights were in order, the operator did everything
right, and the only problem was that the provider was briefly unavailable.

`store.JobStore` was deleted: `JobQueue` supersedes it and it had no callers.

---

## TESTING

Thirty-five new tests — 11 in `internal/backoff`, 11 in `internal/store`,
9 in `internal/workers` and 4 in `internal/api` — plus
`TestRetriesAndDeadLetter` in `internal/jobs`, rewritten to drive the retry path
with an injected clock instead of relying on timing.

| Test | Covers |
|---|---|
| `TestTwoWorkersNeverClaimTheSameJob` | 4 goroutines over 25 jobs; every job claimed exactly once |
| `TestIdempotencyKeyPreventsASecondJob` | two enqueues, one row, existing id returned |
| `TestFailureWaitsOutItsBackoff` | the deadline is enforced, not just recorded |
| `TestBackoffGrowsWithEachFailure` | 1 m, 2 m, 4 m |
| `TestExhaustedAttemptsAreParkedWithAReason` | parking records why |
| `TestPermanentFailureParksImmediately` | one attempt, not the whole schedule |
| `TestRequeueDeadIsTheOperationalHalf` | released with a fresh count |
| `TestStaleClaimIsReturnedToTheQueue` | a dead worker's job comes back |
| `TestLargeAttemptCannotOverflow` | attempt 2²⁰ does not produce a negative delay |
| `TestTransportFaultIsRetryableButRejectedTokenIsNot` | the classification |
| `TestUnconfiguredHandlerParksRatherThanSucceeding` | the stub defect, guarded |
| `TestQueuedGenerationRunsThroughTheWorker` | producer → queue → worker → real pipeline |
| `TestRepeatedFailureDoesNotQueueTwice` | three clicks against a down provider, one job |

**Defect injection — six for six:**

| Injection | Result |
|---|---|
| `FOR UPDATE SKIP LOCKED` removed | `TestTwoWorkersNeverClaimTheSameJob` FAIL |
| `ON CONFLICT (idempotency_key)` removed | `TestIdempotencyKeyPreventsASecondJob` FAIL |
| `policy.Next(attempts)` → `Next(1)` | `TestBackoffGrowsWithEachFailure` FAIL |
| `available_at` filter removed from the claim | `TestFailureWaitsOutItsBackoff` FAIL |
| every push failure made permanent | `TestTransportFaultIsRetryableButRejectedTokenIsNot` FAIL |
| retry producer removed | `TestRetryableProviderFaultQueuesARetry` and `TestQueuedGenerationRunsThroughTheWorker` FAIL |

Full suite: **28 packages, 0 failures.** `make verify`: **all checks passed.**

---

## SECURITY REVIEW

- The two new admin routes are role-gated and have `/v1/` twins, so the route
  parity guard passes.
- No new dependencies. PostgreSQL only; no Redis, no Kafka (§26, §47).
- The claim is a single statement, so there is no window in which two workers
  hold the same job — which is also what stops a paid provider call happening
  twice for one request.
- `RequeueDead` is bounded to 1000 per call and cannot resurrect a completed job.
- Payloads are stored as text and decoded with errors ignored only where an
  empty payload is acceptable; a malformed payload fails the handler rather than
  panicking the worker.

## DOCUMENTATION

This file. `docs/PROJECT-STATUS.md` updated with the PHASE 16 verdict.

---

## EXIT CRITERIA

| Criterion | Status |
|---|---|
| Job queue, durable | PASS — PostgreSQL, survives restart |
| Worker | PASS — pool over the queue, graceful stop |
| Retries | PASS — bounded by `max_attempts` |
| Exponential backoff | PASS — one shared policy, growth verified |
| Idempotency | PASS — database unique key, survives restart |
| Dead-letter strategy | PASS — park with reason, requeue, stale reclaim |
| Generation jobs | PASS — drives the real pipeline |
| Notification jobs | PASS WITH CONDITION — handler real, no producer (C-2) |
| Processing jobs | PARTIAL — handler exists, no implementation (C-3) |
| Tests fail without the fix | PASS — 6 of 6 injections |
| `make verify` clean | PASS |

## VERDICT

**PASS WITH CONDITIONS — 8/10**

- **C-1** The admin generate endpoint is still synchronous; only its retry path
  uses the queue. `Handler.Generate` and `adminGenerateAudio` walk the same steps
  through the same building blocks, but they are two orchestrations of one
  process. Extract a shared runner before the generation API is revisited.
- **C-2** `notification.send` has a real handler and **no producer**. The
  scheduler still calls `push.Sender` inline, so a reminder is delivered on the
  scheduler's goroutine and a transient transport fault is not retried by the
  queue. Moving the dispatcher onto the queue changes its tests and belongs with
  a notification phase.
- **C-3** `audio.process` has a handler and no `Processor` implementation, so it
  is deliberately not registered. Upload-time inspection still runs synchronously
  in `admin.go`.
- **C-4** The worker polls on a 1 s ticker. There is no `LISTEN/NOTIFY`, so
  queued work can wait up to a second to start. Fine at this scale; worth
  revisiting before generation latency becomes user-visible.
- **C-5** `analytics_event` and `recommendation_compute` job types are gone. They
  were stubs with no producer and no consumer, and there is no analytics
  warehouse or recommendation engine behind them (G-27). They should come back
  when those do.
