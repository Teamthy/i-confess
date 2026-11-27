<p align="center">
  <img src="web/public/brand/icon-192.png" width="88" height="88" alt="I CONFESS">
</p>

<h1 align="center">I CONFESS</h1>

<p align="center"><b>Speak it. Believe it. Live it.</b><br>
A daily practice of spoken confession: reviewed, Scripture-rooted words, licensed narration voices and sessions sized to the day you actually have.</p>

<p align="center">
  <a href="https://github.com/Teamthy/i-confess/actions/workflows/ci.yml"><img src="https://github.com/Teamthy/i-confess/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <img src="https://img.shields.io/badge/Go-1.26-00ADD8" alt="Go 1.26">
  <img src="https://img.shields.io/badge/Next.js-15-000000" alt="Next.js 15">
  <img src="https://img.shields.io/badge/Flutter-stable-02569B" alt="Flutter">
  <img src="https://img.shields.io/badge/license-MIT-blue" alt="MIT">
</p>

---

## What it is

The product loop is **discover → choose → build or schedule a session → listen → confess → return**.
Every confession is human-reviewed against the Scripture it stands on before it is published, voices
appear only after their rights are cleared, and nothing autoplays or nags.

## Repository layout

| Path | What lives there |
|---|---|
| [`server/`](server) | Go API, background workers and PostgreSQL migrations — the single source of truth for content, sessions, entitlements and the Bible |
| [`web/`](web) | Next.js app: public site, signed-in web app and admin console, talking to the API through a same-origin `/api` proxy |
| [`apps/mobile/`](apps/mobile) | Flutter app for iOS and Android |
| [`clients/dart/`](clients/dart) | Typed Dart client for the API, used by the Flutter app |
| [`voice-engine/`](voice-engine) | Python speech-synthesis worker (licence-gated, GPU in production) |
| [`design/`](design) | Design tokens, information architecture and brand assets; generates code for web and mobile |
| [`contracts/`](contracts) | OpenAPI contract, regenerated from the live route table and checked in CI |
| [`infrastructure/`](infrastructure) | CloudFormation for the media edge |
| [`scripts/`](scripts) | Release, backup and CI helper scripts |
| [`docs/`](docs) | Specifications, runbooks, audits and session handoffs — start at [`docs/README.md`](docs/README.md) |

## Quick start

Prerequisites: Go 1.26, Node 22, Docker (or a local PostgreSQL 16+ and Redis 7), and Flutter stable for the mobile app.

```bash
# 1. Services
docker compose -f server/docker-compose.yml up -d postgres redis

# 2. API on :8080 — development mode migrates and seeds the catalogue
cd server
ENV=development \
DATABASE_URL='host=127.0.0.1 port=5432 user=iconfess password=iconfess-dev-password dbname=iconfess sslmode=disable' \
go run ./cmd/server

# 3. Web on :3000 (in another terminal)
cd web && npm ci && npm run dev
```

The full list of settings, with production requirements, is in [`server/.env.example`](server/.env.example).

**Bible text.** The repository ships the registry of public-domain translations (which edition, its
licence, its checksum), never the text itself, so the Bible reader is empty until a translation is
loaded into the target database once:

```bash
make bible-load BIBLE_DSN='host=127.0.0.1 port=5432 user=iconfess password=iconfess-dev-password dbname=iconfess sslmode=disable'
```

In staging and production, run the **Deploy** workflow with operation `load-bible` ([how](docs/DEPLOYMENT.md#loading-the-bible-text)).

## Quality gates

`make verify` runs the Go, design and mobile checks CI runs; `make help` lists every target.

| Area | Checks |
|---|---|
| Go | build, vet, gofmt, golangci-lint, the race-enabled suite against real PostgreSQL and Redis, and a blocking `govulncheck` |
| Web | typecheck, unit tests, production build |
| Mobile | `flutter analyze`, `flutter test`, plus an SDK-free Dart symbol check |
| Voice engine | pytest, `pip-audit`, model-licence register |
| Design | generated tokens and brand assets are current; IA and route coverage; WCAG contrast |
| Deployment | non-root image, Kubernetes schema validation, CloudFormation lint, Trivy |

## Documentation

- [Product source of truth](docs/00-PRODUCT-SOURCE-OF-TRUTH.md) and [system architecture](docs/02-SYSTEM-ARCHITECTURE.md)
- [Open work](docs/OPEN-WORK-RECONCILIATION.md) — what is outstanding and what needs an owner decision
- [Deployment](docs/DEPLOYMENT.md) and [database role separation](docs/MIGRATION-ROLE-SEPARATION.md)
- [Latest session handoff](docs/handoffs) — where the last piece of work stopped

Phase reports in [`docs/archive`](docs/archive) are a historical record, not release approval.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Work lands through pull requests on a feature branch; `main` is always deployable.

## License

[MIT](LICENSE). The product's [privacy policy](docs/legal/PRIVACY.md) and [terms](docs/legal/TERMS.md) live in `docs/legal`.
