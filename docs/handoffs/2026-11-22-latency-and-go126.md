# Handoff — request latency metrics and the Go 1.26 move

**Date:** 2026-11-22 · **Branch:** `claude/dreamy-faraday-7th0my` (restarted from `main` at `fd1da9a` after PR #95 merged) · **Commits:** `47f06ff`, `2266a19` · **PR:** not opened yet

## Goal

Continue from `2026-11-21-ai-parser.md`: pick up the open items that need no
product decision. That meant the p99 latency metric, then the Go
vulnerability gate (VE-018).

## What changed

**Request latency (`47f06ff`)**
- The p99 "TODO" was in `server/internal/metrics`, a package nothing
  imported, so the API never measured latency at all. That package is deleted.
- `server/internal/api/latency.go`: a lock-free fixed-bucket histogram (5 ms
  to 10 s, then +Inf). The existing access-log middleware feeds it
  (`middleware.go`), so every request is timed once.
- Health probes, `/metrics` scrapes, `/media/` delivery and
  `…/voices/stream` live previews are excluded, because they would distort
  the percentiles.
- `/metrics` (admin) JSON now has `latency.{count, errors_5xx, mean_ms,
  p50_ms, p95_ms, p99_ms, buckets}`. The quantiles are estimated from the
  buckets the way PromQL's `histogram_quantile` does.
- The Prometheus text (`Accept: text/plain` or `?format=prom`) now carries the
  `http_request_duration_seconds` histogram and `http_requests_5xx_total`.
  Auth counters are sorted. Before, this branch printed only auth counters,
  and nothing at all when they were unset.

**Go 1.26 and a blocking vulnerability gate (`2266a19`)**
- Reviewed `govulncheck` (v1.10.0) results:

  | Go version | Reachable stdlib vulnerabilities | Note |
  |---|---|---|
  | go1.25.0 | 40 | The version `go.mod` named |
  | go1.25.14 | 12 | Newest 1.25 patch; all fixed only in 1.26.x |
  | go1.26.11 | 0 | |

  Three unreachable findings were in `golang.org/x/crypto` v0.55.0.
- Changes:
  - `go.mod`: `go 1.26.0`, x/crypto v0.59.0 (which itself requires Go 1.26)
  - CI: `1.26.x`
  - `server/Dockerfile`: `golang:1.26-alpine`
  - The govulncheck step is blocking, with the scanner pinned at v1.10.0
- Docs: Makefile hint, `docs/PROJECT-STATUS.md`,
  `docs/03-TECHNOLOGY-DECISIONS.md`. VE-018 is marked closed in
  `docs/OPEN-WORK-RECONCILIATION.md`. Historical reports that mention 1.25
  were left as written.

**Also observed:** all 7 CI checks passed on PR #95, including Flutter and
Dart, which can't run here.

## Verification

On go1.26.11, local PostgreSQL 16 and Redis 7:

| Check | Result |
|---|---|
| `govulncheck ./...` | No vulnerabilities found |
| `go build`, `go vet`, gofmt | Clean |
| The CI `r2staging` compile check | Passes |
| `go test -race ./... -count=1` | No failures |
| golangci-lint v2.13.0, built here with go1.26.11 (the CI version) | 0 issues |

The local 2.5 binary can't load Go 1.26 code, so it wasn't used.

New tests are in `server/internal/api/latency_test.go`:
- quantile interpolation on known data
- the +Inf bound
- which routes are excluded
- an end-to-end test that drives real requests and reads them back from
  `/metrics` in both formats, and checks that scraping doesn't count itself

**Not verified here:**
- The `golang:1.26-alpine` Docker build. There's no Docker here; the CI
  "Container and deployment checks" job builds it.
- Flutter and Dart. Nothing changed for them.

## Open items

- **Needs a decision before work can start:**
  - **G-48, trial journey:** should Days 2, 5 and 6 build the sessions
    automatically or guide the user to do it? See
    `docs/41-TRIAL-ENGAGEMENT.md`.
  - **G-33, breached passwords:** which privacy-preserving source to use (for
    example a k-anonymity range API or a bundled hash list), and what to do
    when it's unreachable.
  - **`PRIVACY.md`:** wording for the Claude parser (carried over).
- **Carried over:**
  - One live staging request with `AI_PROVIDER=anthropic`.
  - No client calls `/ai/parse` yet.
  - Stray root `android/` and `ios/` directories.
  - Out-of-date status files at the repo root.
- **Operational:** a scraper can now compute fleet-wide quantiles from
  `http_request_duration_seconds`. The JSON p50/p95/p99 cover one instance
  only.
- **Blocking gate:** a newly published Go vulnerability that the code can
  reach will turn CI red until the toolchain or module is upgraded. That's the
  intended behaviour of the gate.

## Picking it up

Open a PR for `47f06ff` and `2266a19` and watch the "Container and
deployment checks" job: it is the first build of the `golang:1.26-alpine`
image. Then take G-48 or G-33 once their decisions are made.
