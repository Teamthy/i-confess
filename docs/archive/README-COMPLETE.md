# i-confess — Global Christian Confession & Audio Platform

**A premium audio platform for daily Christian confessions, Scripture-based declarations, personalized spiritual routines, and authorized minister voices.**

Build target: **Spotify-grade reliability and user experience** for Christian affirmations and confessions.

---

## Table of Contents

- [Quick Start](#quick-start)
- [Architecture](#architecture)
- [Deployment](#deployment)
- [API](#api)
- [Development](#development)
- [Contributing](#contributing)

---

## Quick Start

### Prerequisites

- **Go 1.25+** ([download](https://go.dev/dl/))
- **Docker & Docker Compose** (optional, for local services)
- **PostgreSQL 16+** (production) or **SQLite** (development)

### Development Setup

1. **Clone the repository**
   ```bash
   git clone https://github.com/Teamthy/i-confess.git
   cd i-confess
   ```

2. **Set environment** (copy defaults)
   ```bash
   cp .env.example .env
   ```

3. **Start local services** (PostgreSQL + Redis optional)
   ```bash
   docker-compose up -d
   ```

4. **Build and run**
   ```bash
   go run ./cmd/server
   ```

   Server starts at `http://localhost:8080`

5. **Test the API**
   ```bash
   # Health check
   curl localhost:8080/healthz

   # Login (demo user)
   TOKEN=$(curl -s -X POST localhost:8080/auth/login \
     -H 'Content-Type: application/json' \
     -d '{"email":"demo@iconfess.dev","password":"password123"}' | jq -r .token)

   # List categories
   curl -s localhost:8080/categories | jq

   # Create a session
   curl -s -X POST localhost:8080/sessions \
     -H "Authorization: Bearer $TOKEN" \
     -H 'Content-Type: application/json' \
     -d '{
       "category_ids": ["healing", "faith"],
       "duration_seconds": 600,
       "voice_id": "grace"
     }' | jq
   ```

6. **Admin console**
   Open `http://localhost:8080/admin/` → Sign in with `admin@iconfess.dev` / `admin12345`

---

## Architecture

### System Design

```
Mobile (Flutter)  │  Web (Next.js)  │  Admin (Next.js)
     ↓                     ↓                   ↓
        Go Backend API (Modular Monolith)
        ├─ Session Engine (confession composition)
        ├─ Async Job System (audio, analytics, notifications)
        ├─ Entitlements (free vs premium)
        ├─ Voice Rights (authorization)
        ├─ Search (full-text)
        └─ Admin RBAC
            ↓
      PostgreSQL  │  Redis
            ↓            ↓
      Audio CDN ← Signed URLs

```

### Core Modules

| Module | Purpose |
|--------|---------|
| `auth/` | JWT authentication, bcrypt password hashing |
| `config/` | Environment-based configuration (12-factor app) |
| `db/` | Database migration and schema |
| `engine/` | Session composition engine (deterministic) |
| `api/` | HTTP handlers and router |
| `store/` | Data access layer (repositories) |
| `jobs/` | Async job queue with workers |
| `entitlements/` | Plan-based access control |
| `search/` | Full-text content discovery |
| `health/` | Health checks (liveness/readiness) |
| `log/` | Structured logging (12-factor) |
| `seed/` | Development data seeding |

See [ARCHITECTURE.md](ARCHITECTURE.md) for complete system design, scalability, and deployment strategy.

---

## Deployment

### Development (Local)

```bash
go run ./cmd/server
# Uses SQLite, local media, in-memory job queue
```

### Staging (Docker)

```bash
docker-compose up --build
# Uses PostgreSQL, Redis, real job persistence
```

### Production (Kubernetes)

```bash
# 1. Build and push image
docker build -t iconfess:latest .
docker push your-registry.com/iconfess:latest

# 2. Deploy to Kubernetes
kubectl apply -f k8s/deployment-prod.yaml

# 3. Verify rollout
kubectl rollout status deployment/iconfess-backend -n iconfess-prod
```

See [k8s/deployment-prod.yaml](k8s/deployment-prod.yaml) for production Kubernetes manifest with:
- ✅ Resource requests/limits
- ✅ Horizontal Pod Autoscaling (3–20 replicas)
- ✅ Health checks (liveness + readiness probes)
- ✅ Security policies (NetworkPolicy, securityContext)
- ✅ Pod disruption budgets (zero-downtime rolling updates)

### Environment Variables

See [.env.example](.env.example) for complete list. Key variables:

| Variable | Required | Production Value |
|----------|----------|-------------------|
| `PORT` | ✗ | 8080 |
| `ENV` | ✗ | production |
| `POSTGRES_DSN` | ✓ | postgres://... (with SSL) |
| `JWT_SECRET` | ✓ | **Use AWS Secrets Manager** |
| `REDIS_ADDR` | ✓ | redis-cluster.prod:6379 |
| `QUEUE_WORKERS` | ✗ | 16 (tunable) |
| `LOG_LEVEL` | ✗ | warn (prod) |

**Secrets**: Never commit `.env` files or hardcode secrets. Use:
- AWS Secrets Manager
- HashiCorp Vault
- Google Secret Manager
- Azure Key Vault

---

## API

### Core Endpoints

#### Public (No Auth)

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/healthz` | Health check |
| POST | `/auth/register` | Sign up |
| POST | `/auth/login` | Sign in |
| GET | `/collections` | List collections |
| GET | `/categories` | List categories |
| GET | `/categories/{id}/confessions` | Confessions by category |
| GET | `/confessions/{id}` | Confession detail |
| GET | `/voices` | List voices |

#### Authenticated (JWT Required)

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/me` | User profile + plan |
| POST | `/sessions` | Create session (session engine) |
| GET | `/sessions/{id}` | Fetch session with signed audio URLs |
| PATCH | `/sessions/{id}` | Update session status |
| GET | `/schedules` | List recurring playback schedules |
| POST | `/schedules` | Create schedule (alarms) |
| POST | `/me/favorites` | Add favorite |
| GET | `/me/history` | Listening history |
| POST | `/me/confessions` | Create personal confession |

#### Admin (Admin Role Required)

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/admin/stats` | Dashboard stats |
| POST | `/admin/categories` | Create category |
| POST | `/admin/confessions` | Create confession |
| POST | `/admin/voices` | Create voice |
| POST | `/admin/users/role` | Set admin role |
| POST | `/admin/users/subscription` | Update subscription |

### Example: Create and Play a Session

```bash
# 1. Authenticate
TOKEN=$(curl -s -X POST localhost:8080/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"user@example.com","password":"password123"}' | jq -r .token)

# 2. Get categories
curl -s localhost:8080/categories | jq '.[] | {id, name}'

# 3. Create session (30 minutes, Healing + Faith categories)
SESSION=$(curl -s -X POST localhost:8080/sessions \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "category_ids": ["healing-id", "faith-id"],
    "duration_seconds": 1800,
    "voice_id": "grace"
  }')

echo $SESSION | jq '.items[] | {confession_id, audio_url, duration_seconds}'

# 4. Stream audio from CDN (signed URL, valid 1 hour)
curl -I -H "Range: bytes=0-999" "$(echo $SESSION | jq -r '.items[0].audio_url')"

# 5. Mark session complete
curl -s -X PATCH "localhost:8080/sessions/$(echo $SESSION | jq -r .id)" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"status":"completed"}'
```

See [docs/API.md](docs/API.md) for complete endpoint reference.

---

## Development

### Project Structure

```
i-confess/
├── cmd/server/                 # Entrypoint
├── internal/
│   ├── api/                    # HTTP handlers + router
│   ├── auth/                   # JWT + bcrypt
│   ├── config/                 # Environment config
│   ├── db/                     # Database migration
│   ├── engine/                 # Session composition
│   ├── entitlements/           # Feature gating
│   ├── health/                 # Liveness/readiness checks
│   ├── jobs/                   # Async job queue
│   ├── log/                    # Structured logging
│   ├── media/                  # Placeholder audio
│   ├── models/                 # Domain models
│   ├── search/                 # Full-text search
│   ├── seed/                   # Dev data
│   ├── store/                  # Data access layer
│   └── adminui/                # Admin console SPA
├── migrations/postgres/        # PostgreSQL schema (production)
├── docs/                       # API documentation
├── k8s/                        # Kubernetes manifests
├── Dockerfile                  # Container image
├── docker-compose.yml          # Local development
├── ARCHITECTURE.md             # System design
└── go.mod                      # Dependencies
```

### Build

```bash
# Development build
go build -o bin/iconfess-server ./cmd/server

# Production build (stripped binary)
go build -ldflags="-s -w" -o bin/iconfess-server ./cmd/server

# Docker image
docker build -t iconfess:latest .
```

### Testing

```bash
# Run all tests
go test ./...

# Run specific package
go test ./internal/config ./internal/jobs

# Run with coverage
go test -cover ./...

# Run with race detector
go test -race ./...
```

### Code Quality

```bash
# Format code
go fmt ./...

# Vet (static analysis)
go vet ./...

# Lint (install golangci-lint first)
golangci-lint run ./...
```

### Database

**Development**: SQLite (zero external services)
```bash
# Schema auto-migrates on startup
go run ./cmd/server
# Database created at: data/iconfess.db
```

**Production**: PostgreSQL
```bash
# Create database
psql -U postgres -c "CREATE DATABASE iconfess;"

# Run migrations (manual or via init script)
psql -U postgres -d iconfess < migrations/postgres/0001_schema.sql

# Connect in app
export POSTGRES_DSN="postgres://user:pass@localhost:5432/iconfess?sslmode=require"
go run ./cmd/server
```

---

## Async Job System

Background jobs process without blocking requests:

```
Request                                    Response (immediate)
  │                                              │
  ├─ Create Job (status: queued)
  │   ├─ audio_generate (TTS, transcoding)
  │   ├─ analytics_event (user interaction)
  │   ├─ notification_send (email, push, SMS)
  │   └─ recommendation_compute (personalization)
  │
  └─→ Return to client                         │
                                               ↓
                                   Worker processes job
                                   ├─ Retry with backoff
                                   ├─ Idempotent execution
                                   ├─ Dead-letter on failure
                                   └─ Persist result to DB
```

Workers auto-start on server boot. Tunable via `QUEUE_WORKERS` env var.

---

## Entitlements & Voice Rights

### Free Plan
- Account creation
- 10 personal confessions
- Basic voices
- Background playback
- One schedule

### Premium Plan
- Unlimited personal confessions
- Download up to 500 sessions
- All voices (professional + minister)
- Audio generation (TTS)
- Playback speed control
- Advanced analytics

### Voice Rights
Every voice tracks:
- ✅ Owner name
- ✅ License status (active/pending/expired/revoked)
- ✅ Commercial use permission
- ✅ AI generation permission (critical: no voice cloning without rights)
- ✅ Territories (ISO country codes)
- ✅ Languages
- ✅ Marketing use
- ✅ Expiration date

**No voice is used for audio generation unless all rights are confirmed.**

---

## Security

### Authentication
- JWT (HS256) with rotating secrets
- Bcrypt password hashing (12 rounds)
- Refresh tokens (4h production, 24h staging)

### Authorization
- Admin RBAC (6+ roles)
- Plan-based entitlements
- Voice rights enforcement
- Content status lifecycle (draft → published → archived)

### Data Protection
- TLS 1.3+ for all communication
- PII at-rest encryption (production)
- Audit logging for sensitive ops
- GDPR/CCPA compliance (right to delete, data export)

### Secrets Management
- 12-factor app (environment variables only)
- No hardcoded credentials
- Rotate secrets every 30 days
- Use AWS Secrets Manager / Vault in production

---

## Observability

### Logging
- Structured JSON logs (context, user_id, request_id)
- Log levels: debug (dev), info (staging), warn (production)
- Centralized to ELK, Datadog, or CloudWatch

### Metrics
- Request latency (p50, p95, p99)
- Job queue depth
- Database connection pool
- Cache hit rates
- Error rates by endpoint

### Health Checks
```bash
# Liveness probe (is the app running?)
GET /healthz

# Used by Kubernetes to restart unhealthy pods
# Returns 200 if healthy, 503 if degraded/unhealthy
```

### Tracing
- Distributed tracing with OpenTelemetry
- Trace every request + async job
- Database query performance
- Integration with Datadog / Jaeger

---

## Contributing

### Code Style
- Follow Go conventions (gofmt, go vet)
- Write clear, idiomatic code
- Avoid premature optimization
- Document public types/functions

### Testing
- Write tests for new features
- Aim for >80% coverage
- Use table-driven tests
- Mock external services

### Commits
```
feat: add category search
fix: resolve race condition in job queue
docs: update API.md
chore: upgrade dependencies
```

### Pull Requests
1. Fork the repository
2. Create a feature branch
3. Write tests
4. Ensure all tests pass
5. Submit PR with description

---

## Roadmap

### MVP ✅
- Auth + users + profiles
- Content (categories, confessions, voices)
- Session engine
- Async job system
- Free + Premium plans
- Admin console
- SQLite + PostgreSQL support

### V1 (Q2 2026)
- Mobile app (Flutter iOS/Android)
- Web app (Next.js)
- Audio TTS pipeline
- Recommendations engine
- Analytics warehouse sync
- Payment integration (Stripe)

### V2 (Q3–Q4 2026)
- Multi-language support
- Advanced search (semantic + intent)
- Offline downloads
- Streaming analytics
- Real-time notifications
- Multi-region deployment

### V3 (2027+)
- AI-powered personalization
- Live sessions (social playback)
- Minister community features
- Mobile app for church leaders
- Analytics dashboard for publishers

---

## License

Proprietary. All rights reserved.

---

## Contact

- Email: team@iconfess.dev
- Website: https://iconfess.dev
- Issues: https://github.com/Teamthy/i-confess/issues

---

**Built with engineering rigor comparable to serious consumer audio products (Spotify, Apple Music) while maintaining an original Christian confession identity.**
