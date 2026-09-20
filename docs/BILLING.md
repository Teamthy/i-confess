# Billing and Store Receipts

How a purchase becomes premium, what the server refuses, and what an operator
has to configure. Covers `internal/billing`, `internal/playapi` and
`POST /subscriptions/verify`.

## The rule

**Premium is granted only by a verifier's verdict, and only until the store says
the paid period ends.**

1. The mobile client sends a receipt to `POST /subscriptions/verify`. It never
   sends a plan, an expiry, or a status — the request body is a provider name and
   a receipt string, and nothing else in it is read.
2. The server verifies that receipt with the provider. On the App Store that
   means an ES256 JWS signature checked against Apple's certificate chain. On
   Play it means asking the Play Developer API what the purchase token is.
3. The plan, the expiry, the store's transaction identity, the auto-renew flag
   and the lifecycle state are all taken from the store's answer and written to
   `subscriptions` (migration `0010_subscription_provider_state.sql`).
4. Every entitlement check reads that row and applies the clock: a plan is
   entitled only while its period is live (`models.Subscription.Entitled`).

The status column records what the store last said. It is not the entitlement,
because nothing rewrites it at the instant a paid period ends — a row that still
reads `active` can be months out of date. That mistake is IC-003.

## Request and response

```
POST /subscriptions/verify
Authorization: Bearer <access token>

{ "provider": "apple", "receipt": "<base64 JWS or receipt string>" }
```

```
200 { "verified": true,  "plan": "premium", "state": "active",
      "provider": "apple", "expires_at": "2027-09-20T09:33:42Z",
      "entitlements": { "plan": "premium", "premium_voices": true, ... } }

200 { "verified": false, "state": "refunded", ... }   # genuine purchase the store revoked
```

An unverified receipt is a `200` with `verified: false` when the store answered
and said no (refunded, expired, on hold). It is an error status when the server
could not get an answer at all:

| Status | Code | Meaning | What to do |
|---|---|---|---|
| `400` | `RECEIPT_INVALID` | The store rejected this receipt | Client shows a purchase failure |
| `401` | — | No access token | Sign in |
| `409` | `RECEIPT_ALREADY_REDEEMED` | This purchase is bound to another account | Support case, not an automatic retry |
| `502` | `PROVIDER_UNAVAILABLE` | Apple or Google could not be reached, or answered something unusable | Retry; alert if it persists |
| `503` | `VERIFIER_UNCONFIGURED` | This deployment has no working verifier | Fix configuration — see below |

`503` is deliberately distinct from `400`. Reporting a missing service account
as an invalid receipt sends a paying customer to support with the wrong story
and hides a deployment fault behind a client bug.

## Lifecycle states

The vocabulary is `active | trial | grace | cancelled | expired | refunded |
suspended`, and it maps onto the stores like this:

| State | Entitled | Source |
|---|---|---|
| `active` | Yes, while the period runs | App Store `expiresDate`; Play `ACTIVE` |
| `trial` | Yes, while the period runs | Introductory offers; Play `ACTIVE` on a trial line item |
| `grace` | Yes, while the period runs | Play `IN_GRACE_PERIOD` — the store has extended the expiry |
| `cancelled` | Yes, until the period ends | Play `CANCELED`, or a cancellation notification. Cancel means "do not renew", not "revoke what was paid for" |
| `expired` | No | Play `EXPIRED`; the clock passing an App Store `expiresDate` |
| `refunded` | No | Apple `REFUND`, Play voids; `revoked_` receipts in development |
| `suspended` | No | Play `ON_HOLD`, `PAUSED`, `PENDING` — billing is interrupted |

`models.Subscription.Entitled(now)` is the only place this is decided. It is a
pure function of the stored row and the clock, so it is unit-tested directly
rather than through a handler.

Two cases that look alike and are not:

- **No expiry at all** on an `active` row is a grant without a clock: an
  administrative override (`SetSubscription`), or a legacy row from before
  verification existed. It stays entitled.
- **An expiry that will not parse** is a corrupt row. It does not entitle
  anything. Treating a data error as "no clock" would turn it into a permanent
  subscription.

## Store identity and duplicate purchases

Verification records `provider` and `original_transaction_id` — Apple's
`originalTransactionId`, Play's `linkedPurchaseToken`/order lineage — and
`subscriptions` carries a partial unique index on that pair.

A receipt is a bearer token: presenting it proves the purchase happened, not
that the presenter made it. Without the binding, one genuine purchase can be
redeemed by every account that obtains a copy of the string. The handler checks
ownership before writing anything and answers `409`.

Renewals do not break this: the original transaction id is stable across
renewals of the same subscription, and the upsert keys on `user_id`, so a
renewal updates the existing row and preserves `started_at`.

## Configuration

Verification fails closed. `RequireVerification` runs at boot and refuses to
start outside development and test unless a real verifier can be built; the
process also logs which verifier it installed. In development an unconfigured
deployment logs a warning and uses the stub, because a guard that breaks the
local purchase flow gets removed.

| Variable | Purpose |
|---|---|
| `BILLING_VERIFIER` | `apple`, `google`, `chained`, or empty for the development stub |
| `APPLE_BUNDLE_ID` | Bundle id the JWS `bundleId` claim must match |
| `APPLE_ENVIRONMENT` | `Production` or `Sandbox`; the JWS `environment` claim must match |
| `APPLE_PRODUCT_MONTHLY`, `APPLE_PRODUCT_ANNUAL` | Product id to plan mapping |
| `APPLE_PRODUCT_PLANS` | Optional extra mappings, `product=plan` comma-separated |
| `APPLE_ROOT_CA_PATH` | Optional override for the pinned Apple root (rotation) |
| `GOOGLE_PLAY_PACKAGE_NAME` | Package name queried through the Play Developer API |
| `GOOGLE_PLAY_SERVICE_ACCOUNT` | Service-account JSON with Play Console access; falls back to `FCM_SERVICE_ACCOUNT` |
| `GOOGLE_PLAY_PRODUCT_MONTHLY`, `GOOGLE_PLAY_PRODUCT_ANNUAL` | Product id to plan mapping |
| `GOOGLE_PLAY_PRODUCT_PLANS` | Optional extra mappings, `product=plan` comma-separated |
| `PUBSUB_PUSH_AUDIENCE` | Audience configured on the Pub/Sub push subscription. Required for the Play webhook: without it nothing can authenticate a delivery |
| `PUBSUB_PUSH_SERVICE_ACCOUNT` | Optional sender email. Recommended — it narrows acceptance from "any Google-signed token for this audience" to "this project's push subscription" |
| `PUBSUB_JWKS_URL` | Optional override for Google's key set. Tests use it; there is no production reason to change it |

An unrecognised `BILLING_VERIFIER` is refused rather than ignored: a typo must
not silently disable verification.

## Store notification webhooks

A receipt describes a purchase. It does not describe what happens afterwards:
the renewal at 3am, the card that failed and then recovered, the cancellation
made from the App Store settings screen, the refund Apple granted after a
support call. Both stores push those events, and until this server handles them
the entitlement row is a snapshot of the last app launch.

| Endpoint | Provider | Authentication |
|---|---|---|
| `POST /v1/subscriptions/webhooks/apple` | App Store Server Notifications V2 | The JWS signature on the payload, verified against the pinned Apple Root CA — G3. No shared secret. |
| `POST /v1/subscriptions/webhooks/google` | Play Real Time Developer Notifications, delivered by Pub/Sub push | The OIDC token on the request (`iss`, `aud`, `exp`, signature against Google's key set, and optionally the sender), plus a signed call to the Play Developer API. |

Both are public routes. Their authentication is the store's signature, not a
session: the caller is a store, not a user, and a shared secret in the URL would
be strictly worse — it cannot be rotated per deployment without coordinating
with the store's console, and it proves only that the caller knows a string.

Two properties make a push stream safe to write entitlement from, and both are
enforced in `ApplyStoreNotification` rather than in the handlers:

- **Idempotency.** Both stores deliver at least once and retry until
  acknowledged. The notification's own id (Apple's `notificationUUID`, Pub/Sub's
  `messageId`) is inserted into `store_notifications` under a unique index
  before anything else in the transaction, so a duplicate inserts nothing and
  never reaches the statement that writes a subscription. The second delivery
  answers `200 {"status":"duplicate"}`.
- **Ordering.** `subscriptions.last_store_event_at` is a watermark. An event
  older than the last one applied is recorded as `stale` and refused, so
  reordered delivery cannot move a subscription backwards — a `DID_RENEW` from
  yesterday landing after today's `EXPIRED` leaves it expired.

Every delivery is recorded in `store_notifications` with its outcome
(`applied`, `duplicate`, `stale`, `unmatched`, `ignored`). That ledger is the
audit trail: "we applied REFUND at 14:02 with this notification id" is an answer
to a customer dispute, and a rotated log line is not.

A notification that arrives before any account has redeemed the purchase is
recorded as `unmatched` rather than guessed at. The stores push the moment a
subscription is bought, which can precede the app's first `POST
/subscriptions/verify`.

### Play acknowledgement

Play refunds a purchase that is not acknowledged within three days. The verify
endpoint acknowledges as soon as it sees an unacknowledged purchase, and the
notification webhook does the same for a purchase whose app never got that far.
Outside development, a missing service account is reported as an error rather
than skipped: an unacknowledged purchase is money taken and then returned three
days later, with the customer still holding the entitlement.

## App Store verification

A verified App Store receipt is a JWS with an `x5c` header: a three-certificate
chain of leaf, intermediate (WWDR) and root. The verifier:

1. requires exactly that shape and an `ES256` algorithm;
2. checks each certificate was issued by the next, with the leaf valid at the
   payload's `signedDate` rather than at the current time (a receipt signed
   while a certificate was valid stays valid after it expires);
3. requires the chain to end at Apple Root CA - G3, pinned in
   `internal/billing/appleroot.go` — 847 bytes, SHA-256
   `9a30e66217898cbc3fff23756803bb135c486b62b5d102e89338f39a3043c54f`, valid to
   2039-04-30. `APPLE_ROOT_CA_PATH` overrides it for rotation;
4. verifies the ES256 signature over the signing input with the leaf's public
   key;
5. checks `bundleId`, `environment` and `productId` against configuration.

Nothing is read from the payload before the signature is checked. Trusting the
claims is IC-003: an unsigned token naming a premium product granted premium.

## Google Play verification

`internal/playapi` calls `purchases/subscriptionsv2.get`, the only supported
endpoint — `purchases.subscriptions.get` was deprecated on 2025-05-21 and shuts
down in 2027. A purchase token from the client is not evidence; the API answer
is.

- The line item with the furthest expiry among the mapped products decides the
  plan. Unmapped products are ignored, so an old SKU does not win.
- A token Google does not recognise is `RECEIPT_INVALID`; anything else —
  network, 5xx, an unparsable expiry — is `PROVIDER_UNAVAILABLE`. The two must
  not be confused, in either direction.
- Access tokens are minted from the service account and cached, so verification
  does not cost a token per request.

## Webhooks

Server-to-server notifications (App Store Server Notifications V2 and Play
Real-time Developer Notifications) are not implemented yet; the next change adds
them on top of this state model. Today a subscription's state is refreshed when
the client verifies a receipt — on launch and on purchase — which is enough to
grant and to revoke, but a cancellation or refund reaches the account only at
the next verification.

## Development

With `BILLING_VERIFIER` unset and `ENV=development` (or `test`), the stub
verifier accepts receipts beginning `valid_`, reports `revoked_` as a refund and
rejects everything else. It fabricates an expiry (30 days, 365 for a receipt
containing `annual`) and a stable transaction id derived from the receipt, so
the expiry clock, the duplicate-receipt rule and the revocation path are all
reachable locally:

```
valid_monthly   -> premium until 30 days from now
valid_annual    -> premium until 365 days from now
revoked_monthly -> same purchase as valid_monthly, reported refunded
invalid_monthly -> verification refused
```

`valid_x` and `revoked_x` name the same purchase on purpose: a store reports one
transaction as active when it is bought and as refunded later, so a refund can be
tested without a real refund from Apple.

The stub is unreachable outside development and test — see
`TestProductionRefusesStubReceipts` and `RequireVerification`.

---

## The client: a purchase is not an entitlement

`apps/mobile/lib/src/features/premium/` runs the flow in one order, and the
order is the design:

1. **Buy.** `in_app_purchase` charges the App Store or Play. The paywall shows
   the store's own localised price, because that is what will be charged.
2. **Verify.** The receipt (iOS) or purchase token (Play) goes to
   `POST /subscriptions/verify`. Nothing on the device interprets it; the server
   is the only thing that decides what an account owns.
3. **Finish.** `completePurchase` is called *after* the server has recorded the
   entitlement.

Step 3 is where money is lost either way round. Finishing early tells the store
to forget a sale the server never saw; never finishing after a permanent
rejection makes the store replay it on every launch forever. So the rule is:
complete on success and on a permanent rejection, leave the transaction open on
anything retryable — offline, 429, 5xx, or a verification provider that is not
configured — and let the store replay it. The user is told their purchase is
safe while that happens.

Store product ids are per platform and per deployment (`ICONFESS_APPLE_MONTHLY`,
`ICONFESS_GOOGLE_MONTHLY`, … via `--dart-define`), never derived from the
catalogue plan id: the plan is what the product grants, the product id is what
the store sells, and the server maps one to the other from its own environment
(`APPLE_PRODUCT_MONTHLY`, `GOOGLE_PLAY_PRODUCT_MONTHLY`). A build whose ids do
not exist in the store says so instead of charging for the wrong thing.

Restore is present and wired (`restorePurchases` → server verification), which
App Store review requires for a non-consumable and which is the only way back for
someone who reinstalled.

### Mobile purchase integration (PR C)

The paywall resolves product ids using `ICONFESS_APPLE_MONTHLY`,
`ICONFESS_APPLE_ANNUAL`, `ICONFESS_GOOGLE_MONTHLY` and `ICONFESS_GOOGLE_ANNUAL`
Dart defines. These must match the store listings **and** server product mappings.
The displayed price/availability comes from the native storefront, not the
illustrative regional API catalogue. Trial eligibility is left to the store;
the app does not promise a trial based only on catalogue metadata.

StoreKit 2 is required (iOS deployment target 15+). The StoreKit plugin lower
bound includes JWS `serverVerificationData` and corrected pending/cancellation
callbacks. Do not enable StoreKit 1: its legacy base64 receipt is not the signed
transaction format accepted by the server. Play supplies the purchase token.
Only the backend's verified entitlement response is used to announce Premium.

The app subscribes at startup, serializes purchase callbacks, and completes a
purchase only after successful backend verification or a definitive
`RECEIPT_INVALID` response. Offline/server/authentication/account-conflict errors
leave it unfinished. “Restore purchases” is the explicit retry path; completion
failure retains the native transaction for another attempt. Cancellation clears
the busy state without granting entitlement. Storefront sandbox purchase,
cancel, restore, interrupted verification and Play acknowledgement must still be
exercised on signed native builds before release; fakes do not prove store billing.
