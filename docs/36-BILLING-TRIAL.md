# PHASE 36 — Billing and trial lifecycle

**Gap closed:** `G-29 (PHASE 36: production billing verification and one
trial-entitlement authority)`

**Verdict:** PASS

## OBJECTIVE

Replace the receipt-accepting production stub with provider verification that
can grant Premium only from an Apple-signed transaction or a Google Play
Developer API response. Persist the six-state account trial instead of deriving
it from account creation time, and make one code path write the temporary
`premium/trial` entitlement projection. A running trial is entitled by the same
clocked subscription rule as any other subscription; a client request never
asserts a plan.

The checkout does not contain `docs/35-TRIAL-LIFECYCLE.md`. Its Conditions were
therefore not available to this phase and are not represented here as if they
had been read. This document records only changes and evidence made in PHASE 36.

## INPUTS

- `internal/billing/apple.go`: StoreKit/App Store signed-transaction verifier.
- `internal/billing/google.go`: Play purchase evaluator.
- `internal/billing/verify_prod.go`: environment selection and fail-closed boot
  guard.
- `internal/models/models.go`: clocked subscription entitlement rule.
- `internal/db/migrations/0010_subscription_provider_state.sql`: provider
  identity and subscription status vocabulary.
- `internal/api/billing.go` and `internal/api/router.go`: existing subscription
  surface.
- `docs/BILLING.md` and `docs/PROJECT-STATUS.md`: billing contract and ledger.

The input audit was performed with:

```sh
grep -R "VerifierFromEnv\|RequireVerification\|Subscription(ctx\|getTrial" -n server/internal server/cmd
grep -R "TestProductionRefusesStubReceipts\|TestTrial" -n server
```

It found the real Apple/Google verifier implementations and production refusal
coverage already present in the checkout, but no persistent trial table, trial
start route, or trial state-machine tests. The implementation below completes
that contract rather than adding a second receipt verifier beside the existing
one.

## DEPENDENCIES

- PostgreSQL 17 is the only database used by the application and tests.
- The migration runner embeds `server/internal/db/migrations/*.sql`.
- Production boot calls `billing.RequireVerification`; development/test may use
  `NoopVerifier`, while staging and production may not.
- The target route contract is generated from the live handler table. The three
  trial operations are registered under both legacy and `/v1` prefixes.

Verification commands for these dependencies:

```sh
source /tmp/toolchain/env.sh
cd server
TEST_DATABASE_URL="host=127.0.0.1 port=5432 user=iconfess dbname=postgres sslmode=disable" \
  GOFLAGS=-modfile=/tmp/local.mod go test ./internal/db -run 'TestPostgresSchemaLoads|TestMigrationsAreOrderedAndImmutable' -count=1
```

## IMPLEMENTATION

### Provider verification

`AppleVerifier` validates the exact `ES256` JWS algorithm, parses the `x5c`
certificate chain, verifies that chain against the pinned Apple Root CA, checks
the JWS signature, and only then evaluates bundle id, store environment, mapped
product, revocation, and expiry. `GooglePlayVerifier` extracts the purchase
token, asks the Play Developer API for the purchase, evaluates the mapped line
item and provider state, preserves the Play acknowledgement flag, and refuses
unknown or non-entitling states. `VerifierFromEnv` caches real verifiers by
configuration, while incomplete provider configuration produces a fail-closed
`prodBlocker` outside development/test. Boot refuses that blocker through
`RequireVerification`.

The existing production refusal tests were retained and the provider-selection
coverage remains in `internal/billing/verify_prod_test.go`; the API tests also
prove that an unconfigured production verifier returns `503` without upgrading
an account.

### Persistent trial

Migration `0014_trial_lifecycle.sql` adds `trials`, one row per user, with the
closed vocabulary:

```text
ELIGIBLE → STARTED → ACTIVE → EXPIRING → EXPIRED
                              └────────→ CONVERTED
```

`internal/trial/lifecycle.go` is the domain state machine. Its named edge table
is explicit, not an ordinal comparison. `TrialStore.Start` applies
`ELIGIBLE→STARTED→ACTIVE` atomically, uses a fixed seven-day expiry, and is
idempotent for a running trial without extending its clock. `Refresh` walks the
clock-derived `EXPIRING` edge before `EXPIRED`. `Convert` is terminal and never
stands in for a store purchase.

`TrialStore` is the one trial-specific projection writer. Starting a trial may
change a free subscription row to `plan=premium,status=trial`; terminal trial
transitions revoke only that still-trial projection and never overwrite a paid
subscription that arrived later. `models.Subscription.Entitled` remains the
single entitlement decision: it requires an entitled status and an unexpired
clock, so the rest of the API and the session engine do not each invent a trial
rule. Administrative grants and verified store purchases remain their own
explicit write paths; neither is a trial entitlement shortcut.

The existing `GET /subscriptions/trial` now reads the trial row rather than
`users.created_at`. The new authenticated operations are:

| Method | Path | Behaviour |
|---|---|---|
| `POST` | `/subscriptions/trial` | Start the one-time trial |
| `GET` | `/subscriptions/trial/status` | Advance and return clock-derived state |
| `POST` | `/subscriptions/trial/convert` | End the trial without granting paid Premium |

Each is also served under `/v1`. The typed Dart client, paywall IA, route
export, and OpenAPI document were updated from those live registrations.

## TESTING

Named lifecycle and provider tests:

- `TestTrialTransitions` enumerates every allowed and forbidden directed edge.
- `TestTrialLifecycle` covers activation, the 24-hour expiring boundary, expiry,
  terminal conversion, journey day boundaries, and skipped-edge rejection.
- `store.TestTrialLifecycle` proves persistence, one-time start, idempotent retry,
  Premium while active, and Free after expiry.
- `TestTrialConversionDoesNotGrantPaidPremium` proves conversion does not bypass
  receipt verification.
- `TestProductionRefusesStubReceipts` remains in
  `internal/billing/verify_prod_test.go`.
- `TestProductionVerifiesWithTheRealAppStoreVerifier` rejects the forged receipt
  shape that the former implementation accepted.
- `TestTrialEndpointsPersistTheLifecycle` exercises both route prefixes.

The phase verification was run with:

```sh
source /tmp/toolchain/env.sh
export TEST_DATABASE_URL='host=127.0.0.1 port=5432 user=iconfess dbname=postgres sslmode=disable'
cd server
GOFLAGS=-modfile=/tmp/local.mod go test ./internal/trial ./internal/store ./internal/api -run 'TestTrial|TestProduction|TestVerifier|TestRequireVerification' -count=1
GOFLAGS=-modfile=/tmp/local.mod go test ./internal/billing ./internal/trial ./internal/store ./internal/api -count=1
```

The first targeted command passed. The full command is the required proving
command for this phase and must remain green in CI.

Schema and contract evidence:

```sh
GOFLAGS=-modfile=/tmp/local.mod go test ./internal/db -run 'TestPostgresSchemaLoads|TestMigrationsAreOrderedAndImmutable|TestSchemaSQLIncludesEveryMigration' -count=1
EXPORT_ROUTES=1 EXPORT_ROUTES_PATH=/home/user/i-confess/design/routes.json \
  GOFLAGS=-modfile=/tmp/local.mod go test ./internal/api -run '^TestExportRouteTable$' -count=1
GOFLAGS=-modfile=/tmp/local.mod go run ./cmd/genspec /home/user/i-confess/contracts/openapi.json
cd ..
python3 design/test_ia.py
python3 scripts/check_dart_symbols.py
```

The resulting contract count is **306 route entries, 240 OpenAPI paths, and 306
operations**. The Dart check is **85/85**. `design/routes.json` and
`contracts/openapi.json` are generated artifacts, not hand-counted claims.

## SECURITY REVIEW

- A production or staging process cannot use `NoopVerifier`; missing
  credentials, unknown verifier names, missing product mappings, missing Apple
  environment, and missing Play service-account configuration fail closed.
- Apple claims are not read as authority until the certificate chain and JWS
  signature pass. A sandbox receipt cannot be accepted by a production config.
- Google tokens are not trusted because they are syntactically non-empty; the
  purchase is fetched from Google and mapped to a known product.
- Receipt ownership and expiry remain enforced by the existing subscription
  store and `models.Subscription.Entitled` rule.
- A trial is never inferred from registration, can be consumed once, cannot
  replace an existing paid entitlement, and cannot be converted into paid
  access without a verified store purchase.
- The trial endpoints require the existing user authentication middleware and
  mutating operations retain idempotency wrappers.

## DOCUMENTATION

- `docs/BILLING.md` is the provider and entitlement contract.
- `docs/PROJECT-STATUS.md` records G-29 as closed, the 66/77 live schema count,
  the 85 Dart assertions, and the 306/240/306 route contract.
- `design/ia.json` names all new client dependencies without round-tripping the
  JSON file.
- `clients/dart` exposes the trial start, status, and conversion methods and a
  `TrialStatus` model.

## EXIT CRITERIA

- [x] Provider verification is real Apple JWS plus Google Play API evaluation;
      proving command: `go test ./internal/billing -count=1`.
- [x] Production refusal coverage remains; proving command:
      `go test ./internal/billing -run 'TestProductionRefusesStubReceipts'`.
- [x] One trial projection writer and one entitlement decision are documented
      and tested; proving command: `go test ./internal/trial ./internal/store
      ./internal/api -run 'TestTrial' -count=1`.
- [x] Trial state is persistent, one-time, clocked, and terminal; proving
      command: `go test ./internal/store -run 'TestTrialLifecycle' -count=1`.
- [x] Schema counts are reconciled at 66 tables / 77 foreign keys; proving
      command: `go test ./internal/db -run 'TestPostgresSchemaLoads'`.
- [x] Live route export and generated OpenAPI are 306/240/306; proving command:
      route export followed by `go run ./cmd/genspec` and the count probe.
- [x] `python3 design/test_ia.py` and `python3 scripts/check_dart_symbols.py`
      pass.

## VERDICT

**PASS — `G-29 (PHASE 36: production receipt verification and one trial
entitlement authority)` is closed.**
