# Authentication — Definition of Done audit

**395 tests pass**, race-clean, `go vet` and `gofmt` clean.

I audited every line of the DoD against the actual code rather than from
memory. Five items were genuinely missing or broken. This documents what was
found, what was fixed, and what honestly remains.

---

## What the audit found broken

### 1. Refresh did not rotate — the biggest gap

`POST /auth/refresh` re-signed a token against the **same session id**. So
"rotation works" and "reuse detection works" were both false: a stolen refresh
token stayed valid for its full lifetime and its theft was undetectable.

Now implemented properly:

- Refresh **retires** the presented session and issues a successor, linked via
  `replaced_by` (a column that already existed and was unused).
- If a retired session is ever presented again, someone kept a copy. There is
  no way to tell whether the replay is the attacker or the victim, so **the
  entire rotation chain is revoked** and both must re-authenticate.
- The user is emailed, because being signed out with no explanation is worse
  than the theft.

Walking the chain rather than revoking all sessions is deliberate: a compromise
on one phone should not sign you out of every other device you own. A test
covers exactly that.

**Verified live:** refresh → token changes → new token `200` → replay old token
→ `AUTH_TOKEN_REUSED` → new token now `401`.

One test caught a subtlety worth recording: checking the *old* token before the
*new* one makes the assertion fail, because the check is itself the reuse event.
The implementation was right; my test ordering was wrong.

### 2. CI was silently passing by doing nothing

`.github/workflows/ci.yml` ran `go build ./...` from the repository root. The
module is in `server/`. With no `working-directory`, Go found no packages and
**every step passed without compiling anything**.

Fixed, plus:
- `go test -race` — the email queue, limiter and dispatcher are all concurrent,
  and a data race in a rate limiter is a security bug.
- `govulncheck` — fails the build, because a vulnerable auth dependency is not
  a review-later item.
- `gitleaks` — auth work touches keys constantly, which is when one slips in.

### 3. Audit logs were `log.Printf`, not records

`RecordAudit` existed in my earlier work and was **lost in the merge** with the
remote. Voice-rights changes were going to stdout only. Log retention is days;
a licence decision matters for years, and a rights dispute needs a queryable
record naming the attestation it was granted under.

Now persisted with actor, detail and result, exposed at `GET /admin/audit`.
**Verified live** — the attestation text survives into the record.

### 4. No metrics, no readiness probe

`internal/metrics` and `internal/log` existed as packages that nothing imported.

Added counters an operator would actually page on — login failures, **token
reuse**, MFA failures, rate limiting — behind admin auth, because failure counts
tell an attacker whether their attack is landing.

Health probes are now split. `/health/live` **deliberately checks nothing**: a
liveness probe that fails on a database blip makes the orchestrator restart a
healthy process and turns an outage into a crash loop. `/health/ready` checks
the database and *reports* degraded email/push without failing over them.

### 5. The test suite was too slow to be run

At production bcrypt cost the race-enabled suite took **324 seconds**. A slow
security check is one that stops being run. Tests now use `bcrypt.MinCost` via
a test-only setter; production is untouched, and a test asserts a hash made at
production cost still verifies.

324s → 134s for the api package; full race suite 2m17s.

---

## Definition of Done — honest status

### Identity
| Item | Status |
|---|---|
| Registration | ✅ with verification email, enumeration-safe |
| Login | ✅ throttled per-IP and per-account |
| Email verification | ✅ CSPRNG tokens, hashed, single-use |
| Password reset | ✅ revokes all sessions |
| Password change | ✅ revokes *other* sessions |
| Social login | ✅ Google + Apple, JWKS-verified |

### Sessions
| Item | Status |
|---|---|
| Access tokens | ✅ bound to a server-side session |
| Refresh | ✅ |
| **Rotation** | ✅ **fixed this slice** |
| **Reuse detection** | ✅ **fixed this slice** |
| Logout / logout-all | ✅ |
| Session management | ✅ list + revoke per device |

### Security
| Item | Status |
|---|---|
| Rate limiting | ✅ Redis-backed, fails open to local |
| Enumeration protection | ✅ register, login, reset all neutral |
| Password hashing | ✅ bcrypt |
| Secrets | ✅ env-only; production boot refuses dev defaults |
| **CSRF** | ⚠️ **N/A today** — see below |
| OAuth protections | ✅ audience, issuer, expiry, `alg:none` |
| Server-side authorization | ✅ |
| Admin hardening | ✅ RBAC + MFA + audit |

**On CSRF:** the API is Bearer-token only and sets no cookies, so CSRF does not
apply — a cross-site request cannot attach a header it cannot read. This becomes
required the moment the Next.js app uses cookie sessions, and I have *not*
pre-built it, because CSRF protection written against an imagined cookie scheme
is likely to be wrong.

### Admin
MFA ✅ · RBAC ✅ · Permissions ✅ (role-scoped) · Audit logs ✅

### Operations
Metrics ✅ · Structured logs ⚠️ (stdlib `log`, not JSON) · Alerts ⚠️ (counters
exist; no alerting rules — those belong in your monitoring stack) · Health
checks ✅ · CI security ✅

### Testing
Unit ✅ · Integration ✅ · Security ✅ · Abuse ✅ · E2E ⚠️ (API-level, not
through a real client) · **Mobile ❌ · Web ❌**

---

## What I cannot honestly tick

**Mobile and Web rows of the DoD.** Secure storage, startup restoration,
background/foreground handling, offline behaviour, cookie sessions, protected
routes — these are properties of Flutter and Next.js applications that do not
exist. No amount of backend work ticks them.

The backend is ready for them: session validation, rotation, reuse detection
and entitlements are all server-authoritative, which is what those clients need
to be thin against.

---

## On proceeding to the User/Profile prompt

Most of that prompt is **already built** — profile, preferences, explicit-vs-
inferred interests, collections, devices, notifications, favorites, history,
sessions, schedules, downloads, export, deletion, entitlements, bootstrap.
Sections 5–56 and 71–99 are largely done and tested.

What remains in it is substantially **§57–§62 and §100–§105: the Flutter and
Next.js clients**, plus V2/V3 personalization explicitly scoped as future work.

So the honest next step is the clients — and I want to flag the same constraint
as before rather than quietly producing unverifiable code. A Flutter app and a
Next.js app cannot be run or tested in this environment: no simulator, and
`node_modules` alone breaks the workspace budget you set. I can write them, but
I would be handing you thousands of lines I have never seen execute.

**Two options, your call:**

1. **A typed API client + OpenAPI spec** generated from the real routes —
   verifiable here, consumed by both clients, makes the real client work faster
   and removes a whole class of integration bugs.
2. **Write the Flutter app anyway**, with the explicit understanding that it is
   unverified and will need a real device pass.

I would recommend (1), then (2) as a follow-up you can actually run. But if you
want the app scaffolded now, say so and I will start on it.
