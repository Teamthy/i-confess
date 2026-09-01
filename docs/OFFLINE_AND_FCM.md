# Offline downloads & FCM service-account auth

**383 tests pass**, `go vet` and `gofmt` clean.

---

## 1. Offline downloads (§28, §44)

### A download is a licence, not a copy

The server cannot reach into a phone and delete a file. So offline access works
by **expiry**: every download carries a licence with an end date, and the client
must stop playing when it lapses. Renewal re-checks entitlement, which is how a
cancelled subscription eventually stops working offline without the app having
to police itself.

Premium gets 168 hours (7 days) — long enough to cover a holiday, short enough
that a lapsed subscriber loses access within a week.

### What is enforced server-side

| Rule | Behaviour |
|---|---|
| Free plan | `402` with `ENTITLEMENT_REQUIRED` — checked *before* any lookup, so error codes can't be used to probe which assets exist |
| Plan limit | Counts **live** licences, so removing a download frees a slot |
| Premium voice | Needs the premium entitlement, same as streaming |
| Unpublished audio | Refused — QA material must not escape onto a device where it can't be recalled |
| Storage key | `json:"-"` — the client gets a signed URL, never the key |

That last one matters: if the key reached the client, a lapsed licence would
still be redeemable from a record cached on the device.

The fetch URL (1 hour) is deliberately **separate** from the licence (7 days).
One says how long you have to pull the bytes; the other says how long you may
keep them.

Re-requesting a download you already hold **renews** rather than failing. A
client retrying after a dropped connection shouldn't be punished for it.

**Verified live:** free user refused → upgraded → licence issued expiring
2026-09-08 → signed URL fetched `200` → library reports 1 of 500 used.

### A real leak found on the way

`models.AudioAsset.URL` was `json:"url"` — the remote's version, which
overwrote mine during the merge. The admin generate endpoint returns the asset,
so **storage keys were being serialised to clients**. Now `json:"-"`, with
`Checksum` and `Language` restored alongside.

---

## 2. FCM service-account minting (§47)

The previous implementation took a **pre-minted OAuth2 token from the
environment**. Google's tokens expire in an hour and nothing refreshed them, so
Android push would have worked in a demo and silently died within sixty minutes
in production.

`internal/push/googleauth.go` now signs a JWT with the service-account key and
exchanges it at Google's token endpoint — the standard `jwt-bearer` grant, about
eighty lines against a stable documented protocol. That is cheaper than pulling
in `google.golang.org/api`, a large dependency tree for one grant type.

Three details worth noting:

- **Tokens are cached and refreshed 5 minutes early**, so an in-flight send
  never races the expiry boundary.
- **The mutex is held across the network call** deliberately. On a cold start
  many sends arrive at once; without it each would mint its own token, and
  Google rate-limits that. The tokens are interchangeable, so serialising costs
  nothing.
- **The project id comes from the key**, not a separate env var, so the two
  can't drift out of sync.

A test asserts that a token-endpoint failure **does not leak the signed
assertion** into the error message — the assertion is itself a credential.

Config change: `FCM_ACCESS_TOKEN` and `FCM_PROJECT_ID` are replaced by a single
`FCM_SERVICE_ACCOUNT` path.

---

## 3. Infrastructure fix

The build started failing with `no space left on device`. `/tmp` is a 993 MB
tmpfs and the Go build cache had filled it (516 MB cache + 307 MB modules).
Caches moved to `~/.cache`, which is on the 20 GB root disk and excluded from
workspace snapshots.

---

## Still outstanding: the clients

**Flutter mobile and Next.js web/admin remain unbuilt.** This is now the bulk of
what's left, and it is genuinely large — the master prompt specifies a layered
Flutter architecture (presentation → state → domain → repositories → API),
offline caching, background audio, secure token storage, plus a Next.js web app
*and* an admin console.

What exists instead:

- `internal/webapp` — a vanilla-JS listener client embedded in the Go binary.
  It exercises the full API (auth, categories, sessions, player with Media
  Session API) and is genuinely usable, but it is **not** the specified Next.js
  app.
- `internal/adminui` — a similar embedded admin console.

I'd flag a scoping question before starting: a Flutter app and a Next.js app are
each a multi-week effort with their own toolchains, and neither can be
meaningfully verified in this environment — no iOS/Android simulator, and
`node_modules` alone would exceed the workspace budget I was asked to respect.

**My recommendation:** rather than generate thousands of lines of unverifiable
client code, the higher-value next step is a **typed API client package plus an
OpenAPI specification** generated from the actual routes. That is verifiable
here, it is what both clients will consume, and it makes the eventual Flutter
and Next.js work faster and less error-prone. Say the word if you'd prefer I
attempt the clients directly and I will — I'd just want you to know the
verification limits going in.

---

## Remaining backlog

- **Passkeys / WebAuthn** — TOTP covers MFA; passkeys need CBOR and attestation
  parsing.
- **Silent/background push** to pre-download a session before its scheduled
  time, which is what would make an offline 6 AM session work on poor
  connectivity.
- **Sweeper coordination** across replicas — safe today via the occurrence key,
  just duplicated work.
- **Flutter / Next.js clients** — see above.
