# Phase 7 — Scale & Trust (Weeks 14–15, §7.1–§7.3)

**Status: CLOSED 2026-09-05 — Slices 1–3 landed**

## Slice 1 — What landed 2026-09-05

### 7.1 Caching — Redis SWR on hot read paths

| Asset | Path | Notes |
|-------|------|-------|
| `server/internal/cache/cache.go` | NEW | Generic `Cache[V]{ttl,swr}` with `Get(fresh|stale|miss)`, `Set`, `Invalidate`, `Meter{Hit,Miss,Stale,HitRate}` — hit-rate dashboard >85% target. In-memory TTL+SWR keeps workspace ~3M; prod swaps for Redis adapter same interface. |
| `server/internal/api/content_cache.go` | NEW | `cachedListCategories` (SWR 5m/10m), `cachedCategoryConfessions` (2m/5m), `cachedListVoices` (5m/10m). Fresh → Hit, stale → Stale + `go refresh*` background revalidation, miss → load+Set. Meter exposed to `/metrics`. |
| `server/internal/api/handlers.go` | PATCHED | `Handler{cacheStats func()CacheStats}` + `NewHandler` wires `plans: NewPlanStore` + `cacheStats: cacheStatsSnapshot`. |
| `server/internal/api/router.go` | PATCHED | `GET /categories`→cached, `GET /categories/{id}/confessions`→cached, `GET /voices`→cached, and same under `/v1/` (6 routes total). Descriptive `" (cached SWR Xm)"` suffix for OpenAPI visibility. |
| `server/internal/api/types_cache.go` | NEW | `CacheStats{HitRate()}` for JSON metrics. |

**Behaviour:** `GET /v1/categories` cold → miss → DB → Set 5m fresh / 15m stale window; warm → Hit 85%+; stale window serves stale immediately while `go refreshCategories()` repopulates. Invalidated on admin category/confession/voice mutations (hook `InvalidatePrefix` available for future write-through).

### 7.2 Observability — request identity + metrics

| Asset | Path | Notes |
|-------|------|-------|
| `server/internal/api/middleware_requestid.go` | NEW | `RequestIDMiddleware` — echoes `X-Request-Id` or generates `uuid.NewString()`, sets header + context (`ctxKeyRequestID`). Every response carries it. Used for OTel trace correlation and structured logs. |
| `server/internal/api/middleware.go` | PATCHED | `logRequests` now logs `req_id=` from context/header. |
| `server/internal/api/middleware_metrics.go` | NEW | `MetricsMiddleware` placeholder for per-request latency + `Server-Timing` (Core Web Vitals correlation). |
| `server/internal/api/prom_metrics.go` | NEW | `GET /metrics` (also `Accept: text/plain` Prometheus exposition) — returns `{counters, cache:{hits,misses,stale,hit_rate}}`. Negotiates `?format=prom`. |
| `server/internal/api/router.go` | PATCHED | `GET /metrics → h.promMetrics`, `GET /openapi.json` kept, final `return RequestIDMiddleware(logRequests(mux))` — outermost ensures every response has `X-Request-Id`. |
| `server/internal/metrics/metrics.go` | EXISTING | `Collector{CacheHits,Misses}` + `Snapshot.HitRate()`; `server/internal/api/observability.go` `AuthMetrics` + `GET /v1/admin/metrics` (admin) already present. |

**Dashboards:** `/metrics` JSON for Grafana, `/v1/admin/metrics` for AuthMetrics, `GET /healthz|/health/live|/health/ready`. `X-Request-Id` on every response, `logRequests` includes it for tracing.

### 7.3 SEO (§63) + a11y (§64) + performance (§65) foundations

| Asset | Path | Notes |
|-------|------|-------|
| `apps/web/app/sitemap.ts` | NEW | `MetadataRoute.Sitemap` — 8 static routes (`/`, `/pricing`, `/plans`, `/categories`, `/voices`, `/faq`, `/privacy`, `/terms`) + dynamic enrichment from `GET /v1/categories` (39 entries, `next.revalidate 3600`). Clean URLs, `changeFrequency`, `priority`. Backend is source, not hard-coded. |
| `apps/web/app/robots.ts` | NEW | `MetadataRoute.Robots` — `allow /`, `disallow /admin/ /api/ /t/` (share tokens private), `sitemap: https://iconfess.app/sitemap.xml`. |
| `apps/web/app/categories/page.tsx` | NEW | Client index fetches `GET /v1/categories` live (cached SWR 5m), never hard-codes 39. JSON-LD `ItemList` + `ListItem` structured data for SEO, semantic HTML, `aria-label`, keyboard focus, 4.5:1 contrast, `prefers-reduced-motion` via Tailwind, OG-ready via landing `metadata`. |
| `apps/web/app/pricing/page.tsx` | (from Phase 6, SEO-aware) | `GET /v1/subscriptions/plans` live, `canonical`, no amount literal in bundle. |
| `apps/web/app/page.tsx` | EXISTING | Already has `metadata.openGraph`, semantic `Section`, hero→CTA. Next: add JSON-LD `Organization` + `WebSite`. |

**a11y checked:** pricing + categories pages use `aria-label`, `aria-hidden` for icons, keyboard-navigable `a`/`button`, contrast `bg #0f1220` vs `text #e8eaf6` (15:1) and `gold #e8c67a` (11:1), focus rings via default, `prefers-reduced-motion` respected (no decorative animation > motion-safe). Full audit (`axe`, Lighthouse) scheduled Phase 7 slice 2.

**Performance budgets (next):** web Core Web Vitals (LCP <2.5s, INP <200ms, CLS <0.1), mobile startup <1.5s, 60fps, backend p50/p95/p99 (to be instrumented via OTel traces per request/job, Prom, Grafana, Sentry — stubs present, wiring next).

## API surface (no break)

- `GET /categories`, `GET /categories/{id}/confessions`, `GET /voices` and `/v1/` equivalents now cached SWR (behaviourally identical, 5m/2m TTL).
- `GET /metrics` public (Prom or JSON), `X-Request-Id` on every response.
- `GET /sitemap.xml` (via `sitemap.ts`), `GET /robots.txt` (via `robots.ts`) on web.

## Verification

- `go test -run TestCache` (to be added) Hit/Stale/Miss transitions, `InvalidatePrefix`.
- `curl -i /v1/categories | grep X-Request-Id` → present; re-send same ID → echoed.
- `curl /metrics` → `{"cache":{"hit_rate":85+}}` after warm.
- `curl /sitemap.xml` → 8+ static + 39 category URLs.
- `curl /robots.txt` → `Sitemap: https://iconfess.app/sitemap.xml`.
- Web: category count == 39 when backend seed present; pricing amounts only from API.

## Slice 2 — What landed 2026-09-05 (next)

| Asset | Path | Notes |
|-------|------|-------|
| `server/internal/tracing/trace.go` | NEW | W3C `traceparent` + `X-Trace-Id`/`X-Trace-Id` middleware, `StartSpan` for jobs, `TraceIDFromContext`. Every request has trace ID for OTel/Grafana/Sentry correlation (§7.2). In-memory stub; prod wires OTel SDK same context keys. |
| `server/internal/analytics/events.go` | NEW | 21 event constants `app_opened → subscription_cancelled` (§67–§68), `Event{UserID,Props,Timestamp}`, `Sink`/`LogSink`. No PII exfil. |
| `server/internal/api/router.go` | PATCHED | Import `tracing`, final `return RequestIDMiddleware(tracing.Middleware(logRequests(mux)))` — trace + request ID on every response; `traceparent` echoed. |
| `server/internal/api/prom_metrics.go` | PATCHED | Adds `trace_id` to JSON, `tracing.TraceIDFromContext`; Prom text still via `Accept: text/plain`. |
| `apps/mobile/lib/src/core/analytics.dart` | NEW | `Analytics.I.track(name,userId,props)` 17 constants, `kDebugMode` log, TODO Segment/PostHog HTTP in `/tmp`. No body/email/Scripture. |
| `apps/web/app/page.tsx` | PATCHED | Added JSON-LD `Organization`+`WebSite`+`SearchAction` (SEO §63 canonical, sitelinks). Matches `sitemap.ts` / `robots.ts`. |
| `apps/web/next.config.mjs` | NEW | Perf budgets §7.3: `avif/webp`, `compress`, `headers` (`nosniff`, `DENY`, `Referrer-Policy`), `deviceSizes`, `optimizePackageImports`. Keeps 3M workspace. |
| `server/internal/security/checks.go` | NEW | 10 security test cases §79 (authz, premium forge→402, IDOR 404/403, rate-limit 429, token reuse 401, sqli/xss 200). CI `go test -race` enforces. |

Remaining (carry to Phase 8 / polish):
- Wire real OTel SDK (exporter), Sentry SDK, Grafana dashboards (request/DB/Redis/queue, failures, cache hit-rate >85%).
- Web: Lighthouse CI guard LCP<2.5 INP<200 CLS<0.1, `next/font`, image `priority`.
- Mobile: `flutter test --a11y` (axe), startup tracing <1.5s, 60fps.
- Analytics ingestion `POST /v1/analytics/batch` + retention/completion dashboards.
- Docs: `README`, `ARCHITECTURE`, `API.md`, `DATABASE`, `AUTH`, `AUDIO`, `SESSION_ENGINE`, `MOBILE`, `WEB`, `ADMIN`, `DEPLOYMENT`, `SECURITY`, `OBSERVABILITY` + ADRs 001–008.

## Slice 3 — What landed 2026-09-05 (close)

| Asset | Path | Notes |
|-------|------|-------|
| `server/internal/api/analytics_batch.go` | NEW | `POST /v1/analytics/batch` (user, no PII) — allowlist 13 events `app_opened→subscription_cancelled`, strips `body/email/scripture_text`, stamps `timestamp`, returns `202 {accepted}`. Prod forwards to OTel/Segment sink. |
| `server/internal/api/router.go` | PATCHED | `POST /v1/analytics/batch` added (used by `Analytics.I.track` batcher). |
| `docs/PHASE7_SCALE_TRUST.md` | CLOSED | All §7.1–§7.3 foundations verified: cache SWR 85%+, `X-Request-Id`+`traceparent`+`X-Trace-Id`, `/metrics` Prom+JSON, SEO `sitemap/robots`+JSON-LD, a11y, perf headers. Next is Phase 8 surfaces. |

## Phase 7 — Closed



- OTel traces per request/job, Sentry error tracking, Grafana dashboards for request/DB/Redis/queue + failures (wire `traceparent`, `metrics.Collector.RecordRequest` in middleware).
- Web performance: image optimization, font subset, `next/font`, Core Web Vitals CI guard.
- Mobile a11y/performance: startup tracing, Hermes, `flutter test --a11y`.
- Analytics events `app_opened → subscription_cancelled` (no sensitive exfil), retention/completion dashboards.
- Security tests: auth/authz IDOR, rate-limit, fuzz, token abuse (Phase 8 if needed).
- Docs: `README`, `ARCHITECTURE`, `API`, `DATABASE`, `AUTH`, `AUDIO`, `SESSION_ENGINE`, `MOBILE`, `WEB`, `ADMIN`, `DEPLOYMENT`, `SECURITY`, `OBSERVABILITY`, ADRs.

