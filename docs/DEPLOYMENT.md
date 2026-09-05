# Deployment — I CONFESS

**Env:** `NEXT_PUBLIC_API_URL`, `NEXT_PUBLIC_SITE_URL`, `BILLING_VERIFIER=apple|google|chained` (default `noop`), `AI_PROVIDER=keyword|openai|anthropic` (default `keyword`), `MEDIA_BASE_URL` (signed CDN 1h), `BILLING_VERIFIER` prod wires App Store Server API + Play Developer API (stubs in `server/internal/billing/verify_prod.go` with TODO).
**DB:** SQLite `gendb` dev → Postgres `migrations/postgres/0001..0010` (0010 community, 0009 plans, 0008 templates, 0007 search). Seed `scripts/seed.sh` 39 categories (Healing..Future+Love) never hard-coded in mobile.
**Cache:** in-memory SWR 5m/10m for `ListCategories/CategoryConfessions/ListVoices` (`server/internal/cache`) — swap to Redis adapter same interface; hit-rate `GET /metrics` >85% dashboard.
**Observability:** `X-Request-Id`+`traceparent`/`X-Trace-Id` on every response (`tracing.Middleware` + `RequestIDMiddleware`), `GET /metrics` (Prom `Accept:text/plain` or JSON `{counters, cache, trace_id}`), `logRequests` req_id, `GET /healthz|/health/live|/health/ready`, `GET /v1/admin/metrics` counters, `POST /v1/analytics/batch` allowlist no PII.
**Web:** `apps/web` Next.js 15 — `sitemap.ts` (39 + static) + `robots.ts`, `next.config.mjs` avif/webp + security headers, `pricing`/`categories` live from API (no literal).
**Mobile:** Flutter `apps/mobile` `Home·Discover·Create·Activity·Profile`, `onboarding` 90s, `paywall_screen` live pricing, `player_ui` Mini+Full, `offline_license_store` AES-GCM, `BillingService`+`TrialService`+`Analytics` no hard-code.
**Budget:** workspace 3.2M source only; `node_modules`/go cache/LLM SDK/Redis/in_app_purchase in `/tmp`.
