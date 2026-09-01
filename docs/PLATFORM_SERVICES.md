# Email, Redis, social login, avatars & the user library

This slice worked through the outstanding backlog. **278 tests pass**, `go vet`
and `gofmt` clean.

---

## 1. Email delivery (§57, §58) — was non-functional

Tokens were generated correctly and handed to a sink that did nothing, so
verification and reset never worked end to end. Now:

- **`internal/email`** — templates, a `Sender` interface, a Postmark adapter,
  and an async queue.
- **Delivery is never inline.** `Enqueue` returns immediately and cannot block a
  handler; if the buffer is full the message is dropped and counted rather than
  stalling an HTTP request behind a struggling provider. A test asserts
  registration does not wait on a 2-second provider, and another asserts
  registration still succeeds during a total mail outage.
- **Retries are classified.** 429/5xx retry with capped exponential backoff;
  401/403/4xx do not — repeated hard bounces damage sender reputation for every
  other user.
- **Registration now actually sends a verification link**, and the emailed token
  verifies the account (tested end to end by parsing the link out of the
  message body).

Templates avoid two traps: the token appears only in the link, never in a
subject or log; and **security alerts contain no actionable link**, because
security mail that can be acted on directly is itself a phishing template.

### Bonus defect: registration leaked account existence

`POST /auth/register` returned **409 for an existing address**, making it an
enumeration oracle. It now returns a neutral 200 with no session token, and
emails the *real owner* that someone tried to register with their address. The
pre-existing test asserting 409 was updated — that assertion was encoding the
bug.

---

## 2. Redis rate limiting (§20)

The in-memory limiter was correct for one process and silently wrong for two:
N replicas would each allow the full burst, multiplying every limit by N.

- **`ratelimit.Distributed`** shares counters through a `Store`.
- A minimal RESP client rather than a Redis library — rate limiting needs
  exactly `INCR` + `EXPIRE NX`, pipelined into one round trip. Taking a large
  dependency for 1% of its surface wasn't worth it; `Store` is the seam if that
  changes.
- `EXPIRE ... NX` matters: without `NX`, steady traffic keeps pushing the TTL
  forward and the window never resets.
- **Fails open to local enforcement.** If Redis is unreachable the limiter
  degrades to per-instance rather than refusing all logins. A rate limiter that
  takes the site down when its backing store fails is worse than one that
  briefly allows N× the limit. `Degraded()` exposes this as a metric.

---

## 3. Social login (§36, §37, §38, §64)

**`internal/oauth`** verifies Google and Apple identity tokens server-side.
14 tests, because the failure mode is total — a verifier that wrongly accepts
lets anyone sign in as anyone. Covered: `alg:none`, foreign signing key,
tampered payload, wrong audience, wrong issuer, expired, missing subject.

Two decisions worth flagging:

- **An unconfigured audience fails closed.** An empty client id disables the
  provider rather than accepting tokens minted for *any* application.
- **The join key is `(provider, subject)`, never email.** Emails change hands
  and Apple issues per-app relay addresses, so keying on email would eventually
  merge two different people into one account.

### Account linking is where takeover lives

| Situation | Behaviour |
|---|---|
| Known `(provider, subject)` | Sign in |
| **Verified** provider email matches an account | Link — the provider's verification is equivalent proof |
| **Unverified** provider email matches an account | **Refused, 409** — sign in first, then link from Settings |
| No match | Create a new account |

That third row is the important one: linking on an unverified address is the
classic OAuth takeover path. Also enforced: a provider identity already owned by
another user cannot be re-pointed, and the **last remaining credential cannot be
unlinked** — that would lock a user out of their own account.

---

## 4. Avatar pipeline (§10, §11)

**`internal/images`** validates by *decoding*, then re-encodes into
thumbnail/small/medium/large.

- **The client's Content-Type is ignored.** A test uploads a PHP payload
  labelled `image/jpeg`; it is refused.
- **Re-encoding strips EXIF**, including GPS coordinates a user did not know
  their phone attached. A test splices a recognisable marker into an APP1
  segment and asserts it does not survive into any variant.
- **Decompression bombs are rejected from the header.** A 12000×12000 PNG is
  tiny compressed and ~576 MB decoded, so the guard is a *pixel* budget checked
  via `DecodeConfig` before any pixels are allocated — a byte-size limit alone
  would not catch it.
- Non-square images are centre-cropped, not stretched; small sources are never
  upscaled.
- Avatar keys are **versioned**, so replacing a photo produces a new URL rather
  than serving a stale image from CDN caches for its full TTL.

`storage.ValidKey` moved from a hard-coded `audio/` check to a prefix
**allowlist** — a new caller opts in deliberately rather than writing anywhere
the path checks happen not to forbid.

---

## 5. User library (§35–§37, §45, §46, §49)

New tables (`user_collections`, `user_collection_items`,
`notification_preferences`) plus handlers for collections, devices, notification
preferences and data export.

- **Collections default to private** — including when an unrecognised visibility
  is supplied. A personal collection must never become public by omission.
- **Ownership is enforced in SQL**, not filtered in Go, so a handler that forgets
  to check returns nothing rather than someone else's data. Another user's
  collection returns **404, not 403**: 403 confirms the id is real.
- **Adding an item is idempotent** — a double-tap must not create two rows.
- **Marketing notifications default OFF**, functional ones ON. Users opt in to
  marketing, not out of it. Security notifications are deliberately absent from
  the preferences model so a compromised account cannot silence its own alerts.
- **Devices store no hardware identifiers**, and the push token is accepted but
  never echoed back — it is a delivery credential, not profile data.
- **Export excludes all security material**: password hashes, session/refresh
  tokens, one-time tokens, provider credentials. A leaked export must not be a
  credential. Tests assert no `$2a$`/`$2b$` bcrypt prefix appears anywhere, and
  that one user's export never contains another's data.

---

## Production safety

`Config.Validate()` now refuses to boot production when:

- `EMAIL_PROVIDER=log` — the dev sender **prints reset links**, which would
  write working credentials to the log stream
- `PUBLIC_BASE_URL` is `http://` — links carry one-time tokens
- `REDIS_ADDR` is unset — limits would not hold across replicas
- `POSTMARK_TOKEN` is missing while Postmark is selected

Startup logs now state plainly which subsystems are unconfigured rather than
failing obscurely later.

---

## Still outstanding

- **MFA and passkeys.** Not started. `mfa_enabled` exists on `users`; no TOTP
  enrolment, no recovery codes, no WebAuthn.
- **Push notification delivery.** Devices and preferences are modelled, but
  nothing sends to APNs/FCM.
- **Offline downloads** (§28) — `audio_downloads` exists; no handlers.
- **Account deletion cascade** (§50, §84). Deletion is not implemented; the
  per-resource retention policy needs deciding before it is.
- **Flutter and Next.js clients.** Still backend-only. `internal/webapp` is a
  vanilla-JS listener stand-in, not the specified Next.js app.
- **Durable email queue.** In-process and buffered: messages in flight are lost
  if the process dies. A Postgres outbox is the fix, and `Enqueue` is the seam.

Highest-value next: **account deletion** (a legal requirement in several target
markets and currently absent), then **push delivery** to make schedules
actually fire on a phone.
