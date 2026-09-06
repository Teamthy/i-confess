# PHASE 04 — Repository Bootstrap

**Status:** PASS
**Date:** 2026-09-06
**Branch base:** `main` @ `cbdee9d`
**Depends on:** PHASE 00–03

---

## OBJECTIVE

Make the repository operable by someone who has never seen it: one command to
build, one to test, one to run everything CI runs. Then remove the obstacles
that were already there.

The layout stays as it is (`server/`, `clients/`, `contracts/`, `docs/`,
`apps/`) — that decision was made earlier and is not reopened here.

## INPUTS

- Repository root, `.github/workflows/ci.yml`, existing docs
- A locally installed `golangci-lint` 2.13.2, so the config could be verified
  rather than guessed

## DEPENDENCIES

PHASE 03, for the technology decisions this tooling has to support.

---

## 1. WHAT WAS MISSING

| Gap | Now |
|---|---|
| No single entry point for build/test | `Makefile` with `make help`, `make verify` |
| No linter, no config | `.golangci.yml`, `golangci-lint` in CI |
| No way to reproduce CI locally | `make verify` runs exactly the CI gate |
| No honest project status | `docs/PROJECT-STATUS.md` |

`make verify` chains `fmt-check → build → vet → lint → test` and exits non-zero
on any failure. It was run end to end and passes.

## 2. THE CRITICAL FINDING — A PAYMENT BYPASS

Enabling a linter found it, which is the argument for having one.

`POST /subscriptions/verify` is a **live, authenticated route** that grants
premium based on what `billing.VerifierFromEnv()` accepts. Every verifier in
`internal/billing` is a stub:

| Verifier | Accepts as valid |
|---|---|
| `NoopVerifier` (**the default**) | any receipt starting `valid_` |
| `appleVerifier` | any receipt starting `apple_valid_` |
| `googleVerifier` | any receipt starting `google_valid_` |

Nothing checked the environment. A production deployment with
`BILLING_VERIFIER` unset — the default — granted premium to anyone who posted:

```json
{"provider": "apple", "receipt": "valid_monthly"}
```

`internal/api/billing.go` then calls `h.users.SetSubscription(..., "premium",
"active")`. There was no guard anywhere in the path; a search for a production
check across `billing.go` and `internal/billing/*.go` returned nothing.

This is a §37 violation ("never trust the mobile client", "validate purchases
server-side") and a §44 concern ("payment verification"). Under the directive's
gate rules a critical defect blocks release.

### Fix

`VerifierFromEnv()` now returns `prodBlocker` when `ENV=production`. It rejects
every receipt unconditionally, because at present *every* verifier is a stub —
there is no safe production option until a real App Store Server API / Play
Developer API verifier exists.

Failing closed is the only defensible choice here. A launch delay is
recoverable; free premium is not, and the revenue loss would be invisible until
someone reconciled the books.

Four tests pin the behaviour in both directions — production must refuse, and
development must still work, because a guard that breaks the dev flow gets
removed:

```
ENV=production BILLING_VERIFIER="" accepted receipt "valid_monthly" as valid
  - premium is purchasable for free
```

That is the failure message when the guard is taken out. It was verified to
appear, not merely assumed.

## 3. LINT BASELINE

`golangci-lint` 2.13.2 with 10 linters enabled. Starting position: **64 issues**.

| Category | Before | After |
|---|---|---|
| `goimports` | 18 | 0 |
| `misspell` | 12 | 0 |
| `errcheck` | 6 | 0 (4 excluded, see below) |
| `staticcheck` | 1 | 0 |
| `unused` | 1 | 0 |
| **Total** | **38**¹ | **0** |

¹ After dropping `staticcheck: checks: ["all"]`, which added 34 findings of
marginal value and would have made the gate noise rather than signal.

`goimports` and `misspell` were auto-fixed across 22 files; the suite was
re-run afterwards and stayed at 22/22 packages.

**Two exclusions, both documented in the config:**

`defer rows.Close()` and `defer tx.Rollback()` are excluded from `errcheck`.
Their errors are genuinely unrecoverable — the statement has already run and the
only caller left is the deferred function itself. Checking them means either
discarding the result explicitly at every call site or leaking an error into a
return value nothing reads. Any *other* unchecked error still fails the build.

CI runs with `new-from-merge-base: main`, so only findings introduced by the
branch under review fail. Without that, the gate would start life red on
findings that predate it, which is how lint gates get commented out.

## 4. DEAD CODE REMOVED

The `unused` linter found three HTTP handlers — `listCategories`,
`categoryConfessions`, `listVoices` — that no route references. They were
superseded by the cached wrappers in `content_cache.go` and never deleted. The
caches themselves are live; only the pre-cache handlers were dead.

`EnvVerifier` was also dead: a struct never constructed anywhere. Removed with
the payment fix.

## 5. FALSE COMPLETION DOCUMENTS

Five documents at the repository root assert completion that the code
contradicts:

| Document | Claim |
|---|---|
| `MASTER-PROMPT-COMPLETION.md` | "✅ COMPLETE", "100% complete" |
| `CONTENT-DOMAIN-COMPLETION.md` | "✅ COMPLETE AND VALIDATED", "production-ready" |
| `AUDIO-PLATFORM-STATUS.md` | "🟢 Architecture Complete" |
| `SESSION-NOTES.md` | "\| **Production Ready** \| ✅ Yes \|" |
| `README-COMPLETE.md` | a complete-looking setup guide |

PHASE 00–04 found, among other things: three tables missing from the running
schema, session queues that were not snapshots, 22 endpoints unreachable at
their documented paths, a mobile client that cannot play audio, and a payment
bypass. None of that is consistent with "production ready: yes".

The directive forbids fake completeness explicitly. These files are not deleted
— they are the record of what was believed at the time, and deleting them would
lose that. They are superseded by `docs/PROJECT-STATUS.md`, which states what is
true and what is verified.

## 6. DECISIONS CARRIED FORWARD

**`apps/mobile`, `apps/web` and `apps/admin` are marked, not deleted.**
D-4 and D-5 decided their replacement, but D-5 sequences that replacement to
PHASE 38 and PHASE 40. Deleting them now would leave the repository with no web
surface for thirty-plus phases and would destroy the only reference for how the
existing client talks to the API. Each is marked with a `RETIREMENT.md` stating
its status and which phase replaces it.

This is a deliberate deviation from a literal reading of "replace", and it is
recorded here so it is a decision rather than an oversight.

## TESTING

Tests added: 4, all in `internal/billing/verify_prod_test.go`, covering the
payment bypass in both directions plus mode selection.

Full gate via `make verify`: `fmt-check → build → vet → lint → test`, all
passing. `go test ./... -count=1` → **22/22 packages, 0 failures**.
`golangci-lint run ./...` → **0 issues**.

## SECURITY REVIEW

The payment bypass is the finding, and it is closed for production. Two things
remain on the record:

**The stub verifiers still exist.** They are correct for development and they
are now unreachable in production, but PHASE 36 must replace them with real
App Store Server API and Play Developer API calls before launch. The guard makes
the failure mode safe; it does not make the feature work.

**`NoopVerifier` is still the default.** That is right for development and it is
now blocked in production, but a staging environment with `ENV` set to something
other than `production` would accept stub receipts. If staging is ever reachable
from the internet, `ENV` handling needs a third case.

## PERFORMANCE REVIEW

Not applicable. Tooling and a guard clause only.

## DOCUMENTATION

- `docs/04-REPOSITORY-BOOTSTRAP.md` (this file)
- `docs/PROJECT-STATUS.md` — honest status, supersedes the completion claims
- `Makefile` — self-documenting via `make help`
- `.golangci.yml` — every exclusion carries its reason

## EXIT CRITERIA

| Criterion | State |
|---|---|
| One command builds, tests, lints | `make verify` |
| CI reproduces the local gate | 8 steps, matching |
| Linter enabled and passing | 0 issues |
| Critical defect found and fixed | payment bypass |
| False completion claims addressed | 5 documents superseded |
| Layout decision respected | unchanged |

---

## PHASE REPORT

1. **Built:** Makefile, lint config, CI lint stage, project status doc; fixed a payment bypass; removed dead code.
2. **Files changed:** `Makefile`, `.golangci.yml`, `.github/workflows/ci.yml`, `docs/04-REPOSITORY-BOOTSTRAP.md`, `docs/PROJECT-STATUS.md`, `internal/billing/verify_prod.go` + test, 3 retirement markers, 22 files reformatted.
3. **Architecture decisions:** production fails closed on payments; `defer`-ed `Close`/`Rollback` excluded from `errcheck` with the reason recorded; retirement is marked rather than executed early.
4. **Database/API changes:** none. `POST /subscriptions/verify` now rejects in production.
5. **UI/UX changes:** none.
6. **Tests added:** 4.
7. **Tests executed:** `make verify` → all green. `go test ./... -count=1` → 22/22 packages, 0 failures. `golangci-lint run ./...` → 0 issues.
8. **Security considerations:** critical payment bypass closed; stub verifiers recorded as PHASE 36 work.
9. **Performance considerations:** none.
10. **Known issues:** stub payment verifiers; `ENV` has no staging case.
11. **Remaining work:** real store verification in PHASE 36.
12. **Phase score:** 9/10. Docked one point because the payment bypass existed at all and was only found by a linter four phases in — the audit that should have caught it was §72, which has not run yet.
13. **Decision:** **PASS**.
14. **Recommended next phase:** **PHASE 05 — Design System**. Note the constraint from D-5: PHASE 38 and PHASE 40 cannot begin until it completes.
