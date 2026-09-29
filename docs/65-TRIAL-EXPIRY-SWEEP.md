# 65 — Scheduled trial expiry sweep (G-49)

**Status: implemented and verified.**

Trial expiry no longer depends on the account owner making another request.
`TrialStore.SweepExpired` selects only due `ACTIVE`/`EXPIRING` trials in bounded
pages, then delegates each state and subscription transition to `Refresh`. That
preserves the single transition writer and records the `trial_expired` funnel
event through the existing once-only lifecycle path. A partial index on
`(expires_at, user_id)` covers due active/expiring rows.

The API exposes `RunTrialExpirySweep`, which drains batches of 500. The server
runs it once at startup to recover missed work and every 15 minutes thereafter.
Overlapping replicas are safe: `Refresh` locks each trial row and a concurrent
sweep sees the terminal state rather than recording a second transition event.
Failures are logged, and the next scheduled run retries still-due rows.

## Verification

- Store race tests cover a multi-page backlog, due versus future trials,
  subscription projection back to free, exactly-once expiry analytics, and two
  overlapping sweepers.
- Full Go race suite passed with PostgreSQL and Redis:
  `go test -modfile=/tmp/local.mod -race -count=1 ./...`.
- `go build -modfile=/tmp/local.mod ./...` and
  `go vet -modfile=/tmp/local.mod ./...` passed.

G-52 remains absent from this checkout and unverified; the handoff PR gate is
still blocked. See `docs/PROJECT-STATUS.md` for the current ledger.
