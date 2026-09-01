# Authentication hardening & the User/Profile domain

Work against the two master prompts. This slice prioritised **the gap between
what the routes claimed and what they enforced** — most auth endpoints already
existed, but several did not do what their names implied.

## Part 1 — Six authentication vulnerabilities found and fixed

Each was demonstrated by a failing test before the fix, and the test now guards
against regression.

### 1. Logout did not log you out (critical)

The auth middleware validated the JWT signature and nothing else. `POST
/auth/logout` marked a refresh row revoked, but the **access token kept working
for its full 720-hour lifetime**. A stolen token survived logout, "log out all
devices" was cosmetic, and there was no way to end a session.

Fixed by binding tokens to a server-side session (`sid` claim) and validating it
on every authenticated request. Verified live: after logout the same token
returns `401 AUTH_SESSION_REVOKED`.

### 2. Suspension did not take effect until expiry

Account status was checked only at login, so suspending an abusive account left
them active for up to 30 days. Status is now re-read on every request.

### 3. Password change left other sessions alive

The usual reason to change a password is that someone else may have it. Other
sessions are now revoked; the caller keeps theirs, to avoid signing them out of
the device they just used. A password *reset* revokes everything, including the
caller — a reset is a recovery action where the holder of the old password may
be an attacker.

### 4. Password reset tokens were `"reset-" + userID` (critical)

Fully predictable. Anyone who learned a user id — from any endpoint that emits
one — could mint a valid reset link and take over the account. Email
verification had the same flaw (`"verify-" + userID`).

Both now use 32 bytes of CSPRNG entropy, and only a SHA-256 hash is stored, so a
leaked database yields no working links. Tokens are never returned in an HTTP
response.

### 5. Account enumeration via password reset

`POST /auth/request-password-reset` returned **404 for unknown emails and 200
for known ones**, turning it into an oracle for which addresses are registered.
Both paths now return an identical 200 and message.

### 6. Demoted admins kept their privileges

The role was read from the token, so a revoked admin retained access until the
token expired. Role is now read from the database per request. Separately,
`RoleBasedMiddleware` matched only listed roles and so **locked SUPER_ADMIN out
of every role-scoped route** — the one account you need in an incident.

### Plus: shutdown crashed

`Worker.Stop()` closed a channel that both the signal handler and a `defer`
closed, panicking on a clean shutdown. Now idempotent via `sync.Once`.

## Rate limiting (§20, §21)

There was none: login accepted unlimited guesses.

Keyed on **both** address and account. Keying only on IP lets a distributed
attack spread guesses across hosts; keying only on the account lets one host
grind through many accounts. Deliberately **throttles rather than locks** —
account lockout after N failures hands an attacker a way to lock any user out on
demand. A successful sign-in clears the counter.

| Endpoint | Limit |
|---|---|
| Login (per IP) | 20 / min |
| Login (per account) | 8 / min |
| Registration | 5 / 10 min |
| Reset & verification email | 4 / 15 min |

Verified live: `401 ×8` then `429` with `Retry-After` and `AUTH_RATE_LIMITED`.

## Session management (§30, §31, §54)

`GET /auth/sessions` lists live sign-ins with the current one marked; `DELETE
/auth/sessions/{id}` signs out one device. The revoke is scoped by `user_id` in
SQL, so guessing another user's session id achieves nothing.

Deliberately **no geolocation**. An IP-derived city is often wrong and turns a
security feature into a false-alarm generator; device and last-used time are
enough to recognise a session.

## Part 2 — The User/Profile domain

The schema already had `user_profiles`, `user_preferences` and `user_interests`
— all three completely unused by any handler. Built the domain on top.

### Explicit vs inferred interests (§15)

The distinction the prompt is emphatic about, made structural:

- `GET /me/interests` returns **two separate lists**, not one merged array, so a
  client cannot accidentally render a machine guess as the user's own words.
- `PUT /me/interests` **rejects** `AI_INFERENCE`, `LISTENING_BEHAVIOR` and
  `RECOMMENDATION` sources. A client cannot write an inference and have it read
  back as a stated preference.
- Replacing stated interests leaves inferred rows untouched, and vice versa.

### Public profile as an explicit DTO (§6, §63)

`PublicProfile` is a **separate type**, not a filtered `UserProfile`. That is
the whole point: adding a field to the internal struct must not silently publish
it. A test marshals the projection and asserts that user id, timezone, locale
and country do not appear.

### Validation (§7, §8, §73)

- **Timezone** is checked against the real IANA database, since scheduling
  depends on daylight-saving rules. Empty is rejected too — `LoadLocation("")`
  silently resolves to UTC and would quietly move a user's 6 AM.
- **Display names** accept any script (Yoruba, Arabic, Chinese, Cyrillic all
  tested). Only control characters and the bidi/zero-width ranges
  (U+202A–202E, U+2066–2069) are refused, because those enable display
  spoofing — the restriction is about deception, not about language.
- **Usernames** are ASCII-only and lowercased, with case-insensitive uniqueness
  and a reserved list. ASCII-only is deliberate: it prevents homograph
  impersonation with lookalike Unicode.
- **Avatar URLs** must be `https://` — blocking `javascript:` and `data:`.
- **Voices** are validated server-side, and a premium voice is refused to a free
  user rather than stored and silently downgraded on every future session.

### Partial updates

Every PATCH field is a pointer, so `{"bio": ""}` clears the bio while leaving
the display name alone. A value-typed struct cannot express that difference and
would blank whatever the client omitted.

### Bootstrap (§56)

`GET /me/bootstrap` returns user, profile, preferences, explicit interests,
subscription, entitlements and profile completion. It deliberately **excludes**
history, favourites and library contents — those are paginated surfaces, and
including them would make cold start scale with account age.

Profile completion is advisory: a test asserts an incomplete profile does not
block anything.

## Test coverage

**188 tests**, `go vet` and `gofmt` clean.

New this slice: session revocation, logout-all, suspension, password
change/reset, enumeration, forged tokens (`alg:none`, truncated, flipped
signature), brute-force throttling, DoS-resistance of throttling, per-device
session listing and revocation, cross-user ownership, profile/preference/
interest validation, public-projection leakage, and bootstrap shape.

## Not done — and why

These specs describe a very large surface. What remains, honestly:

- **Social login (Apple/Google), MFA, passkeys.** Not started. The identity
  model supports multiple identities per user, but no provider is wired.
- **Avatar upload pipeline** (§10, §11). Validation accepts a URL; there is no
  upload initialisation, image processing, variant generation or EXIF stripping.
- **Collections, downloads, devices, notification preferences, data export.**
  Schema exists for several; no handlers.
- **Redis.** Rate limiting is in-memory, which is correct for one instance and
  wrong the moment you run two. The `Limiter` interface is the seam.
- **Email delivery.** Tokens are generated correctly and handed to a sink;
  no transactional provider or queue is wired, so verification emails do not
  actually send.
- **Flutter and Next.js clients.** Backend only. The listener web app
  (`internal/webapp`) is a vanilla-JS stand-in, not the specified Next.js app.

Two things I'd flag as the highest-value next steps: **email delivery**, because
verification and reset are non-functional end-to-end without it, and **Redis
rate limiting**, because the in-memory limiter silently stops working correctly
as soon as the service is scaled horizontally.
