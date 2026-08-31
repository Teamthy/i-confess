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

Implemented now (backend): auth, users, collections, categories, confessions,
variants, scripture references, voices, audio assets, sessions + session engine,
schedules, favorites, playback history, user confessions, subscriptions
(free/premium flags), admin RBAC + audit-ready endpoints, content lifecycle status.

Not yet built (next layers): mobile app, admin console UI, async audio
job/queue/worker (PRD §77), Redis caching, observability (§79), offline download
orchestration, premium entitlement gating in the player.

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
