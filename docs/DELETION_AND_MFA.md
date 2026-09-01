# Account deletion & multi-factor authentication

**328 tests pass**, `go vet` and `gofmt` clean.

Two items from the backlog, prioritised as asked: deletion first (a legal
requirement that was entirely absent), then MFA.

---

## 1. Account deletion (§40, §50, §84, §85)

### The policy is explicit and enforced by a test

`internal/deletion/policy.go` names **every** table holding user data and states
what happens to it: `Erase`, `Anonymise`, or `Retain`. `Retain` and `Anonymise`
must carry a written reason — an unexplained retention is indistinguishable from
an oversight.

`TestEveryUserTableHasAPolicy` parses the schema, finds every table referencing
`users(id)`, and fails if any lacks a policy. **A table added in six months
without a deletion rule breaks the build** rather than silently surviving a
user's erasure request — which is precisely the failure regulators care about.

### A real defect the mapping exposed

`consent_records` and `subscriptions` were `ON DELETE CASCADE`. Deleting a user
would have destroyed the financial record tax law requires *and* the consent
record proving the deletion itself was lawful. Both are now explicitly retained
against the tombstoned id.

| Category | Action | Why |
|---|---|---|
| Credentials, sessions, identities | Erase | An in-flight session must not outlive its account |
| Profile, preferences, interests, devices | Erase | The person themselves |
| Collections, confessions, favourites, schedules | Erase | User-created content |
| Listening history & progress | Erase | A record of what someone prayed about — usually the whole point of the request |
| QoE metrics, audio events, signed URLs | Anonymise | Aggregate reporting; no identity remains once detached |
| Subscriptions | **Retain** | Tax and accounting obligations |
| Consent records | **Retain** | Proves consent was obtained and withdrawn |

### Two-phase with a grace period

Requesting deletion **revokes every session immediately** but does not erase.
Seven days later a sweeper erases. The grace period exists because deletion is
irreversible and is a favourite move in account takeover: an attacker holding a
stolen session is cut off at once, while the owner — who has the password — can
sign in and cancel.

**A design flaw the tests caught:** my first version set the account status to
`pending_deletion`, which the login handler rejected. That made the grace period
*unusable* — nobody could sign in to cancel. Login and session validation now
admit `pending_deletion` explicitly. The account is locked, not destroyed.

### Erasure

One transaction, because a partial erasure that removes a profile but leaves
listening history is worse than a failure: it looks complete while the sensitive
data survives.

The identity is **tombstoned**, not deleted — the row survives so retained
records keep a valid foreign key, but email, password hash and display name are
destroyed. The email becomes `deleted-<ref>@invalid`: unique (two deletions must
not collide on the UNIQUE index) and clearly not a real address someone could
later register and inherit. Verified live: after erasure the address is free to
register again and the new account inherits nothing.

A `deletion_records` row survives holding timestamps, counts and the retained
categories — enough to demonstrate the deletion was honoured, containing none of
the person it concerned.

### Endpoints

| Endpoint | Notes |
|---|---|
| `GET /me/deletion` | Preview: what is deleted, what is kept and why, grace period |
| `POST /me/deletion` | Requires password **and** `{"confirm":"DELETE"}` |
| `DELETE /me/deletion` | Cancel — deliberately needs no password, so someone racing to save their account is not slowed |
| `POST /admin/users/{id}/erase` | SUPER_ADMIN only, audited, for regulator-ordered erasure |

Federated accounts have no password, so possession of a live session is the only
proof available — requiring a password they never set would lock them out of
deleting their own account.

---

## 2. Multi-factor authentication (§41, §42, §81)

### The previous endpoint was MFA in name only

`POST /auth/mfa/enable` accepted a **client-supplied secret** and never verified
a code. An attacker could enrol a factor they controlled onto someone else's
account. That endpoint is deleted, not deprecated.

### TOTP verified against the RFC

`internal/mfa` implements RFC 6238 directly — it is HMAC-SHA1 over a time
counter, and the tests check it against **the RFC's own published vectors**.
Matching the standard's answers is stronger assurance than a round-trip test,
which would pass even if the algorithm were wrong.

Skew is one step (±30s), enough for clock drift. Widening it multiplies the
guesses an attacker gets per window.

### Enrolment is two-step

The server issues the secret; the factor activates only once the user proves
they can produce a code. A one-step "enable" lets someone lock themselves out
with a mis-scanned QR code.

### Replay defence

A TOTP code stays valid for its whole 30-second window, so the accepted counter
is recorded and codes at or below it are refused. A code observed over someone's
shoulder cannot be reused. (This surfaced in testing: confirmation consumes the
current window, so a following login legitimately needs the next one.)

### Recovery codes

Ten codes, shown **once**, stored only as SHA-256 hashes — the platform
genuinely cannot show them again. The alphabet excludes `0/O/1/I/L` because
these get transcribed from paper under stress, and matching ignores case and
separators for the same reason. Each works exactly once; using one emails the
owner, since it usually means the authenticator is gone.

Recovery matters more than the factor itself: phones are lost far more often
than accounts are attacked, and "email us to turn it off" would quietly reduce
MFA to the security of an inbox.

### Verified live

Server-issued secret → confirm with a real TOTP → password alone returns
`mfa_required: true` **with no token** → recovery code signs in → the same
recovery code is refused on reuse.

Disabling requires the password **and** a current code: removing a second factor
is exactly what an attacker with a stolen session does first.

---

## Still outstanding

- **Passkeys / WebAuthn.** Not started. TOTP covers the MFA requirement; passkeys
  are a separate credential type needing CBOR and attestation handling.
- **Push delivery to APNs/FCM.** Devices and preferences are modelled and tokens
  are collected, but nothing sends. Scheduled sessions therefore do not fire on
  a phone — the largest remaining functional gap.
- **Offline downloads** (§28). `audio_downloads` exists; no handlers.
- **Flutter and Next.js clients.** Still backend-only.
- **Sweeper coordination.** The deletion sweep runs hourly in-process. Correct
  for one instance; at multiple replicas it needs a lock so two sweepers do not
  duplicate work. The per-account transaction makes it safe, just wasteful.

Next I would do **push delivery**: scheduling is the product's core habit loop,
and without it a scheduled 6 AM session silently never arrives.
