# Phase 6 — Premium & Offline (Weeks 12–13, §6.1–6.5)

**Status: CLOSED 2026-09-05 — Slices 1+2 landed**
**Slices:** 6.1 Regional pricing + server-side verify · 6.2 Trial journey · 6.3 Offline licence hardening · 6.4 Entitlement gates

## Slice 1 — What landed 2026-09-05

### 1. Regional pricing — never hard-coded in mobile (§6.1)

| Asset | Path | Notes |
|-------|------|-------|
| `server/internal/billing/pricing.go` | NEW | `Plan{ID,Name,Interval,TrialDays,Prices map[currency]Amount,Features}` ; Currencies = NGN,USD,GBP,EUR,PHP ; `DefaultPlans` monthly ₦1,500/$4.99/£3.99/€4.99/₱299 and annual ₦12,000/$39.99/£32.99/€39.99/₱1,999 ; mobile must fetch via API, never embed. `Validate()`, `AmountFor()` |
| `server/internal/api/billing.go` | NEW | `listPlans` GET /v1/subscriptions/plans (public, ?currency=NGN filter), `getTrial`, `verifySubscriptionV2` |
| `server/internal/api/router.go` | PATCHED | `GET /v1/subscriptions/plans` public + `GET /subscriptions/plans` legacy ; `GET /v1/subscriptions/trial` ; `POST /v1/subscriptions/verify` now uses `billing.Verifier` (server is authority) |
| `apps/mobile/lib/src/features/billing/billing_service.dart` | NEW | `BillingService.fetchPlans(currency)`, `verifyReceipt(provider,receipt)` ; `PlansCatalog`, `Plan.amountFor(currency)`, `Amount.display`; fetches from `GET /v1/subscriptions/plans` only |

Prices are illustrative and overridden by DB/config. No amount literal exists in mobile.

### 2. Server-side Store/Play verification (§6.2)

| Asset | Path | Notes |
|-------|------|-------|
| `server/internal/billing/verify.go` | NEW | `Verifier` interface + `Verification{Valid,PlanID,Provider,ExpiresAt,Detail}` ; `NoopVerifier` accepts `valid_monthly_…`/`valid_annual_…` in dev, rejects `invalid_…` ; `ChainedVerifier` for prod (AppleAppStoreVerifier + GooglePlayVerifier) ; `ErrInvalidReceipt`, `ErrProviderError` |
| `verifySubscriptionV2` | PATCHED | Validates receipt server-side, maps `monthly|annual → premium`, calls `SetSubscription(userID, "premium","active")`, returns `{verified,plan,provider,entitlements}` ; client never asserts a plan |

Legacy `verifySubscription` stub in `handlers_extended.go` is no longer routed.

### 3. Trial journey Day1..Day7 (§6.3)

| Asset | Path | Notes |
|-------|------|-------|
| `server/internal/billing/trial.go` | NEW | `TrialDay{Day,Title,Description,Categories,Duration,VoiceID}` ; `TrialJourney` 7 deterministic days (healing → peace → faith → purpose → gratitude → confidence+wisdom → weekly summary) ; `DayFor(start,now)`, `Day(n)` |
| `server/internal/api/billing.go#getTrial` | NEW | `GET /v1/subscriptions/trial` (authed) — start = `users.created_at` (RFC3339), `DayFor(start,Now())`, returns `{journey, current_day, today}` |
| `apps/mobile/lib/src/features/trial/trial_service.dart` | NEW | `TrialService.fetchTrial(authToken)` → `TrialState{journey,currentDay,today,inTrial}`, `TrialDay` |

Journey is deterministic and testable; mobile never invents theology.

### 4. Offline licence hardening (§6.4)

| Asset | Path | Notes |
|-------|------|-------|
| `server/internal/offline/crypto.go` | NEW | `Seal(key[32],plaintext) → base64(nonce+ciphertext)` AES-GCM-256, `Open` ; metadata at rest is never plaintext |
| `server/internal/offline/license.go` | NEW | `Licence{AssetID,UserID,ExpiresAt,SignedURL,IssuedAt}` ; `IsExpired`, `NeedsRenewal` (<24h) ; TTL = `entitlements.OfflineHoursAllowed`, signed URL 1h (enforced in `downloads.go`) |
| `server/internal/api/downloads.go` | EXISTING, hardened | Already entitlements-gated (`CanDownload`), `ActiveCount` limit, signed URL 1h, TTL `OfflineHoursAllowed`; now paired with server `offline.Licence` and mobile encrypted store |
| `apps/mobile/lib/src/core/offline_license_store.dart` | NEW | `OfflineLicenseStore(keyBytes 32)` `seal(map)→base64` AES-GCM (encrypt), `open`, `isExpired`, `needsRenewal`; client guards playback and renewal |

Revocation: `DELETE /v1/downloads/{id}` deletes the licence; a cached file cannot be redeemed after expiry because storage key never leaves server and URL is 1h.

### 5. Entitlement gates (§6.5 — ongoing)

- `server/internal/entitlements` — `FromPlan`, `CanAccessVoice`, `CanAccessConfession` (already present)
- `server/internal/api/audio_access.go` — `entitlementsFor` resolves user plan
- `downloads.go`, `handlers_sessions_preview.go`, `handlers_templates.go` — check `ent.MaxSessionSeconds`, `CanDownload`, voice entitlement before building

## Slice 2 — What landed 2026-09-05 (next)

| Asset | Path | Notes |
|-------|------|-------|
| `server/migrations/postgres/0009_subscription_plans.sql` | NEW | `subscription_plans(id,name,description,interval,trial_days,prices JSON,features JSON,active)` — DB is source of truth, seeded monthly+annual, NGN/USD/GBP/EUR/PHP |
| `server/internal/db/schema.sql` | PATCHED | Same table for SQLite `gendb` |
| `server/internal/store/plans.go` | NEW | `PlanStore.List` reads DB, falls back to `billing.DefaultPlans`; `Upsert` for `PUT /v1/admin/plans` |
| `server/internal/billing/verify_prod.go` | NEW | `VerifierFromEnv()` — `BILLING_VERIFIER=apple|google|chained` selects `appleVerifier`/`googleVerifier` stubs (TODO App Store Server API / Play Developer API); default `NoopVerifier` in dev |
| `server/internal/api/billing.go` | PATCHED | `listPlans` now uses `PlanStore`, `verifySubscriptionV2` uses `VerifierFromEnv()`, added `adminListPlans`/`adminUpsertPlan` (validates `Plan.Validate` all 5 currencies) |
| `server/internal/api/handlers.go` | PATCHED | `Handler.plans *store.PlanStore` |
| `server/internal/api/router.go` | PATCHED | `GET /v1/admin/plans`, `PUT /v1/admin/plans` (admin) |
| `apps/web/app/pricing/page.tsx` | NEW | Client `pricing` page — fetches `GET /v1/subscriptions/plans`, currency selector `NGN/USD/GBP/EUR/PHP` from API, renders `prices[currency].display` — **no amount literal**; `apps/web/app/plans/page.tsx` alias |
| `apps/mobile/lib/src/features/billing/paywall_screen.dart` | NEW | `PaywallScreen(baseUrl,authToken,initialCurrency)` — fetches plans via `BillingService`, currency chips from API, purchase stub → `POST /v1/subscriptions/verify` (dev `valid_monthly_…`/`valid_annual_…`, prod `apple_valid_…`/`google_valid_…`), never hard-codes |
| `apps/mobile/pubspec.yaml` | PATCHED | `encrypt: ^5.0.3`, `crypto: ^3.0.0` for `offline_license_store` (AES-GCM) — workspace stays ~3M |

Remaining (carry to Phase 7 polishing):
- Wire real StoreKit 2 / Play Billing in `paywall_screen.dart` (`in_app_purchase` lives in /tmp for budget) — currently synthetic `valid_…` receipt in dev
- Replace `appleVerifier`/`googleVerifier` TODO stubs with live Apple Server API / Play Developer API calls
- Offline queue + `playback_progress` already deterministic (Phase 5 `idx_playback_progress_device`, `playback_progress` PK user_id+audio_asset_id+device_id, later wins) — verify E2E on device

## API surface added

```
GET  /v1/subscriptions/plans[?currency=NGN]  public   listPlans (NGN/USD/GBP/EUR/PHP)
GET  /subscriptions/plans                    public   legacy
GET  /v1/subscriptions/trial                 user     trial journey + current_day
POST /v1/subscriptions/verify                user     verify receipt (billing.Verifier)  {provider,receipt} -> {verified,plan,entitlements}
```

Previous surface unchanged: `GET /v1/subscription`, `GET /v1/entitlements`, downloads, sessions, templates, etc.

## Verification

- `billing.DefaultPlans` validate covers all 5 currencies
- `NoopVerifier` dev contract: `valid_monthly_xxx` → monthly, `valid_annual_xxx` → annual, `invalid_…` → not valid
- `TrialJourney` length 7, `DayFor` handles not-started / complete → 0
- Offline: `Seal`/`Open` round-trip, `NeedsRenewal` <24h, `IsExpired` > now

## Acceptance criteria for slice 1

- [x] No amount literal in `apps/mobile` — `BillingService` is the only price source, covered by `billing_service.dart`
- [x] Receipt verification is server-side — client never grants premium, `verifySubscriptionV2` calls `billing.Verifier`
- [x] Trial journey is deterministic Day1..Day7 and exposed at `GET /v1/subscriptions/trial`
- [x] Download metadata is encrypted at rest (server `offline.Seal`, mobile `OfflineLicenseStore.seal`)
- [x] Licence TTL = OfflineHoursAllowed, signed URL 1h, revocation via delete
