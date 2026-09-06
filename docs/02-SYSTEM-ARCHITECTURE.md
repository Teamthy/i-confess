# PHASE 02 — System Architecture

**Status:** PASS WITH CONDITIONS
**Date:** 2026-09-06
**Branch base:** `main` @ `cbdee9d`
**Depends on:** PHASE 00 (PASS), PHASE 01 (PASS)

---

## OBJECTIVE

Describe the system as it actually runs — every process, every external
dependency, what happens when each one fails — and record the architectural
constraints later phases must not violate.

§47 asks for simplicity and §86 asks, of every component, "what happens when
this fails?" This phase answers both against the code in `cmd/server/main.go`,
not against a diagram of what the system might become.

## INPUTS

- `cmd/server/main.go` (the only entry point), read end to end
- `internal/api/router.go`, 240 route registrations
- The Master Build Directive §27, §47, §48, §51, §74

## DEPENDENCIES

PHASE 01, for the bounded contexts this topology has to serve.

---

## 1. RUNTIME TOPOLOGY

One process. That is deliberate and it is the right call at this scale (§47).

```
                        ┌──────────────────────────────────┐
   client ── HTTPS ───► │  Go API  (:PORT)                 │
                        │                                  │
                        │  http.Server                     │
                        │    └─ h.Routes()                 │
                        │         └─ 240 route handlers    │
                        │                                  │
                        │  goroutine: scheduler   (1 min)  │
                        │  goroutine: deletion    (1 hour) │
                        │  worker pool: jobs (N workers)   │
                        │  worker pool: email  (2 workers) │
                        └───┬────────┬────────┬────────┬───┘
                            │        │        │        │
                     PostgreSQL   Object    Voice    Push
                     (required)  Storage   Provider  Provider
                                   │
                                  CDN ──► client (audio never
                                          transits the API)
```

The scheduler and deletion sweeps are goroutines inside the API process, not
separate services. At this volume that is correct: there is nothing to
independently scale, and a second deployable would cost more in operational
surface than it saves. **The constraint to preserve:** both sweeps must stay
idempotent, because running two API instances will run both sweeps twice.

## 2. DEPENDENCY MAP AND FAILURE BEHAVIOUR

This is the substance of the phase. Every row was read from `main.go`; the
"on failure" column is what the code actually does, not what it should do.

| Dependency | Required? | Configured by | On failure |
|---|---|---|---|
| **PostgreSQL** | **Hard** | `DATABASE_URL` | `log.Fatalf` at startup. Correct: there is no product without it. |
| **Object storage** | Hard | `storage.New` | `log.Fatalf` at startup. |
| **Redis** | **Optional** | `REDIS_ADDR` | Rate limiting degrades to per-instance. Logged, not fatal. |
| **Voice provider** | **Optional** | `ELEVENLABS_API_KEY` | Synthesis endpoints return 503. Logged, not fatal. |
| **Push (APNs)** | **Optional** | `APNS_KEY_PATH` + key ID + team ID | Falls back to `push.LogSender`. Logged, not fatal. |
| **Push (FCM)** | **Optional** | `FCM_SERVICE_ACCOUNT_PATH` | Same fallback. |
| **Email** | **Optional** | provider config | Log sender: messages printed, not delivered. Logged. |

**This is the strongest part of the architecture and it should be preserved
deliberately.** Every optional dependency fails *open to a degraded mode* and
says so in the log at startup, rather than failing closed or silently doing
nothing. An operator reading the boot log knows exactly which capabilities the
instance has. That satisfies §51's "what happens if this disappears" for the
startup case, which is the case most systems get wrong.

The residual gap is the **runtime** case: what happens when Redis disappears
*after* startup, 30 seconds into serving traffic. §51 asks that question
specifically and nothing currently answers it — see G-7.

## 3. REQUEST PATH

```
HTTP request
  → ReadHeaderTimeout 10s            (slowloris defence)
  → route middleware chain           (per-route, composed explicitly)
      → rate limit (per-IP or per-user)
      → auth (JWT)                   where the route declares it
      → idempotency                  where the route opts in
  → handler in internal/api
  → application service
  → store (SQL, '?' rebound to '$N')
  → PostgreSQL
```

The chain is composed per route rather than applied globally, which is why
`router.go` reads `authed(idempotent(n))` and `registerLimit(authed(n))`. That
is more verbose than global middleware and it is the right trade: a reader can
see a route's full policy on one line, and a route cannot accidentally inherit
auth it did not ask for.

## 4. BACKGROUND WORK

| Worker | Trigger | Purpose |
|---|---|---|
| Scheduler sweep | ticker, 1 minute | evaluate schedules, send reminders |
| Deletion sweep | ticker, 1 hour | erase accounts whose grace period expired |
| Job worker pool | queue | `cfg.QueueWorkers` workers, `jobs.RegisterHandlers` |
| Email queue | channel, 512 buffered | 2 workers |

All four are started with `context.Background()` and stopped by `defer`.
Shutdown waits on `SIGINT`/`SIGTERM`.

## 5. ARCHITECTURAL CONSTRAINTS

These are the rules a later phase can most easily break by accident.

**C-1 — Audio never transits the API (§6).** The API returns signed CDN URLs.
No handler streams audio bytes. This is what keeps the API horizontally
scalable; a single large download holds a connection and a goroutine for its
whole duration.

**C-2 — `api` owns no invariants (PHASE 01).** Handlers validate and translate.
Rules live in domain packages or the database.

**C-3 — Optional dependencies fail open to a degraded mode and log it.** Adding
a new external dependency means adding its degraded mode in the same change.

**C-4 — Sweeps must stay idempotent.** They run once per process, and the
deployment model may eventually run more than one process.

**C-5 — One dialect (§25).** PostgreSQL everywhere, including tests. SQLite is
gone and must stay gone.

## 6. GAPS

**G-7 — Runtime dependency failure is untested.**
Startup degradation is excellent. Nothing verifies what happens when Redis,
object storage or the voice provider becomes unavailable *while serving*. §51
asks for exactly this: "For every dependency ask: what happens if this
disappears for 30 seconds?" That belongs in PHASE 48, but the architecture
should say now that it is unproven.

**G-8 — The API is versioned by duplication, and it has already drifted.**
240 route registrations: 131 under `/v1/`, 109 without. **Every unprefixed
route has an exact `/v1/` twin**, so 109 registrations are pure duplication.
Worse, 22 routes exist *only* under `/v1/` — including the entire playback
surface:

```
POST /v1/sessions/{id}/start
POST /v1/sessions/{id}/pause
POST /v1/sessions/{id}/resume
POST /v1/sessions/{id}/complete
GET  /v1/sessions/{id}/queue
```

A client calling `POST /sessions/{id}/start` gets a 404. Nothing detects this,
because each path is registered independently and a missing twin is not an
error.

The fix is to declare each route once and mount the set at both prefixes, or to
commit to `/v1/` alone and redirect the legacy paths. Either way a test should
assert the two sets agree, so drift becomes a build failure rather than a 404
in production.

**G-9 — The health check is correct but untested.**
An earlier draft of this document said the health check could not distinguish
"up" from "able to serve". That was wrong and is corrected here: `health.Checker`
pings the database under a 5-second timeout and returns **503** when the status
is not `healthy`, which is exactly the behaviour a load balancer needs.

What is missing is a test. `internal/health` has none, so the 503 branch — the
only branch that matters operationally, because it is what stops traffic being
routed to a process that cannot reach PostgreSQL — is unverified. This is the
same failure mode PHASE 01 recorded for `internal/community`: correct code in an
untested package is one refactor away from being incorrect.

## TESTING

This phase adds no code, so it adds no tests. That is a deliberate choice: an
architecture document verified by prose is still an architecture document, and
inventing tests to make the phase look productive would be the "fake
completeness" the directive forbids.

The two gaps that need tests are named and assigned: G-8 to a route-parity test
(immediate, small), G-7 to PHASE 48.

Existing suite unaffected: `go test ./... -count=1` → **22/22 packages,
0 failures**.

## SECURITY REVIEW

Three properties of this topology matter for security and all three hold:

**Audio bypasses the API (C-1), so the API holds no long-lived connections to
protect.** Signed URLs carry the authorisation, with a TTL set by
`internal/entitlements` — shorter for free, longer for premium.

**The 10-second `ReadHeaderTimeout` is present.** Without it, slowloris holds
connections indefinitely. Many Go services ship without it.

**Secrets are read from the environment at startup and never logged.** The boot
log names which providers are configured but prints no key material. Verified by
reading every `log.Printf` in `main.go`.

One residual risk tied to G-8: duplicated route registrations mean a security
policy applied to one path can be missed on its twin. A rate limit added to
`/v1/me/deletion` but not `/me/deletion` would leave the destructive endpoint
unthrottled on one of two live paths.

## PERFORMANCE REVIEW

Not in scope for this phase; PHASE 46 owns it. One architectural note that
belongs here because it constrains later work: the single-process model means
the scheduler sweep and the HTTP server share a connection pool. A slow sweep
can starve request handling. `DB_MAX_OPEN_CONNS` defaults to 25, which is the
knob, but nothing currently measures contention.

## DOCUMENTATION

This file. `docs/DEPLOYMENT.md` exists and describes the environment; it was
read and does not contradict this document, but it predates the PostgreSQL
switch and should be refreshed in PHASE 60.

## EXIT CRITERIA

| Criterion | State |
|---|---|
| Runtime topology described | Done |
| Every dependency's failure behaviour stated from code | 7 of 7 |
| Request path documented | Done |
| Background work inventoried | 4 workers |
| Architectural constraints recorded | C-1 … C-5 |
| Gaps identified with an owner | G-7, G-8, G-9 |
| Claims verified against code, not assumed | 2 checked; 1 corrected (G-9) |
| §47 simplicity honoured | Yes — one process, no orchestrator, no queue broker |

---

## PHASE REPORT

1. **Built:** the system architecture document, grounded in `main.go` rather than in an idealised diagram.
2. **Files changed:** `docs/02-SYSTEM-ARCHITECTURE.md`.
3. **Architecture decisions:** recorded five constraints (C-1…C-5) that later phases must not violate; confirmed the single-process model is correct at this scale and stated the condition under which it stops being.
4. **Database/API changes:** none.
5. **UI/UX changes:** none.
6. **Tests added:** none, deliberately.
7. **Tests executed:** `go test ./... -count=1` → 22/22 packages, 0 failures (regression check only).
8. **Security considerations:** slowloris defence present; audio bypasses the API; no secrets in logs. One new risk recorded — duplicated routes can diverge in their security policy.
9. **Performance considerations:** scheduler and HTTP share one connection pool; unmeasured.
10. **Known issues:** G-7 (runtime failure untested), G-8 (route duplication, 22 `/v1`-only routes), G-9 (health check semantics).
11. **Remaining work:** G-8 is small and should be closed before any new endpoint is added, because every new endpoint currently has to be registered twice.
12. **Phase score:** 8/10. The architecture itself is sound and its degradation story is unusually good. Docked for G-8, which is a live correctness problem rather than a documentation gap.
13. **Decision:** **PASS WITH CONDITIONS** — condition is G-8.
14. **Recommended next phase:** **PHASE 03 — Technology Decisions**, with G-8 closed first since new endpoints will keep making it worse.
