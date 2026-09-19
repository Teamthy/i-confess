# PHASE 30 — Premium & Paywall

**Status:** PASS
**Date:** 2026-09-19
**Depends on:** PHASE 04 (payment bypass fix), PHASE 13 (entitlements)

## Objective

Paywall with regional pricing (NGN/USD/GBP/EUR/PHP), trial journey, entitlements, subscription state. Prices from server, never hard-coded. Verification server-side.

## Implementation

### Dart client

- Added `getSubscriptionsPlans(currency)`, `getSubscription`, `getEntitlements`, `getSubscriptionsTrial`, `postSubscriptionsVerify` to endpoints.
- Added `Plan` model with `priceFor(currency)` handling both map and num shapes, `Subscription` (plan, status, active, maxSessionSeconds, isPremium), `TrialDay` (day, title, description, cta).
- Added `SubscriptionRepository`: plans(), subscription(), entitlements() (cached 5m), trial(), verifyReceipt(provider, receipt).

### Flutter

- `premium_providers.dart`: `plansProvider`, `subscriptionProvider`, `entitlementsProvider`, `trialProvider`.
- `premium_screen.dart`:
  - Current plan card (gold if premium, shows max minutes).
  - Unlock copy.
  - Entitlements chips (Premium voices, Premium content, Offline, Personal) with check/close.
  - Plans list: _PlanCard with name, interval badge (gold), description, price chips for NGN/USD/GBP/EUR/PHP via `priceFor`, features with check icons, CTA (Start trial or Subscribe).
  - Trial journey: 7 days from `GET /subscriptions/trial`, ListTile per day.
- Router: `/premium` and `/me/premium` -> `PremiumScreen`.

### Server

- `billing.go` already had `listPlans` (DB source of truth, fallback DefaultPlans, currency filter), `getTrial` (DayFor), `verifySubscriptionV2` (VerifierFromEnv, server-side authority, maps monthly/annual to premium).
- `home.go` `getSubscription` and `getEntitlements` already fixed in PHASE 08 (were 501).

### Security

- Client never asserts plan; only valid receipt grants premium (NoopVerifier in dev accepts valid_monthly_*).
- Entitlements re-checked server-side on every privileged action.

## Exit Criteria

- Plans from server with regional pricing, not hard-coded ✔
- Entitlements shown and enforced server-side ✔
- Trial journey displayed ✔
- Current plan active flag correct (free active row not shown as premium) ✔

## Verdict — PASS
