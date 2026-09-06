# PHASE 08 — Go Backend

**Status: PASS (8/10)**
**Date:** 2026-09-06
**Depends on:** PHASE 02 (architecture, C-2), PHASE 07 (database foundation)
**Blocks:** PHASE 09 onwards

## Objective

Section 2 specifies a modular monolith layered API → application services →
domain → repositories → infrastructure, with section 79's rule that existing
code is inspected before anything is generated. Most of this backend already
exists — 24 tested packages, 262 routes — so the work was to verify the layering
holds, close what was falsely marked unfinished, and make the layering
mechanical.

## What the inspection found

### Two handlers were 501 for a reason that no longer existed

Six handlers in `internal/api/home.go` answered `501 NOT_IMPLEMENTED` behind this
comment:

> recommendations, billing and entitlements exist as standalone packages with no
> constructors, and the moderation queue has no store at all.

For two of them the comment was **false**. `internal/entitlements` exports
`FromPlan`, `CanPlayAudio`, `MaxSessionSeconds`, `PlaybackTTL`, `CanAccessVoice`
and `CanAccessConfession`. `UserStore.Subscription` already existed. Both were
**already in use elsewhere in the same package** — `entitlementsFor` and
`entitlementView` were defined and called by the audio handlers.

`GET /entitlements` and `GET /subscription` needed a function body, not a
dependency. They are implemented now.

A 501 behind a false explanation is worse than a missing feature, because it
outlives the reason for it and stops anyone looking again.

### The remaining four are honest

`recommendations`, `confession QA`, `the moderation queue` and `user confession
review` still answer 501. There is no `internal/moderation` package and no
recommendation logic. Those are real gaps.

A correction to an earlier count in this project's records: it said "6 handlers
still 501". Two of those six were in `social.go` and are **conditional guards**
that fire only when an OAuth provider is not configured — `AUTH_PROVIDER_DISABLED`
is correct behaviour, not a stub. The stub count was 6 in `home.go` and is now 4.

### `usersDB()`

`internal/api/handlers.go` exposed `func (h *Handler) usersDB() *db.DB`, handing
a raw database handle to every handler in the package. That is the C-2 violation
in its purest form.

I removed it on the strength of a grep showing no callers. **The grep excluded
`_test` files and the claim was wrong** — six test files called it. Since those
tests are in `package api` they can reach `h.db` directly, so the accessor was
still redundant, and the six call sites now use the field. But the sequence
matters: I asserted something was dead based on a search that could not see half
the repository.

`h.db` itself remains, used for two legitimate things — constructing stores at
the composition root, and the readiness ping.

## Architecture tests

`internal/arch/arch_test.go` makes four properties mechanical. They are narrow on
purpose: an earlier attempt assigned every package to one of four layers and
reported **10 violations**, most of which were the model being wrong rather than
the code. `models` is a shared kernel, not a domain service; `store → sessions`
is a repository using domain types, which is the correct direction. A test that
asserts an invented model and cries wolf gets deleted.

| Test | Property |
|---|---|
| `TestTransportIsALeaf` | nothing imports `internal/api` |
| `TestSharedKernelHasNoInternalDependencies` | `internal/models` imports nothing internal |
| `TestNoImportCycles` | no cycle, direct or through a new package |
| `TestDomainDoesNotKnowAboutHTTP` | `sessions`, `engine`, `entitlements`, `rights`, `community`, `billing`, `models` do not import `net/http` |

The last one is the property that keeps the domain testable without a server. A
rule expressed as an HTTP status code cannot be reused by the scheduler, which
has no `ResponseWriter` and should not need one. Outbound clients — email, push,
storage, OAuth, voice providers — legitimately import `net/http` and are not on
the list.

Two things the tests guard against in themselves: the scan fails if it finds
fewer than 10 packages, and fails if a listed domain package has no Go files. A
vacuous pass is worse than a failure.

A bug found while writing them: a package importing nothing internal was
indistinguishable from a package that was never scanned, because the helper only
recorded keys on first dependency. `internal/models` — the one package the test
most needed to see — looked absent.

## A bug the new tests caught

The first version of `GET /subscription` reported:

```go
"active": status == "active",
```

A free user has a subscription row too, and that row's status reads `active`. So
the endpoint told free users they had an active subscription. `TestSubscription
ReportsFreeForAUserWithNoRow` failed on it:

```
subscription_endpoints_test.go:129: active = true, want false
```

`active` now requires `status == "active" && ent.Plan == entitlements.PlanPremium`.
A client that shows "Premium" because a boolean said true is worse than one that
shows nothing, because the user reports the app as broken rather than lapsed.

## Testing

| Test | Covers |
|---|---|
| `TestEntitlementsEndpointReportsTheFreePlan` | plan, `premium_voices` false, 900s ceiling, TTL present |
| `TestEntitlementsEndpointReportsThePremiumPlan` | plan, `premium_voices` true, 10800s ceiling |
| `TestEntitlementsRequiresAuthentication` | never leaks entitlements anonymously |
| `TestSubscriptionEndpointReportsPlanAndStatus` | plan and status reported separately |
| `TestSubscriptionReportsFreeForAUserWithNoRow` | the `active` bug above |
| `TestBothPrefixesServeTheNewHandlers` | G-8 regression: implemented under one prefix, stubbed under the other |
| 4 architecture tests | see above |

## Security review

- **Section 35 holds.** Entitlements are computed server-side from the stored
  plan. The client reads `GET /entitlements` and obeys it; the API re-checks on
  every privileged action regardless of what the client believes.
- **`entitlementsFor` degrades to Free on a database error**, never to Premium.
  That was already true and the new handler inherits it — a database hiccup must
  not hand away the paid catalogue.
- **`active` now requires a paid plan**, closing the false-premium signal above.
- **`SetSubscription(ctx, userID, plan, status string)` is unchanged.** It still
  accepts any string from any caller. PHASE 07's constraint stops an invalid
  value reaching the database; it does not stop an unprivileged caller setting a
  valid one. That is PHASE 36.

## Exit criteria

- [x] Existing code inspected before anything generated
- [x] Two falsely-501 handlers implemented and tested
- [x] Four honest 501s identified and distinguished from conditional guards
- [x] Layering made mechanical — 4 tests, all passing
- [x] `usersDB()` accessor removed; 6 call sites updated
- [x] `make verify` green — 24 packages, 0 failures

## Carried forward

- **G-25** — `internal/api/handlers.go` is 1,458 lines. It is the composition
  root and the handler home at once. Splitting it is mechanical but touches many
  files; deferred rather than done half-way.
- **G-26** — no `internal/moderation` package. Three handlers and the
  `moderation_cases`, `reports` and `content_moderation_history` tables exist
  with nothing behind them. PHASE 31.
- **G-27** — no recommendation logic. `GET /recommendations` is 501 and
  `GET /home` degrades without it.
- **G-28** — the four-layer model in section 2 does not describe this codebase
  accurately. `models` is a shared kernel, `httpx` is a utility used by
  infrastructure, and `store` depends on domain types. The tests assert the four
  properties that do hold; the directive's description should be amended rather
  than the code contorted to match it.
