# I CONFESS — notes for Claude

## Session handoff (required)

Every session that changes the repository ends with a handoff, so the next
session (or person) can continue without re-deriving anything.

- **Start of a session:** read the newest file in `docs/handoffs/` before
  starting work.
- **End of a session:** write `docs/handoffs/YYYY-MM-DD-<short-topic>.md` from
  `docs/handoffs/TEMPLATE.md`, commit it on the working branch with the work,
  and push. If a session continues after a handoff and commits more work,
  update that handoff (or add a new one) before finishing.
- Report only what was actually run and observed. Say plainly what was not
  verified and why (no SDK here, no credentials, no staging access).

`.claude/hooks/require-handoff.sh` runs as a Stop hook and refuses to let a
session finish while the branch has commits newer than its latest handoff.

## Running things

The Go module is in `server/`, not the repo root. `make build`, `make test`,
`make lint` from the root handle that. Tests need PostgreSQL via
`TEST_DATABASE_URL`; the Makefile has the local default. CI
(`.github/workflows/ci.yml`) is the full list of checks.
