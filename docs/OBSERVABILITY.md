# Observability — §66

**Request:** `RequestIDMiddleware`+`tracing.Middleware` (W3C traceparent) → `X-Request-Id`+`traceparent`+`X-Trace-Id` on every response; `logRequests` logs `req_id`.
**Metrics:** `GET /metrics` Prom or JSON `{counters, cache{hit_rate}, trace_id}` (`AuthMetrics`+`cache.Meter`), `GET /v1/admin/metrics` counters, `GET /healthz|/health/live|/health/ready` subsystems.
**Traces:** `tracing.TraceIDFromContext`, `StartSpan` for jobs; prod TODO OTel SDK exporter + Sentry + Grafana dashboards (request/DB/Redis/queue, cache hit-rate >85%, failures).
**Analytics:** `POST /v1/analytics/batch` (user, allowlist 13 `app_opened→subscription_cancelled`, strips PII, `202 {accepted}`), mobile `Analytics.I.track` debug log, web `sitemap`/`robots`+JSON-LD, `next.config` Server-Timing.
