# i-confess Backend Architecture

## System Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Client Applications                          │
├────────────────────────────────────────────────────────────────────┤
│  Mobile (Flutter)  │  Web (Next.js)  │  Admin (Next.js)  │ Analytics│
└──────────┬──────────────────────┬────────────────────────┬──────────┘
           │                      │                        │
           │                  HTTP/2 API                   │
           │                  TLS 1.3+                     │
           ↓                      ↓                        ↓
     ┌─────────────────────────────────────────────────────────┐
     │              Go Backend (Modular Monolith)               │
     │                   Port: 8080                             │
     ├─────────────────────────────────────────────────────────┤
     │ API Layer                                                 │
     │  ├─ auth/           (JWT, bcrypt)                        │
     │  ├─ handlers/       (HTTP endpoint handlers)             │
     │  ├─ middleware/     (CORS, logging, auth checks)         │
     │  └─ router/         (HTTP route configuration)           │
     ├─────────────────────────────────────────────────────────┤
     │ Domain Services                                           │
     │  ├─ sessions/       (Session engine, playback state)     │
     │  ├─ schedules/      (Scheduled playback)                 │
     │  ├─ favorites/      (User bookmarks)                     │
     │  ├─ history/        (Listening history, analytics)       │
     │  ├─ recommendations (Personalization)                    │
     │  ├─ search/         (Content discovery)                  │
     │  ├─ entitlements/   (Plan-based access control)          │
     │  └─ notifications/  (Email, push, SMS)                   │
     ├─────────────────────────────────────────────────────────┤
     │ Data Access Layer (Store)                                 │
     │  ├─ users/          (Users, profiles, subscriptions)     │
     │  ├─ content/        (Categories, confessions, voices)    │
     │  ├─ audio/          (Audio assets, metadata)             │
     │  ├─ sessions/       (Session state)                      │
     │  ├─ schedules/      (Recurring playback)                 │
     │  ├─ engagement/     (Favorites, history)                 │
     │  ├─ voice_rights/   (Voice authorization)               │
     │  └─ jobs/           (Async job state)                    │
     ├─────────────────────────────────────────────────────────┤
     │ Async Job System                                          │
     │  ├─ queue/          (In-memory job queue)                │
     │  ├─ workers/        (Goroutine-based worker pool)        │
     │  ├─ handlers/       (Job type handlers)                  │
     │  └─ persistence/    (Database job store)                 │
     │                                                            │
     │  Handlers:                                                │
     │  ├─ audio_generate  (TTS, transcoding, CDN upload)       │
     │  ├─ analytics_event (Analytics warehouse sync)           │
     │  ├─ notification_send (Email/push/SMS dispatch)          │
     │  └─ recommendation_compute (ML-based personalization)    │
     ├─────────────────────────────────────────────────────────┤
     │ Infrastructure                                            │
     │  ├─ config/         (Environment & deployment config)    │
     │  ├─ db/             (Database migration, schema)         │
     │  ├─ seed/           (Development data seeding)           │
     │  ├─ models/         (Domain models & types)              │
     │  ├─ httpx/          (JSON encoding/error helpers)        │
     │  ├─ auth/           (JWT, password hashing)              │
     │  ├─ engine/         (Session composition engine)         │
     │  └─ media/          (Placeholder audio generation)       │
     └─────────────────────────────────────────────────────────┘
             ↓                  ↓                  ↓
    ┌─────────────────────────────────────────────────────────┐
    │                  Data Persistence Layer                  │
    ├─────────────────────────────────────────────────────────┤
    │  PostgreSQL                │  Redis                      │
    │  (Primary store)           │  (Cache/locks/temp state)  │
    │  - Users                   │  - Hot content cache       │
    │  - Content (28K+ items)    │  - Session cache           │
    │  - Audio assets            │  - Rate limiting           │
    │  - Job queue               │  - Distributed locks       │
    │  - Subscriptions           │  - Idempotency keys        │
    │  - Analytics               │  - Job queue               │
    └─────────────────────────────────────────────────────────┘
             ↓                            ↓
    ┌────────────────────────────────────────────────────────┐
    │                    External Services                    │
    ├────────────────────────────────────────────────────────┤
    │ TTS Providers   │ Storage         │  Analytics          │
    │ - Google Cloud  │ - S3/GCS        │  - Segment          │
    │ - AWS Polly     │ - Backblaze     │  - Mixpanel         │
    │ - Azure TTS     │ - Cloudinary    │  - Amplitude        │
    │                 │                 │  - BigQuery         │
    │ Payments        │ Notifications   │  - Datadog          │
    │ - Stripe        │ - Twilio        │  - New Relic        │
    │ - PayPal        │ - SendGrid      │  - Sentry           │
    │ - Apple IAP     │ - Firebase FCM  │                     │
    │ - Google Play   │ - AWS SNS       │                     │
    └────────────────────────────────────────────────────────┘
                      ↓
            ┌──────────────────────────┐
            │  CDN for Audio Delivery  │
            │  - Cloudflare            │
            │  - Fastly                │
            │  - AWS CloudFront        │
            └──────────────────────────┘
```

## Deployment Architecture

### Development
- Single machine with SQLite + memory job queue
- Local media directory for placeholder audio
- No external services required
- Command: `go run ./cmd/server`

### Staging
- PostgreSQL (managed)
- Redis (managed)
- Kubernetes or Docker Swarm
- All external services (TTS, payments) in sandbox/test mode
- Monitoring with Datadog/New Relic

### Production
- PostgreSQL replicated cluster (HA)
- Redis cluster with sentinel
- Kubernetes (EKS/GKE/AKS) with auto-scaling
- Load balancer (AWS ALB, Google Cloud LB, Azure LB)
- CDN for all audio delivery
- All external services in production mode
- Multi-region deployment for 5-nines reliability
- Real-time monitoring, alerting, on-call rotation

## Request Flow: Create and Play a Session

```
1. User selects categories, duration, voice
   POST /sessions
   ├─ Auth check (JWT verification)
   ├─ Entitlements check (free vs premium features)
   ├─ Category validation
   ├─ Voice validation + rights check
   ├─ Create Session record (status: queued)
   ├─ Call Session Engine
   │  └─ Fetch eligible confessions (published + audio ready for voice)
   │  └─ Compose ordered items (round-robin by category)
   │  └─ Return SessionItems array with durations
   └─ Return Session with signed CDN URLs (read-only for 1 hour)

2. Backend enqueues async jobs (non-blocking)
   ├─ audio_generate (if premium user wants new audio)
   ├─ analytics_event (session_created)
   └─ notification_send (optional: schedule reminder)

3. Mobile app streams audio via CDN
   GET /media/{audio_asset_id}
   ├─ CDN serves cached audio (geo-distributed)
   ├─ Fallback to origin if not cached
   └─ Audio range requests supported for seeking

4. User completes session
   PATCH /sessions/{id}
   ├─ Update session status (completed/abandoned)
   ├─ Enqueue analytics_event (session_completed, duration_listened)
   ├─ Update user listening history
   ├─ Trigger recommendation_compute (if premium)
   └─ Return updated session

5. Background workers process async jobs
   ├─ Pull from job queue (status: queued)
   ├─ Execute handler with exponential backoff
   ├─ On success: update status to completed, emit event
   ├─ On failure: retry (up to max_attempts), then dead_letter
   └─ Persist job state to database
```

## Data Model: Key Relationships

```
User
├─ Subscription (free|premium)
├─ Sessions (many)
│  └─ SessionItems (many, ordered)
│     ├─ Confession (reference)
│     └─ AudioAsset (reference + signed URL)
├─ Schedules (many, recurring playback)
│  ├─ Categories (many)
│  ├─ Voice (reference)
│  └─ Triggers local platform notifications
├─ Favorites (many)
│  └─ Entity (confession | category | voice | collection)
└─ History (listening events, analytics)

Confession
├─ Category (required)
├─ Voice Rights (per voice)
├─ Variants (30s, 1m, 3m, 5m durations)
├─ AudioAssets (one per variant+voice combo)
├─ Scriptures (many, with quote/paraphrase flag)
└─ Status (draft → review → published → archived)

Voice
├─ VoiceRights (owner, territories, languages, AI rights, expiration)
├─ Provider (google_cloud_tts | aws_polly | minister_recording)
├─ Premium flag (free vs paid voices)
└─ AudioAssets (all recordings using this voice)

Category
├─ Collections (many)
├─ Confessions (many)
├─ Premium flag
└─ Ordering (sort_order column)

AudioAsset
├─ Confession (parent)
├─ Variant (duration variant)
├─ Voice (generated for which voice)
├─ URL (signed CDN URL, short-lived)
└─ Status (queued|processing|ready|failed|archived)
```

## Scalability Strategy

### MVP → V1 (5K–10K users)
- Single PostgreSQL instance (16GB RAM, 100GB storage)
- Redis single instance
- 2–4 backend instances behind load balancer
- CDN for audio delivery (standard tier)
- Job queue: in-process + database persistence

### V1 → V2 (10K–100K users)
- PostgreSQL replicas (read-only followers)
- Redis cluster (3+ nodes)
- 10–20 backend instances
- CDN: premium tier with edge compute
- Job queue: Redis-backed with async workers
- Recommendation engine: separate microservice
- Analytics warehouse: real-time sync

### V2 → V3 (100K–1M+ users)
- PostgreSQL multi-region (geo-sharding)
- Dedicated microservices:
  - Auth & API Gateway
  - Content Service (categories, confessions)
  - Session Engine
  - Recommendation Engine (ML)
  - Audio Pipeline (TTS, transcoding)
  - Analytics Collector
  - Notification Service
- Message queue (Kafka or AWS SQS)
- CDN: multi-region with regional origin caches
- Kubernetes with autoscaling

## Security

### Authentication & Authorization
- JWT (HS256) for stateless auth
- Refresh tokens (short-lived: 4h prod, 24h staging)
- Admin RBAC: super_admin, content_admin, audio_producer, etc.
- Audit logging for sensitive operations

### Secrets Management
- Environment variables (12-factor app)
- AWS Secrets Manager or HashiCorp Vault in production
- Rotating credentials (30-day cycle)
- No secrets in code, logs, or version control

### Data Protection
- TLS 1.3+ for all communication
- Password hashing: bcrypt with 12 rounds
- No plaintext passwords in logs
- PII data: at-rest encryption in production
- GDPR/CCPA compliance: right to deletion, data export

### Voice Rights
- Strict authorization checks before voice generation
- Territory/language restrictions enforced
- Expiration date validation
- Commercial use rights tracked
- AI generation rights separate from recording rights

## Observability

### Logging
- Structured JSON logs (context, user_id, request_id)
- Log levels: debug (dev), info (staging), warn (prod)
- Centralized logging (ELK, Datadog, CloudWatch)
- Correlation IDs for request tracing

### Metrics
- Request latency (p50, p95, p99)
- Queue depth (pending jobs)
- Database connection pool
- Cache hit rates (Redis)
- Error rates by endpoint
- Background job success/failure rates
- Voice generation latency

### Tracing
- Distributed tracing (OpenTelemetry, Jaeger, DataDog)
- Trace every request from entry to exit
- Trace async job execution
- Database query performance

### Alerting
- SLO-based alerts (availability, latency)
- Queue depth alerts (if backed up)
- Database performance alerts
- Error rate spikes
- On-call escalation (PagerDuty)

## Testing Strategy

### Unit Tests
- Service/handler functions
- Model validation
- Authorization logic

### Integration Tests
- Database layer (with test fixtures)
- Job queue processing
- API endpoints (mocked external services)

### End-to-End Tests
- Create session → fetch audio → mark complete
- Scheduled playback trigger
- Premium feature access control

### Load Testing
- 1000 concurrent users
- 10K jobs/minute through queue
- CDN audio delivery (simulated)
- Database connection pool

### Security Testing
- SQL injection prevention
- XSS/CSRF checks (API design)
- JWT validation edge cases
- Rate limiting under load
