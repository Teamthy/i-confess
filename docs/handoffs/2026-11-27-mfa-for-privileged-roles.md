# Handoff — MFA required for privileged roles (fix-everything row #3)

**Date:** 2026-11-27 · **Branch:** `arena/2a377dd9-i-confess` · **Commits:** none yet (working tree) · **PR:** none opened yet

## Goal

Execute fix-everything plan row #3: admin roles must require a session that passed a second factor; federated (Google/Apple) sign-in must ask for the code when the user has MFA enrolled.

## What changed

**Migration** — `server/internal/db/migrations/0039_mfa_verified_sessions.sql`: adds `mfa_verified INTEGER NOT NULL DEFAULT 0` to `refresh_tokens`. Applied automatically to test DBs via `db.Migrate` (verified by the passing store test).

**`server/internal/store/users.go`**
- `CreateAuthSession` now takes `mfaVerified bool` and stores it.
- `SessionStatus` gained `MFAVerified bool`; `AuthSessionStatus` reads the column.
- `RotateSession` preserves the source session's `mfa_verified` so refresh doesn't demote an admin session.

**`server/internal/auth/session.go`**
- `SessionState` gained `MFAVerified bool`.
- `RequireRoleWithSessions` (the middleware behind every `/admin/*` route) now refuses sessions with `!MFAVerified` → 403 `AUTH_MFA_REQUIRED` after the role check passes. Added `forbiddenCode` helper.

**`server/internal/api/session_validator.go`** — `ValidateSession` propagates `st.MFAVerified` into `auth.SessionState`.

**`server/internal/api/handlers_auth.go`** — `issueToken` takes `mfaVerified`; `register` and `login` pass `true` (no factor enrolled, or factor satisfied).

**`server/internal/api/social.go`** — `socialSignIn` now checks MFA enrolment: an enrolled user gets `mfa_required: true` (200) and no session; a non-enrolled federated user gets an MFA-verified session.

**Tests** (real PostgreSQL via `dbtest`, plus unit tests)
- `internal/store/users_mfa_test.go`: `mfa_verified` stored/read, rotation preserves it.
- `internal/auth/session_mfa_test.go`: `RequireRoleWithSessions` rejects unverified (403), admits verified (200), rejects invalid (401).
- `internal/api/mfa_admin_test.go`: admin without MFA → 403 `AUTH_MFA_REQUIRED`; admin with MFA → 200; social of enrolled user → `mfa_required`; social without MFA → session. (Not run locally — see below.)

## Verification

Run locally against real PostgreSQL (TEST_DATABASE_URL set, Go 1.27.1 from bootstrap):
- `go test ./internal/auth/` → ok
- `go test ./internal/store/` → ok
- `go test ./internal/mfa/` → ok
- `gofmt -l` on all changed files → clean

**Not run locally:** `./internal/api/` does not compile in this sandbox because `go.opentelemetry.io/auto/sdk` (an indirect dep of `internal/ai`→anthropics) is unreachable and has no usable GitHub mirror; the sandbox modfile only substitutes `golang.org/x/*`. The api-package tests (`mfa_admin_test.go`, plus pre-existing api tests) will run in CI. `go vet`/full `api` build deferred to CI.

## Open items

- Open a PR from `arena/2a377dd9-i-confess` for row #3; wait for green CI; review and merge to `main`.
- Row #3's `mfa_admin_test.go` depends on CI compiling `./internal/api/` (blocked in this sandbox only by the otel/auto/sdk module).
- Rows #4–#12 not started; next is row #4 (analytics allowlist).

## Picking it up

Open the row #3 PR, confirm CI green (CI builds `./internal/api/`), review, merge. Then take row #4.
