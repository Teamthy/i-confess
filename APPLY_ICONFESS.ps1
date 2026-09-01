# i-confess - apply backend files and prepare git push
# Usage:  powershell -NoProfile -ExecutionPolicy Bypass -File .\APPLY_ICONFESS.ps1 [RepoRoot]
# If RepoRoot is omitted, the current directory is used.
$ErrorActionPreference = 'Stop'

$RepoRoot = if ($args.Count -ge 1) { $args[0] } else { $PWD.Path }
$RepoRoot = (Resolve-Path $RepoRoot).Path

Write-Host "==> i-confess MVP backend - full-file write"
Write-Host "==> Repo root: $RepoRoot"

$count = 0

# ---- .gitattributes ----
$c = @'
# Normalize line endings; keep Go/SQL/Makefile as LF to avoid CRLF build issues.
* text=auto

*.go       text eol=lf
*.sql      text eol=lf
*.md       text eol=lf
*.mod      text eol=lf
*.sum      text eol=lf
Makefile   text eol=lf
.gitignore text eol=lf
.gitattributes text eol=lf

'@
$full = Join-Path $RepoRoot '.gitattributes'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f '.gitattributes', $bytes)
$count++

# ---- .gitignore ----
$c = @'
# Binaries
/bin/
*.exe

# Local database + generated media
/data/
*.db
*.db-wal
*.db-shm

# Go
/go/        # (only relevant if GOROOT is vendored in this workspace)

'@
$full = Join-Path $RepoRoot '.gitignore'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f '.gitignore', $bytes)
$count++

# ---- CONTRIBUTING.md ----
$c = @'
# Contributing to i-confess

Thanks for contributing. i-confess is a scheduled spoken biblical-confession app
(backend in Go, mobile + admin console). This guide covers the workflow everyone
should follow so the repo stays clean and reviewable.

## Table of contents

1. [Workflow at a glance](#workflow-at-a-glance)
2. [Branching model](#branching-model)
3. [Conventional commits](#conventional-commits)
4. [Pull requests](#pull-requests)
5. [Local development](#local-development)
6. [Code style](#code-style)

## Workflow at a glance

```text
main (protected)
   │
   ├── create feature branch
   │       │
   │       ├── commit small, well-scoped changes
   │       │
   │       ├── push branch
   │       │
   │       ├── open Pull Request → review → merge
   │       │
   │       └── delete branch
   └── repeat
```

**Never commit directly to `main`.** All changes land via a feature branch and a
pull request. This keeps `main` always deployable.

## Branching model

Branch names follow `type/scope`:

| Prefix   | Use for                                  | Example                    |
|----------|------------------------------------------|----------------------------|
| `feat/`  | new features                             | `feat/session-engine`      |
| `fix/`   | bug fixes                                | `fix/premium-fallback`     |
| `docs/`  | documentation                            | `docs/api-contract`        |
| `chore/` | build, tooling, non-code changes         | `chore/ci-lint`            |
| `refactor/` | restructuring without behavior change | `refactor/store-package`   |

```powershell
# 1. Start from a fresh main
git checkout main
git pull

# 2. Create and switch to a feature branch
git checkout -b feat/session-engine

# 3. ... make changes ...

# 4. Stage and commit (see conventional commits below)
git add -A
git commit -m "feat(sessions): cycle categories round-robin to fill duration"

# 5. Push and open a PR
git push -u origin feat/session-engine
```

## Conventional commits

Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/):

```text
<type>(<scope>): <short summary>

[optional body]
[optional footer]
```

Types: `feat`, `fix`, `docs`, `chore`, `refactor`, `test`, `style`, `perf`, `ci`.

Examples:

```text
feat(engine): deterministic session composition with voice fallback
fix(auth): reject tokens signed with an unexpected algorithm
docs(api): document schedule and favorite endpoints
chore(db): add canonical postgres migration for v1
test(engine): cover duration-fill and no-content paths
```

Rules:

- First line ≤ 72 characters, imperative mood ("add", not "added").
- One logical change per commit; don't bundle unrelated edits.
- Reference issues in the footer when applicable: `Closes #12`.

## Pull requests

1. Open a PR from your feature branch into `main`.
2. Fill the PR template (title, summary, test plan).
3. Request at least one review; address feedback.
4. Keep PRs small and focused — easier to review, fewer conflicts.
5. CI must pass (build + vet + tests) before merge.
6. Squash-merge when the branch contains WIP commits.

## Local development

See `README.md`. Quick start:

```powershell
go run ./cmd/server        # seeds a dev DB on first boot, listens on :8080
go build ./...             # compile check
go vet ./...               # static analysis
go test ./...              # unit tests
```

Pre-push checklist:

```powershell
go fmt ./...
go vet ./...
go test ./...
```

## Code style

- `gofmt` / `goimports` on every file (no manual alignment fights).
- Package layout follows the existing `internal/` structure:
  `api/`, `auth/`, `config/`, `db/`, `engine/`, `httpx/`, `models/`,
  `store/`, `media/`, `adminui/`, `seed/`.
- New handlers go in `internal/api/`; data access in `internal/store/`.
- Keep the SQLite (dev) and PostgreSQL (prod) schemas in sync — both live in
  `internal/db/schema.sql` and `migrations/postgres/` respectively.
- Write a test for any new non-trivial logic (see `internal/engine/engine_test.go`).

Questions? Open an issue or start a discussion.

'@
$full = Join-Path $RepoRoot 'CONTRIBUTING.md'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'CONTRIBUTING.md', $bytes)
$count++

# ---- Makefile ----
$c = @'
.PHONY: run build vet test seed-reset

GO ?= go

run:
	$(GO) run ./cmd/server

build:
	$(GO) build -o bin/iconfess ./cmd/server

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

seed-reset:
	rm -f data/iconfess.db data/iconfess.db-wal data/iconfess.db-shm
	rm -rf data/media
	@echo "Database + media reset. Next 'make run' will re-seed."

'@
$full = Join-Path $RepoRoot 'Makefile'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'Makefile', $bytes)
$count++

# ---- go.mod ----
$c = @'
module github.com/Teamthy/i-confess

go 1.25.0

require (
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/uuid v1.6.0
	golang.org/x/crypto v0.55.0
	modernc.org/sqlite v1.57.0
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/sys v0.47.0 // indirect
	modernc.org/libc v1.74.4 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

'@
$full = Join-Path $RepoRoot 'go.mod'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'go.mod', $bytes)
$count++

# ---- go.sum ----
$c = @'
github.com/dustin/go-humanize v1.0.1 h1:GzkhY7T5VNhEkwH0PVJgjz+fX1rhBrR7pRT3mDkpeCY=
github.com/dustin/go-humanize v1.0.1/go.mod h1:Mu1zIs6XwVuF/gI1OepvI0qD18qycQx+mFykh5fBlto=
github.com/golang-jwt/jwt/v5 v5.3.1 h1:kYf81DTWFe7t+1VvL7eS+jKFVWaUnK9cB1qbwn63YCY=
github.com/golang-jwt/jwt/v5 v5.3.1/go.mod h1:fxCRLWMO43lRc8nhHWY6LGqRcf+1gQWArsqaEUEa5bE=
github.com/google/pprof v0.0.0-20260802141513-ef3492d7dac3 h1:LMLX+LgTNWpfvCBdFebv6EsYotImrt/Ppc5cXIriCSo=
github.com/google/pprof v0.0.0-20260802141513-ef3492d7dac3/go.mod h1:jl5iWTm0/hd5PjEYEOuwAJ57L/CibdZfrqZ5XA5GrCk=
github.com/google/uuid v1.6.0 h1:NIvaJDMOsjHA8n1jAhLSgzrAzy1Hgr+hNrb57e+94F0=
github.com/google/uuid v1.6.0/go.mod h1:TIyPZe4MgqvfeYDBFedMoGGpEw/LqOeaOT+nhxU+yHo=
github.com/hashicorp/golang-lru/v2 v2.0.7 h1:a+bsQ5rvGLjzHuww6tVxozPZFVghXaHOwFs4luLUK2k=
github.com/hashicorp/golang-lru/v2 v2.0.7/go.mod h1:QeFd9opnmA6QUJc5vARoKUSoFhyfM2/ZepoAG6RGpeM=
github.com/mattn/go-isatty v0.0.24 h1:tGZZoVgT/KiqK1c8ocVLeDS8BSWMRd47J3Lbz7vsReI=
github.com/mattn/go-isatty v0.0.24/go.mod h1:nMCL3Zebbrt45jsMDgnfIwz6ydEQApk5oEI3HqDio6A=
github.com/ncruces/go-strftime v1.0.0 h1:HMFp8mLCTPp341M/ZnA4qaf7ZlsbTc+miZjCLOFAw7w=
github.com/ncruces/go-strftime v1.0.0/go.mod h1:Fwc5htZGVVkseilnfgOVb9mKy6w1naJmn9CehxcKcls=
github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec h1:W09IVJc94icq4NjY3clb7Lk8O1qJ8BdBEF8z0ibU0rE=
github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec/go.mod h1:qqbHyh8v60DhA7CoWK5oRCqLrMHRGoxYCSS9EjAz6Eo=
golang.org/x/crypto v0.55.0 h1:+KWHjbgOaAQ66dh/YlkZKHlz9ZUlq61AFirAR9ntP8M=
golang.org/x/crypto v0.55.0/go.mod h1:uq0V9dE/fzQuJtbnL+2EhWOE63vo164FY8xqEnV9xis=
golang.org/x/mod v0.37.0 h1:vF1DjpVEshcIqoEaauuHebaLk1O1forxjxBaVn884JQ=
golang.org/x/mod v0.37.0/go.mod h1:m8S8VeM9r4dzDwjrKO0a1sZP3YjeMamRRlD+fmR2Q/0=
golang.org/x/sync v0.21.0 h1:HLII4xRRTtCRkxYp4HNFF0Js/Og6q2i++KXbg0gHCwM=
golang.org/x/sync v0.21.0/go.mod h1:9xrNwdLfx4jkKbNva9FpL6vEN7evnE43NNNJQ2LF3+0=
golang.org/x/sys v0.47.0 h1:o7XGOvZQCADBQQ4Y7VNq2dRWQR7JmOUW8Kxx4ZsNgWs=
golang.org/x/sys v0.47.0/go.mod h1:4GL1E5IUh+htKOUEOaiffhrAeqysfVGipDYzABqnCmw=
golang.org/x/tools v0.47.0 h1:7Kn5x/d1svx/PzryTsqeoZN4TZwqeH5pGWjefhLi/1Q=
golang.org/x/tools v0.47.0/go.mod h1:dFHnyTvFWY212G+h7ZY4Vsp/K3U4/7W9TyVaAul8uCA=
modernc.org/cc/v4 v4.29.1 h1:MKgdCV3WykTSPqpVrnxdEDS0HEd2FHpKZDzxzU5LyeI=
modernc.org/cc/v4 v4.29.1/go.mod h1:OnovgIhbbMXMu1aISnJ0wvVD1KnW+cAUJkIrAWh+kVI=
modernc.org/ccgo/v4 v4.34.6 h1:sBgfIwyN0TQ9C5hwIeuqyeAKyMWnbvj2fvpF4L11uzU=
modernc.org/ccgo/v4 v4.34.6/go.mod h1:SZ8YcN9NG7XVsQYdm6jYBvi8PQP1qi+kqB6OhjqI3Fk=
modernc.org/fileutil v1.4.0 h1:j6ZzNTftVS054gi281TyLjHPp6CPHr2KCxEXjEbD6SM=
modernc.org/fileutil v1.4.0/go.mod h1:EqdKFDxiByqxLk8ozOxObDSfcVOv/54xDs/DUHdvCUU=
modernc.org/gc/v2 v2.6.5 h1:nyqdV8q46KvTpZlsw66kWqwXRHdjIlJOhG6kxiV/9xI=
modernc.org/gc/v2 v2.6.5/go.mod h1:YgIahr1ypgfe7chRuJi2gD7DBQiKSLMPgBQe9oIiito=
modernc.org/gc/v3 v3.1.4 h1:2g65LGVSmFQrXeITAw97x7hCRvZFcyE1uDP+7Vng7JI=
modernc.org/gc/v3 v3.1.4/go.mod h1:HFK/6AGESC7Ex+EZJhJ2Gni6cTaYpSMmU/cT9RmlfYY=
modernc.org/goabi0 v0.2.0 h1:HvEowk7LxcPd0eq6mVOAEMai46V+i7Jrj13t4AzuNks=
modernc.org/goabi0 v0.2.0/go.mod h1:CEFRnnJhKvWT1c1JTI3Avm+tgOWbkOu5oPA8eH8LnMI=
modernc.org/libc v1.74.4 h1:fX1Omw4o2/1C2iRkkIsrQTasJQldLhRmuPreXLoWs9k=
modernc.org/libc v1.74.4/go.mod h1:eeQAS9W3sZeKYMFubydxJpII9ybHWshk+7or7bLG9co=
modernc.org/mathutil v1.7.1 h1:GCZVGXdaN8gTqB1Mf/usp1Y/hSqgI2vAGGP4jZMCxOU=
modernc.org/mathutil v1.7.1/go.mod h1:4p5IwJITfppl0G4sUEDtCr4DthTaT47/N3aT6MhfgJg=
modernc.org/memory v1.11.0 h1:o4QC8aMQzmcwCK3t3Ux/ZHmwFPzE6hf2Y5LbkRs+hbI=
modernc.org/memory v1.11.0/go.mod h1:/JP4VbVC+K5sU2wZi9bHoq2MAkCnrt2r98UGeSK7Mjw=
modernc.org/opt v0.2.0 h1:tGyef5ApycA7FSEOMraay9SaTk5zmbx7Tu+cJs4QKZg=
modernc.org/opt v0.2.0/go.mod h1:03fq9lsNfvkYSfxrfUhZCWPk1lm4cq4N+Bh//bEtgns=
modernc.org/sortutil v1.2.1 h1:+xyoGf15mM3NMlPDnFqrteY07klSFxLElE2PVuWIJ7w=
modernc.org/sortutil v1.2.1/go.mod h1:7ZI3a3REbai7gzCLcotuw9AC4VZVpYMjDzETGsSMqJE=
modernc.org/sqlite v1.57.0 h1:qNQP6xnx5M0ISNtlnxoOX0+cD5bJ0/gr9aMmndFczzg=
modernc.org/sqlite v1.57.0/go.mod h1:yCJ2cmAaIkHQ25oXWrF8H4O1lIfPYPR26yCEDj2P3pQ=
modernc.org/strutil v1.2.1 h1:UneZBkQA+DX2Rp35KcM69cSsNES9ly8mQWD71HKlOA0=
modernc.org/strutil v1.2.1/go.mod h1:EHkiggD70koQxjVdSBM3JKM7k6L0FbGE5eymy9i3B9A=
modernc.org/token v1.1.0 h1:Xl7Ap9dKaEs5kLoOQeQmPWevfnk/DM5qcLcYlA8ys6Y=
modernc.org/token v1.1.0/go.mod h1:UGzOrNV1mAFSEB63lOFHIpNRUVMvYTc6yu1SMY/XTDM=

'@
$full = Join-Path $RepoRoot 'go.sum'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'go.sum', $bytes)
$count++

# ---- README.md ----
$c = @'
# i-confess — Backend

The backend platform for **i-confess**, a scheduled spoken biblical-confession app
(Wake → Listen → Declare → Reflect → Repeat).

This repository currently contains the **MVP backend** — a modular Go monolith that
implements the content, audio, session, scheduling, and subscription primitives
defined in the PRD (v1.0). The mobile app (React Native/Expo) and admin console are
the next layers and will consume this API.

## Stack (per PRD §71)

| Layer       | Choice                                   | Notes                                              |
|-------------|------------------------------------------|----------------------------------------------------|
| Backend     | Go (modular monolith)                    | PRD-recommended                                    |
| Database    | PostgreSQL (prod) / SQLite (dev/demo)    | Equivalent schemas; see `migrations/postgres/`     |
| Cache       | Redis                                    | Planned (not required for MVP dev loop)            |
| Storage/CDN | Object storage + CDN                     | Dev: local `data/media/` served at `/media/`       |
| Auth        | JWT (HS256) + bcrypt                     | `internal/auth`                                    |

## Project structure

```
cmd/server/           entrypoint (HTTP server, seeding, static media)
internal/
  api/                HTTP handlers + router (public / user / admin)
  auth/               JWT + bcrypt, middleware
  config/             env-based config
  db/                 SQLite open + migrate (dev schema)
  engine/             Session Engine (deterministic MVP composition)
  httpx/              JSON helpers
  media/              local placeholder WAV generator (dev only)
  models/             shared structs
  adminui/            embedded Admin Console SPA (served at /admin/)
  seed/               idempotent dev seed (content + demo users)
  store/              data access (users, content, audio, sessions, schedules, engagement)
migrations/postgres/  canonical production schema
docs/API.md           full API contract
```

## Run locally

Requires Go 1.24+ (toolchain in this workspace: `/home/user/go/bin/go`).

```bash
export PATH=/home/user/go/bin:$PATH
go run ./cmd/server
```

Defaults (override with env vars):

| Env var      | Default            | Purpose                          |
|--------------|--------------------|----------------------------------|
| `PORT`       | `8080`             | HTTP listen port                 |
| `DB_PATH`    | `data/iconfess.db` | SQLite database file             |
| `JWT_SECRET` | `dev-only-change-me` | Token signing key (SET IN PROD) |
| `TOKEN_TTL`  | `720h`             | JWT lifetime                     |
| `ENV`        | `development`      | `development` auto-seeds on boot |

### Admin Console

The Admin Console is a single-page web app embedded in the server, served at
**`http://localhost:8080/admin/`**. It manages categories, confessions, voices,
audio attachments, roles and subscriptions. Sign in with the demo admin below.

On first boot in development the database is seeded with a representative MVP
library: **2 collections** (The 28, The 38), **16 categories**, **16 confessions**
(each with 30s/1m/3m/5m variants + scripture references), **1 voice** ("Grace"),
local placeholder audio, a demo admin and a demo user.

### Demo credentials

| Role  | Email               | Password      |
|-------|---------------------|---------------|
| Admin | `admin@iconfess.dev` | `admin12345`  |
| User  | `demo@iconfess.dev`  | `password123` |

## What the Session Engine does (MVP)

Given `{categories, duration, voice}` it returns an ordered session:

1. Only **published** confessions with a **ready** audio asset for the resolved voice are used.
2. Categories are cycled **round-robin**; confessions within a category cycle and
   repeat as needed to reach the requested duration (PRD §19).
3. The **longest variant that fits** the remaining budget is preferred, so sessions
   fill to the exact requested duration.
4. A **premium voice** requested by a free user falls back to an available free voice
   (PRD §87 fallback rule).

## Testing the loop

```bash
# health
curl localhost:8080/healthz

# login
TOKEN=$(curl -s -X POST localhost:8080/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"demo@iconfess.dev","password":"password123"}' | jq -r .token)

# list categories, pick ids, build a 10-minute session
curl -s localhost:8080/categories | jq '.[] | {slug,id}'
curl -s -X POST localhost:8080/sessions \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"category_ids":["<healing-id>","<faith-id>"],"duration_seconds":600}' | jq
```

See `docs/API.md` for the complete contract.

## MVP scope vs. roadmap

Implemented now (backend + admin console): auth, users, collections, categories,
confessions, variants, scripture references, voices, audio assets, sessions +
session engine, schedules, favorites, playback history, user confessions,
subscriptions (free/premium flags), admin RBAC + audit-ready endpoints, content
lifecycle status, and an embedded Admin Console UI.

Not yet built (next layers): mobile app, async audio job/queue/worker (PRD §77),
Redis caching, observability (§79), offline download orchestration, premium
entitlement gating in the player.

## Notes / conventions

- **Dev database is SQLite** (pure-Go `modernc.org/sqlite`) so the full loop runs
  with zero external services. Production targets PostgreSQL; both schemas are kept
  structurally equivalent (`migrations/postgres/0001_schema.sql`).
- **Placeholder audio** is a locally generated sine-tone WAV so the playback loop
  can be exercised end-to-end. Real audio comes from the TTS/recording pipeline
  (PRD §17) and will live on a CDN.
- **Confession lifecycle** (`draft → … → published → archived`) and **scripture
  quote-vs-paraphrase** flags (`is_direct_quote`) are enforced in the data model
  per PRD §11–12.
- **User confessions are private by default** (PRD §22).

'@
$full = Join-Path $RepoRoot 'README.md'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'README.md', $bytes)
$count++

# ---- .github\PULL_REQUEST_TEMPLATE.md ----
$c = @'
## Summary

<!-- What does this PR do? One or two sentences. -->

## Motivation & context

<!-- Why is this change needed? Link any issue: Closes #123 -->

## Type of change

- [ ] Bug fix
- [ ] New feature
- [ ] Refactor
- [ ] Documentation
- [ ] Build / tooling / CI
- [ ] Other

## Test plan

<!-- How did you verify this? Commands, manual steps, screenshots. -->

```text
go fmt ./...
go vet ./...
go test ./...
```

## Checklist

- [ ] I have run `go fmt`, `go vet`, and `go test` locally.
- [ ] I have added/updated tests for new behavior.
- [ ] I have updated documentation (`README.md`, `docs/API.md`) where relevant.
- [ ] I have kept the SQLite dev schema and PostgreSQL migration in sync (if schema changed).
- [ ] The change follows the [contributing guide](CONTRIBUTING.md).

'@
$full = Join-Path $RepoRoot '.github\PULL_REQUEST_TEMPLATE.md'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f '.github\PULL_REQUEST_TEMPLATE.md', $bytes)
$count++

# ---- .github\ISSUE_TEMPLATE\bug_report.md ----
$c = @'
---
name: Bug report
about: Report a bug or unexpected behaviour
title: "[bug] "
labels: bug
assignees: ""
---

## Description

<!-- A clear description of the bug. -->

## Steps to reproduce

1.
2.
3.

## Expected behaviour

<!-- What should have happened? -->

## Actual behaviour

<!-- What actually happened? Include error messages / logs. -->

## Environment

- OS / platform:
- Go version (`go version`):
- Commit / branch:
- Database (SQLite dev / PostgreSQL):

## Additional context

<!-- Anything else: screenshots, curl output, stack traces. -->

'@
$full = Join-Path $RepoRoot '.github\ISSUE_TEMPLATE\bug_report.md'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f '.github\ISSUE_TEMPLATE\bug_report.md', $bytes)
$count++

# ---- .github\ISSUE_TEMPLATE\feature_request.md ----
$c = @'
---
name: Feature request
about: Suggest a new feature or improvement
title: "[feature] "
labels: enhancement
assignees: ""
---

## Problem

<!-- What problem does this solve? -->

## Proposed solution

<!-- Describe the feature. Link to the relevant PRD section if any. -->

## Alternatives considered

<!-- Any other approaches you thought about. -->

## Scope

<!-- Is this MVP, V1.1, V2, V3? See PRD roadmap. -->

## Additional context

'@
$full = Join-Path $RepoRoot '.github\ISSUE_TEMPLATE\feature_request.md'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f '.github\ISSUE_TEMPLATE\feature_request.md', $bytes)
$count++

# ---- .github\workflows\ci.yml ----
$c = @'
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

permissions:
  contents: read

jobs:
  test:
    name: Build, vet & test
    runs-on: ubuntu-latest
    strategy:
      matrix:
        go-version: ["1.25.x"]
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go-version }}
          cache: true

      - name: Verify dependencies
        run: go mod verify

      - name: Build
        run: go build ./...

      - name: Vet
        run: go vet ./...

      - name: Test
        run: go test -race ./...

      - name: gofmt check
        run: |
          files=$(gofmt -l .)
          if [ -n "$files" ]; then
            echo "The following files are not gofmt-formatted:"
            echo "$files"
            exit 1
          fi

'@
$full = Join-Path $RepoRoot '.github\workflows\ci.yml'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f '.github\workflows\ci.yml', $bytes)
$count++

# ---- cmd\server\main.go ----
$c = @'
package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Teamthy/i-confess/internal/api"
	"github.com/Teamthy/i-confess/internal/config"
	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/seed"
)

func main() {
	cfg := config.Load()

	conn, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer conn.Close()

	// Seed dev content when the database is empty (development only).
	if cfg.Env == "development" || os.Getenv("SEED") == "1" {
		if err := seed.Seed(conn); err != nil {
			log.Printf("seed: %v", err)
		}
	}

	h := api.NewHandler(api.Config{JWTSecret: cfg.JWTSecret, TokenTTL: cfg.TokenTTL}, conn)
	h.BuildEngine()

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           h.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("i-confess backend listening on :%s (env=%s)", cfg.Port, cfg.Env)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}

'@
$full = Join-Path $RepoRoot 'cmd\server\main.go'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'cmd\server\main.go', $bytes)
$count++

# ---- docs\API.md ----
$c = @'
# i-confess — API Contract (MVP)

Base URL: `http://<host>:8080`

- All bodies are JSON.
- Auth: `Authorization: Bearer <token>` (JWT).
- Errors: `{"error": "<message>"}` with an appropriate status.

## Authentication

| Method | Path              | Auth | Body                                  | Returns            |
|--------|-------------------|------|---------------------------------------|--------------------|
| POST   | `/auth/register`  | —    | `{email, password, display_name?, timezone?}` | `{token, user}` |
| POST   | `/auth/login`     | —    | `{email, password}`                   | `{token, user}`    |

`password` must be ≥ 8 chars. `timezone` defaults to `UTC`.

## Content (public, published only)

| Method | Path                            | Returns                                        |
|--------|---------------------------------|------------------------------------------------|
| GET    | `/collections`                  | `Collection[]`                                 |
| GET    | `/categories`                   | `Category[]`                                   |
| GET    | `/categories/{id}/confessions`  | `Confession[]` (published)                     |
| GET    | `/confessions/{id}`             | `Confession` (with variants + scriptures)      |
| GET    | `/voices`                       | `Voice[]`                                      |

## Profile & preferences

| Method | Path    | Auth | Returns                     |
|--------|---------|------|------------------------------|
| GET    | `/me`   | ✓    | `{user, plan}`               |

## Sessions

| Method | Path               | Auth | Body                                                  | Returns    |
|--------|--------------------|------|-------------------------------------------------------|------------|
| POST   | `/sessions`        | ✓    | `{category_ids[], duration_seconds, voice_id?}`       | `Session`  |
| GET    | `/sessions/{id}`   | ✓    | —                                                     | `Session`  |
| PATCH  | `/sessions/{id}`   | ✓    | `{status}` (`playing|completed|abandoned`)            | `{status}` |
| GET    | `/sessions`        | ✓    | —                                                     | `Session[]`|

`duration_seconds` ∈ [60, 10800]. The engine returns ordered `items`, each with
`audio_url`, `title`, `category`, `text`, and `duration_seconds`.

## Schedules

| Method | Path                | Auth | Body                                                                  | Returns     |
|--------|---------------------|------|-----------------------------------------------------------------------|-------------|
| GET    | `/schedules`        | ✓    | —                                                                     | `Schedule[]`|
| POST   | `/schedules`        | ✓    | `{label, time "HH:MM", days_of_week[], timezone, duration_seconds, voice_id?, category_ids[]?, enabled?}` | `Schedule` |
| PATCH  | `/schedules/{id}`   | ✓    | partial update of the above                                           | `Schedule`  |
| DELETE | `/schedules/{id}`   | ✓    | —                                                                     | `204`       |

`days_of_week`: `1=Mon … 7=Sun`. Scheduling is **timezone-aware**; the mobile client
is responsible for local alarm/notification delivery (PRD §27).

## Favorites

| Method | Path             | Auth | Body                                    | Returns     |
|--------|------------------|------|-----------------------------------------|-------------|
| POST   | `/me/favorites`  | ✓    | `{entity_type, entity_id}`              | `Favorite`  |
| DELETE | `/me/favorites`  | ✓    | `{entity_type, entity_id}`              | `204`       |
| GET    | `/me/favorites`  | ✓    | `?type=` optional filter                | `Favorite[]`|

`entity_type` ∈ `confession | category | session | voice`.

## History

| Method | Path          | Auth | Body                                                       | Returns            |
|--------|---------------|------|------------------------------------------------------------|--------------------|
| GET    | `/me/history` | ✓    | —                                                          | `PlaybackRecord[]` |
| POST   | `/me/history` | ✓    | `{session_id?, confession_id?, duration_seconds, completed, skipped}` | `PlaybackRecord` |

## Personal confessions (private by default)

| Method | Path               | Auth | Body                          | Returns            |
|--------|--------------------|------|-------------------------------|--------------------|
| POST   | `/me/confessions`  | ✓    | `{title, text, category_id?}` | `UserConfession`   |
| GET    | `/me/confessions`  | ✓    | —                             | `UserConfession[]` |

## Admin (requires an admin role token)

| Method | Path                            | Body / Notes                                   |
|--------|---------------------------------|------------------------------------------------|
| GET    | `/admin/stats`                  | dashboard summary                              |
| POST   | `/admin/categories`             | `{name, slug, description?, icon?, premium?, status?, sort_order?}` |
| GET    | `/admin/categories`             | all categories (incl. unpublished)             |
| POST   | `/admin/confessions`            | full confession incl. `variants[]` + `scriptures[]` |
| GET    | `/admin/confessions`            | all confessions (incl. draft)                  |
| GET    | `/admin/confessions/{id}`       | —                                              |
| PATCH  | `/admin/confessions/{id}`       | `{status}` (lifecycle transition)              |
| POST   | `/admin/voices`                 | `{name, description?, type?, provider?, gender?, language?, premium?, status?, sample_url?}` |
| GET    | `/admin/voices`                 | —                                              |
| POST   | `/admin/audio`                  | `{confession_id, variant_id?, voice_id, url, duration_seconds?, size_bytes?, status?}` |
| POST   | `/admin/users/role`             | `{user_id, role}`                              |
| POST   | `/admin/users/subscription`     | `{user_id, plan "free|premium", status?}`      |

### Admin roles (PRD §75)

`super_admin` · `content_admin` · `audio_producer` · `theological_reviewer` ·
`support_admin` · `analytics_admin`

### Confession lifecycle (PRD §12)

`draft → content_review → theological_review → audio_production → audio_qa →
approved → published → archived`

## Data model (summary)

See `migrations/postgres/0001_schema.sql` for the canonical schema. Core entities:
`collections`, `categories`, `collection_categories`, `confessions`,
`confession_variants`, `scripture_references`, `voices`, `voice_licenses`,
`audio_assets`, `users`, `subscriptions`, `session_preferences`, `schedules`,
`sessions`, `session_items`, `favorites`, `playback_history`, `user_confessions`,
`user_confession_audio`, `admin_users`, `audit_logs`.

'@
$full = Join-Path $RepoRoot 'docs\API.md'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'docs\API.md', $bytes)
$count++

# ---- migrations\postgres\0001_schema.sql ----
$c = @'
-- i-confess — canonical PostgreSQL schema (production target)
-- Mirrors internal/db/schema.sql (SQLite dev schema). Use this for `goose`/`atlas`/`golang-migrate`.

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ============================= CONTENT =============================
CREATE TABLE collections (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    description TEXT,
    premium     BOOLEAN NOT NULL DEFAULT FALSE,
    status      TEXT NOT NULL DEFAULT 'draft',
    sort_order  INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE categories (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    description TEXT,
    icon        TEXT,
    premium     BOOLEAN NOT NULL DEFAULT FALSE,
    status      TEXT NOT NULL DEFAULT 'draft',
    sort_order  INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE collection_categories (
    collection_id UUID NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    category_id   UUID NOT NULL REFERENCES categories(id)  ON DELETE CASCADE,
    sort_order    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (collection_id, category_id)
);

CREATE TABLE confessions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    category_id  UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    title        TEXT NOT NULL,
    short_text   TEXT,
    medium_text  TEXT,
    long_text    TEXT,
    description  TEXT,
    tags         TEXT[],
    intensity    SMALLINT NOT NULL DEFAULT 1 CHECK (intensity BETWEEN 1 AND 5),
    language     TEXT NOT NULL DEFAULT 'en',
    status       TEXT NOT NULL DEFAULT 'draft',
    author       TEXT,
    version      INTEGER NOT NULL DEFAULT 1,
    published_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_confessions_category ON confessions(category_id);
CREATE INDEX idx_confessions_status   ON confessions(status);

CREATE TABLE confession_variants (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    confession_id    UUID NOT NULL REFERENCES confessions(id) ON DELETE CASCADE,
    label            TEXT NOT NULL,
    duration_seconds INTEGER NOT NULL,
    sort_order       INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE scripture_references (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    confession_id   UUID NOT NULL REFERENCES confessions(id) ON DELETE CASCADE,
    book            TEXT NOT NULL,
    chapter         INTEGER,
    verse           TEXT,
    translation     TEXT NOT NULL DEFAULT 'KJV',
    is_direct_quote BOOLEAN NOT NULL DEFAULT FALSE,
    notes           TEXT,
    sort_order      INTEGER NOT NULL DEFAULT 0
);

-- ============================= AUDIO =============================
CREATE TABLE voices (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    description TEXT,
    type        TEXT NOT NULL DEFAULT 'professional',
    provider    TEXT,
    gender      TEXT,
    language    TEXT NOT NULL DEFAULT 'en',
    premium     BOOLEAN NOT NULL DEFAULT FALSE,
    status      TEXT NOT NULL DEFAULT 'active',
    sample_url  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE voice_licenses (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    voice_id         UUID NOT NULL REFERENCES voices(id) ON DELETE CASCADE,
    owner            TEXT,
    provider         TEXT,
    license_status   TEXT NOT NULL DEFAULT 'none',
    license_start    TIMESTAMPTZ,
    license_expiry   TIMESTAMPTZ,
    allowed_regions  TEXT[],
    commercial_usage BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE audio_assets (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    confession_id    UUID NOT NULL REFERENCES confessions(id) ON DELETE CASCADE,
    variant_id       UUID REFERENCES confession_variants(id) ON DELETE SET NULL,
    voice_id         UUID NOT NULL REFERENCES voices(id) ON DELETE CASCADE,
    url              TEXT NOT NULL,
    duration_seconds INTEGER,
    size_bytes       BIGINT,
    status           TEXT NOT NULL DEFAULT 'ready',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audio_assets_confession_voice ON audio_assets(confession_id, voice_id);

-- ============================= USERS =============================
CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         CITEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_name  TEXT,
    timezone      TEXT NOT NULL DEFAULT 'UTC',
    status        TEXT NOT NULL DEFAULT 'active',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE subscriptions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan       TEXT NOT NULL DEFAULT 'free',
    status     TEXT NOT NULL DEFAULT 'active',
    started_at TIMESTAMPTZ,
    ends_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE session_preferences (
    user_id                  UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    default_duration_seconds INTEGER NOT NULL DEFAULT 1800,
    default_voice_id         UUID,
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE schedules (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label            TEXT NOT NULL,
    time             TIME NOT NULL,
    days_of_week     SMALLINT[] NOT NULL DEFAULT '{1,2,3,4,5,6,7}',
    timezone         TEXT NOT NULL DEFAULT 'UTC',
    duration_seconds INTEGER NOT NULL DEFAULT 1800,
    voice_id         UUID,
    category_ids     UUID[],
    enabled          BOOLEAN NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_schedules_user ON schedules(user_id);

-- ============================= SESSIONS =============================
CREATE TABLE sessions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type             TEXT NOT NULL DEFAULT 'standard',
    duration_seconds INTEGER NOT NULL,
    voice_id         UUID,
    status           TEXT NOT NULL DEFAULT 'created',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at       TIMESTAMPTZ,
    completed_at     TIMESTAMPTZ
);

CREATE TABLE session_items (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id       UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    confession_id    UUID NOT NULL REFERENCES confessions(id) ON DELETE CASCADE,
    variant_id       UUID,
    voice_id         UUID,
    audio_asset_id   UUID,
    position         INTEGER NOT NULL,
    duration_seconds INTEGER NOT NULL,
    status           TEXT NOT NULL DEFAULT 'queued'
);
CREATE INDEX idx_session_items_session ON session_items(session_id);

-- ============================= ENGAGEMENT =============================
CREATE TABLE favorites (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL,
    entity_id   UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_favorites_user ON favorites(user_id);

CREATE TABLE playback_history (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id       UUID,
    confession_id    UUID,
    duration_seconds INTEGER,
    completed        BOOLEAN NOT NULL DEFAULT FALSE,
    skipped          BOOLEAN NOT NULL DEFAULT FALSE,
    listened_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================= USER CONTENT =============================
CREATE TABLE user_confessions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    text        TEXT NOT NULL,
    category_id UUID,
    is_private  BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE user_confession_audio (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_confession_id UUID NOT NULL REFERENCES user_confessions(id) ON DELETE CASCADE,
    voice_id           UUID,
    url                TEXT,
    status             TEXT NOT NULL DEFAULT 'queued',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================= ADMIN =============================
CREATE TABLE admin_users (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    role       TEXT NOT NULL DEFAULT 'support',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE audit_logs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_user_id UUID,
    action        TEXT NOT NULL,
    entity        TEXT NOT NULL,
    entity_id     UUID,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

'@
$full = Join-Path $RepoRoot 'migrations\postgres\0001_schema.sql'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'migrations\postgres\0001_schema.sql', $bytes)
$count++

# ---- internal\api\admin.go ----
$c = @'
package api

import (
	"net/http"
	"strings"

	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/models"
)

// ---------- Admin: categories ----------

func (h *Handler) adminCreateCategory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Slug        string `json:"slug"`
		Description string `json:"description"`
		Icon        string `json:"icon"`
		Premium     bool   `json:"premium"`
		Status      string `json:"status"`
		SortOrder   int    `json:"sort_order"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.Slug == "" {
		httpx.WriteError(w, http.StatusBadRequest, "name and slug are required")
		return
	}
	if req.Status == "" {
		req.Status = "draft"
	}
	c := &models.Category{
		Name: req.Name, Slug: req.Slug, Description: req.Description, Icon: req.Icon,
		Premium: req.Premium, Status: req.Status, SortOrder: req.SortOrder,
	}
	if err := h.cont.CreateCategory(r.Context(), c); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			httpx.WriteError(w, http.StatusConflict, "a category with this slug already exists")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create category")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, c)
}

func (h *Handler) adminListCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := h.cont.ListCategories(r.Context(), true)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load categories")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, cats)
}

// ---------- Admin: confessions ----------

func (h *Handler) adminCreateConfession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CategoryID  string                     `json:"category_id"`
		Title       string                     `json:"title"`
		ShortText   string                     `json:"short_text"`
		MediumText  string                     `json:"medium_text"`
		LongText    string                     `json:"long_text"`
		Description string                     `json:"description"`
		Tags        []string                   `json:"tags"`
		Intensity   int                        `json:"intensity"`
		Language    string                     `json:"language"`
		Status      string                     `json:"status"`
		Author      string                     `json:"author"`
		Variants    []models.ConfessionVariant `json:"variants"`
		Scriptures  []models.ScriptureRef      `json:"scriptures"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.CategoryID == "" || req.Title == "" {
		httpx.WriteError(w, http.StatusBadRequest, "category_id and title are required")
		return
	}
	if _, err := h.cont.CategoryByID(r.Context(), req.CategoryID); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "category not found")
		return
	}
	if req.Status == "" {
		req.Status = "draft"
	}
	if req.Language == "" {
		req.Language = "en"
	}
	if req.Intensity == 0 {
		req.Intensity = 1
	}
	c := &models.Confession{
		CategoryID: req.CategoryID, Title: req.Title, ShortText: req.ShortText,
		MediumText: req.MediumText, LongText: req.LongText, Description: req.Description,
		Tags: req.Tags, Intensity: req.Intensity, Language: req.Language, Status: req.Status,
		Author: req.Author, Variants: req.Variants, Scriptures: req.Scriptures,
	}
	if err := h.cont.CreateConfession(r.Context(), c); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create confession")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, c)
}

func (h *Handler) adminListConfessions(w http.ResponseWriter, r *http.Request) {
	confs, err := h.cont.ListConfessions(r.Context(), false)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load confessions")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, confs)
}

func (h *Handler) adminGetConfession(w http.ResponseWriter, r *http.Request) {
	c, err := h.cont.ConfessionByID(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "confession not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, c)
}

func (h *Handler) adminUpdateConfessionStatus(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Status string `json:"status"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || !validConfessionStatus(req.Status) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid status")
		return
	}
	if err := h.cont.UpdateConfessionStatus(r.Context(), r.PathValue("id"), req.Status); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to update confession")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": req.Status})
}

func validConfessionStatus(s string) bool {
	switch s {
	case "draft", "content_review", "theological_review", "audio_production", "audio_qa", "approved", "published", "archived":
		return true
	}
	return false
}

// ---------- Admin: voices ----------

func (h *Handler) adminCreateVoice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Type        string `json:"type"`
		Provider    string `json:"provider"`
		Gender      string `json:"gender"`
		Language    string `json:"language"`
		Premium     bool   `json:"premium"`
		Status      string `json:"status"`
		SampleURL   string `json:"sample_url"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Type == "" {
		req.Type = "professional"
	}
	if req.Language == "" {
		req.Language = "en"
	}
	if req.Status == "" {
		req.Status = "active"
	}
	v := &models.Voice{
		Name: req.Name, Description: req.Description, Type: req.Type, Provider: req.Provider,
		Gender: req.Gender, Language: req.Language, Premium: req.Premium, Status: req.Status, SampleURL: req.SampleURL,
	}
	if err := h.audio.CreateVoice(r.Context(), v); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create voice")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, v)
}

func (h *Handler) adminListVoices(w http.ResponseWriter, r *http.Request) {
	voices, err := h.audio.ListVoices(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load voices")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, voices)
}

// ---------- Admin: audio ----------

func (h *Handler) adminUpsertAudio(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ConfessionID    string `json:"confession_id"`
		VariantID       string `json:"variant_id"`
		VoiceID         string `json:"voice_id"`
		URL             string `json:"url"`
		DurationSeconds int    `json:"duration_seconds"`
		SizeBytes       int64  `json:"size_bytes"`
		Status          string `json:"status"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ConfessionID == "" || req.VoiceID == "" || req.URL == "" {
		httpx.WriteError(w, http.StatusBadRequest, "confession_id, voice_id and url are required")
		return
	}
	if _, err := h.audio.VoiceByID(r.Context(), req.VoiceID); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "voice not found")
		return
	}
	if _, err := h.cont.ConfessionByID(r.Context(), req.ConfessionID); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "confession not found")
		return
	}
	a := &models.AudioAsset{
		ConfessionID: req.ConfessionID, VariantID: req.VariantID, VoiceID: req.VoiceID,
		URL: req.URL, DurationSeconds: req.DurationSeconds, SizeBytes: req.SizeBytes, Status: req.Status,
	}
	if err := h.audio.UpsertAsset(r.Context(), a); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to save audio")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, a)
}

// ---------- Admin: users ----------

func (h *Handler) adminSetRole(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || !validRole(req.Role) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid role")
		return
	}
	if _, err := h.users.ByID(r.Context(), req.UserID); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "user not found")
		return
	}
	if err := h.users.SetAdminRole(r.Context(), req.UserID, req.Role); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to set role")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"user_id": req.UserID, "role": req.Role})
}

func validRole(s string) bool {
	switch s {
	case "super_admin", "content_admin", "audio_producer", "theological_reviewer", "support_admin", "analytics_admin":
		return true
	}
	return false
}

func (h *Handler) adminSetSubscription(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
		Plan   string `json:"plan"`
		Status string `json:"status"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Plan != "free" && req.Plan != "premium" {
		httpx.WriteError(w, http.StatusBadRequest, "plan must be free or premium")
		return
	}
	if req.Status == "" {
		req.Status = "active"
	}
	if err := h.users.SetSubscription(r.Context(), req.UserID, req.Plan, req.Status); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to set subscription")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"user_id": req.UserID, "plan": req.Plan, "status": req.Status})
}

// ---------- Admin: dashboard summary ----------

func (h *Handler) adminStats(w http.ResponseWriter, r *http.Request) {
	confs, _ := h.cont.ListConfessions(r.Context(), false)
	cats, _ := h.cont.ListCategories(r.Context(), true)
	voices, _ := h.audio.ListVoices(r.Context())
	published := 0
	for _, c := range confs {
		if c.Status == "published" {
			published++
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"categories":            len(cats),
		"confessions":           len(confs),
		"published":             published,
		"voices":                len(voices),
		"confessions_by_status": statusCounts(confs),
	})
}

func statusCounts(confs []models.Confession) map[string]int {
	m := map[string]int{}
	for _, c := range confs {
		m[c.Status]++
	}
	return m
}

'@
$full = Join-Path $RepoRoot 'internal\api\admin.go'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'internal\api\admin.go', $bytes)
$count++

# ---- internal\api\handlers.go ----
$c = @'
package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/engine"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/store"
)

// Handler bundles all stores and config needed by the API.
type Handler struct {
	cfg   Config
	users *store.UserStore
	cont  *store.ContentStore
	audio *store.AudioStore
	sess  *store.SessionStore
	sched *store.ScheduleStore
	eng   *store.EngagementStore
	engn  *engine.Engine
}

type Config struct {
	JWTSecret string
	TokenTTL  string
}

func NewHandler(cfg Config, db *sql.DB) *Handler {
	return &Handler{
		cfg:   cfg,
		users: store.NewUserStore(db),
		cont:  store.NewContentStore(db),
		audio: store.NewAudioStore(db),
		sess:  store.NewSessionStore(db),
		sched: store.NewScheduleStore(db),
		eng:   store.NewEngagementStore(db),
	}
}

// BuildEngine wires the session engine after handler construction.
func (h *Handler) BuildEngine() {
	h.engn = engine.New(h.cont, h.audio, h.users)
}

func (h *Handler) userID(r *http.Request) string {
	if c := auth.FromContext(r); c != nil {
		return c.Sub
	}
	return ""
}

// ---------- Auth ----------

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"display_name"`
		Timezone string `json:"timezone"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || len(req.Password) < 8 {
		httpx.WriteError(w, http.StatusBadRequest, "email and a password of at least 8 characters are required")
		return
	}
	if _, _, err := h.users.ByEmail(r.Context(), req.Email); err == nil {
		httpx.WriteError(w, http.StatusConflict, "an account with this email already exists")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	tz := req.Timezone
	if tz == "" {
		tz = "UTC"
	}
	u, err := h.users.Create(r.Context(), req.Email, hash, req.Name, tz)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create account")
		return
	}
	h.issueToken(w, r, u)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	u, hash, err := h.users.ByEmail(r.Context(), req.Email)
	if err != nil || !auth.CheckPassword(hash, req.Password) {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if u.Status != "active" {
		httpx.WriteError(w, http.StatusForbidden, "account is disabled")
		return
	}
	h.issueToken(w, r, u)
}

func (h *Handler) issueToken(w http.ResponseWriter, r *http.Request, u *models.User) {
	role, _ := h.users.AdminRole(r.Context(), u.ID)
	tok, err := auth.SignToken(h.cfg.JWTSecret, h.cfg.TokenTTL, u.ID, u.Email, role)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to issue token")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"token": tok,
		"user":  u,
	})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	u, err := h.users.ByID(r.Context(), h.userID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "user not found")
		return
	}
	plan, _ := h.users.Subscription(r.Context(), u.ID)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"user": u, "plan": plan})
}

// ---------- Content ----------

func (h *Handler) listCollections(w http.ResponseWriter, r *http.Request) {
	cols, err := h.cont.ListCollections(r.Context(), false)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load collections")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, cols)
}

func (h *Handler) listCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := h.cont.ListCategories(r.Context(), false)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load categories")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, cats)
}

func (h *Handler) categoryConfessions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	confs, err := h.cont.ConfessionsByCategory(r.Context(), id, true)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load confessions")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, confs)
}

func (h *Handler) getConfession(w http.ResponseWriter, r *http.Request) {
	c, err := h.cont.ConfessionByID(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "confession not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load confession")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, c)
}

func (h *Handler) listVoices(w http.ResponseWriter, r *http.Request) {
	voices, err := h.audio.ListVoices(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load voices")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, voices)
}

// ---------- Sessions ----------

func (h *Handler) createSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CategoryIDs     []string `json:"category_ids"`
		DurationSeconds int      `json:"duration_seconds"`
		VoiceID         string   `json:"voice_id"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.CategoryIDs) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "at least one category is required")
		return
	}
	if req.DurationSeconds < 60 || req.DurationSeconds > 3*3600 {
		httpx.WriteError(w, http.StatusBadRequest, "duration must be between 1 minute and 3 hours")
		return
	}
	sess, err := h.engn.Build(r.Context(), engine.Request{
		UserID:          h.userID(r),
		CategoryIDs:     req.CategoryIDs,
		DurationSeconds: req.DurationSeconds,
		VoiceID:         req.VoiceID,
	})
	if errors.Is(err, engine.ErrNoContent) {
		httpx.WriteError(w, http.StatusUnprocessableEntity, "no content available for the selected categories and voice")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to build session")
		return
	}
	if err := h.sess.Create(r.Context(), sess); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to save session")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, sess)
}

func (h *Handler) getSession(w http.ResponseWriter, r *http.Request) {
	sess, err := h.sess.ByID(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "session not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load session")
		return
	}
	if sess.UserID != h.userID(r) {
		httpx.WriteError(w, http.StatusForbidden, "not your session")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sess)
}

func (h *Handler) updateSessionStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, err := h.sess.ByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "session not found")
		return
	}
	if sess.UserID != h.userID(r) {
		httpx.WriteError(w, http.StatusForbidden, "not your session")
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || !validSessionStatus(req.Status) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid status")
		return
	}
	if err := h.sess.UpdateStatus(r.Context(), id, req.Status); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to update session")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": req.Status})
}

func (h *Handler) listMySessions(w http.ResponseWriter, r *http.Request) {
	sess, err := h.sess.ListByUser(r.Context(), h.userID(r), 50)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load sessions")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sess)
}

func validSessionStatus(s string) bool {
	switch s {
	case "created", "playing", "completed", "abandoned":
		return true
	}
	return false
}

// ---------- Schedules ----------

func (h *Handler) listSchedules(w http.ResponseWriter, r *http.Request) {
	list, err := h.sched.ListByUser(r.Context(), h.userID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load schedules")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, list)
}

func (h *Handler) createSchedule(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Label           string   `json:"label"`
		Time            string   `json:"time"`
		DaysOfWeek      []int    `json:"days_of_week"`
		Timezone        string   `json:"timezone"`
		DurationSeconds int      `json:"duration_seconds"`
		VoiceID         string   `json:"voice_id"`
		CategoryIDs     []string `json:"category_ids"`
		Enabled         *bool    `json:"enabled"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Label == "" || req.Time == "" {
		httpx.WriteError(w, http.StatusBadRequest, "label and time are required")
		return
	}
	if _, err := time.Parse("15:04", req.Time); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "time must be HH:MM")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.DurationSeconds == 0 {
		req.DurationSeconds = 1800
	}
	if req.Timezone == "" {
		req.Timezone = "UTC"
	}
	sc := &models.Schedule{
		UserID:          h.userID(r),
		Label:           req.Label,
		Time:            req.Time,
		DaysOfWeek:      req.DaysOfWeek,
		Timezone:        req.Timezone,
		DurationSeconds: req.DurationSeconds,
		VoiceID:         req.VoiceID,
		CategoryIDs:     req.CategoryIDs,
		Enabled:         enabled,
	}
	if err := h.sched.Create(r.Context(), sc); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create schedule")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, sc)
}

func (h *Handler) updateSchedule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sc, err := h.sched.ByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "schedule not found")
		return
	}
	if sc.UserID != h.userID(r) {
		httpx.WriteError(w, http.StatusForbidden, "not your schedule")
		return
	}
	var req struct {
		Label           *string  `json:"label"`
		Time            *string  `json:"time"`
		DaysOfWeek      []int    `json:"days_of_week"`
		Timezone        *string  `json:"timezone"`
		DurationSeconds *int     `json:"duration_seconds"`
		VoiceID         *string  `json:"voice_id"`
		CategoryIDs     []string `json:"category_ids"`
		Enabled         *bool    `json:"enabled"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Label != nil {
		sc.Label = *req.Label
	}
	if req.Time != nil {
		if _, err := time.Parse("15:04", *req.Time); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "time must be HH:MM")
			return
		}
		sc.Time = *req.Time
	}
	if req.DaysOfWeek != nil {
		sc.DaysOfWeek = req.DaysOfWeek
	}
	if req.Timezone != nil {
		sc.Timezone = *req.Timezone
	}
	if req.DurationSeconds != nil {
		sc.DurationSeconds = *req.DurationSeconds
	}
	if req.VoiceID != nil {
		sc.VoiceID = *req.VoiceID
	}
	if req.CategoryIDs != nil {
		sc.CategoryIDs = req.CategoryIDs
	}
	if req.Enabled != nil {
		sc.Enabled = *req.Enabled
	}
	if err := h.sched.Update(r.Context(), sc); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to update schedule")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, sc)
}

func (h *Handler) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	err := h.sched.Delete(r.Context(), r.PathValue("id"), h.userID(r))
	if errors.Is(err, store.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "schedule not found")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to delete schedule")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- Engagement ----------

func (h *Handler) addFavorite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || !validEntityType(req.EntityType) || req.EntityID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid favorite")
		return
	}
	f, err := h.eng.AddFavorite(r.Context(), h.userID(r), req.EntityType, req.EntityID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to add favorite")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, f)
}

func (h *Handler) removeFavorite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.EntityID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid favorite")
		return
	}
	if err := h.eng.RemoveFavorite(r.Context(), h.userID(r), req.EntityType, req.EntityID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to remove favorite")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listFavorites(w http.ResponseWriter, r *http.Request) {
	list, err := h.eng.ListFavorites(r.Context(), h.userID(r), r.URL.Query().Get("type"))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load favorites")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, list)
}

func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	list, err := h.eng.History(r.Context(), h.userID(r), 50)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load history")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, list)
}

func (h *Handler) recordPlayback(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID       string `json:"session_id"`
		ConfessionID    string `json:"confession_id"`
		DurationSeconds int    `json:"duration_seconds"`
		Completed       bool   `json:"completed"`
		Skipped         bool   `json:"skipped"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	rec := &models.PlaybackRecord{
		UserID:          h.userID(r),
		SessionID:       req.SessionID,
		ConfessionID:    req.ConfessionID,
		DurationSeconds: req.DurationSeconds,
		Completed:       req.Completed,
		Skipped:         req.Skipped,
	}
	if err := h.eng.RecordPlayback(r.Context(), rec); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to record playback")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, rec)
}

// ---------- User confessions ----------

func (h *Handler) createUserConfession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title      string `json:"title"`
		Text       string `json:"text"`
		CategoryID string `json:"category_id"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Title == "" || req.Text == "" {
		httpx.WriteError(w, http.StatusBadRequest, "title and text are required")
		return
	}
	uc := &models.UserConfession{
		UserID:     h.userID(r),
		Title:      req.Title,
		Text:       req.Text,
		CategoryID: req.CategoryID,
		IsPrivate:  true, // private by default (PRD §22)
	}
	if err := h.eng.CreateUserConfession(r.Context(), uc); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to create confession")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, uc)
}

func (h *Handler) listUserConfessions(w http.ResponseWriter, r *http.Request) {
	list, err := h.eng.ListUserConfessions(r.Context(), h.userID(r))
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "failed to load confessions")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, list)
}

func validEntityType(t string) bool {
	switch t {
	case "confession", "category", "session", "voice":
		return true
	}
	return false
}

'@
$full = Join-Path $RepoRoot 'internal\api\handlers.go'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'internal\api\handlers.go', $bytes)
$count++

# ---- internal\api\middleware.go ----
$c = @'
package api

import (
	"log"
	"net/http"
	"time"
)

// logRequests is a minimal structured access-log middleware.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

'@
$full = Join-Path $RepoRoot 'internal\api\middleware.go'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'internal\api\middleware.go', $bytes)
$count++

# ---- internal\api\router.go ----
$c = @'
package api

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/Teamthy/i-confess/internal/adminui"
	"github.com/Teamthy/i-confess/internal/auth"
)

// Routes builds the full HTTP handler with all routes registered.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()

	// Health
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Admin console (SPA)
	mux.Handle("GET /admin", http.RedirectHandler("/admin/", http.StatusMovedPermanently))
	mux.Handle("GET /admin/", http.StripPrefix("/admin/", adminui.Handler()))

	// Placeholder/dev audio assets (generated locally; replaced by CDN in production).
	mediaPath := os.Getenv("MEDIA_DIR")
	if mediaPath == "" {
		mediaPath = "data/media"
	}
	if abs, err := filepath.Abs(mediaPath); err == nil {
		mux.Handle("GET /media/", http.StripPrefix("/media/", http.FileServer(http.Dir(abs))))
	}

	// Public auth
	mux.HandleFunc("POST /auth/register", h.register)
	mux.HandleFunc("POST /auth/login", h.login)

	// Public content (read-only, published only)
	mux.HandleFunc("GET /collections", h.listCollections)
	mux.HandleFunc("GET /categories", h.listCategories)
	mux.HandleFunc("GET /categories/{id}/confessions", h.categoryConfessions)
	mux.HandleFunc("GET /confessions/{id}", h.getConfession)
	mux.HandleFunc("GET /voices", h.listVoices)

	// Authenticated user routes
	authed := auth.Middleware(h.cfg.JWTSecret)
	mux.Handle("GET /me", authed(http.HandlerFunc(h.me)))

	mux.Handle("POST /sessions", authed(http.HandlerFunc(h.createSession)))
	mux.Handle("GET /sessions/{id}", authed(http.HandlerFunc(h.getSession)))
	mux.Handle("PATCH /sessions/{id}", authed(http.HandlerFunc(h.updateSessionStatus)))
	mux.Handle("GET /sessions", authed(http.HandlerFunc(h.listMySessions)))

	mux.Handle("GET /schedules", authed(http.HandlerFunc(h.listSchedules)))
	mux.Handle("POST /schedules", authed(http.HandlerFunc(h.createSchedule)))
	mux.Handle("PATCH /schedules/{id}", authed(http.HandlerFunc(h.updateSchedule)))
	mux.Handle("DELETE /schedules/{id}", authed(http.HandlerFunc(h.deleteSchedule)))

	mux.Handle("POST /me/favorites", authed(http.HandlerFunc(h.addFavorite)))
	mux.Handle("DELETE /me/favorites", authed(http.HandlerFunc(h.removeFavorite)))
	mux.Handle("GET /me/favorites", authed(http.HandlerFunc(h.listFavorites)))

	mux.Handle("GET /me/history", authed(http.HandlerFunc(h.history)))
	mux.Handle("POST /me/history", authed(http.HandlerFunc(h.recordPlayback)))

	mux.Handle("POST /me/confessions", authed(http.HandlerFunc(h.createUserConfession)))
	mux.Handle("GET /me/confessions", authed(http.HandlerFunc(h.listUserConfessions)))

	// Admin routes
	admin := auth.AdminMiddleware(h.cfg.JWTSecret)
	mux.Handle("GET /admin/stats", admin(http.HandlerFunc(h.adminStats)))

	mux.Handle("POST /admin/categories", admin(http.HandlerFunc(h.adminCreateCategory)))
	mux.Handle("GET /admin/categories", admin(http.HandlerFunc(h.adminListCategories)))

	mux.Handle("POST /admin/confessions", admin(http.HandlerFunc(h.adminCreateConfession)))
	mux.Handle("GET /admin/confessions", admin(http.HandlerFunc(h.adminListConfessions)))
	mux.Handle("GET /admin/confessions/{id}", admin(http.HandlerFunc(h.adminGetConfession)))
	mux.Handle("PATCH /admin/confessions/{id}", admin(http.HandlerFunc(h.adminUpdateConfessionStatus)))

	mux.Handle("POST /admin/voices", admin(http.HandlerFunc(h.adminCreateVoice)))
	mux.Handle("GET /admin/voices", admin(http.HandlerFunc(h.adminListVoices)))

	mux.Handle("POST /admin/audio", admin(http.HandlerFunc(h.adminUpsertAudio)))

	mux.Handle("POST /admin/users/role", admin(http.HandlerFunc(h.adminSetRole)))
	mux.Handle("POST /admin/users/subscription", admin(http.HandlerFunc(h.adminSetSubscription)))

	return logRequests(mux)
}

'@
$full = Join-Path $RepoRoot 'internal\api\router.go'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'internal\api\router.go', $bytes)
$count++

# ---- internal\adminui\index.html ----
$c = @'
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>i-confess · Admin Console</title>
<style>
  :root {
    --bg: #0f1220;
    --bg-2: #161a2e;
    --panel: #1c2138;
    --panel-2: #232946;
    --border: #2d3350;
    --text: #e8eaf6;
    --muted: #9aa1c0;
    --accent: #7c8cf8;
    --accent-2: #b7a1ff;
    --gold: #e8c67a;
    --ok: #4ade80;
    --warn: #fbbf24;
    --err: #f87171;
    --radius: 12px;
    --serif: Georgia, 'Times New Roman', serif;
  }
  * { box-sizing: border-box; }
  body {
    margin: 0;
    font-family: -apple-system, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;
    background: var(--bg);
    color: var(--text);
    font-size: 14px;
    line-height: 1.5;
  }
  a { color: var(--accent); }
  /* ---------- login ---------- */
  .login-wrap {
    min-height: 100vh; display: flex; align-items: center; justify-content: center;
    background: radial-gradient(1200px 600px at 50% -10%, #232946 0%, var(--bg) 60%);
  }
  .login-card {
    width: 380px; background: var(--panel); border: 1px solid var(--border);
    border-radius: 16px; padding: 36px 32px;
  }
  .brand { font-family: var(--serif); font-size: 26px; color: var(--gold); margin: 0 0 4px; }
  .brand span { color: var(--text); }
  .login-sub { color: var(--muted); margin: 0 0 24px; font-size: 13px; }
  /* ---------- shell ---------- */
  .shell { display: grid; grid-template-columns: 220px 1fr; min-height: 100vh; }
  .sidebar {
    background: var(--bg-2); border-right: 1px solid var(--border); padding: 20px 14px;
    display: flex; flex-direction: column; gap: 4px;
  }
  .sidebar .brand { font-size: 22px; padding: 0 8px 16px; }
  .nav-btn {
    display: flex; align-items: center; gap: 10px; padding: 10px 12px; border-radius: 8px;
    background: none; border: none; color: var(--muted); cursor: pointer; font-size: 14px;
    text-align: left; width: 100%;
  }
  .nav-btn:hover { background: var(--panel-2); color: var(--text); }
  .nav-btn.active { background: var(--panel-2); color: var(--accent-2); }
  .nav-btn .ico { width: 18px; text-align: center; }
  .sidebar .spacer { flex: 1; }
  .sidebar .who { padding: 10px 8px; color: var(--muted); font-size: 12px; border-top: 1px solid var(--border); }
  .sidebar .who b { color: var(--text); display: block; }
  .logout { background: none; border: 1px solid var(--border); color: var(--muted); border-radius: 8px; padding: 8px; cursor: pointer; width: 100%; margin-top: 8px; }
  .logout:hover { color: var(--err); border-color: var(--err); }
  /* ---------- content ---------- */
  .main { padding: 28px 32px; max-width: 1100px; }
  .page-title { font-family: var(--serif); font-size: 24px; margin: 0 0 4px; color: var(--gold); }
  .page-sub { color: var(--muted); margin: 0 0 24px; }
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(180px, 1fr)); gap: 14px; margin-bottom: 28px; }
  .card { background: var(--panel); border: 1px solid var(--border); border-radius: var(--radius); padding: 18px; }
  .card .num { font-size: 30px; font-weight: 700; font-family: var(--serif); }
  .card .lbl { color: var(--muted); font-size: 12px; text-transform: uppercase; letter-spacing: .05em; }
  .panel { background: var(--panel); border: 1px solid var(--border); border-radius: var(--radius); padding: 20px; margin-bottom: 22px; }
  .panel h3 { margin: 0 0 14px; font-size: 15px; color: var(--accent-2); }
  table { width: 100%; border-collapse: collapse; }
  th, td { text-align: left; padding: 9px 10px; border-bottom: 1px solid var(--border); font-size: 13px; vertical-align: top; }
  th { color: var(--muted); font-weight: 600; font-size: 11px; text-transform: uppercase; letter-spacing: .04em; }
  tr:hover td { background: var(--panel-2); }
  .badge { display: inline-block; padding: 2px 9px; border-radius: 20px; font-size: 11px; font-weight: 600; }
  .b-published { background: rgba(74,222,128,.15); color: var(--ok); }
  .b-draft { background: rgba(251,191,36,.15); color: var(--warn); }
  .b-other { background: rgba(154,161,192,.15); color: var(--muted); }
  .b-premium { background: rgba(232,198,122,.18); color: var(--gold); }
  .b-free { background: rgba(124,140,248,.15); color: var(--accent-2); }
  /* ---------- forms ---------- */
  label { display: block; font-size: 12px; color: var(--muted); margin: 12px 0 4px; }
  input, select, textarea {
    width: 100%; background: var(--bg-2); border: 1px solid var(--border); color: var(--text);
    border-radius: 8px; padding: 9px 11px; font-size: 13px; font-family: inherit;
  }
  textarea { resize: vertical; min-height: 70px; }
  input:focus, select:focus, textarea:focus { outline: none; border-color: var(--accent); }
  .row { display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px; }
  .row3 { display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 0 16px; }
  .btn {
    display: inline-flex; align-items: center; gap: 6px; background: var(--accent); color: #0b0e1c;
    border: none; border-radius: 8px; padding: 10px 16px; font-size: 13px; font-weight: 600; cursor: pointer;
  }
  .btn:hover { filter: brightness(1.08); }
  .btn.ghost { background: transparent; border: 1px solid var(--border); color: var(--text); }
  .btn.sm { padding: 5px 10px; font-size: 12px; }
  .btn.danger { background: transparent; border: 1px solid var(--err); color: var(--err); }
  .btn.gold { background: var(--gold); color: #241b05; }
  .actions { display: flex; gap: 8px; flex-wrap: wrap; align-items: center; }
  .muted { color: var(--muted); }
  .toast {
    position: fixed; bottom: 22px; right: 22px; background: var(--panel-2); border: 1px solid var(--border);
    border-radius: 10px; padding: 12px 18px; font-size: 13px; z-index: 50; box-shadow: 0 8px 30px rgba(0,0,0,.4);
    animation: fade .2s ease;
  }
  .toast.ok { border-color: var(--ok); }
  .toast.err { border-color: var(--err); }
  @keyframes fade { from { opacity: 0; transform: translateY(6px);} to { opacity: 1; } }
  .hidden { display: none !important; }
  .hint { font-size: 12px; color: var(--muted); margin-top: 6px; }
  .empty { color: var(--muted); padding: 18px; text-align: center; }
  .mt { margin-top: 16px; }
  .split { display: grid; grid-template-columns: 1fr 1fr; gap: 22px; }
  @media (max-width: 760px) { .split, .row, .row3 { grid-template-columns: 1fr; } .shell { grid-template-columns: 1fr; } .sidebar { flex-direction: row; flex-wrap: wrap; } }
</style>
</head>
<body>
<div id="app"></div>
<script>
const API = ''; // same origin

// ---------- state ----------
const state = { token: localStorage.getItem('ic_token') || '', user: null };

function api(path, opts = {}) {
  const headers = { 'Content-Type': 'application/json' };
  if (state.token) headers['Authorization'] = 'Bearer ' + state.token;
  return fetch(API + path, { ...opts, headers: { ...headers, ...(opts.headers || {}) } })
    .then(async r => {
      const body = await r.json().catch(() => ({}));
      if (!r.ok) throw new Error(body.error || ('HTTP ' + r.status));
      return body;
    });
}

// ---------- toast ----------
function toast(msg, ok = true) {
  const el = document.createElement('div');
  el.className = 'toast ' + (ok ? 'ok' : 'err');
  el.textContent = msg;
  document.body.appendChild(el);
  setTimeout(() => el.remove(), 2800);
}

// ---------- helpers ----------
function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
}
function badge(status) {
  const cls = status === 'published' ? 'b-published' : status === 'draft' ? 'b-draft' : 'b-other';
  return `<span class="badge ${cls}">${esc(status)}</span>`;
}
function fmtDate(s) { return s ? new Date(s).toLocaleString() : '—'; }

// ---------- router ----------
const views = ['dashboard','categories','confessions','voices','audio','users'];
let current = 'dashboard';

function nav(btn, view) {
  current = view;
  document.querySelectorAll('.nav-btn').forEach(b => b.classList.toggle('active', b === btn));
  render();
}

function shell(inner) {
  return `
  <div class="shell">
    <div class="sidebar">
      <div class="brand">i-<span>confess</span></div>
      ${[['dashboard','▦','Dashboard'],['categories','▤','Categories'],['confessions','≡','Confessions'],['voices','◉','Voices'],['audio','♪','Audio'],['users','⛁','Users']].map(([v,i,l]) =>
        `<button class="nav-btn ${current===v?'active':''}" onclick="nav(this,'${v}')"><span class="ico">${i}</span>${l}</button>`).join('')}
      <div class="spacer"></div>
      <div class="who"><b>${esc(state.user?.email || '')}</b>${esc(state.user?.display_name || '')}</div>
      <button class="logout" onclick="logout()">Sign out</button>
    </div>
    <div class="main">${inner}</div>
  </div>`;
}

// ---------- login ----------
function loginScreen() {
  document.getElementById('app').innerHTML = `
  <div class="login-wrap">
    <div class="login-card">
      <h1 class="brand">i-<span>confess</span></h1>
      <p class="login-sub">Admin Console · sign in</p>
      <label>Email</label>
      <input id="lg-email" type="email" placeholder="admin@iconfess.dev">
      <label>Password</label>
      <input id="lg-pass" type="password" placeholder="••••••••">
      <div class="mt"><button class="btn" style="width:100%;justify-content:center" onclick="doLogin()">Sign in</button></div>
      <p class="hint" id="lg-err" style="color:var(--err)"></p>
    </div>
  </div>`;
  document.getElementById('lg-pass').addEventListener('keydown', e => { if (e.key === 'Enter') doLogin(); });
}

async function doLogin() {
  const email = document.getElementById('lg-email').value.trim();
  const password = document.getElementById('lg-pass').value;
  try {
    const res = await fetch(API + '/auth/login', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password })
    });
    const body = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(body.error || 'Login failed');
    state.token = body.token; state.user = body.user;
    localStorage.setItem('ic_token', state.token);
    render();
  } catch (e) {
    document.getElementById('lg-err').textContent = e.message;
  }
}

function logout() {
  state.token = ''; state.user = null;
  localStorage.removeItem('ic_token');
  render();
}

// ---------- views ----------
async function viewDashboard() {
  const s = await api('/admin/stats');
  const cats = s.categories, conf = s.confessions, pub = s.published, voices = s.voices;
  return `
    <h1 class="page-title">Dashboard</h1>
    <p class="page-sub">Content health and publishing status</p>
    <div class="grid">
      <div class="card"><div class="num">${cats}</div><div class="lbl">Categories</div></div>
      <div class="card"><div class="num">${conf}</div><div class="lbl">Confessions</div></div>
      <div class="card"><div class="num">${pub}</div><div class="lbl">Published</div></div>
      <div class="card"><div class="num">${voices}</div><div class="lbl">Voices</div></div>
    </div>
    <div class="panel"><h3>Confessions by status</h3>
      ${Object.keys(s.confessions_by_status || {}).length
        ? `<table><tr><th>Status</th><th>Count</th></tr>${Object.entries(s.confessions_by_status).map(([k,v]) => `<tr><td>${badge(k)}</td><td>${v}</td></tr>`).join('')}</table>`
        : '<div class="empty">No confessions yet</div>'}
    </div>`;
}

async function viewCategories() {
  const cats = await api('/admin/categories');
  return `
    <h1 class="page-title">Categories</h1>
    <p class="page-sub">The category system (28 / 38 collections are built from these)</p>
    <div class="panel"><h3>New category</h3>
      <div class="row">
        <div><label>Name *</label><input id="c-name" placeholder="e.g. Healing"></div>
        <div><label>Slug *</label><input id="c-slug" placeholder="e.g. healing"></div>
      </div>
      <div class="row">
        <div><label>Description</label><input id="c-desc" placeholder="Short description"></div>
        <div><label>Icon</label><input id="c-icon" placeholder="emoji or icon key"></div>
      </div>
      <div class="row3">
        <div><label>Status</label><select id="c-status"><option value="draft">draft</option><option value="published" selected>published</option></select></div>
        <div><label>Premium</label><select id="c-premium"><option value="false">Free</option><option value="true">Premium</option></select></div>
        <div><label>Sort order</label><input id="c-sort" type="number" value="0"></div>
      </div>
      <div class="mt"><button class="btn" onclick="createCategory()">Create category</button></div>
    </div>
    <div class="panel"><h3>All categories (${cats.length})</h3>
      ${cats.length ? `<table><tr><th>Name</th><th>Slug</th><th>Premium</th><th>Status</th><th>Created</th></tr>
        ${cats.map(c => `<tr><td><b>${esc(c.name)}</b>${c.description ? `<div class="muted">${esc(c.description)}</div>` : ''}</td><td>${esc(c.slug)}</td><td>${c.premium ? '<span class="badge b-premium">premium</span>' : '<span class="badge b-free">free</span>'}</td><td>${badge(c.status)}</td><td>${fmtDate(c.created_at)}</td></tr>`).join('')}</table>`
      : '<div class="empty">No categories yet</div>'}
    </div>`;
}

async function createCategory() {
  const body = {
    name: document.getElementById('c-name').value.trim(),
    slug: document.getElementById('c-slug').value.trim().toLowerCase(),
    description: document.getElementById('c-desc').value.trim(),
    icon: document.getElementById('c-icon').value.trim(),
    status: document.getElementById('c-status').value,
    premium: document.getElementById('c-premium').value === 'true',
    sort_order: parseInt(document.getElementById('c-sort').value || '0', 10),
  };
  if (!body.name || !body.slug) { toast('Name and slug are required', false); return; }
  try { await api('/admin/categories', { method: 'POST', body: JSON.stringify(body) }); toast('Category created'); render(); }
  catch (e) { toast(e.message, false); }
}

async function viewVoices() {
  const voices = await api('/admin/voices');
  return `
    <h1 class="page-title">Voices</h1>
    <p class="page-sub">Voice library (premium voices require licensing per policy)</p>
    <div class="panel"><h3>New voice</h3>
      <div class="row">
        <div><label>Name *</label><input id="v-name" placeholder="e.g. Grace"></div>
        <div><label>Type</label><select id="v-type"><option value="professional" selected>professional</option><option value="minister">minister</option><option value="generic">generic</option></select></div>
      </div>
      <div class="row">
        <div><label>Provider</label><input id="v-provider" placeholder="e.g. i-confess"></div>
        <div><label>Gender</label><select id="v-gender"><option value="">—</option><option value="female">female</option><option value="male">male</option><option value="neutral">neutral</option></select></div>
      </div>
      <div class="row3">
        <div><label>Language</label><input id="v-lang" value="en"></div>
        <div><label>Tier</label><select id="v-premium"><option value="false" selected>Free</option><option value="true">Premium</option></select></div>
        <div><label>Status</label><select id="v-status"><option value="active" selected>active</option><option value="inactive">inactive</option></select></div>
      </div>
      <label>Description</label><input id="v-desc">
      <div class="mt"><button class="btn" onclick="createVoice()">Create voice</button></div>
    </div>
    <div class="panel"><h3>All voices (${voices.length})</h3>
      ${voices.length ? `<table><tr><th>Name</th><th>Type</th><th>Tier</th><th>Status</th></tr>
        ${voices.map(v => `<tr><td><b>${esc(v.name)}</b>${v.description ? `<div class="muted">${esc(v.description)}</div>` : ''}</td><td>${esc(v.type)}</td><td>${v.premium ? '<span class="badge b-premium">premium</span>' : '<span class="badge b-free">free</span>'}</td><td>${badge(v.status)}</td></tr>`).join('')}</table>`
      : '<div class="empty">No voices yet</div>'}
    </div>`;
}

async function createVoice() {
  const body = {
    name: document.getElementById('v-name').value.trim(),
    type: document.getElementById('v-type').value,
    provider: document.getElementById('v-provider').value.trim(),
    gender: document.getElementById('v-gender').value,
    language: document.getElementById('v-lang').value.trim() || 'en',
    premium: document.getElementById('v-premium').value === 'true',
    status: document.getElementById('v-status').value,
    description: document.getElementById('v-desc').value.trim(),
  };
  if (!body.name) { toast('Name is required', false); return; }
  try { await api('/admin/voices', { method: 'POST', body: JSON.stringify(body) }); toast('Voice created'); render(); }
  catch (e) { toast(e.message, false); }
}

async function viewConfessions() {
  const [confs, cats] = await Promise.all([api('/admin/confessions'), api('/admin/categories')]);
  return `
    <h1 class="page-title">Confessions</h1>
    <p class="page-sub">Manage confession content, variants and scripture references</p>
    <div class="panel"><h3>New confession</h3>
      <div class="row">
        <div><label>Category *</label><select id="f-cat">${cats.map(c => `<option value="${c.id}">${esc(c.name)}</option>`).join('')}</select></div>
        <div><label>Title *</label><input id="f-title" placeholder="e.g. I Am Healed"></div>
      </div>
      <div class="row3">
        <div><label>Status</label><select id="f-status"><option value="draft">draft</option><option value="published" selected>published</option><option value="content_review">content_review</option><option value="theological_review">theological_review</option></select></div>
        <div><label>Intensity (1–5)</label><input id="f-intensity" type="number" min="1" max="5" value="1"></div>
        <div><label>Author</label><input id="f-author" placeholder="content team"></div>
      </div>
      <label>Short text</label><textarea id="f-short" placeholder="One-line declaration"></textarea>
      <label>Medium text (used in session player)</label><textarea id="f-medium"></textarea>
      <label>Long text</label><textarea id="f-long"></textarea>
      <label>Scriptures <span class="muted">— one per line: book|chapter|verse|translation|quote(0/1)</span></label>
      <textarea id="f-scriptures" placeholder="Isaiah|53|5|KJV|1&#10;James|1|5|KJV|1"></textarea>
      <label>Variants <span class="muted">— one per line: label|seconds</span></label>
      <textarea id="f-variants" placeholder="30s|30&#10;1m|60&#10;3m|180&#10;5m|300"></textarea>
      <div class="mt"><button class="btn" onclick="createConfession()">Create confession</button></div>
    </div>
    <div class="panel"><h3>All confessions (${confs.length})</h3>
      ${confs.length ? `<table><tr><th>Title</th><th>Status</th><th>Intensity</th><th>Created</th><th></th></tr>
        ${confs.map(c => `<tr>
          <td><b>${esc(c.title)}</b><div class="muted">${esc((c.short_text||'').slice(0,70))}</div></td>
          <td>${badge(c.status)}</td>
          <td>${c.intensity}</td>
          <td>${fmtDate(c.created_at)}</td>
          <td><div class="actions">
            ${c.status !== 'published' ? `<button class="btn sm gold" onclick="setStatus('${c.id}','published')">Publish</button>` : `<button class="btn sm ghost" onclick="setStatus('${c.id}','draft')">Unpublish</button>`}
            <button class="btn sm ghost" onclick="setStatus('${c.id}','archived')">Archive</button>
          </div></td>
        </tr>`).join('')}</table>`
      : '<div class="empty">No confessions yet</div>'}
    </div>`;
}

async function createConfession() {
  const cat = document.getElementById('f-cat').value;
  const title = document.getElementById('f-title').value.trim();
  if (!cat || !title) { toast('Category and title are required', false); return; }

  const parseLines = (val) => val.trim().split('\n').map(l => l.trim()).filter(Boolean);
  const scriptures = parseLines(document.getElementById('f-scriptures').value).map((l, i) => {
    const [book, chapter, verse, translation, quote] = l.split('|');
    return { book, chapter: parseInt(chapter||'0',10), verse: verse||'', translation: translation||'KJV', is_direct_quote: (quote||'1') === '1', sort_order: i };
  });
  const variants = parseLines(document.getElementById('f-variants').value).map((l, i) => {
    const [label, sec] = l.split('|');
    return { label, duration_seconds: parseInt(sec||'60',10), sort_order: i };
  });

  const body = {
    category_id: cat, title,
    short_text: document.getElementById('f-short').value,
    medium_text: document.getElementById('f-medium').value,
    long_text: document.getElementById('f-long').value,
    status: document.getElementById('f-status').value,
    intensity: parseInt(document.getElementById('f-intensity').value || '1', 10),
    author: document.getElementById('f-author').value.trim(),
    language: 'en', scriptures, variants,
  };
  try { await api('/admin/confessions', { method: 'POST', body: JSON.stringify(body) }); toast('Confession created'); render(); }
  catch (e) { toast(e.message, false); }
}

async function setStatus(id, status) {
  try { await api('/admin/confessions/' + id, { method: 'PATCH', body: JSON.stringify({ status }) }); toast('Status → ' + status); render(); }
  catch (e) { toast(e.message, false); }
}

async function viewAudio() {
  const [confs, voices] = await Promise.all([api('/admin/confessions'), api('/admin/voices')]);
  return `
    <h1 class="page-title">Audio</h1>
    <p class="page-sub">Attach audio assets to confessions (production audio via TTS/recording pipeline)</p>
    <div class="panel"><h3>Attach audio asset</h3>
      <div class="row">
        <div><label>Confession *</label><select id="a-conf">${confs.map(c => `<option value="${c.id}">${esc(c.title)}</option>`).join('')}</select></div>
        <div><label>Voice *</label><select id="a-voice">${voices.map(v => `<option value="${v.id}">${esc(v.name)}${v.premium ? ' ★' : ''}</option>`).join('')}</select></div>
      </div>
      <div class="row">
        <div><label>Variant label</label><input id="a-variant" placeholder="e.g. 1m, 5m (optional)"></div>
        <div><label>Duration (seconds)</label><input id="a-duration" type="number" placeholder="e.g. 60"></div>
      </div>
      <label>Audio URL *</label><input id="a-url" placeholder="https://cdn.example.com/audio/healing-1m.mp3">
      <div class="mt"><button class="btn" onclick="attachAudio()">Attach audio</button></div>
      <p class="hint">In production, audio is generated asynchronously (queued → processing → ready) and stored on a CDN. Here you attach a ready URL directly.</p>
    </div>`;
}

async function attachAudio() {
  const body = {
    confession_id: document.getElementById('a-conf').value,
    voice_id: document.getElementById('a-voice').value,
    url: document.getElementById('a-url').value.trim(),
    duration_seconds: parseInt(document.getElementById('a-duration').value || '0', 10),
    status: 'ready',
  };
  const vlabel = document.getElementById('a-variant').value.trim();
  if (!body.url || !body.confession_id || !body.voice_id) { toast('Confession, voice and URL are required', false); return; }
  if (vlabel) {
    // find variant by label
    const conf = await api('/admin/confessions/' + body.confession_id).catch(() => null);
    const v = (conf?.variants || []).find(x => x.label === vlabel);
    if (v) body.variant_id = v.id; else { toast('Variant label not found on this confession', false); return; }
  }
  try { await api('/admin/audio', { method: 'POST', body: JSON.stringify(body) }); toast('Audio attached'); render(); }
  catch (e) { toast(e.message, false); }
}

async function viewUsers() {
  return `
    <h1 class="page-title">Users</h1>
    <p class="page-sub">Manage admin roles and subscriptions</p>
    <div class="split">
      <div class="panel"><h3>Grant admin role</h3>
        <label>User ID</label><input id="u-role-id" placeholder="uuid">
        <label>Role</label><select id="u-role">
          <option value="super_admin">super_admin</option><option value="content_admin">content_admin</option>
          <option value="audio_producer">audio_producer</option><option value="theological_reviewer">theological_reviewer</option>
          <option value="support_admin">support_admin</option><option value="analytics_admin">analytics_admin</option>
        </select>
        <div class="mt"><button class="btn" onclick="setRole()">Set role</button></div>
      </div>
      <div class="panel"><h3>Set subscription</h3>
        <label>User ID</label><input id="u-sub-id" placeholder="uuid">
        <label>Plan</label><select id="u-sub-plan"><option value="free">free</option><option value="premium">premium</option></select>
        <div class="mt"><button class="btn" onclick="setSubscription()">Set plan</button></div>
      </div>
    </div>
    <p class="hint">User IDs are visible via <code>GET /me</code> for the signed-in user, or from your database. The admin console intentionally avoids browsing private user content (per PRD §39).</p>`;
}

async function setRole() {
  const body = { user_id: document.getElementById('u-role-id').value.trim(), role: document.getElementById('u-role').value };
  if (!body.user_id) { toast('User ID required', false); return; }
  try { await api('/admin/users/role', { method: 'POST', body: JSON.stringify(body) }); toast('Role set'); }
  catch (e) { toast(e.message, false); }
}
async function setSubscription() {
  const body = { user_id: document.getElementById('u-sub-id').value.trim(), plan: document.getElementById('u-sub-plan').value, status: 'active' };
  if (!body.user_id) { toast('User ID required', false); return; }
  try { await api('/admin/users/subscription', { method: 'POST', body: JSON.stringify(body) }); toast('Subscription set'); }
  catch (e) { toast(e.message, false); }
}

// ---------- render ----------
async function render() {
  if (!state.token) { loginScreen(); return; }
  // show shell immediately with a loading placeholder
  document.getElementById('app').innerHTML = shell('<div class="muted">Loading…</div>');
  try {
    let inner = '';
    if (current === 'dashboard') inner = await viewDashboard();
    else if (current === 'categories') inner = await viewCategories();
    else if (current === 'confessions') inner = await viewConfessions();
    else if (current === 'voices') inner = await viewVoices();
    else if (current === 'audio') inner = await viewAudio();
    else if (current === 'users') inner = await viewUsers();
    document.getElementById('app').innerHTML = shell(inner);
  } catch (e) {
    if (e.message.includes('admin access') || e.message.includes('unauthorized') || e.message.includes('403')) {
      logout();
    } else {
      document.getElementById('app').innerHTML = shell(`<div class="panel"><h3>Error</h3><p class="muted">${esc(e.message)}</p></div>`);
    }
  }
}

render();
</script>
</body>
</html>

'@
$full = Join-Path $RepoRoot 'internal\adminui\index.html'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'internal\adminui\index.html', $bytes)
$count++

# ---- internal\adminui\ui.go ----
$c = @'
// Package adminui embeds and serves the admin console single-page app.
package adminui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed index.html
var files embed.FS

// Handler serves the admin console SPA. It is served unauthenticated because the
// app itself performs login and calls protected API endpoints; the page contains
// no privileged data.
func Handler() http.Handler {
	sub, _ := fs.Sub(files, ".")
	return http.FileServer(http.FS(sub))
}

'@
$full = Join-Path $RepoRoot 'internal\adminui\ui.go'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'internal\adminui\ui.go', $bytes)
$count++

# ---- internal\auth\auth.go ----
$c = @'
package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var ErrUnauthorized = errors.New("unauthorized")

// Claims are the JWT claims issued to authenticated users and admins.
type Claims struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Role  string `json:"role,omitempty"` // admin role if present, else ""
	jwt.RegisteredClaims
}

func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

func SignToken(secret, ttl, sub, email, role string) (string, error) {
	d, err := time.ParseDuration(ttl)
	if err != nil {
		d = 720 * time.Hour
	}
	claims := Claims{
		Sub:   sub,
		Email: email,
		Role:  role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(d)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "i-confess",
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func ParseToken(secret, tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		return nil, ErrUnauthorized
	}
	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, ErrUnauthorized
	}
	return claims, nil
}

// Middleware returns an http middleware that requires a valid bearer token.
func Middleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := fromRequest(secret, r)
			if err != nil {
				httpx.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			next.ServeHTTP(w, withClaims(r, c))
		})
	}
}

// AdminMiddleware requires a valid token AND an admin role.
func AdminMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := fromRequest(secret, r)
			if err != nil || c.Role == "" {
				httpx.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "admin access required"})
				return
			}
			next.ServeHTTP(w, withClaims(r, c))
		})
	}
}

func fromRequest(secret string, r *http.Request) (*Claims, error) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return nil, ErrUnauthorized
	}
	return ParseToken(secret, strings.TrimPrefix(h, "Bearer "))
}

type ctxKey int

const claimsKey ctxKey = 0

func withClaims(r *http.Request, c *Claims) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), claimsKey, c))
}

// FromContext returns the claims attached by the middleware (nil if absent).
func FromContext(r *http.Request) *Claims {
	c, _ := r.Context().Value(claimsKey).(*Claims)
	return c
}

'@
$full = Join-Path $RepoRoot 'internal\auth\auth.go'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'internal\auth\auth.go', $bytes)
$count++

# ---- internal\config\config.go ----
$c = @'
package config

import "os"

// Config holds runtime configuration sourced from environment variables.
type Config struct {
	Port      string
	DBPath    string
	JWTSecret string
	TokenTTL  string
	Env       string
}

func Load() Config {
	return Config{
		Port:      getenv("PORT", "8080"),
		DBPath:    getenv("DB_PATH", "data/iconfess.db"),
		JWTSecret: getenv("JWT_SECRET", "dev-only-change-me"),
		TokenTTL:  getenv("TOKEN_TTL", "720h"),
		Env:       getenv("ENV", "development"),
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

'@
$full = Join-Path $RepoRoot 'internal\config\config.go'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'internal\config\config.go', $bytes)
$count++

# ---- internal\db\db.go ----
$c = @'
package db

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// Open opens (and migrates) the SQLite database at path.
// The canonical production schema is PostgreSQL (see migrations/postgres); SQLite
// is used for the local/demo environment with a structurally equivalent schema.
func Open(path string) (*sql.DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}

	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)", path)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	conn.SetMaxOpenConns(1) // SQLite: serialize writes, avoid SQLITE_BUSY

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := conn.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	if err := migrate(conn); err != nil {
		return nil, err
	}
	return conn, nil
}

func migrate(conn *sql.DB) error {
	// Strip comment lines before splitting on statement terminators.
	var lines []string
	for _, line := range strings.Split(schemaSQL, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		lines = append(lines, line)
	}
	clean := strings.Join(lines, "\n")

	for _, stmt := range strings.Split(clean, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := conn.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: %w (stmt: %.60s...)", err, stmt)
		}
	}
	return nil
}

'@
$full = Join-Path $RepoRoot 'internal\db\db.go'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'internal\db\db.go', $bytes)
$count++

# ---- internal\db\schema.sql ----
$c = @'
-- i-confess — SQLite (dev/local) schema
-- Canonical production schema is PostgreSQL (migrations/postgres/0001_schema.sql)
-- The two are kept structurally equivalent (TEXT ids = UUID, RFC3339 TEXT = TIMESTAMPTZ)

PRAGMA foreign_keys = ON;

-- ============================= CONTENT =============================
CREATE TABLE IF NOT EXISTS collections (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    description TEXT,
    premium     INTEGER NOT NULL DEFAULT 0,
    status      TEXT NOT NULL DEFAULT 'draft',          -- draft | published | archived
    sort_order  INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS categories (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    description TEXT,
    icon        TEXT,
    premium     INTEGER NOT NULL DEFAULT 0,
    status      TEXT NOT NULL DEFAULT 'draft',
    sort_order  INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

-- A category may belong to many collections (e.g. the "28" and "38" share categories).
CREATE TABLE IF NOT EXISTS collection_categories (
    collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    category_id   TEXT NOT NULL REFERENCES categories(id)  ON DELETE CASCADE,
    sort_order    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (collection_id, category_id)
);

CREATE TABLE IF NOT EXISTS confessions (
    id          TEXT PRIMARY KEY,
    category_id TEXT NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    short_text  TEXT,
    medium_text TEXT,
    long_text   TEXT,
    description TEXT,
    tags        TEXT,                                    -- comma-separated
    intensity   INTEGER NOT NULL DEFAULT 1,              -- 1..5
    language    TEXT NOT NULL DEFAULT 'en',
    status      TEXT NOT NULL DEFAULT 'draft',
    -- lifecycle: draft|content_review|theological_review|audio_production|audio_qa|approved|published|archived
    author        TEXT,
    version       INTEGER NOT NULL DEFAULT 1,
    published_at  TEXT,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS confession_variants (
    id               TEXT PRIMARY KEY,
    confession_id    TEXT NOT NULL REFERENCES confessions(id) ON DELETE CASCADE,
    label            TEXT NOT NULL,                      -- '30s' | '1m' | '3m' | '5m' | '10m'
    duration_seconds INTEGER NOT NULL,
    sort_order       INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS scripture_references (
    id              TEXT PRIMARY KEY,
    confession_id   TEXT NOT NULL REFERENCES confessions(id) ON DELETE CASCADE,
    book            TEXT NOT NULL,
    chapter         INTEGER,
    verse           TEXT,                                -- e.g. '5' or '3-6'
    translation     TEXT NOT NULL DEFAULT 'KJV',
    is_direct_quote INTEGER NOT NULL DEFAULT 0,          -- 1 = verbatim quotation, 0 = paraphrase inspired by scripture
    notes           TEXT,
    sort_order      INTEGER NOT NULL DEFAULT 0
);

-- ============================= AUDIO =============================
CREATE TABLE IF NOT EXISTS voices (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT,
    type        TEXT NOT NULL DEFAULT 'professional',    -- professional | minister | generic
    provider    TEXT,
    gender      TEXT,                                    -- male | female | neutral
    language    TEXT NOT NULL DEFAULT 'en',
    premium     INTEGER NOT NULL DEFAULT 0,
    status      TEXT NOT NULL DEFAULT 'active',
    sample_url  TEXT,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS voice_licenses (
    id               TEXT PRIMARY KEY,
    voice_id         TEXT NOT NULL REFERENCES voices(id) ON DELETE CASCADE,
    owner            TEXT,
    provider         TEXT,
    license_status   TEXT NOT NULL DEFAULT 'none',       -- none | pending | active | expired | revoked
    license_start    TEXT,
    license_expiry   TEXT,
    allowed_regions  TEXT,
    commercial_usage INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS audio_assets (
    id               TEXT PRIMARY KEY,
    confession_id    TEXT NOT NULL REFERENCES confessions(id) ON DELETE CASCADE,
    variant_id       TEXT REFERENCES confession_variants(id) ON DELETE SET NULL,
    voice_id         TEXT NOT NULL REFERENCES voices(id) ON DELETE CASCADE,
    url              TEXT NOT NULL,
    duration_seconds INTEGER,
    size_bytes       INTEGER,
    status           TEXT NOT NULL DEFAULT 'ready',      -- queued | processing | ready | failed | archived
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL
);

-- ============================= USERS =============================
CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_name  TEXT,
    timezone      TEXT NOT NULL DEFAULT 'UTC',
    status        TEXT NOT NULL DEFAULT 'active',
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS subscriptions (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan       TEXT NOT NULL DEFAULT 'free',             -- free | premium
    status     TEXT NOT NULL DEFAULT 'active',
    started_at TEXT,
    ends_at    TEXT,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS session_preferences (
    user_id                  TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    default_duration_seconds INTEGER NOT NULL DEFAULT 1800,
    default_voice_id         TEXT,
    updated_at               TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS schedules (
    id               TEXT PRIMARY KEY,
    user_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label            TEXT NOT NULL,
    time             TEXT NOT NULL,                      -- 'HH:MM' local
    days_of_week     TEXT NOT NULL DEFAULT '1,2,3,4,5,6,7', -- 1=Mon .. 7=Sun
    timezone         TEXT NOT NULL DEFAULT 'UTC',
    duration_seconds INTEGER NOT NULL DEFAULT 1800,
    voice_id         TEXT,
    category_ids     TEXT,                               -- comma-separated category ids
    enabled          INTEGER NOT NULL DEFAULT 1,
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL
);

-- ============================= SESSIONS =============================
CREATE TABLE IF NOT EXISTS sessions (
    id               TEXT PRIMARY KEY,
    user_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type             TEXT NOT NULL DEFAULT 'standard',   -- quick | standard | deep | custom | personal
    duration_seconds INTEGER NOT NULL,
    voice_id         TEXT,
    status           TEXT NOT NULL DEFAULT 'created',    -- created | playing | completed | abandoned
    created_at       TEXT NOT NULL,
    started_at       TEXT,
    completed_at     TEXT
);

CREATE TABLE IF NOT EXISTS session_items (
    id               TEXT PRIMARY KEY,
    session_id       TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    confession_id    TEXT NOT NULL REFERENCES confessions(id) ON DELETE CASCADE,
    variant_id       TEXT,
    voice_id         TEXT,
    audio_asset_id   TEXT,
    position         INTEGER NOT NULL,
    duration_seconds INTEGER NOT NULL,
    status           TEXT NOT NULL DEFAULT 'queued'      -- queued | played | skipped
);

-- ============================= ENGAGEMENT =============================
CREATE TABLE IF NOT EXISTS favorites (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL,                           -- confession | category | session | voice
    entity_id   TEXT NOT NULL,
    created_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS playback_history (
    id               TEXT PRIMARY KEY,
    user_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id       TEXT,
    confession_id    TEXT,
    duration_seconds INTEGER,
    completed        INTEGER NOT NULL DEFAULT 0,
    skipped          INTEGER NOT NULL DEFAULT 0,
    listened_at      TEXT NOT NULL
);

-- ============================= USER CONTENT =============================
CREATE TABLE IF NOT EXISTS user_confessions (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title       TEXT NOT NULL,
    text        TEXT NOT NULL,
    category_id TEXT,
    is_private  INTEGER NOT NULL DEFAULT 1,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS user_confession_audio (
    id                 TEXT PRIMARY KEY,
    user_confession_id TEXT NOT NULL REFERENCES user_confessions(id) ON DELETE CASCADE,
    voice_id           TEXT,
    url                TEXT,
    status             TEXT NOT NULL DEFAULT 'queued',
    created_at         TEXT NOT NULL
);

-- ============================= ADMIN =============================
CREATE TABLE IF NOT EXISTS admin_users (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    role       TEXT NOT NULL DEFAULT 'support',          -- super_admin | content_admin | audio_producer | theological_reviewer | support_admin | analytics_admin
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id            TEXT PRIMARY KEY,
    admin_user_id TEXT,
    action        TEXT NOT NULL,
    entity        TEXT NOT NULL,
    entity_id     TEXT,
    created_at    TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_confessions_category ON confessions(category_id);
CREATE INDEX IF NOT EXISTS idx_confessions_status   ON confessions(status);
CREATE INDEX IF NOT EXISTS idx_audio_assets_confession_voice ON audio_assets(confession_id, voice_id);
CREATE INDEX IF NOT EXISTS idx_session_items_session ON session_items(session_id);
CREATE INDEX IF NOT EXISTS idx_schedules_user ON schedules(user_id);
CREATE INDEX IF NOT EXISTS idx_favorites_user ON favorites(user_id);

'@
$full = Join-Path $RepoRoot 'internal\db\schema.sql'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'internal\db\schema.sql', $bytes)
$count++

# ---- internal\engine\engine.go ----
$c = @'
// Package engine implements the Session Engine: the deterministic MVP logic that
// turns (categories, duration, voice, preferences) into an ordered confession session.
package engine

import (
	"context"
	"errors"
	"sort"

	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/store"
)

var (
	ErrNoContent = errors.New("no published content available for the selected categories")
	ErrNoVoice   = errors.New("no available voice")
)

// Engine composes a session from published content with ready audio.
type Engine struct {
	content *store.ContentStore
	audio   *store.AudioStore
	users   *store.UserStore
}

func New(content *store.ContentStore, audio *store.AudioStore, users *store.UserStore) *Engine {
	return &Engine{content: content, audio: audio, users: users}
}

// Request is the session-engine input.
type Request struct {
	UserID          string
	CategoryIDs     []string
	DurationSeconds int
	VoiceID         string
}

// Build assembles an ordered, deterministic session.
//
// MVP rules:
//  1. Only published confessions with a ready audio asset for the chosen voice are used.
//  2. Categories are cycled round-robin so each requested category is represented evenly.
//  3. The shortest variant that still fits the remaining budget is preferred (packing).
//  4. If a premium voice is requested by a free user, it falls back to an available free voice.
func (e *Engine) Build(ctx context.Context, req Request) (*models.Session, error) {
	if req.DurationSeconds <= 0 {
		req.DurationSeconds = 1800
	}

	voice, err := e.resolveVoice(ctx, req)
	if err != nil {
		return nil, err
	}

	// Collect eligible confessions (with audio for this voice), grouped by category.
	byCat := map[string][]*models.Confession{}
	available, err := e.audio.ConfessionIDsWithVoice(ctx, voice.ID)
	if err != nil {
		return nil, err
	}

	for _, catID := range req.CategoryIDs {
		confs, err := e.content.ConfessionsByCategory(ctx, catID, true)
		if err != nil {
			return nil, err
		}
		for i := range confs {
			c := &confs[i]
			if !available[c.ID] {
				continue
			}
			c.Variants, _ = e.content.Variants(ctx, c.ID)
			if len(c.Variants) == 0 {
				continue
			}
			byCat[catID] = append(byCat[catID], c)
		}
	}
	if len(byCat) == 0 {
		return nil, ErrNoContent
	}

	items, total := e.pack(ctx, req, voice.ID, byCat)
	if len(items) == 0 {
		return nil, ErrNoContent
	}

	return &models.Session{
		UserID:          req.UserID,
		Type:            classifyType(total),
		DurationSeconds: total,
		VoiceID:         voice.ID,
		Status:          "created",
		Items:           items,
	}, nil
}

// resolveVoice validates the requested voice and applies the premium fallback.
func (e *Engine) resolveVoice(ctx context.Context, req Request) (*models.Voice, error) {
	if req.VoiceID != "" {
		v, err := e.audio.VoiceByID(ctx, req.VoiceID)
		if err != nil {
			return nil, err
		}
		if !v.Premium {
			return v, nil
		}
		plan, _ := e.users.Subscription(ctx, req.UserID)
		if plan == "premium" {
			return v, nil
		}
		// Premium voice requested by a free user → fall back to a free voice.
	}
	// Default: first free voice.
	voices, err := e.audio.ListVoices(ctx)
	if err != nil {
		return nil, err
	}
	for _, v := range voices {
		if v.Status == "active" && !v.Premium {
			return &v, nil
		}
	}
	return nil, ErrNoVoice
}

// pack greedily fills the budget by cycling categories round-robin and, within
// each category, cycling its confessions. Confessions repeat as needed to reach
// the requested duration (matching PRD §19), and the longest variant that still
// fits the remaining budget is preferred so sessions fill efficiently.
func (e *Engine) pack(ctx context.Context, req Request, voiceID string, byCat map[string][]*models.Confession) ([]models.SessionItem, int) {
	type variantOption struct {
		variant models.ConfessionVariant
		asset   models.AudioAsset
	}
	type confessionOption struct {
		conf     *models.Confession
		variants []variantOption // sorted descending by duration
	}

	catOptions := map[string][]*confessionOption{}
	var catOrder []string
	for catID, confs := range byCat {
		for _, c := range confs {
			assets, _ := e.audio.AssetsFor(ctx, c.ID, voiceID)
			if len(assets) == 0 {
				continue
			}
			// Sort variants descending by duration for best-fit filling.
			sorted := append([]models.ConfessionVariant(nil), c.Variants...)
			sort.Slice(sorted, func(i, j int) bool { return sorted[i].DurationSeconds > sorted[j].DurationSeconds })
			var opts []variantOption
			for _, v := range sorted {
				if asset, ok := matchAsset(assets, v.ID); ok {
					opts = append(opts, variantOption{variant: v, asset: asset})
				}
			}
			if len(opts) == 0 {
				continue
			}
			catOptions[catID] = append(catOptions[catID], &confessionOption{conf: c, variants: opts})
		}
		if len(catOptions[catID]) > 0 {
			catOrder = append(catOrder, catID)
		}
	}
	if len(catOrder) == 0 {
		return nil, 0
	}

	cursors := map[string]int{}
	remaining := req.DurationSeconds
	var items []models.SessionItem
	idx := 0

	for remaining > 0 {
		progressed := false
		for _, catID := range catOrder {
			opts := catOptions[catID]
			if len(opts) == 0 {
				continue
			}
			opt := opts[cursors[catID]%len(opts)]
			// Longest variant that fits the remaining budget.
			chosen := -1
			for i, vo := range opt.variants {
				if vo.variant.DurationSeconds <= remaining {
					chosen = i
					break
				}
			}
			cursors[catID]++
			if chosen < 0 {
				continue // nothing in this confession fits the remaining time
			}
			vo := opt.variants[chosen]
			remaining -= vo.variant.DurationSeconds
			items = append(items, models.SessionItem{
				ConfessionID:    opt.conf.ID,
				VariantID:       vo.variant.ID,
				VoiceID:         voiceID,
				AudioAssetID:    vo.asset.ID,
				Position:        idx,
				DurationSeconds: vo.variant.DurationSeconds,
				Status:          "queued",
				Title:           opt.conf.Title,
				Category:        catID,
				AudioURL:        vo.asset.URL,
				Text:            opt.conf.MediumText,
			})
			idx++
			progressed = true
			if remaining <= 0 {
				break
			}
		}
		if !progressed {
			break // no variant fits the remaining budget
		}
	}
	total := req.DurationSeconds - remaining
	return items, total
}

func matchAsset(assets []models.AudioAsset, variantID string) (models.AudioAsset, bool) {
	for _, a := range assets {
		if a.VariantID == variantID {
			return a, true
		}
	}
	// If no variant-specific asset, fall back to any asset for the confession.
	if len(assets) > 0 {
		return assets[0], true
	}
	return models.AudioAsset{}, false
}

func classifyType(seconds int) string {
	switch {
	case seconds <= 15*60:
		return "quick"
	case seconds <= 60*60:
		return "standard"
	default:
		return "deep"
	}
}

'@
$full = Join-Path $RepoRoot 'internal\engine\engine.go'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'internal\engine\engine.go', $bytes)
$count++

# ---- internal\engine\engine_test.go ----
$c = @'
package engine

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Teamthy/i-confess/internal/db"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/store"
)

// setup builds an in-memory-ish (temp-file) database with a voice, two categories,
// and confessions with audio, then wires the engine.
func setup(t *testing.T) (*Engine, *store.ContentStore, *store.AudioStore, *store.UserStore) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	content := store.NewContentStore(conn)
	audio := store.NewAudioStore(conn)
	users := store.NewUserStore(conn)
	ctx := context.Background()

	// free user
	u, err := users.Create(ctx, "u@test.com", "hash", "U", "UTC")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	_ = u

	voice := &models.Voice{Name: "V", Type: "professional", Language: "en", Status: "active"}
	if err := audio.CreateVoice(ctx, voice); err != nil {
		t.Fatalf("create voice: %v", err)
	}

	for _, cat := range []string{"healing", "faith"} {
		c := &models.Category{Name: cat, Slug: cat, Status: "published"}
		if err := content.CreateCategory(ctx, c); err != nil {
			t.Fatalf("create category: %v", err)
		}
		// two confessions per category, each with a 60s and 300s variant + audio
		for i := 0; i < 2; i++ {
			conf := &models.Confession{
				CategoryID: c.ID, Title: cat + "-conf", MediumText: "text",
				Status: "published", Language: "en",
				Variants: []models.ConfessionVariant{
					{Label: "1m", DurationSeconds: 60},
					{Label: "5m", DurationSeconds: 300},
				},
			}
			if err := content.CreateConfession(ctx, conf); err != nil {
				t.Fatalf("create confession: %v", err)
			}
			for _, v := range conf.Variants {
				a := &models.AudioAsset{
					ConfessionID: conf.ID, VariantID: v.ID, VoiceID: voice.ID,
					URL: "/media/x.wav", DurationSeconds: v.DurationSeconds, Status: "ready",
				}
				if err := audio.UpsertAsset(ctx, a); err != nil {
					t.Fatalf("upsert asset: %v", err)
				}
			}
		}
	}

	return New(content, audio, users), content, audio, users
}

func TestBuildFillsDurationAndCycles(t *testing.T) {
	ctx := context.Background()
	e, content, _, users := setup(t)

	cats, _ := content.ListCategories(ctx, true)
	u, _, _ := users.ByEmail(ctx, "u@test.com")

	sess, err := e.Build(ctx, Request{
		UserID:          u.ID,
		CategoryIDs:     []string{cats[0].ID, cats[1].ID},
		DurationSeconds: 600,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	total := 0
	for _, it := range sess.Items {
		total += it.DurationSeconds
	}
	if total != 600 {
		t.Errorf("expected 600s total, got %d (items=%d)", total, len(sess.Items))
	}
	if sess.VoiceID == "" {
		t.Error("expected a resolved voice id")
	}
}

func TestBuildNoContent(t *testing.T) {
	ctx := context.Background()
	e, content, _, users := setup(t)

	// A category with no confessions → no eligible content.
	c := &models.Category{Name: "empty", Slug: "empty", Status: "published"}
	_ = content.CreateCategory(ctx, c)

	u, _, _ := users.ByEmail(ctx, "u@test.com")
	if _, err := e.Build(ctx, Request{UserID: u.ID, CategoryIDs: []string{c.ID}, DurationSeconds: 300}); err == nil {
		t.Error("expected ErrNoContent for a category with no content")
	}
}

'@
$full = Join-Path $RepoRoot 'internal\engine\engine_test.go'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'internal\engine\engine_test.go', $bytes)
$count++

# ---- internal\httpx\httpx.go ----
$c = @'
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// WriteJSON writes v as JSON with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError writes a consistent error envelope.
func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

// DecodeJSON decodes the request body, enforcing a size limit and single value.
func DecodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	// Ensure there is no trailing data.
	if dec.More() {
		return errors.New("unexpected trailing JSON")
	}
	return nil
}

'@
$full = Join-Path $RepoRoot 'internal\httpx\httpx.go'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'internal\httpx\httpx.go', $bytes)
$count++

# ---- internal\media\wav.go ----
$c = @'
package media

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
)

// WriteTone generates a gentle sine-tone WAV file (8 kHz, 8-bit, mono) of the
// given duration. It exists only to provide local placeholder audio for the
// demo/dev environment so the full playback loop can be exercised end-to-end.
// Real audio comes from the production TTS/recording pipeline.
func WriteTone(path string, seconds int) error {
	const sampleRate = 8000
	const freq = 220.0 // A3 — calm, low tone

	n := sampleRate * seconds
	data := make([]byte, n)
	for i := 0; i < n; i++ {
		t := float64(i) / sampleRate
		// Soft sine with a slow fade-in/out to avoid clicks.
		env := 1.0
		if i < sampleRate/10 {
			env = float64(i) / (sampleRate / 10)
		} else if i > n-sampleRate/10 {
			env = float64(n-i) / (sampleRate / 10)
		}
		sample := math.Sin(2*math.Pi*freq*t) * 0.35 * env
		data[i] = uint8(int8(sample * 127))
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// RIFF header
	write := func(v any) { _ = binary.Write(f, binary.LittleEndian, v) }
	write([]byte("RIFF"))
	write(uint32(36 + n)) // chunk size
	write([]byte("WAVE"))
	write([]byte("fmt "))
	write(uint32(16))         // fmt chunk size
	write(uint16(1))          // PCM
	write(uint16(1))          // mono
	write(uint32(sampleRate)) // sample rate
	write(uint32(sampleRate)) // byte rate
	write(uint16(1))          // block align
	write(uint16(8))          // bits per sample
	write([]byte("data"))
	write(uint32(n))
	_, err = f.Write(data)
	return err
}

'@
$full = Join-Path $RepoRoot 'internal\media\wav.go'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'internal\media\wav.go', $bytes)
$count++

# ---- internal\models\models.go ----
$c = @'
package models

// Timestamps are stored as RFC3339 strings in SQLite and TIMESTAMPTZ in Postgres.
type Collection struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description,omitempty"`
	Premium     bool   `json:"premium"`
	Status      string `json:"status"`
	SortOrder   int    `json:"sort_order"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type Category struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Premium     bool   `json:"premium"`
	Status      string `json:"status"`
	SortOrder   int    `json:"sort_order"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type Confession struct {
	ID          string              `json:"id"`
	CategoryID  string              `json:"category_id"`
	Title       string              `json:"title"`
	ShortText   string              `json:"short_text,omitempty"`
	MediumText  string              `json:"medium_text,omitempty"`
	LongText    string              `json:"long_text,omitempty"`
	Description string              `json:"description,omitempty"`
	Tags        []string            `json:"tags,omitempty"`
	Intensity   int                 `json:"intensity"`
	Language    string              `json:"language"`
	Status      string              `json:"status"`
	Author      string              `json:"author,omitempty"`
	Version     int                 `json:"version"`
	PublishedAt string              `json:"published_at,omitempty"`
	CreatedAt   string              `json:"created_at"`
	UpdatedAt   string              `json:"updated_at"`
	Variants    []ConfessionVariant `json:"variants,omitempty"`
	Scriptures  []ScriptureRef      `json:"scriptures,omitempty"`
}

type ConfessionVariant struct {
	ID              string `json:"id"`
	ConfessionID    string `json:"confession_id"`
	Label           string `json:"label"`
	DurationSeconds int    `json:"duration_seconds"`
	SortOrder       int    `json:"sort_order"`
}

type ScriptureRef struct {
	ID            string `json:"id"`
	ConfessionID  string `json:"confession_id"`
	Book          string `json:"book"`
	Chapter       int    `json:"chapter,omitempty"`
	Verse         string `json:"verse,omitempty"`
	Translation   string `json:"translation"`
	IsDirectQuote bool   `json:"is_direct_quote"`
	Notes         string `json:"notes,omitempty"`
	SortOrder     int    `json:"sort_order"`
}

type Voice struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type"`
	Provider    string `json:"provider,omitempty"`
	Gender      string `json:"gender,omitempty"`
	Language    string `json:"language"`
	Premium     bool   `json:"premium"`
	Status      string `json:"status"`
	SampleURL   string `json:"sample_url,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type AudioAsset struct {
	ID              string `json:"id"`
	ConfessionID    string `json:"confession_id"`
	VariantID       string `json:"variant_id,omitempty"`
	VoiceID         string `json:"voice_id"`
	URL             string `json:"url"`
	DurationSeconds int    `json:"duration_seconds,omitempty"`
	SizeBytes       int64  `json:"size_bytes,omitempty"`
	Status          string `json:"status"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type User struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name,omitempty"`
	Timezone    string `json:"timezone"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
}

type Subscription struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Plan      string `json:"plan"`
	Status    string `json:"status"`
	StartedAt string `json:"started_at,omitempty"`
	EndsAt    string `json:"ends_at,omitempty"`
	CreatedAt string `json:"created_at"`
}

type Schedule struct {
	ID              string   `json:"id"`
	UserID          string   `json:"user_id"`
	Label           string   `json:"label"`
	Time            string   `json:"time"`
	DaysOfWeek      []int    `json:"days_of_week"`
	Timezone        string   `json:"timezone"`
	DurationSeconds int      `json:"duration_seconds"`
	VoiceID         string   `json:"voice_id,omitempty"`
	CategoryIDs     []string `json:"category_ids,omitempty"`
	Enabled         bool     `json:"enabled"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
}

type Session struct {
	ID              string        `json:"id"`
	UserID          string        `json:"user_id"`
	Type            string        `json:"type"`
	DurationSeconds int           `json:"duration_seconds"`
	VoiceID         string        `json:"voice_id,omitempty"`
	Status          string        `json:"status"`
	CreatedAt       string        `json:"created_at"`
	StartedAt       string        `json:"started_at,omitempty"`
	CompletedAt     string        `json:"completed_at,omitempty"`
	Items           []SessionItem `json:"items,omitempty"`
}

type SessionItem struct {
	ID              string `json:"id"`
	SessionID       string `json:"session_id"`
	ConfessionID    string `json:"confession_id"`
	VariantID       string `json:"variant_id,omitempty"`
	VoiceID         string `json:"voice_id,omitempty"`
	AudioAssetID    string `json:"audio_asset_id,omitempty"`
	Position        int    `json:"position"`
	DurationSeconds int    `json:"duration_seconds"`
	Status          string `json:"status"`
	// Denormalized for the player payload:
	Title    string `json:"title,omitempty"`
	Category string `json:"category,omitempty"`
	AudioURL string `json:"audio_url,omitempty"`
	Text     string `json:"text,omitempty"`
}

type UserConfession struct {
	ID         string `json:"id"`
	UserID     string `json:"user_id"`
	Title      string `json:"title"`
	Text       string `json:"text"`
	CategoryID string `json:"category_id,omitempty"`
	IsPrivate  bool   `json:"is_private"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

type Favorite struct {
	ID         string `json:"id"`
	UserID     string `json:"user_id"`
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	CreatedAt  string `json:"created_at"`
}

type PlaybackRecord struct {
	ID              string `json:"id"`
	UserID          string `json:"user_id"`
	SessionID       string `json:"session_id,omitempty"`
	ConfessionID    string `json:"confession_id,omitempty"`
	DurationSeconds int    `json:"duration_seconds,omitempty"`
	Completed       bool   `json:"completed"`
	Skipped         bool   `json:"skipped"`
	ListenedAt      string `json:"listened_at"`
}

'@
$full = Join-Path $RepoRoot 'internal\models\models.go'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'internal\models\models.go', $bytes)
$count++

# ---- internal\seed\seed.go ----
$c = @'
// Package seed populates a fresh database with a representative MVP content
// library: collections, categories, confessions (with variants + scripture
// references), a default voice, local placeholder audio, and a demo admin.
package seed

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/media"
	"github.com/Teamthy/i-confess/internal/models"
	"github.com/Teamthy/i-confess/internal/store"
)

const mediaDir = "data/media"

// Seed runs idempotently: if the database already has categories, it returns early.
func Seed(db *sql.DB) error {
	bg := context.Background()
	content := store.NewContentStore(db)
	cats, err := content.ListCategories(bg, true)
	if err != nil {
		return err
	}
	if len(cats) > 0 {
		return nil // already seeded
	}

	audio := store.NewAudioStore(db)
	users := store.NewUserStore(db)

	// ---- Collections ----
	for _, c := range []models.Collection{
		{Name: "The 28", Slug: "the-28", Status: "published", SortOrder: 1, Description: "Launch collection — 28 core categories."},
		{Name: "The 38", Slug: "the-38", Status: "published", SortOrder: 2, Description: "Principal expanded content architecture — 38 categories."},
	} {
		if err := content.CreateCollection(bg, &c); err != nil {
			return err
		}
	}

	// ---- Categories (representative subset of the 38) ----
	categorySeeds := []struct {
		name, slug, desc, icon string
	}{
		{"Healing", "healing", "Confessions for physical and emotional wholeness.", "healing"},
		{"Faith", "faith", "Declarations that strengthen your faith.", "faith"},
		{"Finance", "finance", "Biblical wisdom for provision and stewardship.", "finance"},
		{"Family", "family", "Declarations over your household.", "family"},
		{"Marriage", "marriage", "Confessions for a strong, God-centered marriage.", "marriage"},
		{"Purpose", "purpose", "Declarations of calling and destiny.", "purpose"},
		{"Breakthrough", "breakthrough", "Confessions for breaking through barriers.", "breakthrough"},
		{"Peace", "peace", "Declarations of calm and rest.", "peace"},
		{"Protection", "protection", "Confessions of safety and covering.", "protection"},
		{"Wisdom", "wisdom", "Declarations for wisdom and clarity.", "wisdom"},
		{"Favor", "favor", "Confessions of grace and favor.", "favor"},
		{"Strength", "strength", "Declarations of endurance and power.", "strength"},
		{"Business", "business", "Confessions for work and enterprise.", "business"},
		{"Joy", "joy", "Declarations of joy and gladness.", "joy"},
		{"Identity", "identity", "Confessions of who you are in Christ.", "identity"},
		{"Thanksgiving", "thanksgiving", "Declarations of gratitude.", "thanksgiving"},
	}

	var categoryIDs []string
	for i, cs := range categorySeeds {
		c := &models.Category{
			Name: cs.name, Slug: cs.slug, Description: cs.desc, Icon: cs.icon,
			Status: "published", SortOrder: i + 1,
		}
		if err := content.CreateCategory(bg, c); err != nil {
			return err
		}
		categoryIDs = append(categoryIDs, c.ID)
	}

	// Attach all categories to both collections (28 → subset of 38 for demo).
	cols, _ := content.ListCollections(bg, true)
	for _, col := range cols {
		for i, cid := range categoryIDs {
			_ = content.AddCategoryToCollection(bg, col.ID, cid, i)
		}
	}

	// ---- Voice ----
	voice := &models.Voice{
		Name: "Grace", Description: "Warm, calm professional narration voice.",
		Type: "professional", Provider: "i-confess", Gender: "female",
		Language: "en", Premium: false, Status: "active",
	}
	if err := audio.CreateVoice(bg, voice); err != nil {
		return err
	}

	// ---- Confessions ----
	// Each confession: title, texts, scripture refs, and duration variants.
	type confessionSeed struct {
		category   string
		title      string
		short      string
		medium     string
		long       string
		scriptures []models.ScriptureRef
		intensity  int
	}
	seeds := []confessionSeed{
		{category: "Healing", title: "I Am Healed", intensity: 3,
			short:  "By His stripes, I am healed.",
			medium: "I declare that by the stripes of Jesus I am healed. Sickness has no authority over my body. I receive wholeness now.",
			long:   "I declare that by the stripes of Jesus I am healed. Sickness and disease have no authority over my body, for my body is the temple of the Holy Spirit. I receive wholeness in every organ, every tissue, and every cell. The life of God flows through me, restoring strength, health, and vitality. I walk in divine health all the days of my life.",
			scriptures: []models.ScriptureRef{
				{Book: "Isaiah", Chapter: 53, Verse: "5", Translation: "KJV", IsDirectQuote: true},
				{Book: "1 Peter", Chapter: 2, Verse: "24", Translation: "KJV", IsDirectQuote: true},
				{Book: "Psalm", Chapter: 103, Verse: "2-3", Translation: "KJV", IsDirectQuote: false},
			}},
		{category: "Healing", title: "Divine Health", intensity: 2,
			short:  "I walk in divine health.",
			medium: "I walk in divine health and wholeness. Every day my strength is renewed like the eagle's.",
			long:   "I walk in divine health and wholeness. Every day my strength is renewed like the eagle's. The same Spirit that raised Christ from the dead lives in me and quickens my mortal body. I am strong, I am whole, and I refuse to accept any report that contradicts the finished work of the cross.",
			scriptures: []models.ScriptureRef{
				{Book: "Romans", Chapter: 8, Verse: "11", Translation: "KJV", IsDirectQuote: true},
				{Book: "Isaiah", Chapter: 40, Verse: "31", Translation: "KJV", IsDirectQuote: false},
			}},
		{category: "Faith", title: "Faith That Moves", intensity: 3,
			short:  "My faith moves mountains.",
			medium: "I have the faith of God. I speak to my mountains and they move. Nothing is impossible for me.",
			long:   "I have the God-kind of faith. I speak to the mountains in my life and they move. I believe without wavering, for I know that the word of God cannot fail. My faith grows stronger every day as I hear and confess the word. Nothing shall be impossible for me because I believe.",
			scriptures: []models.ScriptureRef{
				{Book: "Mark", Chapter: 11, Verse: "22-24", Translation: "KJV", IsDirectQuote: true},
				{Book: "Romans", Chapter: 10, Verse: "17", Translation: "KJV", IsDirectQuote: false},
			}},
		{category: "Faith", title: "Unshakable Trust", intensity: 2,
			short:  "I trust in the Lord with all my heart.",
			medium: "I trust in the Lord with all my heart and lean not on my own understanding. He directs my paths.",
			long:   "I trust in the Lord with all my heart and lean not on my own understanding. In all my ways I acknowledge Him, and He directs my paths. I will not be shaken, for my confidence is in the Lord and not in man. My steps are ordered by God.",
			scriptures: []models.ScriptureRef{
				{Book: "Proverbs", Chapter: 3, Verse: "5-6", Translation: "KJV", IsDirectQuote: true},
			}},
		{category: "Finance", title: "I Am Prosperous", intensity: 3,
			short:  "I am abundantly provided for.",
			medium: "I declare that I am blessed in the city and blessed in the field. My God supplies all my needs according to His riches.",
			long:   "I declare that I am blessed in the city and blessed in the field, blessed coming in and blessed going out. My God supplies all my needs according to His riches in glory. I am a generous giver, and the windows of heaven are open over my life. Wealth and riches are in my house because I honor the Lord.",
			scriptures: []models.ScriptureRef{
				{Book: "Deuteronomy", Chapter: 28, Verse: "3-6", Translation: "KJV", IsDirectQuote: true},
				{Book: "Philippians", Chapter: 4, Verse: "19", Translation: "KJV", IsDirectQuote: true},
			}},
		{category: "Finance", title: "Steward of Abundance", intensity: 2,
			short:  "I am a wise steward.",
			medium: "I am a faithful steward of all God entrusts to me. Wisdom guides my financial decisions.",
			long:   "I am a faithful and wise steward of everything God entrusts to my hands. Wisdom guides my financial decisions, and I am diligent in my labor. I honor God with the firstfruits of my increase, and my barns are filled with plenty. Poverty is far from me.",
			scriptures: []models.ScriptureRef{
				{Book: "Proverbs", Chapter: 3, Verse: "9-10", Translation: "KJV", IsDirectQuote: false},
				{Book: "Luke", Chapter: 16, Verse: "10", Translation: "KJV", IsDirectQuote: false},
			}},
		{category: "Marriage", title: "A Blessed Union", intensity: 2,
			short:  "My marriage is blessed.",
			medium: "My marriage is founded on Christ. I love and honor my spouse, and our home is filled with peace.",
			long:   "My marriage is founded on the rock of Christ. I love and honor my spouse with patience and kindness. Our home is filled with peace, joy, and the presence of God. What God has joined together, no one can separate. We are heirs together of the grace of life.",
			scriptures: []models.ScriptureRef{
				{Book: "Ephesians", Chapter: 5, Verse: "25", Translation: "KJV", IsDirectQuote: false},
				{Book: "Mark", Chapter: 10, Verse: "9", Translation: "KJV", IsDirectQuote: true},
			}},
		{category: "Purpose", title: "Called With Purpose", intensity: 3,
			short:  "I was created for a purpose.",
			medium: "I was created for a divine purpose. My steps are ordered, and I fulfill the good works prepared for me.",
			long:   "I was created for a divine purpose. Before I was formed in the womb, God knew me and set me apart. My steps are ordered by the Lord, and I fulfill the good works prepared for me before the foundation of the world. I will not live a small life; I will fulfill my assignment with boldness and grace.",
			scriptures: []models.ScriptureRef{
				{Book: "Jeremiah", Chapter: 1, Verse: "5", Translation: "KJV", IsDirectQuote: true},
				{Book: "Ephesians", Chapter: 2, Verse: "10", Translation: "KJV", IsDirectQuote: true},
			}},
		{category: "Peace", title: "Peace Beyond Understanding", intensity: 1,
			short:  "The peace of God guards my heart.",
			medium: "I am not anxious about anything. The peace of God that passes understanding guards my heart and mind.",
			long:   "I refuse to be anxious about anything. In every situation I bring my requests to God with thanksgiving, and the peace of God that passes all understanding guards my heart and mind in Christ Jesus. I am calm, I am still, and I know that God is with me.",
			scriptures: []models.ScriptureRef{
				{Book: "Philippians", Chapter: 4, Verse: "6-7", Translation: "KJV", IsDirectQuote: true},
			}},
		{category: "Wisdom", title: "Wisdom And Clarity", intensity: 2,
			short:  "I walk in wisdom and clarity.",
			medium: "I walk in wisdom and clarity. If I lack wisdom, I ask of God, and He gives generously.",
			long:   "I walk in wisdom and clarity. If I lack wisdom, I ask of God, who gives generously to all without finding fault, and it is given to me. The wisdom of God is in me, and I make excellent decisions. I have insight, understanding, and discernment for every situation.",
			scriptures: []models.ScriptureRef{
				{Book: "James", Chapter: 1, Verse: "5", Translation: "KJV", IsDirectQuote: true},
				{Book: "Proverbs", Chapter: 3, Verse: "5-6", Translation: "KJV", IsDirectQuote: false},
			}},
		{category: "Protection", title: "Under His Wings", intensity: 2,
			short:  "I am safe under His wings.",
			medium: "I dwell in the secret place of the Most High and abide under the shadow of the Almighty. No evil befalls me.",
			long:   "I dwell in the secret place of the Most High and abide under the shadow of the Almighty. He is my refuge and my fortress, my God in whom I trust. No evil shall befall me, nor any plague come near my dwelling, for He gives His angels charge over me.",
			scriptures: []models.ScriptureRef{
				{Book: "Psalm", Chapter: 91, Verse: "1-2", Translation: "KJV", IsDirectQuote: true},
			}},
		{category: "Strength", title: "Renewed Strength", intensity: 3,
			short:  "My strength is renewed.",
			medium: "I am strong in the Lord and in the power of His might. I can do all things through Christ who strengthens me.",
			long:   "I am strong in the Lord and in the power of His might. I can do all things through Christ who strengthens me. Those who wait on the Lord renew their strength; I mount up with wings as eagles, I run and do not grow weary, I walk and do not faint.",
			scriptures: []models.ScriptureRef{
				{Book: "Philippians", Chapter: 4, Verse: "13", Translation: "KJV", IsDirectQuote: true},
				{Book: "Isaiah", Chapter: 40, Verse: "31", Translation: "KJV", IsDirectQuote: true},
			}},
		{category: "Business", title: "Fruitful Work", intensity: 2,
			short:  "The work of my hands is blessed.",
			medium: "The Lord blesses the work of my hands. I am diligent, and my labor is fruitful.",
			long:   "The Lord blesses all the work of my hands. I am diligent in my business and faithful in the small things, therefore I am entrusted with more. My enterprise flourishes, my ideas are excellent, and I bring value to everyone I serve. I operate with wisdom and integrity.",
			scriptures: []models.ScriptureRef{
				{Book: "Deuteronomy", Chapter: 28, Verse: "12", Translation: "KJV", IsDirectQuote: false},
				{Book: "Proverbs", Chapter: 22, Verse: "29", Translation: "KJV", IsDirectQuote: false},
			}},
		{category: "Joy", title: "Joy Unspeakable", intensity: 1,
			short:  "The joy of the Lord is my strength.",
			medium: "The joy of the Lord is my strength. I am filled with joy unspeakable and full of glory.",
			long:   "The joy of the Lord is my strength. I am filled with joy unspeakable and full of glory. This is the day the Lord has made, and I will rejoice and be glad in it. My heart is glad, and my countenance is bright, for the Lord has done great things for me.",
			scriptures: []models.ScriptureRef{
				{Book: "Nehemiah", Chapter: 8, Verse: "10", Translation: "KJV", IsDirectQuote: true},
			}},
		{category: "Identity", title: "I Am Who God Says", intensity: 3,
			short:  "I am a child of God.",
			medium: "I am a child of God, fearfully and wonderfully made. I am accepted in the Beloved.",
			long:   "I am a child of God, fearfully and wonderfully made. I am accepted in the Beloved, chosen before the foundation of the world. I am the head and not the tail, above and not beneath. My identity is not defined by my past, my mistakes, or the opinions of others, but by the word of God.",
			scriptures: []models.ScriptureRef{
				{Book: "1 John", Chapter: 3, Verse: "1", Translation: "KJV", IsDirectQuote: false},
				{Book: "Psalm", Chapter: 139, Verse: "14", Translation: "KJV", IsDirectQuote: true},
			}},
		{category: "Thanksgiving", title: "A Grateful Heart", intensity: 1,
			short:  "I enter His gates with thanksgiving.",
			medium: "I enter His gates with thanksgiving and His courts with praise. My heart is full of gratitude.",
			long:   "I enter His gates with thanksgiving and His courts with praise. I am grateful for the goodness of God in my life. In everything I give thanks, for this is the will of God in Christ Jesus concerning me. My heart overflows with gratitude, and I bless the Lord at all times.",
			scriptures: []models.ScriptureRef{
				{Book: "Psalm", Chapter: 100, Verse: "4", Translation: "KJV", IsDirectQuote: true},
				{Book: "1 Thessalonians", Chapter: 5, Verse: "18", Translation: "KJV", IsDirectQuote: true},
			}},
	}

	// Duration variants used across all confessions.
	variantDefs := []struct {
		label   string
		seconds int
	}{{"30s", 30}, {"1m", 60}, {"3m", 180}, {"5m", 300}}

	catByName := map[string]string{}
	for i, cs := range categorySeeds {
		catByName[cs.name] = categoryIDs[i]
	}

	for _, s := range seeds {
		catID := catByName[s.category]
		var variants []models.ConfessionVariant
		for _, vd := range variantDefs {
			variants = append(variants, models.ConfessionVariant{Label: vd.label, DurationSeconds: vd.seconds})
		}
		c := &models.Confession{
			CategoryID: catID, Title: s.title, ShortText: s.short, MediumText: s.medium, LongText: s.long,
			Intensity: s.intensity, Language: "en", Status: "published", Author: "i-confess content team",
			Variants: variants, Scriptures: s.scriptures,
		}
		if err := content.CreateConfession(bg, c); err != nil {
			return err
		}

		// Generate placeholder audio for each variant and attach it to the voice.
		for _, v := range c.Variants {
			file := fmt.Sprintf("%s/%s-%s.wav", mediaDir, c.ID, v.ID)
			if err := media.WriteTone(file, v.DurationSeconds); err != nil {
				return fmt.Errorf("generate audio: %w", err)
			}
			asset := &models.AudioAsset{
				ConfessionID: c.ID, VariantID: v.ID, VoiceID: voice.ID,
				URL: "/media/" + filepath.Base(file), DurationSeconds: v.DurationSeconds, Status: "ready",
			}
			if err := audio.UpsertAsset(bg, asset); err != nil {
				return err
			}
		}
	}

	// ---- Demo admin + demo user ----
	adminHash, _ := auth.HashPassword("admin12345")
	admin, err := users.Create(bg, "admin@iconfess.dev", adminHash, "Admin", "UTC")
	if err != nil {
		return err
	}
	if err := users.SetAdminRole(bg, admin.ID, "super_admin"); err != nil {
		return err
	}

	userHash, _ := auth.HashPassword("password123")
	if _, err := users.Create(bg, "demo@iconfess.dev", userHash, "Demo User", "Africa/Lagos"); err != nil {
		return err
	}

	log.Printf("seed: created %d categories, %d confessions, 1 voice, demo admin + user",
		len(categorySeeds), len(seeds))
	return nil
}

// EnsureMediaDir creates the media directory if needed (used by tests/tools).
func EnsureMediaDir() error {
	return os.MkdirAll(mediaDir, 0o755)
}

'@
$full = Join-Path $RepoRoot 'internal\seed\seed.go'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'internal\seed\seed.go', $bytes)
$count++

# ---- internal\store\audio.go ----
$c = @'
package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Teamthy/i-confess/internal/models"
)

// AudioStore manages voices and audio assets.
type AudioStore struct{ db *sql.DB }

func NewAudioStore(db *sql.DB) *AudioStore { return &AudioStore{db: db} }

func (s *AudioStore) CreateVoice(ctx context.Context, v *models.Voice) error {
	if v.ID == "" {
		v.ID = newID()
	}
	v.CreatedAt, v.UpdatedAt = now(), now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO voices (id,name,description,type,provider,gender,language,premium,status,sample_url,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		v.ID, v.Name, v.Description, v.Type, v.Provider, v.Gender, v.Language, boolInt(v.Premium), v.Status, v.SampleURL, v.CreatedAt, v.UpdatedAt)
	return err
}

func (s *AudioStore) ListVoices(ctx context.Context) ([]models.Voice, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,name,COALESCE(description,''),type,COALESCE(provider,''),COALESCE(gender,''),language,premium,status,COALESCE(sample_url,''),created_at,updated_at
		 FROM voices ORDER BY premium, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Voice
	for rows.Next() {
		var v models.Voice
		if err := rows.Scan(&v.ID, &v.Name, &v.Description, &v.Type, &v.Provider, &v.Gender, &v.Language, &v.Premium, &v.Status, &v.SampleURL, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *AudioStore) VoiceByID(ctx context.Context, id string) (*models.Voice, error) {
	var v models.Voice
	err := s.db.QueryRowContext(ctx,
		`SELECT id,name,COALESCE(description,''),type,COALESCE(provider,''),COALESCE(gender,''),language,premium,status,COALESCE(sample_url,''),created_at,updated_at
		 FROM voices WHERE id = ?`, id).
		Scan(&v.ID, &v.Name, &v.Description, &v.Type, &v.Provider, &v.Gender, &v.Language, &v.Premium, &v.Status, &v.SampleURL, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &v, err
}

func (s *AudioStore) UpsertAsset(ctx context.Context, a *models.AudioAsset) error {
	if a.ID == "" {
		a.ID = newID()
	}
	if a.Status == "" {
		a.Status = "ready"
	}
	a.CreatedAt, a.UpdatedAt = now(), now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO audio_assets (id,confession_id,variant_id,voice_id,url,duration_seconds,size_bytes,status,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET url=excluded.url, duration_seconds=excluded.duration_seconds, size_bytes=excluded.size_bytes, status=excluded.status, updated_at=excluded.updated_at`,
		a.ID, a.ConfessionID, nullIfEmpty(a.VariantID), a.VoiceID, a.URL, a.DurationSeconds, a.SizeBytes, a.Status, a.CreatedAt, a.UpdatedAt)
	return err
}

// AssetsFor returns the ready audio assets for a confession, filtered by voice if given.
func (s *AudioStore) AssetsFor(ctx context.Context, confessionID, voiceID string) ([]models.AudioAsset, error) {
	q := `SELECT id,confession_id,COALESCE(variant_id,''),voice_id,url,COALESCE(duration_seconds,0),COALESCE(size_bytes,0),status,created_at,updated_at
	      FROM audio_assets WHERE confession_id = ? AND status = 'ready'`
	args := []any{confessionID}
	if voiceID != "" {
		q += ` AND voice_id = ?`
		args = append(args, voiceID)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.AudioAsset
	for rows.Next() {
		var a models.AudioAsset
		if err := rows.Scan(&a.ID, &a.ConfessionID, &a.VariantID, &a.VoiceID, &a.URL, &a.DurationSeconds, &a.SizeBytes, &a.Status, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ConfessionIDsWithVoice returns confession ids that have a ready asset for the given voice.
func (s *AudioStore) ConfessionIDsWithVoice(ctx context.Context, voiceID string) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT confession_id FROM audio_assets WHERE voice_id = ? AND status = 'ready'`, voiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

'@
$full = Join-Path $RepoRoot 'internal\store\audio.go'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'internal\store\audio.go', $bytes)
$count++

# ---- internal\store\content.go ----
$c = @'
package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/Teamthy/i-confess/internal/models"
)

// ContentStore manages collections, categories, confessions, variants, and scriptures.
type ContentStore struct{ db *sql.DB }

func NewContentStore(db *sql.DB) *ContentStore { return &ContentStore{db: db} }

// ---------- Collections ----------

func (s *ContentStore) CreateCollection(ctx context.Context, c *models.Collection) error {
	if c.ID == "" {
		c.ID = newID()
	}
	c.CreatedAt, c.UpdatedAt = now(), now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO collections (id,name,slug,description,premium,status,sort_order,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		c.ID, c.Name, c.Slug, c.Description, boolInt(c.Premium), c.Status, c.SortOrder, c.CreatedAt, c.UpdatedAt)
	return err
}

func (s *ContentStore) ListCollections(ctx context.Context, includeUnpublished bool) ([]models.Collection, error) {
	q := `SELECT id,name,slug,COALESCE(description,''),premium,status,sort_order,created_at,updated_at FROM collections`
	if !includeUnpublished {
		q += ` WHERE status = 'published'`
	}
	q += ` ORDER BY sort_order, name`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Collection
	for rows.Next() {
		var c models.Collection
		if err := rows.Scan(&c.ID, &c.Name, &c.Slug, &c.Description, &c.Premium, &c.Status, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---------- Categories ----------

func (s *ContentStore) CreateCategory(ctx context.Context, c *models.Category) error {
	if c.ID == "" {
		c.ID = newID()
	}
	c.CreatedAt, c.UpdatedAt = now(), now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO categories (id,name,slug,description,icon,premium,status,sort_order,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?)`,
		c.ID, c.Name, c.Slug, c.Description, c.Icon, boolInt(c.Premium), c.Status, c.SortOrder, c.CreatedAt, c.UpdatedAt)
	return err
}

func (s *ContentStore) ListCategories(ctx context.Context, includeUnpublished bool) ([]models.Category, error) {
	q := `SELECT id,name,slug,COALESCE(description,''),COALESCE(icon,''),premium,status,sort_order,created_at,updated_at FROM categories`
	if !includeUnpublished {
		q += ` WHERE status = 'published'`
	}
	q += ` ORDER BY sort_order, name`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Category
	for rows.Next() {
		var c models.Category
		if err := rows.Scan(&c.ID, &c.Name, &c.Slug, &c.Description, &c.Icon, &c.Premium, &c.Status, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *ContentStore) CategoryByID(ctx context.Context, id string) (*models.Category, error) {
	var c models.Category
	err := s.db.QueryRowContext(ctx,
		`SELECT id,name,slug,COALESCE(description,''),COALESCE(icon,''),premium,status,sort_order,created_at,updated_at FROM categories WHERE id = ?`, id).
		Scan(&c.ID, &c.Name, &c.Slug, &c.Description, &c.Icon, &c.Premium, &c.Status, &c.SortOrder, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &c, err
}

func (s *ContentStore) AddCategoryToCollection(ctx context.Context, collectionID, categoryID string, order int) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO collection_categories (collection_id, category_id, sort_order) VALUES (?,?,?)
		 ON CONFLICT(collection_id, category_id) DO UPDATE SET sort_order = excluded.sort_order`,
		collectionID, categoryID, order)
	return err
}

// ---------- Confessions ----------

func (s *ContentStore) CreateConfession(ctx context.Context, c *models.Confession) error {
	if c.ID == "" {
		c.ID = newID()
	}
	if c.Version == 0 {
		c.Version = 1
	}
	c.CreatedAt, c.UpdatedAt = now(), now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO confessions (id,category_id,title,short_text,medium_text,long_text,description,tags,intensity,language,status,author,version,published_at,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.ID, c.CategoryID, c.Title, c.ShortText, c.MediumText, c.LongText, c.Description,
		strings.Join(c.Tags, ","), c.Intensity, c.Language, c.Status, c.Author, c.Version, nullIfEmpty(c.PublishedAt), c.CreatedAt, c.UpdatedAt)
	if err != nil {
		return err
	}
	for _, v := range c.Variants {
		if err := insertVariant(ctx, tx, c.ID, v); err != nil {
			return err
		}
	}
	for _, sc := range c.Scriptures {
		if err := insertScripture(ctx, tx, c.ID, sc); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func insertVariant(ctx context.Context, tx *sql.Tx, confessionID string, v models.ConfessionVariant) error {
	if v.ID == "" {
		v.ID = newID()
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO confession_variants (id,confession_id,label,duration_seconds,sort_order) VALUES (?,?,?,?,?)`,
		v.ID, confessionID, v.Label, v.DurationSeconds, v.SortOrder)
	return err
}

func insertScripture(ctx context.Context, tx *sql.Tx, confessionID string, s models.ScriptureRef) error {
	if s.ID == "" {
		s.ID = newID()
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO scripture_references (id,confession_id,book,chapter,verse,translation,is_direct_quote,notes,sort_order)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		s.ID, confessionID, s.Book, s.Chapter, s.Verse, s.Translation, boolInt(s.IsDirectQuote), s.Notes, s.SortOrder)
	return err
}

func (s *ContentStore) ConfessionByID(ctx context.Context, id string) (*models.Confession, error) {
	var c models.Confession
	var tags, publishedAt sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id,category_id,title,COALESCE(short_text,''),COALESCE(medium_text,''),COALESCE(long_text,''),COALESCE(description,''),COALESCE(tags,''),intensity,language,status,COALESCE(author,''),version,published_at,created_at,updated_at
		 FROM confessions WHERE id = ?`, id).
		Scan(&c.ID, &c.CategoryID, &c.Title, &c.ShortText, &c.MediumText, &c.LongText, &c.Description, &tags, &c.Intensity, &c.Language, &c.Status, &c.Author, &c.Version, &publishedAt, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if tags.Valid && tags.String != "" {
		c.Tags = strings.Split(tags.String, ",")
	}
	if publishedAt.Valid {
		c.PublishedAt = publishedAt.String
	}
	c.Variants, err = s.Variants(ctx, id)
	if err != nil {
		return nil, err
	}
	c.Scriptures, err = s.Scriptures(ctx, id)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *ContentStore) ConfessionsByCategory(ctx context.Context, categoryID string, publishedOnly bool) ([]models.Confession, error) {
	q := `SELECT id,category_id,title,COALESCE(short_text,''),COALESCE(medium_text,''),COALESCE(long_text,''),COALESCE(description,''),COALESCE(tags,''),intensity,language,status,COALESCE(author,''),version,published_at,created_at,updated_at
	      FROM confessions WHERE category_id = ?`
	if publishedOnly {
		q += ` AND status = 'published'`
	}
	q += ` ORDER BY created_at`
	rows, err := s.db.QueryContext(ctx, q, categoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Confession
	for rows.Next() {
		var c models.Confession
		var tags, publishedAt sql.NullString
		if err := rows.Scan(&c.ID, &c.CategoryID, &c.Title, &c.ShortText, &c.MediumText, &c.LongText, &c.Description, &tags, &c.Intensity, &c.Language, &c.Status, &c.Author, &c.Version, &publishedAt, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		if tags.Valid && tags.String != "" {
			c.Tags = strings.Split(tags.String, ",")
		}
		if publishedAt.Valid {
			c.PublishedAt = publishedAt.String
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *ContentStore) ListConfessions(ctx context.Context, publishedOnly bool) ([]models.Confession, error) {
	q := `SELECT id,category_id,title,COALESCE(short_text,''),COALESCE(medium_text,''),COALESCE(long_text,''),COALESCE(description,''),COALESCE(tags,''),intensity,language,status,COALESCE(author,''),version,published_at,created_at,updated_at
	      FROM confessions`
	if publishedOnly {
		q += ` WHERE status = 'published'`
	}
	q += ` ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Confession
	for rows.Next() {
		var c models.Confession
		var tags, publishedAt sql.NullString
		if err := rows.Scan(&c.ID, &c.CategoryID, &c.Title, &c.ShortText, &c.MediumText, &c.LongText, &c.Description, &tags, &c.Intensity, &c.Language, &c.Status, &c.Author, &c.Version, &publishedAt, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		if tags.Valid && tags.String != "" {
			c.Tags = strings.Split(tags.String, ",")
		}
		if publishedAt.Valid {
			c.PublishedAt = publishedAt.String
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *ContentStore) UpdateConfessionStatus(ctx context.Context, id, status string) error {
	ts := now()
	publishedAt := nullIfEmpty("")
	if status == "published" {
		publishedAt = ts
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE confessions SET status = ?, published_at = ?, updated_at = ? WHERE id = ?`,
		status, publishedAt, ts, id)
	return err
}

// ---------- Variants & Scriptures ----------

func (s *ContentStore) Variants(ctx context.Context, confessionID string) ([]models.ConfessionVariant, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,confession_id,label,duration_seconds,sort_order FROM confession_variants WHERE confession_id = ? ORDER BY sort_order`, confessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ConfessionVariant
	for rows.Next() {
		var v models.ConfessionVariant
		if err := rows.Scan(&v.ID, &v.ConfessionID, &v.Label, &v.DurationSeconds, &v.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *ContentStore) Scriptures(ctx context.Context, confessionID string) ([]models.ScriptureRef, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,confession_id,book,COALESCE(chapter,0),COALESCE(verse,''),translation,is_direct_quote,COALESCE(notes,''),sort_order
		 FROM scripture_references WHERE confession_id = ? ORDER BY sort_order`, confessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.ScriptureRef
	for rows.Next() {
		var sc models.ScriptureRef
		if err := rows.Scan(&sc.ID, &sc.ConfessionID, &sc.Book, &sc.Chapter, &sc.Verse, &sc.Translation, &sc.IsDirectQuote, &sc.Notes, &sc.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

'@
$full = Join-Path $RepoRoot 'internal\store\content.go'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'internal\store\content.go', $bytes)
$count++

# ---- internal\store\engagement.go ----
$c = @'
package store

import (
	"context"
	"database/sql"

	"github.com/Teamthy/i-confess/internal/models"
)

// EngagementStore manages favorites, playback history, and user confessions.
type EngagementStore struct{ db *sql.DB }

func NewEngagementStore(db *sql.DB) *EngagementStore { return &EngagementStore{db: db} }

// ---------- Favorites ----------

func (s *EngagementStore) AddFavorite(ctx context.Context, userID, entityType, entityID string) (*models.Favorite, error) {
	f := &models.Favorite{ID: newID(), UserID: userID, EntityType: entityType, EntityID: entityID, CreatedAt: now()}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO favorites (id,user_id,entity_type,entity_id,created_at) VALUES (?,?,?,?,?)
		 ON CONFLICT(id) DO NOTHING`, f.ID, f.UserID, f.EntityType, f.EntityID, f.CreatedAt)
	return f, err
}

func (s *EngagementStore) RemoveFavorite(ctx context.Context, userID, entityType, entityID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM favorites WHERE user_id=? AND entity_type=? AND entity_id=?`, userID, entityType, entityID)
	return err
}

func (s *EngagementStore) ListFavorites(ctx context.Context, userID, entityType string) ([]models.Favorite, error) {
	q := `SELECT id,user_id,entity_type,entity_id,created_at FROM favorites WHERE user_id=?`
	args := []any{userID}
	if entityType != "" {
		q += ` AND entity_type=?`
		args = append(args, entityType)
	}
	q += ` ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Favorite
	for rows.Next() {
		var f models.Favorite
		if err := rows.Scan(&f.ID, &f.UserID, &f.EntityType, &f.EntityID, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ---------- Playback history ----------

func (s *EngagementStore) RecordPlayback(ctx context.Context, rec *models.PlaybackRecord) error {
	if rec.ID == "" {
		rec.ID = newID()
	}
	if rec.ListenedAt == "" {
		rec.ListenedAt = now()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO playback_history (id,user_id,session_id,confession_id,duration_seconds,completed,skipped,listened_at)
		 VALUES (?,?,?,?,?,?,?,?)`,
		rec.ID, rec.UserID, nullIfEmpty(rec.SessionID), nullIfEmpty(rec.ConfessionID), rec.DurationSeconds, boolInt(rec.Completed), boolInt(rec.Skipped), rec.ListenedAt)
	return err
}

func (s *EngagementStore) History(ctx context.Context, userID string, limit int) ([]models.PlaybackRecord, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,user_id,COALESCE(session_id,''),COALESCE(confession_id,''),COALESCE(duration_seconds,0),completed,skipped,listened_at
		 FROM playback_history WHERE user_id=? ORDER BY listened_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.PlaybackRecord
	for rows.Next() {
		var rec models.PlaybackRecord
		if err := rows.Scan(&rec.ID, &rec.UserID, &rec.SessionID, &rec.ConfessionID, &rec.DurationSeconds, &rec.Completed, &rec.Skipped, &rec.ListenedAt); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// ---------- User confessions ----------

func (s *EngagementStore) CreateUserConfession(ctx context.Context, uc *models.UserConfession) error {
	if uc.ID == "" {
		uc.ID = newID()
	}
	uc.CreatedAt, uc.UpdatedAt = now(), now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO user_confessions (id,user_id,title,text,category_id,is_private,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?)`,
		uc.ID, uc.UserID, uc.Title, uc.Text, nullIfEmpty(uc.CategoryID), boolInt(uc.IsPrivate), uc.CreatedAt, uc.UpdatedAt)
	return err
}

func (s *EngagementStore) ListUserConfessions(ctx context.Context, userID string) ([]models.UserConfession, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,user_id,title,text,COALESCE(category_id,''),is_private,created_at,updated_at
		 FROM user_confessions WHERE user_id=? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.UserConfession
	for rows.Next() {
		var uc models.UserConfession
		if err := rows.Scan(&uc.ID, &uc.UserID, &uc.Title, &uc.Text, &uc.CategoryID, &uc.IsPrivate, &uc.CreatedAt, &uc.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, uc)
	}
	return out, rows.Err()
}

'@
$full = Join-Path $RepoRoot 'internal\store\engagement.go'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'internal\store\engagement.go', $bytes)
$count++

# ---- internal\store\schedules.go ----
$c = @'
package store

import (
	"context"
	"database/sql"
	"strconv"
	"strings"

	"github.com/Teamthy/i-confess/internal/models"
)

// ScheduleStore manages user schedules.
type ScheduleStore struct{ db *sql.DB }

func NewScheduleStore(db *sql.DB) *ScheduleStore { return &ScheduleStore{db: db} }

func (s *ScheduleStore) Create(ctx context.Context, sc *models.Schedule) error {
	if sc.ID == "" {
		sc.ID = newID()
	}
	if sc.Timezone == "" {
		sc.Timezone = "UTC"
	}
	sc.CreatedAt, sc.UpdatedAt = now(), now()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO schedules (id,user_id,label,time,days_of_week,timezone,duration_seconds,voice_id,category_ids,enabled,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		sc.ID, sc.UserID, sc.Label, sc.Time, daysToStr(sc.DaysOfWeek), sc.Timezone, sc.DurationSeconds,
		nullIfEmpty(sc.VoiceID), nullIfEmpty(strings.Join(sc.CategoryIDs, ",")), boolInt(sc.Enabled), sc.CreatedAt, sc.UpdatedAt)
	return err
}

func (s *ScheduleStore) ByID(ctx context.Context, id string) (*models.Schedule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,user_id,label,time,days_of_week,timezone,duration_seconds,COALESCE(voice_id,''),COALESCE(category_ids,''),enabled,created_at,updated_at
		 FROM schedules WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, ErrNotFound
	}
	return scanSchedule(rows)
}

func (s *ScheduleStore) ListByUser(ctx context.Context, userID string) ([]models.Schedule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,user_id,label,time,days_of_week,timezone,duration_seconds,COALESCE(voice_id,''),COALESCE(category_ids,''),enabled,created_at,updated_at
		 FROM schedules WHERE user_id = ? ORDER BY time`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Schedule
	for rows.Next() {
		sc, err := scanSchedule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *sc)
	}
	return out, rows.Err()
}

func (s *ScheduleStore) Update(ctx context.Context, sc *models.Schedule) error {
	sc.UpdatedAt = now()
	_, err := s.db.ExecContext(ctx,
		`UPDATE schedules SET label=?, time=?, days_of_week=?, timezone=?, duration_seconds=?, voice_id=?, category_ids=?, enabled=?, updated_at=?
		 WHERE id=? AND user_id=?`,
		sc.Label, sc.Time, daysToStr(sc.DaysOfWeek), sc.Timezone, sc.DurationSeconds, nullIfEmpty(sc.VoiceID),
		nullIfEmpty(strings.Join(sc.CategoryIDs, ",")), boolInt(sc.Enabled), sc.UpdatedAt, sc.ID, sc.UserID)
	return err
}

func (s *ScheduleStore) Delete(ctx context.Context, id, userID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM schedules WHERE id=? AND user_id=?`, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanSchedule(rows *sql.Rows) (*models.Schedule, error) {
	var sc models.Schedule
	var days, cats sql.NullString
	if err := rows.Scan(&sc.ID, &sc.UserID, &sc.Label, &sc.Time, &days, &sc.Timezone, &sc.DurationSeconds, &sc.VoiceID, &cats, &sc.Enabled, &sc.CreatedAt, &sc.UpdatedAt); err != nil {
		return nil, err
	}
	sc.DaysOfWeek = strToDays(days.String)
	if cats.Valid && cats.String != "" {
		sc.CategoryIDs = strings.Split(cats.String, ",")
	}
	return &sc, nil
}

func daysToStr(days []int) string {
	if len(days) == 0 {
		return "1,2,3,4,5,6,7"
	}
	parts := make([]string, len(days))
	for i, d := range days {
		parts[i] = strconv.Itoa(d)
	}
	return strings.Join(parts, ",")
}

func strToDays(s string) []int {
	if s == "" {
		return []int{1, 2, 3, 4, 5, 6, 7}
	}
	parts := strings.Split(s, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil && n >= 1 && n <= 7 {
			out = append(out, n)
		}
	}
	return out
}

'@
$full = Join-Path $RepoRoot 'internal\store\schedules.go'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'internal\store\schedules.go', $bytes)
$count++

# ---- internal\store\sessions.go ----
$c = @'
package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Teamthy/i-confess/internal/models"
)

// SessionStore manages sessions and session items.
type SessionStore struct{ db *sql.DB }

func NewSessionStore(db *sql.DB) *SessionStore { return &SessionStore{db: db} }

func (s *SessionStore) Create(ctx context.Context, sess *models.Session) error {
	if sess.ID == "" {
		sess.ID = newID()
	}
	if sess.Status == "" {
		sess.Status = "created"
	}
	sess.CreatedAt = now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO sessions (id,user_id,type,duration_seconds,voice_id,status,created_at) VALUES (?,?,?,?,?,?,?)`,
		sess.ID, sess.UserID, sess.Type, sess.DurationSeconds, nullIfEmpty(sess.VoiceID), sess.Status, sess.CreatedAt)
	if err != nil {
		return err
	}
	for i := range sess.Items {
		it := &sess.Items[i]
		if it.ID == "" {
			it.ID = newID()
		}
		it.SessionID = sess.ID
		if it.Status == "" {
			it.Status = "queued"
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO session_items (id,session_id,confession_id,variant_id,voice_id,audio_asset_id,position,duration_seconds,status)
			 VALUES (?,?,?,?,?,?,?,?,?)`,
			it.ID, it.SessionID, it.ConfessionID, nullIfEmpty(it.VariantID), nullIfEmpty(it.VoiceID), nullIfEmpty(it.AudioAssetID), it.Position, it.DurationSeconds, it.Status)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SessionStore) ByID(ctx context.Context, id string) (*models.Session, error) {
	var sess models.Session
	var voiceID, startedAt, completedAt sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id,user_id,type,duration_seconds,voice_id,status,created_at,started_at,completed_at FROM sessions WHERE id = ?`, id).
		Scan(&sess.ID, &sess.UserID, &sess.Type, &sess.DurationSeconds, &voiceID, &sess.Status, &sess.CreatedAt, &startedAt, &completedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if voiceID.Valid {
		sess.VoiceID = voiceID.String
	}
	if startedAt.Valid {
		sess.StartedAt = startedAt.String
	}
	if completedAt.Valid {
		sess.CompletedAt = completedAt.String
	}
	items, err := s.Items(ctx, id)
	if err != nil {
		return nil, err
	}
	sess.Items = items
	return &sess, nil
}

func (s *SessionStore) Items(ctx context.Context, sessionID string) ([]models.SessionItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT si.id, si.confession_id, COALESCE(si.variant_id,''), COALESCE(si.voice_id,''), COALESCE(si.audio_asset_id,''), si.position, si.duration_seconds, si.status,
		        c.title, cat.name, COALESCE(a.url,''), COALESCE(c.medium_text, c.short_text, '')
		 FROM session_items si
		 JOIN confessions c ON c.id = si.confession_id
		 JOIN categories cat ON cat.id = c.category_id
		 LEFT JOIN audio_assets a ON a.id = si.audio_asset_id
		 WHERE si.session_id = ? ORDER BY si.position`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.SessionItem
	for rows.Next() {
		var it models.SessionItem
		if err := rows.Scan(&it.ID, &it.ConfessionID, &it.VariantID, &it.VoiceID, &it.AudioAssetID, &it.Position, &it.DurationSeconds, &it.Status, &it.Title, &it.Category, &it.AudioURL, &it.Text); err != nil {
			return nil, err
		}
		it.SessionID = sessionID
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *SessionStore) UpdateStatus(ctx context.Context, id, status string) error {
	ts := now()
	var err error
	switch status {
	case "playing":
		_, err = s.db.ExecContext(ctx, `UPDATE sessions SET status=?, started_at=? WHERE id=?`, status, ts, id)
	case "completed":
		_, err = s.db.ExecContext(ctx, `UPDATE sessions SET status=?, completed_at=? WHERE id=?`, status, ts, id)
	default:
		_, err = s.db.ExecContext(ctx, `UPDATE sessions SET status=? WHERE id=?`, status, id)
	}
	return err
}

func (s *SessionStore) ListByUser(ctx context.Context, userID string, limit int) ([]models.Session, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,user_id,type,duration_seconds,COALESCE(voice_id,''),status,created_at,COALESCE(started_at,''),COALESCE(completed_at,'')
		 FROM sessions WHERE user_id = ? ORDER BY created_at DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Session
	for rows.Next() {
		var sess models.Session
		if err := rows.Scan(&sess.ID, &sess.UserID, &sess.Type, &sess.DurationSeconds, &sess.VoiceID, &sess.Status, &sess.CreatedAt, &sess.StartedAt, &sess.CompletedAt); err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

'@
$full = Join-Path $RepoRoot 'internal\store\sessions.go'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'internal\store\sessions.go', $bytes)
$count++

# ---- internal\store\users.go ----
$c = @'
package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Teamthy/i-confess/internal/models"
	"github.com/google/uuid"
)

var ErrNotFound = errors.New("not found")

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func newID() string { return uuid.NewString() }

// UserStore manages users, subscriptions, and admin roles.
type UserStore struct{ db *sql.DB }

func NewUserStore(db *sql.DB) *UserStore { return &UserStore{db: db} }

func (s *UserStore) Create(ctx context.Context, email, hash, name, tz string) (*models.User, error) {
	u := &models.User{ID: newID(), Email: email, DisplayName: name, Timezone: tz, Status: "active", CreatedAt: now()}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash, display_name, timezone, status, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?)`,
		u.ID, u.Email, hash, u.DisplayName, u.Timezone, u.Status, u.CreatedAt, u.CreatedAt)
	if err != nil {
		return nil, err
	}
	// Every user gets a free subscription and default session preferences.
	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO subscriptions (id, user_id, plan, status, created_at) VALUES (?,?,?,?,?)`,
		newID(), u.ID, "free", "active", now())
	_, _ = s.db.ExecContext(ctx,
		`INSERT INTO session_preferences (user_id, default_duration_seconds, updated_at) VALUES (?,1800,?)`,
		u.ID, now())
	return u, nil
}

func (s *UserStore) ByEmail(ctx context.Context, email string) (*models.User, string, error) {
	var u models.User
	var hash string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, COALESCE(display_name,''), timezone, status, created_at
		 FROM users WHERE email = ?`, email).
		Scan(&u.ID, &u.Email, &hash, &u.DisplayName, &u.Timezone, &u.Status, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	if err != nil {
		return nil, "", err
	}
	return &u, hash, nil
}

func (s *UserStore) ByID(ctx context.Context, id string) (*models.User, error) {
	var u models.User
	err := s.db.QueryRowContext(ctx,
		`SELECT id, email, COALESCE(display_name,''), timezone, status, created_at FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Email, &u.DisplayName, &u.Timezone, &u.Status, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *UserStore) AdminRole(ctx context.Context, userID string) (string, error) {
	var role string
	err := s.db.QueryRowContext(ctx,
		`SELECT role FROM admin_users WHERE user_id = ?`, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return role, err
}

func (s *UserStore) SetAdminRole(ctx context.Context, userID, role string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO admin_users (id, user_id, role, created_at) VALUES (?,?,?,?)
		 ON CONFLICT(user_id) DO UPDATE SET role = excluded.role`,
		newID(), userID, role, now())
	return err
}

func (s *UserStore) Subscription(ctx context.Context, userID string) (string, error) {
	var plan string
	err := s.db.QueryRowContext(ctx,
		`SELECT plan FROM subscriptions WHERE user_id = ? AND status = 'active' ORDER BY created_at DESC LIMIT 1`,
		userID).Scan(&plan)
	if errors.Is(err, sql.ErrNoRows) {
		return "free", nil
	}
	return plan, err
}

func (s *UserStore) SetSubscription(ctx context.Context, userID, plan, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE subscriptions SET plan = ?, status = ? WHERE user_id = ?`, plan, status, userID)
	return err
}

'@
$full = Join-Path $RepoRoot 'internal\store\users.go'
$dir = Split-Path $full -Parent
if ($dir -and -not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
[System.IO.File]::WriteAllText($full, $c)
$bytes = [System.Text.Encoding]::UTF8.GetByteCount($c)
Write-Host ("    wrote {0} ({1} bytes)" -f 'internal\store\users.go', $bytes)
$count++

Write-Host ("==> Wrote {0} files" -f $count)

# ---- git state ----
if (Test-Path (Join-Path $RepoRoot '.git')) {
  git -C $RepoRoot branch --show-current
  git -C $RepoRoot status --short
} else {
  Write-Host "NOTE: no .git folder found. Run: git clone https://github.com/Teamthy/i-confess.git"
}

Write-Host ""
Write-Host "SUCCESS. NEXT:"
Write-Host "  git -C $RepoRoot remote add origin https://github.com/Teamthy/i-confess.git   # only if remote missing"
Write-Host '  git add -A'
Write-Host '  git commit -m "feat: i-confess MVP backend (auth, content, sessions, scheduling, admin)"'
Write-Host '  git push -u origin main'
Write-Host ""
Write-Host 'Then verify locally:  go mod tidy ; go build ./... ; go test ./...'
