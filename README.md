# I CONFESS

> **Daily Confession Ritual. Scripture. Audio. Habit.**

I CONFESS is a Christian daily confession, Scripture meditation and audio-session platform. The central loop is **Discover → Choose → Create/Schedule Session → Listen → Confess → Complete → Return**.

The Go API and workers are in `server/`, the Next.js site and admin console in `web/`, the Flutter app in `apps/mobile/`, and the speech worker in `voice-engine/`.

## Release readiness

Phase completion reports are historical, not deployment approval. Start with the [open-work ledger](docs/OPEN-WORK-RECONCILIATION.md) for outstanding engineering work and owner decisions. In particular, minister-voice release still requires licensed checkpoints, voice rights and real-speech evaluation.

For backend releases, follow the [database cutover and staging rehearsal](docs/MIGRATION-ROLE-SEPARATION.md) and [deployment guide](docs/DEPLOYMENT.md). The Kubernetes release path requires separate database roles for migrations and API replicas. No production rollout should be approved on a unit-test result alone.

## Development

Run `make help` for build and test targets. Backend tests require a **disposable** PostgreSQL instance with `TEST_DATABASE_URL` set; use `REDIS_ADDR` for the Redis integration tests. See the [web guide](web/README.md) and [mobile guide](apps/mobile/README.md) for the clients.
