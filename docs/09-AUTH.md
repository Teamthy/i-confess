# PHASE 09 — Auth

**Status: PASS WITH CONDITIONS (8/10)**
**Date:** 2026-09-06
**Depends on:** PHASE 07 (database), PHASE 08 (backend layering)
**Blocks:** PHASE 10 (users), and anything that trusts a session

## Objective

Eighteen auth routes already exist. The objective was not to build them but to
audit them against what an authentication system has to get right, fix what it
got wrong, and make the fixes stick.

## What was already correct

Worth stating plainly, because it is most of the surface:

- **bcrypt** at `bcrypt.DefaultCost`, with a documented reason for lowering it in
  tests
- **HS256 with an algorithm-confusion guard** — `ParseToken` rejects any token
  whose method is not HMAC, so `"alg": "none"` and RS256-key-confusion do not
  work
- **Constant-time comparison** for one-time tokens, TOTP codes and recovery
  codes
- **Per-account and per-IP login throttling**, with no account lockout — correct,
  because lockout lets anyone lock anyone else out
- **One generic credential error** for both "no such account" and "wrong
  password", so the endpoint cannot enumerate users
- **Ten MFA tests**, including a fatal assertion that a password alone must not
  issue a session once MFA is enrolled, TOTP replay rejection, single-use
  recovery codes, and throttling on second-factor guesses
- **Tokens bound to a server-side session**, so logout, suspension and password
  change can revoke a token that is already in a client's hands

## Finding 1 — the safety gate was inverted

`Config.Validate()` is thorough. It refuses the default JWT secret, the default
audio signing secret, the `log` email provider, `http://` base URLs, and a
missing Redis. `cmd/server/main.go` calls it and `log.Fatalf`s on failure.

It began with:

```go
if !c.IsProduction() {
    return nil
}
```

So every check ran **only when `ENV` was exactly `"production"`**. Measured
against the running code:

| `ENV` | Resolved `Env` | `JWT_SECRET` | `Validate()` |
|---|---|---|---|
| *(unset)* | `development` | `dev-only-change-me` | **nil** |
| `staging` | `staging` | `dev-only-change-me` | **nil** |
| `prod` | `prod` | `dev-only-change-me` | **nil** |
| `Production` | `Production` | `dev-only-change-me` | **nil** |
| `production` | `production` | `dev-only-change-me` | error |

`dev-only-change-me` is a literal in this repository. Anyone who has read the
source can sign a valid session token for any user, **including an
administrator**, on any deployment that did not set `ENV` to that exact string.

Forgetting or abbreviating one environment variable is not an exotic mistake.
This is the same shape as the PHASE 04 payment bypass: a safety check that fires
on one specific configuration value and is silent everywhere else.

**The gate is now an allowlist.** Unsafe defaults are permitted only in
`development` and `test`. `Env` is lower-cased on load, so `Production` and
`PRODUCTION` behave identically. An unrecognised `ENV` is rejected outright
rather than treated as permissive — `prod` now fails at boot with a message
naming the accepted values, because a typo must not silently select a more
relaxed mode.

## Finding 2 — a 30-day access token

`TOKEN_TTL` defaulted to `720h`. That is an access token valid for a month. Even
with server-side session revocation, a stolen token is usable until someone
notices, and nothing in any client needs a token that outlives a working day.
The default is now `24h`. Refresh covers longer sessions.

## Finding 3 — MFA failed open

```go
if enrolment, mErr := h.users.MFAEnrolmentFor(ctx, u.ID); mErr == nil && enrolment.Enabled {
```

Any error from the lookup made the whole condition false, and login continued
**without the second factor**. A dropped connection or a query timeout during
sign-in silently disabled 2FA for that request — an availability incident
becoming an authentication bypass, failing in the direction an attacker would
choose.

The fix is not simply to reject on error, and the first attempt proved it: four
session tests started returning 503, because `MFAEnrolmentFor` returns
`store.ErrNotFound` for a user who has never enrolled. That is a normal answer,
not a failure. The original `mErr == nil && ...` was collapsing two different
facts into one.

The two are now distinguished:

```go
if mErr != nil && !errors.Is(mErr, store.ErrNotFound) {
    // refuse: 503, no session
}
if mErr == nil && enrolment.Enabled {
    // require the second factor
}
```

A user who cannot sign in during an outage is inconvenienced. A user whose 2FA
quietly vanished is compromised.

## A wrong assertion I wrote and removed

The first version of the config test asserted that a rejected production config
still held the default secret. It failed, and it was the test that was wrong:
`Load()` populates the struct and `Validate()` then refuses to boot, and
`cmd/server` aborts on that error. The struct's contents after a rejected load
are not a security property. The assertion was replaced with one that ties the
test to the actual default, so changing `dev-only-change-me` without updating the
tests fails.

## Testing

`internal/config/security_defaults_test.go` — 6 tests, 16 passing cases:

| Test | Covers |
|---|---|
| `TestUnsafeSecretsRefusedOutsideDevelopment` | 10 `ENV` values, including `prod`, `Production`, `PRODUCTION`, `prod ` and `qa` |
| `TestEnvIsNormalised` | four casings of `production` all resolve the same |
| `TestUnrecognisedEnvIsRejectedRatherThanPermissive` | `productionn` — one character off — is refused |
| `TestTokenTTLDefaultIsShort` | the 30-day default cannot come back |
| `TestValidProductionConfigPasses` | a correct deployment still boots |
| `TestDevelopmentDefaultIsThePublishedSecret` | ties the tests to the real default |

The counterpart test matters as much as the rest: hardening that makes a correct
deployment fail to boot gets reverted within a week.

## Security review

- Token forgery via the default secret is closed for every environment except
  `development` and `test`, which are named explicitly.
- Access token lifetime reduced from 30 days to 24 hours.
- MFA cannot be skipped by a database error.
- Existing protections verified present and unchanged: bcrypt, algorithm
  confusion, constant-time comparison, enumeration resistance, throttling,
  session-bound tokens.

## Conditions on the PASS

1. **`ENV=staging` now refuses to boot without real secrets.** That is intended,
   but it is a behaviour change for any existing staging deployment and it will
   fail at startup until secrets are set. It should be announced, not discovered.
2. **`TOKEN_TTL` default changed from `720h` to `24h`.** Any client that assumed
   a month-long token will start seeing expiries. The mobile app is not built
   yet, so the window to change this is now.
3. **`SetSubscription`'s untyped signature is still open** (PHASE 36) and the
   admin subscription route still accepts an arbitrary plan string from an admin
   caller. Admin endpoints are a separate trust boundary and were not audited
   here.

## Carried forward

- **G-29** — `bcrypt.DefaultCost` is 10. OWASP's floor, not its recommendation;
  12 is a better default for a new system and costs roughly 4× per hash, which
  is irrelevant at login volume. Deferred because it invalidates nothing but
  should be decided deliberately.
- **G-30** — no password policy is enforced at registration beyond whatever the
  client sends. Not audited in this phase.
- **G-31** — refresh token rotation and reuse detection were not audited. The
  routes exist; whether a replayed refresh token is detected is unverified.
- **G-32** — admin authentication and the admin trust boundary are unaudited.
