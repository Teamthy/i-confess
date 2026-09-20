# iCONFESS — INDEPENDENT PRODUCTION AUDIT & CERTIFICATION

**Audit date:** 2026-09-20
**Repository:** `Teamthy/i-confess`
**Commit audited:** `32d5df0` (merge of PR #41), branch `arena/01a0bda1-i-confess`
**Auditor stance:** adversarial, evidence-driven. Nothing below is asserted without a command,
a file:line, or a reproduced HTTP exchange. Where something could not be verified it is marked
`NOT VERIFIED`.

---

## 1. EXECUTIVE SUMMARY

### 1.1 What this system actually is

iCONFESS is a **Go 1.25 modular monolith** (`server/`, 226 files, 48,363 lines) over
**PostgreSQL 17** (64 tables, 83 indexes, 76 foreign-key references), with:

* a **Flutter app** (`apps/mobile`, 60 Dart files, 10,336 lines) — real screens, real state
  management, **no audio engine**;
* a **hand-written Dart API client** (`clients/dart`, 3,038 lines, zero runtime dependencies);
* two **embedded single-file SPAs** served by the Go binary — a listener web app
  (`internal/webapp/index.html`, 690 lines) and an admin console
  (`internal/adminui/index.html`, 513 lines);
* two **retired Next.js stubs** (`apps/web`, `apps/admin`) marked "do not extend";
* a **Python design-token toolchain** (`design/`) that generates Dart/CSS/TS from one JSON
  source and is enforced by CI;
* **286 registered HTTP routes** (`design/routes.json`), 222 OpenAPI paths, every route served
  under both `/` and `/v1/`.

### 1.2 Actual product state

The backend is **substantially real**. Authentication, session lifecycle, entitlements,
moderation, deletion, voice-rights gating, job queue, scheduler and signed audio delivery all
execute against a live PostgreSQL and are covered by tests that fail rather than skip without a
database.

The **product is not functional end to end**:

1. Outside `ENV=development` the catalogue contains **39 categories, 78 confessions, 0 voices and
   0 audio assets**. `POST /sessions` returns **HTTP 500**. The central loop
   (Discover → Choose → Create → **Listen** → Complete) cannot complete on a real deployment.
2. The Flutter app — the primary client — contains **no audio playback capability at all**. There
   is no `just_audio`, no `audioplayers`, no `audio_service`, no background-audio or
   interruption handling in `pubspec.yaml` or in the player code.
3. **Payments do not exist.** Every store verifier is a stub. In non-production environments the
   stub grants premium for the literal string `"valid_monthly"`; in production it refuses
   everything. There are no webhooks, no provider SDKs, no renewal/cancellation/refund/grace
   lifecycle.
4. The **mobile client never registers a push token**, so scheduled reminders — the habit loop the
   product is built on — can never reach a phone, even though server-side APNs/FCM dispatch is
   implemented.

### 1.3 Biggest strengths (verified)

| Strength | Evidence |
|---|---|
| Real database-backed test suite; fails rather than skips without PostgreSQL | `internal/db/dbtest/dbtest.go`; CI `services: postgres:17`; `go test -race ./...` run live: 27 packages pass |
| Session state machine is structural, not a whitelist — COMPLETED is unreachable without playback | `internal/sessions/state.go:95-135`; live: `POST /sessions/{id}/complete` from READY → 409 `INVALID_TRANSITION` |
| Refresh-token rotation with **reuse detection that revokes the whole family** | `internal/api/session_validator.go:60-82`; `store/users.go:875-910` |
| Signed, expiring audio URLs; bytes never transit the API; traversal and tampering blocked | live: valid 200 / tampered 403 / unsigned 403 / expired 410 / `..%2f` 403 / no directory listing |
| Boot refuses development secrets outside an allowlist of environments (`ENV=prod`, `Production`, `staging` all rejected) | `internal/config/config.go:143-222` + `security_defaults_test.go` |
| Deletion is policy-driven, transactional, tombstoning, with retention reasons in writing | `internal/deletion/policy.go`, `deletion.go:201-280`; 15 deletion tests |
| Voice-rights gate returns **451** rather than silently synthesising an unlicensed voice | `router.go:222-236`, `internal/rights/` (92.1% covered) |
| Architectural layering is mechanically enforced by a parser-based test | `internal/arch/arch_test.go` |
| Design system is centralised, generated and contrast-verified | live: `design/generate.py --check`, `test_design.py`, `test_ia.py`, `check_contrast.py` — 120 tokens, 37 screens, 30/30 WCAG AA pairings pass |
| MFA is RFC 6238 with server-issued secrets, replay defence via `last_counter`, hashed recovery codes | `internal/mfa/totp.go`, `recovery.go` (94.2% covered) |
| No secrets in history | 941 historical text blobs across all 103 commits scanned with `detect-secrets 1.5.0`: **0 findings** |
| No known-vulnerable dependencies | all 22 Go modules queried against the GitHub Advisory API: **0 advisories** |

### 1.4 Biggest weaknesses

* **A documentation layer that outruns the implementation.** `README-COMPLETE.md` promises
  "Spotify-grade reliability"; `MASTER-PROMPT-COMPLETION.md` declares the audio platform
  "**100% complete**"; `docs/PROJECT-STATUS.md` marks PHASE 24 Player and PHASE 30 Premium
  "**PASS**". None of those three survives contact with the running code.
* **The generated OpenAPI spec lies about authorization.** It is generated from the live route
  table, which records the *intended* auth level as a string, not the middleware actually
  installed. Eight routes documented `bearerAuth` are served with no authentication at all.
* **No client test execution anywhere.** CI runs Go + Python only. `flutter test` / `dart test`
  are never executed — not in CI, not in `make verify` (it skips when Flutter is absent).
* **No security headers, no CORS policy, no secret/dependency scanning, no container build, no
  deployment or rollback automation.**

### 1.5 Biggest risks by category

| Category | Risk | Severity |
|---|---|---|
| **Product** | Core loop returns 500 outside dev; mobile cannot play audio | P0 |
| **Payments** | No real verification; stub grants premium in dev/test/**staging** | P0 |
| **Authorization** | 8 routes documented as authenticated are unauthenticated | P0 |
| **Anonymity/Privacy** | Public community feed serialises `author_id` next to confession text; analytics accepts attacker-chosen `user_id`; erasure leaves `security_events` (IP + UA) behind | P1 |
| **Security** | Admin console: no CSP / no X-Frame-Options / bearer token in `localStorage` / cannot complete MFA login | P1 |
| **Security** | `SEED=1` in any environment creates a super-admin with a password published in this repository | P1 |
| **Reliability** | Unbounded `idempotency_keys` (purge job never called); no `ReadTimeout`/`WriteTimeout`; per-process cache with no cross-instance invalidation; sweepers need a lock at >1 replica | P1/P2 |
| **Performance** | Public unrate-limited `/search` runs 4 unindexed `LIKE '%…%'` scans per request | P2 |
| **Technical debt** | 26 tables with no deletion policy; dead policy layer (`FilterFeed`, `FilterPublic`, `IsValidVisibility` have zero callers); timestamps stored as RFC3339 TEXT | P2 |

### 1.6 Production blockers

**24 verified launch blockers** were found (§12). They are not padded to 25.

### 1.7 Certification

> ## **NO-GO**
>
> Four P0 findings, any one of which independently prevents real users from using the product
> safely: the core loop 500s on a real deployment, the primary client cannot play audio,
> payments are unverifiable, and eight endpoints documented as authenticated are not.

---

## 2. METHOD, TOOLING AND WHAT COULD NOT BE VERIFIED

Nothing in this audit is inferred from documentation. The sandbox had **no Go, no PostgreSQL, no
Dart/Flutter, no Docker** preinstalled; the toolchain was reconstructed to match CI exactly.

| Capability | How it was obtained | Matches CI? |
|---|---|---|
| Go 1.25.5 | PyPI `go-bin==1.25.5` wheel → `/tmp/iconfess-tools/go` | Yes — CI matrix is `1.25.x` |
| PostgreSQL 17.10 | npm `@embedded-postgres/linux-x64@17.10.0-beta.17`; `libpq.so.5` + ICU SONAME symlinks recreated | Yes — CI service is `postgres:17` |
| Module resolution | `GOPROXY=direct` + a temporary `-modfile` replacing `golang.org/x/crypto` with its `github.com/golang/crypto` mirror (`proxy.golang.org` and `golang.org` are unreachable here). **`go.mod`/`go.sum` were not modified.** | Equivalent |
| Secret scanning | `detect-secrets 1.5.0` over all 941 text blobs in all 103 commits (repo unshallowed first) | — |
| Dependency advisories | GitHub Advisory API (`gh api /advisories`) per module@version | — |
| Live behaviour | Two servers booted from this exact tree: `ENV=test` on :18080 and `ENV=development` on :18081, each against its own PostgreSQL, exercised with `curl` | — |

### Gates executed

| Gate | Command | Result |
|---|---|---|
| Format | `cd server && gofmt -l .` | **0 files** |
| Build | `go build ./...` | **pass** |
| Vet | `go vet ./...` | **pass** |
| Tests (race) | `go test -race ./... -count=1` vs PostgreSQL 17.10 | **27 packages ok, 1 FAIL** (`internal/storage TestContentTypeMapping` — this image's `/etc/mime.types` maps `xyz`→`chemical/x-xyz`; environmental, pre-existing, passes on the CI runner) |
| Coverage | `go test ./... -coverprofile` | **50.6% of statements total** |
| Design | `generate.py --check`, `test_design.py`, `test_ia.py`, `check_contrast.py` | **all pass** (120 tokens, 37 screens, 102 endpoints wired, 30/30 AA pairings) |
| Python hygiene | `python3 -m compileall design` | **pass** |
| Secrets in history | `detect-secrets scan` over 941 blobs | **0 findings** |
| Dependency CVEs | 22 modules × GitHub Advisory API | **0 advisories** |

### NOT VERIFIED (and why it matters)

| Item | Reason | Consequence for this audit |
|---|---|---|
| `flutter test`, `flutter analyze` | No Dart/Flutter SDK obtainable in this sandbox | The 15 mobile test files are **unexecuted by anyone** — CI does not run them either. Mobile quality is assessed by code reading only. |
| `dart test` (`clients/dart`) | Same | Same. |
| Real Apple/Google receipt verification | No store credentials exist in the repo | Assessed as absent, not as broken. |
| Real ElevenLabs synthesis | No API key | Endpoints correctly report 503 when unconfigured (boot log). |
| Load/stress/soak at 100–10,000 concurrent users | No load generator obtainable; single-node sandbox would measure the sandbox | The 1K-DAU verdict in §14 is an **analytical model from measured per-request latencies and the schema**, explicitly labelled as such — not a measured throughput claim. |
| Branch protection / secret scanning / Dependabot settings | `gh api …/branches/main/protection` → **403 Resource not accessible by integration** | Cannot confirm whether required checks are enforced. The observable fact is that **PR #40 merged with a `FAILURE` check** (`gh pr list`), so enforcement was absent at that moment. |
| iOS/Android device behaviour, battery, accessibility runtime | No emulator/device | Accessibility assessed from tokens, semantics labels and the contrast checker. |
| Production deployment | No deployment exists in the repo beyond one static K8s manifest with `CHANGE_ME_*` placeholders | Deployment/rollback/DR are assessed as **absent**. |

---

## 3. REPOSITORY FORENSICS

500 tracked files, 4,418,217 bytes. 103 commits (after unshallowing), one contributor.

| Area | Evidence | Purpose | Status |
|---|---|---|---|
| **Mobile** | `apps/mobile/` — 60 Dart files, 10,336 lines; `pubspec.yaml`; full `ios/`+`android/` shells; 15 test files | Primary client: auth, home, explore, confession detail, builder, activity, me, library, downloads, search, premium, settings | **Partial.** Screens, routing, theming, error mapping and secure token storage are real. **No audio playback, no push registration, no offline byte storage.** Android release still signs with the **debug** keystore (`build.gradle.kts:34-36`, with the TODO left in place). |
| **Web** | `server/internal/webapp/index.html` (690 lines, embedded, served at `GET /`); `apps/web/` (retired, 3 Next.js pages) | Listener web client | **Real but secondary.** Zero-dependency vanilla SPA, XSS-escaped via a single `h()` helper, `localStorage` bearer token, Media Session API for lock-screen controls. `apps/web` is dead: no `package.json`, cannot build, and its `NEXT_PUBLIC_API_URL` cross-origin calls would be blocked (server has **no CORS**). |
| **Backend** | `server/` — 226 Go files, 48,363 lines, 37 packages under `internal/` | API, domain, storage, queue, workers | **Substantially real.** Layering enforced by `internal/arch/arch_test.go`. `internal/api/handlers.go` is a 1,524-line god-file. |
| **Admin** | `server/internal/adminui/index.html` (513 lines, served at `/admin/`); `apps/admin/` (retired) | Content/voice/audio/user administration | **Partial.** 6 views (dashboard, categories, confessions, voices, audio, users). **No moderation, queue, reports, deletion, rights or MFA views.** Token in `localStorage`, no CSP, no `X-Frame-Options`, cannot complete an MFA login. |
| **Database** | `server/internal/db/migrations/000{1..9}*.sql`; `migrate.go` | 64 tables, 83 indexes, 76 FKs, checksummed forward-only ledger | **Real and disciplined.** Immutable-migration checksum enforcement is a genuine strength. Timestamps are RFC3339 **TEXT**, not `TIMESTAMPTZ`. `CREATE INDEX` without `CONCURRENTLY`, inside a transaction. |
| **Audio** | `internal/audio`, `internal/voice`, `internal/media`, `internal/storage`, `internal/offline` | Duration measurement, pipeline, rights gate, WAV tone generation, signing | **Plumbing real, content absent.** `internal/media/wav.go` exists only to produce **placeholder tones for dev**. 0 audio assets exist for any of the 78 confessions outside dev. ElevenLabs adapter present but unexercised without a key. |
| **Payments** | `internal/billing/{pricing,trial,verify,verify_prod}.go`; `api/billing.go` | Plans, regional pricing, receipt verification | **Stub.** `NoopVerifier` accepts `"valid_*"`. `prodBlocker` refuses everything in production. No provider SDK, no webhook route in the 286-route table, no `subscriptions.provider/transaction_id` columns. |
| **Infrastructure** | `server/Dockerfile`, `server/docker-compose.yml`, `server/k8s/deployment-prod.yaml` | Container + one K8s manifest | **Thin.** Dockerfile is multi-stage with a healthcheck but runs as **root** (no `USER`), `CGO_ENABLED=1` on Alpine. K8s manifest has good `securityContext`, HPA, PDB, NetworkPolicy — and `image: iconfess:latest`, `imagePullPolicy: IfNotPresent`, `CHANGE_ME_*` secrets inline, no Ingress, no TLS, no migration job, no rollback. |
| **Contracts** | `contracts/openapi.json` (222 paths), `design/routes.json` (286 routes), `internal/api/openapi.go` | Single-source spec generated from the live route table | **Real mechanism, wrong output.** The `auth` column records intent, not installed middleware → 8 endpoints documented `bearerAuth` are unauthenticated. Only one schema component exists (`Error`); every request/response body is `"type":"object"`. 3 routes are registered via raw `mux.Handle*` and never recorded at all. |
| **CI/CD** | `.github/workflows/ci.yml`; `.golangci.yml` | build, vet, `test -race` vs PostgreSQL 17, design checks, golangci-lint v2.13, gofmt | **One job, backend-only.** No Dart/Flutter, no dependency or secret scanning, no container build/push, no deploy, no rollback, no coverage gate. `new-from-merge-base: main` means pre-existing lint debt is invisible. |
| **Testing** | 101 test files, 23,701 lines (Go 36 in `internal/api` alone; Dart 15; Python 4) | Unit + integration + contract + arch | **Backend: genuinely good. Clients: never executed.** Total statement coverage **50.6%**; `internal/api` 55.4%, `internal/store` 26.6%, `internal/auth` 37.5%, `internal/billing` 20.9%, `internal/ratelimit` 32.6%, `internal/content` 23.1%. |
| **Docs** | 60 markdown files, 14,757 lines (`docs/00`–`docs/31`, plus 20 topic docs) | Phase-gated build record | **Exceptional volume, unreliable status claims.** The phase documents record real defects found and fixed — a rare and valuable practice — while the top-level `README-COMPLETE.md` / `MASTER-PROMPT-COMPLETION.md` overstate readiness. |
| **Design** | `design/tokens.json`, `generate.py`, `test_design.py`, `test_ia.py`, `check_contrast.py`, `ia.json`, `routes.json` | Token single-source + IA validation | **Real and enforced in CI.** Generates Dart/CSS/TS; validates spacing scale, radius set, green-primary hue, typography roles, tab order, screen reachability, endpoint↔screen coverage and WCAG AA contrast. |

### Hidden / ignored / generated artefacts

* `.gitignore` is unusually well-annotated and records two real incidents (a 15,885-line patch file
  and a 0-byte `client/me` from a mistyped `curl -o`).
* ⚠️ **`.gitignore` contains a bare `.md` line** (line ~35). As written this ignores **every
  Markdown file in the repository** for *new* additions. The 60 existing docs stay tracked (ignore
  rules do not untrack), but a new `docs/*.md` would be silently dropped from `git add .`. This is
  almost certainly meant to be a specific filename and is a **P2 documentation-loss hazard**.
* `PUSH_ICONFESS.ps1` is tracked while `.ps1` and `APPLY_*.ps1`/`FIX_*.ps1` are ignored — the
  ignore rules contradict the tracked tree.
* `.arena/` holds machine state plus prior audits; consistent with the repo's own convention.
* No `.env` files are tracked. `server/.env.example` exists; the root `.env.example` referenced by
  `README-COMPLETE.md:38` **does not**.

---

## 4. TECHNOLOGY STACK AUDIT

### Server (22 modules, all with `go.sum` hashes; 0 advisories)

| Dependency | Version | Used for | Needed? | Notes |
|---|---|---|---|---|
| `golang-jwt/jwt/v5` | v5.3.1 | HS256 access tokens | Yes | Signing method pinned to HMAC in the keyfunc (`auth.go:88-90`) — `alg=none`/RS256-confusion closed. No `jti`/`aud` validation. |
| `google/uuid` | v1.6.0 | IDs | Yes | — |
| `lib/pq` | v1.12.3 | PostgreSQL driver | Yes | **In maintenance mode upstream**; `pgx` is the recommended driver. No advisory today, but no new features and limited future fixes. |
| `golang.org/x/crypto` | v0.55.0 | bcrypt | Yes | Current. |
| `aws-sdk-go-v2` + `service/s3` | v1.46.0 / v1.111.0 | Object storage + presigning | Yes | Marked `// indirect` in `go.mod` although `internal/storage/providers.go` imports it directly — `go mod tidy` was not run after that import was added. Cosmetic but signals an untidied module graph. |

**Deliberate and defensible:** the Redis client is **hand-written RESP** (~330 lines,
`internal/ratelimit/redis.go`) because only `INCR`/`EXPIRE NX`/`DEL`/`PING` are needed. The
reasoning is documented in the file. It handles pipelining, pooling, AUTH and degradation. This is
a *judgement* call, not an AI artifact — but it is untested against a real Redis in CI (no Redis
service in the workflow), so the RESP parser has never met a real server in automation.

### Mobile

| Dependency | Version | Notes |
|---|---|---|
| `flutter_riverpod` | 3.4.3 | Current major. |
| `go_router` | 18.0.1 | Current. |
| `flutter_secure_storage` | 11.0.0 | Keychain/Keystore-backed tokens — correct choice. |
| `shared_preferences` | 2.5.5 | Non-sensitive prefs only. |
| `characters` | 1.4.1 | Grapheme-correct length limits. |
| **absent** | — | **No audio package. No `firebase_messaging`. No `flutter_local_notifications`. No `in_app_purchase`/`purchases_flutter`. No `path_provider` in `pubspec.yaml` direct deps. No Sentry/Crashlytics. No analytics SDK.** |

`pubspec.lock` is committed for the app (correct) and deliberately **not** for the library
(`clients/dart`, documented reason in `.gitignore`). SDK constraint `^3.13.2`, Flutter `>=3.44.0`.

### Design/CI tooling

Python 3 (stdlib only), `golangci-lint v2.13` via the official action, `actions/checkout@v7`,
`actions/setup-go@v7` — all first-party or vendor-official, all **unpinned by SHA** (tag refs),
which is a supply-chain exposure (P3). `permissions: contents: read` is correctly set at workflow
level — a real hardening signal.

### Licensing

**No `LICENSE` file exists anywhere in the repository** (verified: `git ls-files` has no
LICENSE/COPYING/NOTICE). For a project carrying 78 Scripture-based texts, KJV references and a
voice-rights model, the absence of any licence is a legal blocker (P1, §5 IC-016).

---

## 5. FINDINGS

Severity: **P0** critical blocker · **P1** high · **P2** medium · **P3** low · **P4** minor ·
**P5** informational.

---

### IC-001 · P0 · Product / Content / Backend

**Title:** The core product loop returns HTTP 500 in every environment except `development` — there are no voices and no audio.

**Location:** `server/cmd/server/main.go:64-88`, `server/internal/seed/ensure.go:25-30`, `server/internal/seed/seed.go:81-140`, `server/internal/engine/engine.go:110-140`
**Symbol:** `main`, `seed.EnsureContent`, `seed.Seed`, `Engine.Build`, `Handler.createSession`
**Endpoint/Route:** `GET /voices`, `POST /sessions`, `POST /sessions/preview`

**Evidence:** Server booted from this exact tree with `ENV=test` against PostgreSQL 17.10:

```
2026-09-20 07:16:16 content: ensured 39 categories, 78 confessions
GET  /voices                     → 200  null
POST /sessions/preview           → 422  {"error":"no content for the selected categories and voice"}
POST /sessions                   → 500  {"error":"failed to build session"}
GET  /admin/stats (dev server)   → {"categories":39,"confessions":78,"published":78,"voices":1}
```
All 39 categories enumerated: **78 confessions served, 0 with any audio field.**

`seed/ensure.go:25-30` states the design explicitly: *"What it deliberately does NOT do is create
audio … Real audio comes from the generation pipeline behind the rights gate."* `Seed` — the only
code that creates a voice and audio assets — runs **only** when `cfg.Env == "development" ||
os.Getenv("SEED") == "1"` (`main.go:66`).

**Observed behaviour:** A production or staging deployment boots healthy, passes readiness, serves
39 categories and 78 confessions, and then fails every attempt to create a session with an
opaque 500.

**Expected behaviour:** Either the deployment refuses to serve traffic until playable inventory
exists, or session creation returns a decidable 422 with a reason the client can render
("no audio yet for this category"), and discovery surfaces hide unplayable content.

**Impact:** The product cannot be used. Every downstream surface — player, activity, history,
streaks, downloads, schedules, premium — depends on a session that cannot be built.

**Attack / failure scenario:** Launch day. Marketing drives installs. Onboarding completes,
Explore renders 39 categories, the builder accepts input, and "Create session" fails with
"Something went wrong" for 100% of users. App-store reviews are 1-star within hours. Nothing in
monitoring distinguishes this from a database incident, because `/health/ready` reports
`{"status":"ready","subsystems":{"database":true,…}}`.

**Root cause:** Content inventory was correctly promoted out of dev-only seeding (a real fix
recorded in PHASE 11), but **audio and voices were left behind** and no readiness gate or
operator runbook was added to cover the gap. `docs/PROJECT-STATUS.md` tracks this as G-34
("no audio exists for any of the 78 confessions") — the gap is *known and documented*, but its
**consequence** (500 on the primary write path, undetectable by health checks) is not.

**Recommended fix:**
1. Map `engine.ErrNoVoice` to **422** with `code: "NO_PLAYABLE_CONTENT"` in `createSession`
   (it currently falls through the generic `err != nil` → 500 at `handlers.go:1083-1086`).
2. Add a **boot-time inventory assertion** in non-dev environments: if `voices == 0` or
   `ready audio assets == 0`, log at error and expose it in `/health/ready` as a degraded
   subsystem (still ready, so the process is not crash-looped — but visible).
3. Ship an operator runbook + idempotent admin script for the real launch inventory:
   create voices → set rights → generate audio → QA approve → publish.
4. Gate the release on a staging smoke test that builds and plays one session.

**Validation steps:** `ENV=staging` boot → `GET /voices` non-empty → `POST /sessions` → 201 with
`items[].audio_url` → `GET` that URL → 200 `audio/*`.

**Regression test required:** Yes — an integration test that boots a handler with content but no
audio and asserts `POST /sessions` returns 422 with a stable code, plus a test that
`/health/ready` reports the inventory state.

**Production blocker:** **YES**

---

### IC-002 · P0 · Mobile / Audio

**Title:** The Flutter app has no audio engine — it cannot play, pause, seek or resume real audio.

**Location:** `apps/mobile/pubspec.yaml`, `apps/mobile/lib/src/features/player/player_providers.dart:29-38, 67-127`, `apps/mobile/lib/src/features/player/player_screen.dart:16`
**Symbol:** `PlayerController`, `PlayerScreen`

**Evidence:**
* `pubspec.yaml` dependencies are exactly: `cupertino_icons`, `flutter_riverpod`, `go_router`,
  `shared_preferences`, `flutter_secure_storage`, `iconfess_api`, `characters`.
  `rg 'audio|just_audio|audioplayers|audio_service|firebase' apps/mobile/pubspec.yaml` → **no match**.
* `player_providers.dart:31-34`: *"No real audio engine here (just_audio is commented out per
  D-4); this is the state machine and server sync layer…"*
* `player_screen.dart:16`: *"No real audio engine yet (just_audio commented per D-4); controls
  drive the [server] state."*
* `PlayerController.start()` sets `status: playing` and calls `POST /sessions/{id}/start`. Nothing
  produces sound.

**Observed behaviour:** Tapping play advances a progress bar driven by local state and reports
`position_ms` to the server. No audio is fetched or rendered. No background audio, no lock-screen
controls, no audio-focus/interruption handling, no Bluetooth/headset routing, no
`audio_service`/`MediaSession` integration.

**Expected behaviour:** A daily-confession **audio** product plays audio: background playback,
interruption handling (calls, alarms, Siri), audio focus on Android, lock-screen/Dynamic Island
controls, headset events, resume-from-position.

**Impact:** The primary client cannot deliver the product's only output. Section 19 of the audit
brief (play/pause/resume/seek/queue/background/lock-screen/interruptions) is **untestable because
unimplemented**. Meanwhile `docs/PROJECT-STATUS.md` records "PHASE 24 Mobile Player — **PASS**".

**Attack / failure scenario:** Not an attack — a total functional gap. Note the second-order
damage: because `start()` marks the server session `ACTIVE` without any audio playing, and
`reportProgress` sends `position_ms` from a local counter, **completion metrics become forgeable
by simply leaving the app open**. The server's carefully built invariant ("COMPLETED is
unreachable without playback") is satisfied by a client that never plays anything.

**Root cause:** The player was built as a state machine first with the audio layer deferred
(decision D-4), and the phase was marked PASS on the strength of the state machine.

**Recommended fix:**
1. Add `just_audio` + `audio_service` (or `audioplayers` + a platform audio-session plugin).
2. Configure iOS `UIBackgroundModes: audio`, `AVAudioSession` category `.playback`, and Android
   `FOREGROUND_SERVICE`/media-notification permissions. Neither is present in `Info.plist` or
   `AndroidManifest.xml` today.
3. Drive server state **from** player events, not ahead of them: `start` on first frame, `pause`
   on real pause, `INTERRUPTED` on focus loss, `complete` only when the queue has actually ended.
4. Replace the `catch (_) {}` swallows (see IC-021) with surfaced, recoverable errors.

**Validation steps:** device test matrix: play/pause/resume/seek/skip; backgrounding; lock screen;
incoming call; Bluetooth connect/disconnect; app termination mid-session → resume at position.

**Regression test required:** Yes — widget tests for control→server-call mapping, plus an
integration test asserting `COMPLETED` is only sent after the queue ends.

**Production blocker:** **YES**

---

### IC-003 · P0 · Payments / Subscriptions / Security

**Title:** Purchase verification is a stub in every environment: premium is granted for a literal string outside production, and refused unconditionally inside it. No real billing exists.

**Location:** `server/internal/billing/verify.go:29-50`, `server/internal/billing/verify_prod.go:24-89`, `server/internal/api/billing.go:64-108`, `server/internal/store/users.go:125-129`
**Symbol:** `NoopVerifier.Verify`, `VerifierFromEnv`, `prodBlocker`, `Handler.verifySubscriptionV2`, `UserStore.SetSubscription`
**Endpoint/Route:** `POST /subscriptions/verify`, `POST /v1/subscriptions/verify`, `POST /admin/users/subscription`

**Evidence — live reproduction against `ENV=test`:**
```
GET  /entitlements            → plan "free", max_session_seconds 900, offline_downloads false
POST /subscriptions/verify    → {"provider":"apple","receipt":"valid_monthly"}   → 200
     {"verified":true,"plan":"premium","provider":"apple", …}
GET  /entitlements            → plan "premium", max_session_seconds 10800,
                                offline_downloads true, premium_voices true,
                                max_concurrent_downloads 5, OfflineHoursAllowed 168
GET  /subscription            → {"active":true,"plan":"premium","status":"active"}
```
`verify_prod.go:36-39`: the fail-closed guard fires **only** when
`strings.EqualFold(os.Getenv("ENV"), "production")`. `config.Validate` treats **staging** as an
environment that must not carry development defaults — but billing does not, so staging accepts
stub receipts. Staging is described in this repository's own docs as reachable and holding real
data.

**Also absent:** no webhook route exists in the 286-route table; `subscriptions` has only
`(id,user_id,plan,status,started_at,ends_at,created_at)` — **no provider, transaction id, receipt
hash, original purchase date or auto-renew flag**; `ends_at` is never written by
`verifySubscriptionV2` and never read when resolving entitlements
(`UserStore.Subscription` filters on `status='active'` only); no cancellation, renewal, refund,
grace-period or billing-retry path; `SetSubscription` is an `UPDATE` with **no `WHERE` guard on
row existence** and no transaction.

**Observed behaviour:** Any authenticated user can self-grant premium with a 14-character string
in dev/test/staging. In production no one can ever become premium.

**Expected behaviour:** Server-side verification against the App Store Server API / Play Developer
API, signature-verified webhooks with replay protection, a durable entitlement record with
provider + transaction id + expiry, and a state machine FREE→TRIAL→ACTIVE→GRACE→EXPIRED→CANCELLED.

**Impact:** Two mutually exclusive failures. Non-production: **free premium**, entitlement
inflation, corrupted trial→paid analytics, and a QA environment that cannot distinguish real from
forged entitlements. Production: **zero revenue**, and the paywall UI (PHASE 30, marked PASS)
leads users into a purchase that cannot complete.

**Attack / failure scenario:** An attacker registers in staging, posts `{"receipt":"valid_monthly"}`
10,000 times across accounts, and every downstream analytics funnel (trial→paid, retention by
plan) is poisoned with data indistinguishable from real conversions. Alternatively, the same
payload reaches a mislabelled "production" pod whose `ENV` is set to `prod` — which
`config.Validate` would reject at boot, so this specific path is closed; the exposure is staging
and any environment named `test`.

**Root cause:** A deliberate, well-documented fail-closed guard was added for the exact string
`production`, while the rest of the environment allowlist (`staging`) was not brought into the
same rule. The underlying gap — no real verifier — is acknowledged in `verify_prod.go:58`
(`TODO: call App Store Server API`).

**Recommended fix:**
1. Immediately: gate the stub on `unsafeDefaultsAllowed()` semantics (development **and test
   only**), importing the same allowlist `config` uses, so staging fails closed.
2. Implement real verification: App Store Server API JWS transaction decoding; Play Developer
   `purchases.subscriptionsv2.get`. Persist `provider`, `original_transaction_id`,
   `expires_at`, `auto_renew`.
3. Add signed webhook endpoints (Apple V2 notifications, Google RTDN) with signature validation,
   an idempotency key on notification id, and out-of-order tolerance.
4. Resolve entitlements from `expires_at`/`auto_renew`, not from a mutable `status` string.
5. Wire `in_app_purchase` (or RevenueCat) on mobile; today the premium screen's Subscribe button
   is `onPressed: () {}` — a **no-op** (`premium_screen.dart:220-223`).

**Validation steps:** replayed webhook → single effect; reordered webhooks → correct final state;
expired subscription → premium endpoint 402; refunded purchase → entitlement revoked; duplicate
receipt across two accounts → rejected.

**Regression test required:** Yes — verifier contract tests per provider, webhook replay/ordering
tests, entitlement-expiry tests.

**Production blocker:** **YES**

---

### IC-004 · P0 · Authorization / API Contract

**Title:** Eight routes documented and specified as authenticated are served with no authentication middleware; a revoked session token — or no token at all — is accepted.

**Location:** `server/internal/api/router.go:281-285` and `:419-423`
**Symbol:** `Handler.Routes`, `Handler.createCommunityPost`, `Handler.reactCommunity`, `Handler.aiParse`, `Handler.analyticsBatch`
**Endpoint/Route:** `POST /community/posts`, `POST /community/posts/{id}/react`, `POST /ai/parse`, `POST /analytics/batch` (and all four under `/v1/`)

**Evidence — static:** every one of the eight is registered with the auth string `"user"` but the
wrapper argument is the literal `nil`:
```go
h.route(mux, "POST /community/posts",   "user", "community", "…", nil, h.createCommunityPost)
h.route(mux, "POST /community/posts/{id}/react", "user", "community", "…", nil, h.reactCommunity)
h.route(mux, "POST /ai/parse",          "user", "ai",       "…", nil, h.aiParse)
h.route(mux, "POST /analytics/batch",   "user", "analytics","…", nil, h.analyticsBatch)
```
A parse of all 238 `h.route` call sites found exactly these eight mismatches and no others.

**Evidence — contract:** `contracts/openapi.json` and the live `GET /openapi.json` both publish
`"security":[{"bearerAuth":[]}]` for all four paths. `design/routes.json` lists them as `auth: user`,
and `design/test_ia.py` asserts "every user-facing endpoint has a screen" against that table — so
the **documentation, the spec, the IA test and the route table all agree the routes are
authenticated, and the server does not enforce it.**

**Evidence — live:**
```
POST /me (revoked token)          → 401          # control: middleware works
POST /analytics/batch (revoked)   → 202 {"accepted":1}
POST /ai/parse        (revoked)   → 200
POST /analytics/batch (NO token)  → 202 {"accepted":1}
POST /community/posts (NO token)  → 500 {"error":"failed to create post"}
30× rapid POST /analytics/batch (NO token) → 30× 202   # no rate limiter on this path
```

**Observed behaviour:** `Handler.userID(r)` (`handlers.go:196-207`) falls back to parsing the raw
`Authorization` header when no middleware populated the context. It verifies only the **JWT
signature** — never the server-side session row. So a token from a logged-out, revoked,
suspended or deleted account still identifies a user here, and an absent token yields `""`.

**Expected behaviour:** 401 for missing/invalid/revoked credentials on all four, plus a rate limit
on the two write paths.

**Impact:**
* **Session-revocation bypass** on four endpoints — the exact property `session_validator.go`
  exists to guarantee.
* **Unauthenticated write path to the database.** `POST /community/posts` reaches
  `INSERT INTO community_posts` with `author_id = ''`; the FK to `users(id)` rejects it and the
  client gets a 500. That is an unauthenticated, unthrottled way to generate database errors and
  log noise, and it is one schema change away from being an anonymous posting primitive.
* **Telemetry forgery** (see IC-005).
* **Contract integrity:** the "spec generated from the live route table so it cannot drift"
  guarantee (`routetable.go:8-15`) is false for authorization, because the recorder stores the
  *declared* auth string rather than observing the installed middleware.

**Attack / failure scenario:** An attacker scripts `POST /v1/analytics/batch` at
100 events × N requests/sec from a botnet with arbitrary `user_id` values. No authentication, no
rate limit, no cost. Whatever sink is eventually wired receives fabricated conversion and
retention data attributed to real users; today it is silently discarded, so the attack is
invisible.

**Root cause:** The `auth` parameter of `h.route` is documentation, and the `wrap` parameter is
enforcement. Nothing ties them together, and the route-parity test compares *paths* between
prefixes, not wrappers.

**Recommended fix:**
1. Install `authed` (plus a rate limiter) on all eight routes.
2. Make the coupling mechanical: change `h.route` to take an **auth level enum** and select the
   wrapper itself, so a route cannot declare `"user"` and pass `nil`.
3. Add a test that walks `RouteTable()` and, for every route whose recorded auth is not `public`,
   asserts an unauthenticated request yields 401/403 — extending the pattern already used in
   `observability_test.go:51`.
4. Have `openapi.go` derive `security` from the same source that installs the middleware.

**Validation steps:** for each of the 8: no token → 401; revoked token → 401; suspended account →
401; valid token → 2xx; burst of 50 → 429 on the write paths.

**Regression test required:** Yes — the systematic sweep in (3) is the regression test.

**Production blocker:** **YES**

---

### IC-005 · P1 · Privacy / Analytics / Abuse

**Title:** Analytics ingestion accepts an attacker-supplied `user_id`, is unauthenticated and unthrottled, and discards every event.

**Location:** `server/internal/api/analytics_batch.go:13-56`, `server/internal/analytics/events.go:40-49`
**Endpoint/Route:** `POST /analytics/batch`, `POST /v1/analytics/batch`

**Evidence:**
```go
if req.Events[i].UserID == "" { req.Events[i].UserID = h.userID(r) }   // line 41-43
…
// In prod: forward to OTel/Segment sink; here LogSink no-op preserves API contract
httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"accepted": len(req.Events)})
```
`LogSink.Track` returns `nil` and does nothing. The PII filter deletes exactly three literal keys
— `body`, `email`, `scripture_text` (lines 48-52). Live: an event with
`{"name":"app_opened","user_id":"any-victim-id","props":{"x":1}}` and **no Authorization header**
was accepted with `202`. 30 consecutive unauthenticated POSTs all returned 202.

**Impact:** (a) forged behavioural telemetry attributed to arbitrary real users; (b) an
unbounded, unauthenticated, unthrottled write/parse path; (c) the client-side PII blocklist in
`apps/mobile/lib/src/core/analytics/analytics.dart` (a genuinely thoughtful 24-entry blocklist,
tracked as gap G-33) is **decorative**, because the server neither requires nor validates it and
`props` is an open `map[string]any` — a client can send `props.confession_text` or
`props.query` and the server strips none of it; (d) because nothing is persisted, the retention,
DAU and trial→paid metrics that `docs/PHASE*` and the admin dashboard reference **do not exist**.

**Recommended fix:** require authentication; take `user_id` **only** from the validated session
(never the body); allowlist `props` keys per event name with type and length bounds; add a rate
limit; and either implement a sink or return 501 so the contract is honest.

**Regression test required:** Yes — rejected body-supplied `user_id`, rejected unknown `props`
key, 429 under burst.

**Production blocker:** YES (as an abuse and data-integrity blocker; trivially fixable)

---

### IC-006 · P1 · Anonymity / Privacy

**Title:** The public community feed serialises the author's user id alongside the confession body.

**Location:** `server/internal/community/policy.go:52-59` (`Post` struct), `server/internal/community/store.go:26-46` (`Feed` query), `server/internal/api/handlers_community.go:43-51`
**Endpoint/Route:** `GET /community/feed`, `GET /v1/community/feed` (both **public**)

**Evidence:**
```go
type Post struct {
    ID         string `json:"id"`
    AuthorID   string `json:"author_id"`   // ← no omitempty, always serialised
    Body       string `json:"body"`        // user confession text
    …
}
```
```sql
SELECT id, author_id, body, visibility, status, created_at
FROM community_posts WHERE visibility=? AND status IN (?,?) …
```
The handler writes the slice straight to the response. `policy.go:74-92` defines `FilterFeed` and
`FilterPublic`, which would at least constrain what is returned — **both have zero callers**
(verified across `server/`), and `Store.Feed` filters in SQL on `visibility='shared'` only, so
moderator-approved `public` posts are **not returned at all** by the feed query despite
`VisibilityPublic` being documented as "the destination of the moderation pipeline".

**Impact:** In a confession product, `author_id` next to `body` is a **deanonymization primitive**.
Anyone who learns a user id (from a report, an admin screen, a leaked export, or by correlating
`/community/feed` with a moderation queue response that also returns `user_id` — observed live:
`GET /admin/moderation/queue` includes `"user_id":"…"` beside `"text":"I declare peace."`) can
attribute private religious expression to a person. Even where the audience is "shared", the
identifier should never leave the server.

**Currently masked** because creation fails (IC-004) and no moderator can approve posts through
the console (IC-015). It is a live code path one bug-fix away from being reachable.

**Recommended fix:** introduce a `FeedPost` DTO that omits `author_id` entirely; return
`{"id","body","created_at","reaction_counts"}`; make `Store.Feed` honour both `shared` and
`public`; delete or wire up `FilterFeed`/`FilterPublic`/`IsValidVisibility`.

**Regression test required:** Yes — assert the marshalled feed body contains no `author_id` key.

**Production blocker:** YES (privacy/anonymity is a security property of this product)

---

### IC-007 · P1 · Privacy / Data Retention & Deletion

**Title:** Account erasure leaves identity-bearing rows behind: `security_events` (user id + IP + user agent + metadata), `audit_logs.actor` (admin email), `user_confession_audio.url`, `jobs.payload`, `outbox_events.payload`. The covering test cannot see them.

**Location:** `server/internal/deletion/policy.go:63-154`, `server/internal/deletion/deletion_test.go:99-135`, `server/internal/db/migrations/0001_baseline.sql:577-585, 734-741, 751-758, 761-773, 1071-1079`
**Symbol:** `Policies`, `TestEveryUserTableHasAPolicy`, `Service.Erase`

**Evidence:** The policy list contains **38 entries against 64 tables**. The test that is
documented as the load-bearing guarantee parses the schema for the literal string
`REFERENCES users(id)`:
```go
if strings.Contains(line, "REFERENCES users(id)") && current != "" { … }
```
`security_events.user_id` is declared `TEXT` with **no foreign key**, so the test never sees it.
Same for `audit_logs.admin_user_id`/`actor`, `user_confession_audio` (FK to `user_confessions`,
not to `users`, and not in the policy list), `jobs.payload` and `outbox_events.payload`.

Schema of the most serious omission:
```sql
CREATE TABLE IF NOT EXISTS security_events (
    id TEXT PRIMARY KEY, user_id TEXT, event_type TEXT NOT NULL,
    ip_address TEXT, user_agent TEXT, metadata TEXT, created_at TEXT NOT NULL);
```
`security_events` rows are written by `POST /auth/security-events` and by server-side paths with
`r.RemoteAddr` and `r.UserAgent()` (`handlers.go:805`, `:784`). These are exactly the
"IP addresses / device identifiers / request headers" the audit brief lists as deanonymization
vectors, and they **survive a completed erasure** with the user id intact.

`docs/DELETION_AND_MFA.md:14-22` claims: *"`internal/deletion/policy.go` names **every** table
holding user data … A table added in six months without a deletion rule breaks the build."* That
claim is false for tables whose user reference lacks a foreign key.

**Impact:** A GDPR Art. 17 / CCPA deletion request is reported to the user as honoured
(`deletion_records.rows_deleted`, "will_be_deleted" list shown in the preview) while IP address,
user agent, event metadata and admin-action attribution persist indefinitely, keyed by a
tombstoned-but-stable user id. `deletion_records.user_ref` and `subscriptions`/`consent_records`
(retained by policy) re-link that id.

**Recommended fix:**
1. Add explicit policies: `security_events` → **Anonymise** `user_id` (or Erase; justify in
   writing), and null `ip_address`/`user_agent` on erasure; `audit_logs` → Anonymise `actor`
   and `admin_user_id`; `user_confession_audio` → Erase via `user_confession_id` parent;
   `jobs`/`outbox_events` → Erase where payload references the user, or add a scrub step.
2. Replace the string-match test with a **schema-introspection** test: enumerate every table that
   has a column named `user_id`/`*_user_id`/`actor`/`author_id`/`reporter_id`, or a FK (direct or
   transitive) to `users`, and require a policy for each.
3. Add an end-to-end test that inserts a row into **every** table, erases, and asserts no
   surviving row contains the user id, an IP, or a user agent.

**Regression test required:** Yes — (2) and (3).

**Production blocker:** YES

---

### IC-008 · P1 · Security / Configuration

**Title:** One environment variable creates a super-admin whose password is published in this repository.

**Location:** `server/cmd/server/main.go:66`, `server/internal/seed/seed.go:142-166`
**Symbol:** `main`, `seed.Seed`, `hashDemoPassword`

**Evidence:**
```go
if cfg.Env == "development" || os.Getenv("SEED") == "1" { seed.Seed(conn, objStore) }
```
```go
adminHash, _ := hashDemoPassword("Admin!ChangeMe-2026")
admin, _ := users.Create(bg, "admin@iconfess.dev", adminHash, "Admin", "UTC")
users.SetAdminRole(bg, admin.ID, "super_admin")
```
`Seed` early-returns only if categories already exist — so on a **fresh** production database
started with `SEED=1` (a plausible one-off to populate inventory, and the only way to get voices
and audio today, per IC-001), the deployment gains `admin@iconfess.dev` /
`Admin!ChangeMe-2026` with `super_admin`. `config.Validate` does not check for it.
`docs/01-DOMAIN-MODEL.md:246` dismisses this as "startup only, in development or when `SEED=1`.
Not a concern."

Verified live: those credentials authenticate against the dev server and return a token that
passes `GET /admin/stats` → 200.

**Impact:** Full administrative takeover — content publish/unpublish, role grant to any user,
account suspension, subscription grant, voice-rights changes, immediate user erasure
(`POST /admin/users/{id}/erase`) — from a credential readable by anyone with repo access.

**Recommended fix:** refuse to run `Seed` when `cfg.IsProduction()` **or** when the environment is
not in `unsafeDefaultsAllowed()`; remove the demo accounts from `Seed` entirely and provide a
separate, explicitly-invoked `cmd/admin-bootstrap` that requires an operator-supplied password;
add a boot check that fails if any `admin_users` row belongs to a user whose email ends in a
known demo domain.

**Regression test required:** Yes — `ENV=production SEED=1` must not create any account.

**Production blocker:** YES

---

### IC-009 · P1 · Security / Web Hardening

**Title:** The admin console and the whole API are served with no security headers; the console stores a bearer token in `localStorage` and can be framed.

**Location:** `server/internal/adminui/ui.go:16-19`, `server/internal/adminui/index.html:130, 216`, `server/internal/webapp/app.go:40-46`, `server/internal/api/router.go:463`
**Endpoint/Route:** `GET /admin/`, `GET /`, all API routes

**Evidence — live response headers:**
```
GET /admin/  → Accept-Ranges, Content-Length, Content-Type, Traceparent,
               X-Request-Id, X-Trace-Id, Date            # nothing else
GET /        → + Cache-Control: no-cache, Referrer-Policy: same-origin,
                 X-Content-Type-Options: nosniff        # no CSP, no X-Frame-Options
GET /metrics → no security headers
```
`rg -i 'cors|Access-Control-Allow' server/internal` → **no matches**: there is no CORS policy at
all.

`adminui/index.html:130`: `const state = { token: localStorage.getItem('ic_token') || '' … }`;
`:216`: `localStorage.setItem('ic_token', state.token)`.

`docs/SECURITY.md` claims: *"**Headers:** `next.config.mjs` `nosniff/DENY/strict-origin`
`Permissions-Policy` none"* — that file belongs to `apps/web`, which is **retired and not
deployed**. The surfaces actually served set none of those headers.

**Impact:**
* **Clickjacking:** no `X-Frame-Options`/`frame-ancestors` on a console whose every action is a
  `fetch` with a token read from `localStorage`. An attacker page can frame `/admin/` and drive a
  signed-in administrator through destructive actions (role grant, user erase).
* **XSS → full admin takeover:** no CSP; the console builds views with template literals into
  `innerHTML`. Most interpolations go through `esc()`, but the pattern is fragile — e.g.
  `onclick="setStatus('${c.id}','published')"` embeds a server value into an inline handler, and
  inline handlers make a CSP `script-src` policy impossible without a rewrite.
* **No CORS policy** means the retired Next.js clients cannot call the API cross-origin at all,
  and there is no documented origin allowlist for any future web surface.

**Recommended fix:** add a global header middleware (`Content-Security-Policy` with
`frame-ancestors 'none'`, `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`,
`Referrer-Policy: no-referrer`, `Strict-Transport-Security`, `Permissions-Policy`); move the admin
session to an `HttpOnly; Secure; SameSite=Strict` cookie or at minimum out of `localStorage`;
replace inline `onclick` with `addEventListener` so a real CSP is possible; define an explicit
CORS allowlist.

**Regression test required:** Yes — assert the header set on `/`, `/admin/` and a sample API route.

**Production blocker:** YES

---

### IC-010 · P1 · Cryptography / Authentication

**Title:** TOTP shared secrets are stored in plaintext.

**Location:** `server/internal/db/migrations/0001_baseline.sql:598-603`, `server/internal/store/users.go:730-760`, `server/internal/mfa/totp.go:52-58`

**Evidence:**
```sql
CREATE TABLE IF NOT EXISTS mfa_secrets (
    user_id TEXT PRIMARY KEY, secret TEXT NOT NULL, enabled INTEGER NOT NULL DEFAULT 0, …);
```
`BeginMFAEnrolment` writes the base32 secret verbatim; `MFAEnrolmentFor` reads it back to compute
codes. By contrast recovery codes are stored as SHA-256 hashes and one-time tokens as hashes
(`auth/tokens.go`), so the codebase clearly knows the pattern.

**Impact:** A database read (SQL injection elsewhere, a leaked backup, an over-broad replica, a
compromised analyst account) yields **live second factors for every enrolled user**, silently
downgrading MFA to the strength of the password. `GET /me/export` correctly excludes them, but
the at-rest exposure remains.

**Recommended fix:** encrypt the column with an application-level key from a KMS/env secret
(AES-256-GCM, envelope-encrypted, rotatable), or move to per-user derived storage. Document key
custody and rotation. Add a test asserting no plaintext base32 secret appears in a DB dump fixture.

**Production blocker:** YES (before enrolling real users' MFA)

---

### IC-011 · P1 · Audio Delivery / CDN / Production Configuration

**Title:** Production audio delivery is unconfigured and internally contradictory: `MEDIA_BASE_URL` is never validated, S3 presigning ignores the CDN domain, and the `/media` origin is disabled in production.

**Location:** `server/internal/config/config.go:107, 172-222`, `server/cmd/server/main.go:94-100`, `server/internal/storage/providers.go:164-181, 455-465`
**Symbol:** `Config.Validate`, `S3Storage.GenerateSignedURL`, `LocalStorage.GenerateSignedURL`

**Evidence:**
* `MediaBaseURL: getenv("MEDIA_BASE_URL", "/media")` — `Validate()` checks JWT secret, token TTL,
  audio sign secret, storage provider/bucket, email provider, `PUBLIC_BASE_URL` scheme and
  `REDIS_ADDR`. **`MEDIA_BASE_URL` is not checked.**
* `S3Storage.GenerateSignedURL` returns `res.URL` from `s3.NewPresignClient` — the configured
  `CDNDomain` is used **only** by `LocalStorage` (`providers.go:361`).
* `main.go:96`: `if !cfg.IsProduction() { … h.SetMediaHandler(local.Handler("/media")) }` — in
  production no `/media` handler is registered at all.

**Consequences:** a production deployment that sets `STORAGE_PROVIDER=s3` but leaves
`MEDIA_BASE_URL` at its default hands clients **relative** URLs (`/media/audio/…`) that resolve
against the API origin, where nothing is mounted → 404 for every track. A deployment that sets
`MEDIA_BASE_URL` to a CDN host still receives **S3 presigned URLs pointing at
`bucket.s3.region.amazonaws.com`**, bypassing the CDN entirely — contradicting
`ARCHITECTURE.md` ("CDN for all audio delivery", "Audio range requests supported for seeking") and
the config comment "Set this to the CDN hostname in production". `CacheControl: public,
max-age=31536000, immutable` is set on the objects but is unreachable without a CDN in front.

**Impact:** Audio playback fails or silently incurs S3 egress pricing with no edge caching, no
range-request guarantees and no hotlink protection beyond the 1-hour signature. Bandwidth cost
model in §16 assumes a CDN that this code path does not produce.

**Recommended fix:** validate `MEDIA_BASE_URL` is an absolute `https://` URL outside
dev/test; rewrite the presigned host to the CDN domain (Cloudflare/R2 or CloudFront with a
custom domain and signed cookies/URLs), or sign at the edge with the existing HMAC scheme so the
CDN can verify without an S3 round trip; add a smoke test that fetches a minted URL end to end.

**Production blocker:** YES

---

### IC-012 · P1 · Mobile / Notifications / Scheduling

**Title:** The mobile client never registers a device or push token, so scheduled reminders — the product's habit loop — can never be delivered.

**Location:** `clients/dart/lib/src/endpoints.dart:131` (`postMeDevices` exists), `apps/mobile/lib/**` (no caller), `apps/mobile/pubspec.yaml` (no messaging dependency), `apps/mobile/android/app/src/main/AndroidManifest.xml`, `apps/mobile/ios/Runner/Info.plist`
**Evidence:** `rg -n 'postMeDevices|device_id|push_token|registerDevice' apps/mobile/lib` → **no
matches**. No `firebase_messaging`, no `flutter_local_notifications`, no APNs/FCM plugin, no
`UIBackgroundModes`, no `POST_NOTIFICATIONS` permission, no `NSUserNotificationsUsageDescription`.
Server side is real: `internal/push` (80.6% covered) implements APNs token auth and FCM with its
own service-account OAuth2 flow; `internal/scheduler/dispatcher.go` sweeps every minute with an
occurrence-key uniqueness guarantee.

**Impact:** The server will happily mark deliveries `failed — no registered devices` forever.
"PHASE 24 Activity — PASS (schedules from GET /schedules)" and `docs/PUSH_AND_SCHEDULING.md`
describe a loop that cannot close. Notification privacy (no confession text in previews) is
correctly implemented server-side — the title is the user's schedule label and the body is
`"Your N-minute session is ready."` — but it is untestable in practice.

**Recommended fix:** add a messaging plugin, request permission on an explicit user action,
register `{device_id, platform, push_token}` via `POST /me/devices` on launch and on token
refresh, handle the `iconfess://schedules/{id}/start` deep link (the server already emits it), and
wire an Android foreground service / iOS background modes for audio at the same time as IC-002.

**Production blocker:** YES

---

### IC-013 · P1 · Mobile / Offline

**Title:** Offline downloads are licences without bytes — nothing is fetched, stored, encrypted or played offline.

**Location:** `apps/mobile/lib/src/features/downloads/downloads_screen.dart`, `downloads_providers.dart`, `clients/dart/lib/src/repository.dart:573-645`
**Evidence:** `DownloadTicket.downloadUrl` is parsed and then never used; the screen lists
licences, shows an "expiring within 3 days" banner, and offers renew/remove. No `path_provider`
directory usage, no `File.writeAsBytes`, no HTTP fetch of the signed URL, no use of
`internal/offline/crypto.go`'s AES-GCM `Seal`/`Open` on the client. Server side is real and
well-modelled: entitlement-gated (`402 ENTITLEMENT_REQUIRED` verified live for a free user),
download-count limited, expiring, revocable.

**Impact:** Journey D (offline → download → listen → progress → reconnect → sync) is entirely
unimplemented on the client. Premium's headline "offline listening" entitlement is sold but not
delivered.

**Production blocker:** YES

---

### IC-014 · P1 · API Contract / Backward Compatibility

**Title:** Every JSON body is decoded with `DisallowUnknownFields`, so any client sending an extra field receives a 400 — there is no forward or backward compatibility.

**Location:** `server/internal/httpx/httpx.go:27-39`
**Evidence — live:**
```
POST /templates {"name":…,"category_ids":[…],"duration_seconds":600}                    → 400 "invalid request body"
POST /templates {"name":…,"category_ids":[…]}                                           → 201
POST /admin/moderation/user-confessions/{id}/review {"decision":"approved","visibility":"public"} → 400
```
`duration_seconds` is a *reasonable* field for a session template; `visibility` is a field this
repository's own moderation model uses elsewhere.

**Impact:** A mobile build that adds one field to a request body breaks against an older server;
an older mobile build breaks against a newer server that renames or adds nothing but tightens a
struct. This is the exact "old client + new API" failure the audit brief asks about (§66), and it
is guaranteed rather than probabilistic. It also makes the 400 message useless — "invalid request
body" never names the offending field.

**Recommended fix:** drop `DisallowUnknownFields` in favour of explicit validation of the fields
that matter; if strictness is wanted for internal admin APIs, scope it there and return the
unknown key name in the error.

**Regression test required:** Yes — a contract test that a superset body is accepted.

**Production blocker:** YES (before any second client release)

---

### IC-015 · P1 · Admin / Moderation Operations

**Title:** The admin console cannot complete an MFA login and has no UI for moderation, reports, queue, rights or deletion.

**Location:** `server/internal/adminui/index.html:163, 205-221`, `server/internal/api/moderation.go`, `router.go:196-198, 222-239`
**Evidence:** `const views = ['dashboard','categories','confessions','voices','audio','users']`.
`doLogin()` does `state.token = body.token` — when the server answers
`{"mfa_required":true,"message":"Enter the code…"}` (verified live behaviour of `/auth/login` for
an enrolled account) there is no `token`, so `localStorage.setItem('ic_token', undefined)` stores
the **string** `"undefined"` and every subsequent call sends `Bearer undefined`.
`rg 'mfa|recovery' internal/adminui/index.html` → no handling.

Meanwhile PHASE 31 shipped `GET /admin/moderation/queue`,
`POST /admin/moderation/user-confessions/{id}/review`,
`POST /admin/moderation/reports/{id}/decision`, the audio-QA approve/reject/publish/archive
lifecycle, voice-rights management and `POST /admin/users/{id}/erase` — all verified present and
role-gated (live: non-admin → 403 `AUTH_INSUFFICIENT_PERMISSION`, anonymous → 403), with **no
operator surface**. Moderation is therefore API-only, via `curl`, by people who must hand-roll
request bodies that IC-014 makes unforgiving.

**Impact:** UGC cannot practically be moderated; reports cannot be actioned; audio cannot be QA'd
through a UI; and any administrator who does the right thing and enrols MFA **locks themselves out
of the console**.

**Recommended fix:** handle `mfa_required` in the console login; add moderation/queue/reports,
audio QA, voice rights and deletion views; and — given IC-009 — put the console behind SSO + a
network boundary rather than the public origin.

**Production blocker:** YES

---

### IC-016 · P1 · Legal / Privacy / Content Licensing

**Title:** No licence, no privacy policy, no terms, no cookie/consent surface — while scripture provenance and voice rights are unrecorded.

**Evidence:**
* `git ls-files` contains **no** `LICENSE`, `COPYING`, `NOTICE` or third-party licence file.
* No privacy-policy or terms page exists on any served surface; `apps/web/app/sitemap.ts` is
  documented (`docs/PHASE7_SCALE_TRUST.md:36`) as listing `/privacy` and `/terms` — those pages do
  not exist in `apps/web/app/`, and `apps/web` is retired and unbuilt.
* Consent infrastructure exists server-side (`POST /auth/consent`, `consent_records` table with
  `category/granted/version/ip/user_agent`, retained-by-policy on deletion) but **no client
  surface** records consent; `rg 'consent' apps/mobile/lib` → no matches.
* Scripture: 78 confessions carry `scripture_references` with `translation: "KJV"` and an
  `is_direct_quote` flag — a genuinely good provenance model. But there is **no licence record for
  the translation**, no theological-review artefact, and `author` is hard-coded to
  `"i-confess content team"` for all 78 (`seed/ensure.go:84`). `docs/PROJECT-STATUS.md` tracks
  this honestly as G-35 ("canonical content has had no theological review; `Author` overstates its
  provenance").
* Voice rights: the model is strong (`voice_rights` with territories, AI-generation grant requiring
  a written attestation, 451 on unlicensed synthesis, revocation endpoint) — but **no rights
  records exist** for the one seeded voice, and G-42 records that "the seeded demo catalogue fails
  its own `voices_licensed` gate".

**Impact:** Publishing Scripture-derived audio content, in an app that collects email, IP, user
agent, device identifiers and religious-expression data, with no privacy policy, no terms, no
licence and no consent capture is a legal exposure, not a paperwork gap. Religious belief is
**special-category data** under GDPR Art. 9; a confession product needs an explicit lawful basis
and a published policy.

**Recommended fix (engineering scope):** add `LICENSE`; ship `/privacy` and `/terms` on the served
web surface; wire a consent gate in onboarding that calls `POST /auth/consent` with a version;
record translation licence and reviewer identity per confession version; do not ship content whose
`author` is a placeholder. **Qualified legal review is required and is outside this audit.**

**Production blocker:** YES

---

### IC-017 · P1 · Authentication / Abuse

**Title:** Registration issues a fully privileged session before email verification, and `email_verified` gates nothing.

**Location:** `server/internal/api/handlers.go:211-275` (`register` → `issueToken`), `server/internal/api/session_validator.go:36-50`
**Evidence:** `register` creates the user with `status = "active"` (`store/users.go:29`), sends a
verification token, and immediately calls `h.issueToken(w, r, u)`. `ValidateSession` accepts
`active` and `pending_deletion` and rejects everything else; `pending_verification` is mentioned in
a comment as a rejected state but **nothing ever writes it**. `rg 'email_verified' server/internal/api`
shows it is read only for display. The only signup throttle is `Registration = 5 per 10 minutes`
**per IP** (verified live: 3×200 then 429).

**Impact:** Disposable-email account farms can create working accounts with full UGC, reporting,
favourite, template and community capabilities. Reporting abuse (`ReportSubmission` 8/10min) and
moderation-queue flooding become cheap. There is no CAPTCHA, no device attestation, no
proof-of-work, and no per-device or per-fingerprint control — the audit brief's "do not rely solely
on IP blocking" is exactly the situation here.

**Recommended fix:** set `status='pending_verification'` at creation, admit it only on read-only
public endpoints, and require verification before session creation or UGC; add a challenge
(hCaptcha/Turnstile) on register/login; add a per-account and per-device-fingerprint throttle in
addition to per-IP.

**Production blocker:** YES (abuse resistance at launch)

---

### IC-018 · P1 · CI/CD / Release Engineering

**Title:** The pipeline covers one language, has no security or dependency scanning, no client tests, no artefact build and no deploy/rollback — and code has merged with a red check.

**Location:** `.github/workflows/ci.yml`, `.golangci.yml`, `Makefile`, `server/Dockerfile`, `server/k8s/deployment-prod.yaml`
**Evidence:**
* `gh pr list`: **PR #40 merged 2026-09-20T06:40:13Z with `statusCheckRollup.conclusion: FAILURE`.**
  PRs #22–#33 and #36, #38 also merged with failing checks. The two most recent runs on `main`
  (35495421214) are green, so the tree is currently clean — but the **gate is demonstrably not
  enforced**.
* `gh api repos/Teamthy/i-confess/branches/main/protection` → **403** (`NOT VERIFIED`).
* The workflow runs: `go mod verify`, `go build`, `go vet`, `go test -race`, design scripts,
  `golangci-lint` with `new-from-merge-base: main`, `gofmt -l`. **No** Dart/Flutter job, **no**
  `govulncheck`/Dependabot/CodeQL/secret scanning, **no** `docker build`, **no** SBOM, **no**
  deploy, **no** rollback, **no** coverage threshold.
* `make verify` includes `mobile-check`, which prints `SKIPPED - flutter is not on PATH` and
  **exits 0**. A local "all checks passed" therefore routinely means "the app was never checked".
* `make verify` also ends with `@echo "verify: all checks passed"` while depending on
  `fmt-check design-check build vet lint test mobile-check`; `golangci-lint` is not installed in
  most environments, and `make` would fail — but the *skip* path is the dangerous one.
* `server/Dockerfile` has no `USER` directive (runs as root, contradicting the K8s
  `runAsNonRoot: true`, which will make the pod fail to start with
  `container has runAsNonRoot and image will run as root`).
* `server/k8s/deployment-prod.yaml`: `image: iconfess:latest` + `imagePullPolicy: IfNotPresent`
  (unversioned, non-reproducible, unrollback-able), inline `CHANGE_ME_*` secrets, no Ingress/TLS,
  no migration Job, no `PodSecurityContext` `seccompProfile`, and a `NetworkPolicy` allowing egress
  only to postgres/redis/DNS — which will break ElevenLabs, S3, Postmark, APNs and FCM.

**Impact:** Broken code can and did reach `main`. The Dart clients are entirely unverified by
automation. The deployment manifest, if applied, does not start.

**Recommended fix:** make the CI check required (branch protection); add jobs for
`flutter analyze && flutter test` and `dart test`; add `govulncheck`, `gitleaks`, Dependabot or
Renovate, and `docker build` + image scan; pin actions by SHA; add `USER 1000` to the Dockerfile;
tag images by commit SHA; add a migration Job and a documented rollback; fix the NetworkPolicy
egress allowlist.

**Production blocker:** YES

---

### IC-019 · P1 · Content Quality & Governance

**Title:** The launch catalogue is 78 unreviewed texts with fabricated authorship, no audio, and no editorial audit trail that an operator can read.

**Evidence:**
* `Author: "i-confess content team"` for all 78 (`seed/ensure.go:84`); G-35 acknowledges no
  theological review has occurred.
* 0 audio assets outside dev (IC-001); G-34 acknowledges it.
* `content_versions` and `content_moderation_history` tables exist and PHASE 31 writes history on
  canonical status changes — but `GET /admin/audit` returned **`[]`** live after a UGC submission,
  a report, a queue read and an attempted review, confirming G-41 ("`audit_logs` still does not
  cover content/moderation actions").
* `GET /admin/moderation/queue` returns the **full text** of private user confessions plus
  `user_id` — appropriate for a moderator, but it is the same response shape that IC-006 leaks
  publicly for community posts, and there is no moderator role distinct from `super_admin` in the
  console.
* Duplicated/repetitive content: e.g. "A New Creation" (`confessions_canonical.go:249-252`)
  repeats its medium text verbatim as the opening of its long text. Intensity, tags and category
  assignment are otherwise plausible and consistent; no lorem ipsum, no fake statistics, no fake
  testimonials were found in the corpus.

**Impact:** Shipping unreviewed religious content under a fabricated author, with no audio and no
readable audit trail, is a content-governance and reputational blocker for a faith product.

**Production blocker:** YES (content sign-off, not code)

---

### IC-020 · P2 · Performance / Abuse — Search

`GET /search` is **public, unauthenticated and unrate-limited**, and per request executes up to
four `LIKE '%…%'` scans (`search.go:97-120` and siblings, each with `LIMIT 50`) over `confessions`
(including `medium_text`), `categories`, `collections` and `voices`. A leading wildcard makes
every index unusable. Live: `q=%` and `q=_` each returned the maximum 20 results — **LIKE
metacharacters are not escaped**, so a caller can enumerate the whole catalogue with one character
and can build an expensive scan at will. `rg 'tsvector|GIN|to_tsquery' server/internal/db/migrations`
→ no full-text index exists, despite `docs/SECURITY.md` claiming "sqli 200 (tsvector sanitized)".
**Fix:** escape `%`/`_`/`\`, add a rate limit, cap `q` length, and move to a `tsvector` GIN index
or trigram index. **Blocker:** no, but it is the cheapest scraping/DoS vector in the system.

### IC-021 · P2 · Mobile Reliability — Silent failure

`PlayerController.pause/resume/skip/complete/reportProgress` all end in `catch (_) {}`
(`player_providers.dart:87, 94, 101, 112, 125`). Local state is mutated **before** the network
call, so a failed `complete` leaves the UI showing "completed" while the server says `ACTIVE` —
precisely the client/server divergence §16 of the brief asks about. `reportProgress` always sends
`queue_item_id: ''`, so the server can never bind a resume point to an item. **Fix:** await the
call, roll back local state on failure, surface a recoverable error, send the real item id.

### IC-022 · P2 · API Contract — Inconsistent shapes

* `POST /subscriptions/verify` returns entitlements with **Go field names**
  (`"Plan"`, `"CanAccessPremiumVoices"`, `"MaxConcurrentDownloads"`) because it marshals the
  struct directly (`billing.go:101-107`), while `GET /entitlements` returns snake_case
  (`profile.go:380-390`). Two shapes for one concept, verified live.
* List endpoints return JSON `null` rather than `[]` when empty: `GET /voices` → `null`,
  `GET /me/confessions` → `null`. The Dart client survives this (`parseList`), the vanilla webapp
  survives it (`|| []`), but any third-party consumer will not.
* `POST /sessions` → **500** for the same "no content" condition that `POST /sessions/preview`
  reports as **422** (IC-001).
**Fix:** one response envelope, one casing convention, `[]` not `null`, and error mapping from the
engine's sentinel errors in a single place.

### IC-023 · P2 · Reliability — Unbounded growth and missing timeouts

* `IdempotencyStore.PurgeExpired` exists with the comment *"Called from the periodic cleanup job"*
  — **it has no caller anywhere in `server/`** (verified). `idempotency_keys` grows without bound
  and stores full response bodies, including **signed audio URLs**, for 24h per key.
* `http.Server` sets only `ReadHeaderTimeout` (`main.go:270-274`). No `ReadTimeout`,
  `WriteTimeout` or `IdleTimeout` → slow-body and slow-read resource exhaustion. `httpx.DecodeJSON`
  caps bodies at 1 MiB, which mitigates but does not bound connection hold time.
* `scheduled_deliveries`, `security_events`, `jobs` (completed/dead-lettered), `audio_events`,
  `playback_history` and `session_progress` have **no retention or partitioning strategy**.
* The cache (`internal/cache`) is per-process with **no cross-instance invalidation** (tracked as a
  known gap): with the K8s manifest's 3–20 replicas, an admin publishing a confession leaves up to
  5 minutes of stale category content on other pods and, worse, a *retracted* confession servable
  from cache after removal.
**Fix:** wire the purge into the existing ticker loop; add server timeouts; add retention jobs;
move the content cache to Redis or add a pub/sub invalidation channel.

### IC-024 · P2 · Configuration — Hardcoded public URL

`handlers_templates.go:73` builds `"share_url": "https://iconfess.app/t/" + token`, ignoring
`PUBLIC_BASE_URL` (which exists, is validated as https in production, and is used correctly for
email links). Any deployment on another domain hands users share links that 404. The Dart client
test asserts the hardcoded value (`repository_test.dart:444`), cementing it.

### IC-025 · P2 · Product Integrity — Fabricated statistic

`activity_providers.dart:18-41`: the "streak" provider builds `completedDates` via
`DateTime.parse(s.items.isNotEmpty ? '' : '')`, which **always throws**, so every element falls
back to `DateTime.now()`; `completedDates` is then **never used**, and the returned value is
`min(count of COMPLETED sessions, 7)`. The Activity screen therefore displays a number labelled as
a daily streak that is really a capped session count. This is a user-facing fabricated metric with
dead code beside it — the single clearest "generated, not engineered" artifact in the repository.
**Fix:** compute the streak from session completion dates server-side (the data exists) or remove
the card.

### IC-026 · P2 · Authorization Hygiene — Unrecorded destructive route

`POST /admin/users/{id}/erase` is registered with raw `mux.Handle` (`router.go:230`), so it is
**absent from `RouteTable()`, from `contracts/openapi.json` and from `design/routes.json`**
(verified: `grep -c erase design/routes.json` → 0). The systematic admin-authorization test
iterates `h.RouteTable()` (`admin_authorization_test.go:43-46`), so the most destructive admin
endpoint is outside the sweep that `docs/PROJECT-STATUS.md` cites as "all 44 admin routes asserted
to reject a non-admin". It *is* covered by one bespoke test (`deletion_test.go:243-252`, verified
live: non-admin → 403), so this is a **coverage and documentation** defect rather than an
exploitable hole. Same for `GET /metrics`, `GET /openapi.json`, `GET /v1/openapi.json`, `GET /`,
`GET /admin/` and `GET /media/` — all registered outside the recorder, falsifying
`routetable.go:8-15`'s "a route cannot be served without also being described".

### IC-027 · P2 · Information Disclosure — Public metrics

`GET /metrics` is registered without authentication (`router.go:34`) and returns security-event
counters, cache hit/miss/stale counts and a trace id — verified live, anonymous, `200`.
`observability.go:72-75` says of the same data: *"Restricted to admins: the failure counts reveal
whether an attack is landing, which is exactly what an attacker wants to know"* — and
`GET /admin/metrics` **is** admin-gated. The Prometheus endpoint should be bound to a private
listener or gated, and it currently exposes only a partial exposition format (no `# HELP`/`# TYPE`
lines, no histograms, and `internal/metrics/metrics.go:58` still carries `TODO: update p99 from
histogram`).

### IC-028 · P2 · Database — Type and scale choices

* **All timestamps are `TEXT` in RFC3339** across 64 tables (e.g. `users.created_at`,
  `sessions.created_at`, `jobs.available_at`). Consequences, verified in code: range queries are
  string comparisons; the job queue must sort by `created_at, id` because *"timestamps in this
  schema carry second precision"* (`jobqueue.go:123-129`); `time.Parse` on every read; no
  `TIMESTAMPTZ` semantics, so no DB-level timezone correctness for the scheduler that the product
  depends on.
* `ListConfessions` has **no LIMIT** (`content.go:243-250`); `adminStats` loads every confession
  into memory to count statuses (`admin.go:353-369`); `GET /admin/confessions` returns the whole
  table.
* `SetSubscription` is a bare `UPDATE` with no existence check and no transaction
  (`users.go:125-129`) — a user with no subscription row is silently not upgraded.
* Migrations use `CREATE INDEX` (not `CONCURRENTLY`) inside a transaction — fine at 78 rows, a
  lock-timeout risk at scale. There is no down-migration by design (documented and defensible) but
  also **no zero-downtime guidance**.
**Fix:** migrate to `TIMESTAMPTZ`; add `COUNT(*) … GROUP BY` for stats; paginate admin lists;
`CONCURRENTLY` outside transactions for future indexes.

### IC-029 · P3 · Supply chain / hygiene

`aws-sdk-go-v2` modules are marked `// indirect` in `go.mod` while `internal/storage/providers.go`
imports them directly — `go mod tidy` was not re-run. GitHub Actions are referenced by mutable tag
(`actions/checkout@v7`, `actions/setup-go@v7`, `golangci/golangci-lint-action@v8`) rather than by
commit SHA. No SBOM, no provenance, no artefact signing. `server/docker-compose.yml` ships
`JWT_SECRET: dev-only-change-me-for-production` and `postgres:16-alpine` while CI tests
`postgres:17` — a version skew between local and CI.

### IC-030 · P3 · Code structure

`internal/api/handlers.go` is **1,524 lines** mixing auth, admin, content and helpers;
`internal/api` has 37 production files and 36 test files in one package (no sub-packages), so
every handler test compiles against every other. `internal/models/models.go` is 816 lines with
**0% test coverage** and no test file. `clients/dart/lib/src/models.dart` is 1,055 lines of
hand-written JSON parsing. Duplicate route registration for `/` and `/v1/` doubles the surface for
exactly the drift IC-004 exhibits.

### IC-031 · P3 · Observability privacy

The access log is clean — `logRequests` emits only method, path, status and duration
(`middleware.go:10-17`); no IP, no user id, no body. But targeted `log.Printf` calls do emit
identity: `session_validator.go:73` logs `user=%s session=%s` on reuse detection;
`handlers.go:658` logs the **recipient email address** when mail delivery is unconfigured;
`deletion.go:220` logs erased account ids; `library.go:229` logs `user=%s device=%s`. There is no
documented log retention, redaction or access-control policy, and `internal/log` defines a
`JSONLogger` with a `UserID` field whose `LogJSON` implementation explicitly does **not** emit
JSON (`log.go:132-137`: "In production, this should be marshaled to JSON … For now, just log the
message"). Sentry/New Relic/Datadog appear in `ARCHITECTURE.md` and `.env.example`; **no SDK is
imported anywhere** and `SENTRY_DSN` is never read by the Go code.

### IC-032 · P4 · Documentation accuracy

`README-COMPLETE.md`'s Quick Start fails at step 2: `cp .env.example .env` (no such file at the
root — it is `server/.env.example`), `docker-compose up -d` (it is `server/docker-compose.yml`),
`go run ./cmd/server` from the repo root (the module is in `server/`), "PostgreSQL 16+ (production)
or **SQLite** (development)" (SQLite was removed; `db.Open` supports only `lib/pq`), and
`PATCH … '{"status":"completed"}'` (the canonical vocabulary is now `COMPLETED`; lowercase is
accepted only via a legacy map). `MASTER-PROMPT-COMPLETION.md` states the audio platform is
"**100% complete**" and "**ready for Phase 1 implementation**" in the same document. The root
`Makefile` is, by contrast, excellent and correct — `README.md` should point at it and
`README-COMPLETE.md` should be deleted or rewritten.

### IC-033 · P5 · Positive observations worth preserving

The phase documents (`docs/00`–`docs/31`) record defects *found and fixed* with reproductions —
e.g. the inverted production gate (`config.go:152-165`), the MFA enrolment that accepted a
client-supplied secret, the `mErr == nil && enrolment.Enabled` database-error-becomes-auth-bypass
bug (`handlers.go:326-344`), the audio gate that asserted the literal `"ready"`
(`audio_access.go:96-105`), and the two-sources-of-truth schema that had drifted to 25 tables
against a live 64 (`migrate.go:16-25`). That is real engineering maturity and it is the reason
this codebase is auditable at all. The gap-numbering discipline (G-1 … G-42) is a genuinely good
practice; the failure is that **status labels ("PASS") are assigned to phases whose user-visible
function does not exist**.

---

## 6. MASTER FINDINGS TABLE

| ID | Sev | Category | Finding | Evidence | Impact | Fix | Blocker |
|---|---|---|---|---|---|---|---|
| IC-001 | P0 | Product/Content | Core loop 500s outside dev: 0 voices, 0 audio | live `POST /sessions` → 500; `GET /voices` → `null`; `ensure.go:25-30` | Product unusable | 422 + inventory gate + runbook | **YES** |
| IC-002 | P0 | Mobile/Audio | No audio engine in the Flutter app | `pubspec.yaml` has no audio pkg; `player_providers.dart:31` | Cannot play audio; metrics forgeable | add just_audio/audio_service | **YES** |
| IC-003 | P0 | Payments | Stub verifier grants premium outside prod; refuses in prod; no webhooks/lifecycle | live free→premium flip on `"valid_monthly"` | Free premium or zero revenue | real store verification | **YES** |
| IC-004 | P0 | Authorization | 8 routes documented `bearerAuth` have `nil` middleware | `router.go:281-285,419-423`; live 202 with no token | Revocation bypass, unauth writes | couple auth level to wrapper | **YES** |
| IC-005 | P1 | Privacy/Analytics | Unauth analytics accepts arbitrary `user_id`, no limit, no sink | `analytics_batch.go:41-55`; live 30×202 | Forged telemetry, abuse path | auth + server-side identity | YES |
| IC-006 | P1 | Anonymity | Public feed serialises `author_id` with confession body | `community/policy.go:52-59`, `store.go:30` | Deanonymization | DTO without author id | YES |
| IC-007 | P1 | Deletion | Erasure misses `security_events` (IP+UA), `audit_logs.actor`, `user_confession_audio`, `jobs`, `outbox_events`; test only matches literal FKs | 38 policies / 64 tables; `deletion_test.go:121` | Retained personal data after GDPR request | schema-introspection policy test | YES |
| IC-008 | P1 | Config/Security | `SEED=1` creates super-admin with repo-published password | `main.go:66`, `seed.go:148-158` | Admin takeover | refuse seed outside dev/test | YES |
| IC-009 | P1 | Web hardening | No CSP/XFO/nosniff/HSTS on admin console or API; token in `localStorage`; no CORS | live headers on `/admin/` | Clickjacking, XSS→admin | global header middleware | YES |
| IC-010 | P1 | Crypto | TOTP secrets stored plaintext | `0001_baseline.sql:598-603` | DB read ⇒ live 2FA | encrypt at rest | YES |
| IC-011 | P1 | Audio/CDN | `MEDIA_BASE_URL` unvalidated; S3 presign ignores CDN; `/media` off in prod | `config.go:107`, `providers.go:164-181`, `main.go:96` | Audio 404 or no CDN | validate + rewrite host | YES |
| IC-012 | P1 | Mobile/Push | Client never registers device/push token | no caller of `postMeDevices` | Reminders never arrive | add messaging plugin | YES |
| IC-013 | P1 | Mobile/Offline | Downloads store licences, never bytes | `downloadUrl` unused | Premium offline undeliverable | implement fetch+store+seal | YES |
| IC-014 | P1 | API contract | `DisallowUnknownFields` on every body | live 400 on `duration_seconds` | Breaks old/new clients | drop strict decoding | YES |
| IC-015 | P1 | Admin | Console cannot complete MFA login; no moderation/queue/rights/deletion UI | `adminui/index.html:163,215` | Admin lockout; UGC unmoderatable | handle `mfa_required`, add views | YES |
| IC-016 | P1 | Legal | No LICENSE, privacy policy, terms or consent UI; KJV licence & review unrecorded | `git ls-files`; G-35 | Legal exposure (GDPR Art. 9) | publish + wire consent | YES |
| IC-017 | P1 | Auth/Abuse | Session issued before verification; `email_verified` gates nothing; IP-only throttle | `handlers.go:274`, `session_validator.go:36` | Account farms, report abuse | gate on verification + challenge | YES |
| IC-018 | P1 | CI/CD | One-language CI; no client tests, no scanning, no build/deploy; PR #40 merged red | `gh pr list`; `ci.yml` | Broken code reaches main | required checks + jobs | YES |
| IC-019 | P1 | Content | 78 unreviewed texts, fabricated author, 0 audio, empty audit trail | `ensure.go:84`; live `/admin/audit` → `[]` | Governance/reputational | review + real authorship | YES |
| IC-020 | P2 | Perf/Abuse | Public unthrottled `/search`: 4 unindexed `LIKE '%%'` scans; wildcards unescaped | `search.go:97-120`; live `q=%` → 20 | Scraping, DB load | escape, limit, index | no |
| IC-021 | P2 | Mobile | `catch (_) {}` on all player mutations; state set before network | `player_providers.dart:87-125` | Silent client/server divergence | await + rollback + surface | no |
| IC-022 | P2 | Contract | PascalCase vs snake_case entitlements; `null` vs `[]`; 500 vs 422 | live responses | Client bugs | one envelope | no |
| IC-023 | P2 | Reliability | `PurgeExpired` never called; no read/write timeouts; no retention; cache not shared | `idempotency.go:78`; `main.go:270` | Unbounded growth, exhaustion, stale content | wire purge, timeouts, Redis cache | no |
| IC-024 | P2 | Config | `share_url` hardcoded to `https://iconfess.app` | `handlers_templates.go:73` | Broken share links | use `PUBLIC_BASE_URL` | no |
| IC-025 | P2 | Product | Streak is a capped session count; `DateTime.parse('')` always throws; dead variable | `activity_providers.dart:18-41` | Fabricated user-facing metric | compute server-side or remove | no |
| IC-026 | P2 | Authz hygiene | Destructive `/admin/users/{id}/erase` absent from route table, spec and the systematic authz sweep | `router.go:230`; `grep -c erase design/routes.json` → 0 | Coverage/doc gap | register via `h.route` | no |
| IC-027 | P2 | Disclosure | `GET /metrics` public, contradicting its own comment | `router.go:34`; live anon 200 | Attack-feedback signal | gate or private listener | no |
| IC-028 | P2 | Database | TEXT timestamps, second precision, unbounded admin lists, non-transactional subscription update | `jobqueue.go:123-129`, `content.go:243`, `users.go:125` | Scale & correctness risk | `TIMESTAMPTZ`, pagination, tx | no |
| IC-029 | P3 | Supply chain | `// indirect` mislabels, tag-pinned actions, no SBOM, compose Postgres 16 vs CI 17 | `go.mod`, `ci.yml`, `docker-compose.yml` | Reproducibility | tidy, pin SHA, align versions | no |
| IC-030 | P3 | Structure | 1,524-line `handlers.go`; single 73-file `internal/api` package; `models.go` 0% covered | file metrics | Maintainability | split packages | no |
| IC-031 | P3 | Observability | Identity in targeted logs; `LogJSON` doesn't emit JSON; Sentry/NewRelic/Datadog documented but never imported | `log.go:132-137`; `rg SENTRY` → config only | No error tracking, log privacy | wire structured logging + APM | no |
| IC-032 | P4 | Docs | `README-COMPLETE.md` Quick Start fails at step 2; "100% complete" claims | `README-COMPLETE.md:38-48` | New-engineer friction | rewrite/delete | no |
| IC-033 | P5 | Positive | Defect-driven phase docs, gap numbering, arch tests, contrast/IA enforcement | `docs/*`, `arch_test.go` | Auditable codebase | preserve | no |

---

## 7. SYSTEM SCORECARD

| Domain | Status | P0 | P1 | Notes |
|---|---|---:|---:|---|
| Architecture | **Sound** | 0 | 0 | Modular monolith with mechanically enforced layering (`arch_test.go`); clear dependency direction; no circular imports detected. One 1,524-line handler file and a 73-file flat package are the debt. |
| Backend | **Partial** | 1 | 2 | Broad, real, well-tested in places. IC-001 (500 on the primary write path), IC-023, IC-028. |
| Mobile | **FAIL** | 1 | 3 | Screens/state/theming real; audio (IC-002), push (IC-012), offline (IC-013) absent; 15 test files never executed by anyone. |
| Web | **Partial** | 0 | 1 | Embedded vanilla SPA works and escapes output; `apps/web` dead; no CORS; IC-009 headers. |
| Database | **Sound with debt** | 0 | 1 | 64 tables, checksummed immutable migrations, 83 indexes, 76 FKs, CHECK constraints on 23 status columns. TEXT timestamps and unbounded lists (IC-028). |
| API | **Partial** | 1 | 1 | 286 routes, dual prefixes, generated spec. IC-004, IC-014, IC-022, IC-026. |
| Authentication | **Strong** | 0 | 2 | bcrypt, session-bound JWTs, rotation + reuse detection + family revocation, TOTP MFA with replay defence, hashed one-time tokens, NIST-aligned password policy. IC-010, IC-017. |
| Authorization | **Partial** | 1 | 0 | Server-side session validation on authenticated routes; role re-read on refresh; IDOR closed (live: 403 on cross-user session read/patch/delete/complete). IC-004 is the exception. |
| Anonymity | **FAIL** | 0 | 2 | IC-006 (`author_id` beside confession text), IC-007 (IP/UA survive erasure). No anonymity mode exists: an account is always email-keyed. |
| Privacy | **Partial** | 0 | 3 | Excellent deletion *design* (policy, tombstone, retention reasons, grace period, revocation-on-request). IC-005, IC-007, IC-016. |
| Security | **Partial** | 1 | 3 | Strong crypto and auth engineering; weak web hardening and config guards. IC-003, IC-008, IC-009, IC-010. |
| Audio | **FAIL** | 1 | 1 | Real duration measurement, rights gate, variant model, HMAC signing verified end to end — and **zero audio content**, no CDN path, no client playback. IC-001, IC-002, IC-011. |
| Session Engine | **Strong** | 0 | 0 | 11 states, explicit transition table, terminal states, legacy mapping, COMPLETED unreachable without playback (verified live: 409 from READY, 409 after terminal). Best-engineered subsystem in the repo. |
| Offline | **FAIL** | 0 | 1 | Server licences real and entitlement-gated (live 402 for free); client stores nothing. IC-013. |
| Scheduling | **Partial** | 0 | 1 | Server-side occurrence computation with DST-safe timezone handling, uniqueness-guaranteed dispatch, one-minute sweep. No client to deliver to. IC-012. |
| Moderation | **Partial** | 0 | 1 | Real lifecycle, dedupe, throttling, decidable outcomes, rejection-reason requirement (verified live: create → submit → report → queue). No operator UI; audit trail empty. IC-015, IC-019. |
| Payments | **FAIL** | 1 | 0 | IC-003. |
| Subscriptions | **FAIL** | 1 | 0 | No provider, no expiry enforcement, no lifecycle states in use (`trial`/`expired`/`cancelled` are constrained in the schema but never written). IC-003. |
| Infrastructure | **Weak** | 0 | 1 | Dockerfile runs as root (would fail `runAsNonRoot`); K8s manifest has `latest` tag, inline placeholders, no Ingress/TLS, and a NetworkPolicy that blocks every external provider. IC-018. |
| CI/CD | **FAIL** | 0 | 1 | Backend-only, no scanning, no artefacts, no deploy, gate demonstrably not enforced. IC-018. |
| Testing | **Partial** | 0 | 1 | 101 test files / 23,701 lines; 50.6% statements; genuinely adversarial backend tests (IDOR, reuse, entitlement, parity, arch, schema coverage). Zero client test execution; `billing` 20.9%, `store` 26.6%, `models` 0%. IC-018. |
| Performance | **Partial** | 0 | 0 | Sub-millisecond to ~3 ms for most reads (measured); SWR content cache; pooled connections; `FOR UPDATE SKIP LOCKED` queue. IC-020 and IC-023 are the risks. |
| Accessibility | **Partial** | 0 | 0 | Tokens contrast-verified (30/30 AA), `Semantics` labels on nav, 44pt+ targets, serif/sans role separation, `aria-live`/`role="slider"` in the webapp. Not runtime-verified with a screen reader. |
| UX | **Partial** | 0 | 0 | Deliberate, opinionated IA (5 tabs, no FAB, enforced by test). Undercut by IC-025 (fake streak) and a no-op Subscribe button. |
| Content | **FAIL** | 0 | 1 | 39 categories, 78 confessions, well-structured scripture references; no review, fabricated author, no audio, no translation licence. IC-019. |
| Documentation | **Mixed** | 0 | 1 | 14,757 lines. Phase docs are exceptional; top-level readiness claims are false and the Quick Start does not run. IC-016, IC-032. |
| AI-Generated Quality | **Good** | 0 | 0 | See §17. Deliberate engineering dominates; a small number of generated artifacts (IC-025, dead policy layer, TODO-in-`/tmp` comment). |
| **Production Readiness** | **FAIL** | **4** | **19** | **NO-GO** |

---

## 8. ARCHITECTURE RECONSTRUCTION (as built, not as documented)

```
CLIENTS
  Flutter app (apps/mobile) ──► clients/dart ApiClient (dart:io HttpClient)
        │  bearer token in Keychain/Keystore via flutter_secure_storage
        │  baseUrl = String.fromEnvironment(...) ?? https://api.i-confess.app
        │  NO audio engine · NO push registration · NO offline byte store
  Embedded listener SPA (internal/webapp/index.html)  ─┐
  Embedded admin console (internal/adminui/index.html) ─┤ same origin, no CORS
  Retired Next.js stubs (apps/web, apps/admin) ─────────┘  (cannot build; would be cross-origin)
        │
        ▼   HTTPS terminated OUTSIDE the repo (nothing here terminates TLS)
EDGE / CDN
  ✗ NOT IMPLEMENTED — ARCHITECTURE.md describes Cloudflare/Fastly/CloudFront;
    no config exists and S3 presigning bypasses any CDN (IC-011)
        │
        ▼
API — single Go binary, net/http ServeMux (Go 1.22 patterns), :8080
  RequestIDMiddleware → tracing.Middleware → logRequests → mux
  286 routes; every route duplicated under /v1/; 3 registered outside the recorder
  auth.MiddlewareWithSessions (JWT signature + DB session validation, fail-closed)
  auth.RequireRoleWithSessions(..., roles) for admin/voice_manager/audio_producer
  ratelimit.MiddlewareFor on register/login/social/email/avatar/deletion only
        │
        ├─► APPLICATION SERVICES
        │     engine (session composition) · sessions (state machine) · entitlements
        │     rights/voice_rights (451 gate) · moderation · content lifecycle
        │     deletion (policy-driven erasure) · scheduler (occurrence + dispatch)
        │     ai (KeywordParser; LLM adapter is a TODO) · billing (STUB)
        │     audio (duration measurement, QA lifecycle) · images (EXIF-stripping avatar)
        │
        ├─► DATA ACCESS (internal/store, 17 files) ──► PostgreSQL 17 via lib/pq
        │     '?' rebound to '$N' (internal/db/rebind.go); pool 25/5
        │     64 tables · 83 indexes · 76 FKs · checksummed forward-only migrations
        │
        ├─► CACHE  internal/cache — per-process SWR (5m/2m/10m), NO cross-instance invalidation
        │
        ├─► RATE LIMIT  in-memory fixed window, or Redis via a hand-written RESP client
        │               (degrades to per-instance when Redis is down — deliberate)
        │
        ├─► QUEUE  store.JobQueue (PostgreSQL, FOR UPDATE SKIP LOCKED, idempotency_key unique,
        │          exponential backoff 30s→30m, dead-letter, requeue endpoint)
        │          └─► workers: audio.generate · notification.send
        │              (jobs.Worker pool, 4–16 goroutines, in the API process)
        │
        └─► EXTERNAL PROVIDERS
              S3 / S3-compatible (aws-sdk-go-v2)      — implemented, presigned
              LocalStorage (dev, HMAC-signed /media)  — implemented
              GCS, Azure                              — refused at boot (deliberate)
              ElevenLabs                              — implemented, unconfigured ⇒ 503
              Postmark                                — implemented; LogSender refused in prod
              APNs (token auth, JWT via crypto)       — implemented, no client tokens
              FCM (own service-account OAuth2)        — implemented, no client tokens
              Google/Apple OIDC (JWKS, RS256)         — implemented, disabled without client ids
              Sentry / New Relic / Datadog / Segment / Mixpanel / Amplitude / BigQuery /
              Stripe / PayPal / Twilio / SendGrid / SNS / Cloudinary / Backblaze
                                                      — NAMED IN ARCHITECTURE.md, NOT IMPORTED ANYWHERE
```

**Trust boundaries:** client→API (JWT), API→PostgreSQL (single DSN, no read replica),
API→Redis (optional), API→object storage, API→external providers. Workers run **inside** the API
process, so there is no independent worker failure boundary — a TTS burst competes with request
serving for the same CPU and connection pool.

**Single points of failure:** PostgreSQL (hard dependency; readiness fails without it, correctly);
the in-process scheduler and deletion sweeper (both single-instance-correct, both documented as
needing a lock at >1 replica — `main.go:247-250` says so explicitly); the per-process cache.

**Compared with the intended architecture:** the intended diagram in `ARCHITECTURE.md` names
Segment, Mixpanel, Amplitude, BigQuery, Datadog, New Relic, Sentry, Stripe, PayPal, Twilio,
SendGrid, SNS, Cloudflare/Fastly/CloudFront, Google/AWS/Azure TTS, multi-region Postgres and Redis
Sentinel. **None of these are imported, configured or referenced in code.** The document describes
an aspiration; the binary is a self-contained monolith with S3, Postmark, ElevenLabs, APNs, FCM and
two OIDC providers. That gap should be closed by editing the document, not by pretending the
integrations exist.

---

## 9. CROSS-SYSTEM FAILURE MAP

```
MOBILE (no audio engine, no push token, no offline bytes)
  │  ① local state mutated BEFORE the network call; failures swallowed (IC-021)
  ▼
API (286 routes, dual prefix)
  │  ② 8 routes skip auth middleware entirely (IC-004)
  │  ③ idempotency is opt-in by header; retries without it duplicate side effects
  ▼
AUTH (JWT + DB session validation, fail-closed)
  │  ④ session validation runs on `authed` routes only — the 8 above bypass revocation
  ▼
SESSION ENGINE (state machine — the strongest link)
  │  ⑤ client can reach ACTIVE/COMPLETED without audio ever playing (IC-002)
  ▼
DATABASE (PostgreSQL, TEXT timestamps, second precision)
  │  ⑥ SetSubscription is a non-transactional UPDATE with no existence check
  │  ⑦ no retention on jobs/security_events/scheduled_deliveries/idempotency_keys (IC-023)
  ▼
QUEUE (PostgreSQL, SKIP LOCKED, in-process workers)
  │  ⑧ workers share the API process: a TTS burst starves request latency
  │  ⑨ a job stuck in `running` after a crash is only recoverable by the requeue endpoint
  ▼
AUDIO (rights gate, duration measurement, signing)
  │  ⑩ zero assets outside dev (IC-001); no CDN in the delivery path (IC-011)
  ▼
CDN — DOES NOT EXIST IN THIS REPOSITORY
```

**Every point where inconsistent state can occur:** ①③⑤⑥⑦⑧⑨⑩. The highest-consequence pair is
①+⑤: the mobile player declares progress and completion from local state, so the server's
"completion requires playback" invariant is satisfied by a client that plays nothing — the metric
is structurally forgeable even though the state machine is correct.

---

## 10. FAILURE MATRIX

| Component | Failure | User impact | Detection | Recovery | Data-loss risk |
|---|---|---|---|---|---|
| **API** | Pod crash / OOM | Requests fail; in-flight session state transitions lost | K8s liveness/readiness (present, correct split) | Restart; graceful shutdown implemented (10s, `main.go:280-289`) | Low — queue is durable; **but** in-process workers drop running jobs to `running` until requeued |
| **Database** | PostgreSQL unavailable | Total outage; auth fails **closed** (`session_validator.go:33-36`) | `/health/ready` → 503 with `reason: database`; readiness pulls the pod | Pool reconnects; no replica, no failover configured | **High** — no backup, no PITR, no restore procedure anywhere in the repo (IC-034 below) |
| **Redis** | Unreachable | Rate limits degrade to per-instance (N× limit with N replicas) — deliberate and documented | Boot log line; `Distributed.Degraded()` exists but **is never surfaced as a metric** | Automatic fallback | None |
| **Worker** | Crash mid-job | Audio generation stalls; job stays `running` | `GET /admin/queue` counts by status (admin-only) | `POST /admin/queue/requeue` for dead-lettered; **no stuck-in-running reaper** | Low — durable queue |
| **Audio provider** (ElevenLabs) | Timeout/5xx/quota | Generation job retries 30s→30m, then dead-letters | Job `last_error`; queue view | Retry/backoff/dead-letter implemented; response bodies capped at 2 KiB | None |
| **Storage** (S3) | Unavailable | Signed-URL minting fails → `audio_url` blanked, item unplayable (graceful) | Logged per item (`audio_access.go:117`) | Automatic on restore | **High if local provider used in prod** — refused at boot, correctly |
| **CDN** | — | Not applicable: no CDN is wired (IC-011) | — | — | — |
| **Payments** | Provider unreachable | Cannot happen — no provider integration exists (IC-003) | — | — | **Revenue integrity: total** |
| **Notifications** | APNs/FCM failure | Reminder lost; `scheduled_deliveries` marked `failed`; dead tokens cleared after provider rejection | Sweep log `sent=/failed=/skipped=` | Next occurrence; **no retry of a failed occurrence** | Low |
| **Email** | Postmark failure | Verification/reset mail lost | Queue is async so registration still succeeds (correct) | Queue has no durable retry — it is in-process (`email.NewQueue(…, 512)`), so a restart drops pending mail | Medium — a user can be locked out of reset until they retry |

### IC-034 · P1 · Disaster Recovery — absent

There is **no** backup configuration, no `pg_dump`/`pg_basebackup`/WAL archiving, no restore
runbook, no RPO/RTO statement, no backup encryption or retention policy, and no restore test
anywhere in the repository (verified: no file matches `backup|restore|disaster|RPO|RTO` outside
`internal/offline/license.go`, which is unrelated). The audit brief's rule applies directly: *a
backup that has never been restored is not proven recoverable* — here there is not even a backup.
For a system holding special-category religious data, this is a P1 blocker. **Fix:** managed
PostgreSQL with PITR + daily logical dumps to a separate account, encrypted, 35-day retention, a
documented restore runbook, and a quarterly restore drill whose output is committed.

---

## 11. SECURITY THREAT MODEL

| Asset | Threat | Attack path | Existing control | Residual risk |
|---|---|---|---|---|
| **Anonymous identity / confession authorship** | Deanonymization | `GET /community/feed` → `author_id` + body; correlate with `/admin/moderation/queue` `user_id`; `security_events` IP+UA surviving erasure | Feed currently unreachable (creation 500s); export excludes tokens; deletion tombstones email | **HIGH** — IC-006, IC-007 |
| **Confession content (UGC)** | Unauthorized read | IDOR on `/confessions/{id}`, `/me/confessions`, search | Verified live: private UGC → 404 for other users and anonymous; not in search; `is_private=1` default; `visibility='private'` default | **LOW** |
| **Private sessions** | Cross-user access | Read/patch/delete/complete another user's session | Verified live: all → 403 `not your session`; anonymous → 401 | **LOW** |
| **Audio assets** | Private-audio download / hotlinking | Guess or replay a storage key | HMAC-SHA256 signature + expiry, constant-time compare, key allowlist (`audio/`, `avatars/`), traversal/NUL/`..` rejection; verified 403/410 live; no directory listing | **LOW** in dev. **MEDIUM** in prod (IC-011: path undefined, and a presigned S3 URL is bearer-accessible for its TTL by anyone who receives it) |
| **Subscriptions / entitlements** | Free premium | `POST /subscriptions/verify` with `"valid_monthly"` | `prodBlocker` when `ENV=production` (exactly) | **CRITICAL** outside production (IC-003); entitlement is server-resolved everywhere else (verified: free → 402 on downloads) |
| **Payment data** | Forgery / webhook replay | — | No payment data is stored at all | **N/A today**; **CRITICAL** once IC-003 is implemented without replay protection |
| **Admin credentials** | Takeover | `SEED=1` → known super-admin password; clickjack `/admin/`; XSS → `localStorage` token; MFA lockout | Role re-read on every request and on refresh; 44 admin routes reject non-admins (verified live 403); `RequireRoleWithSessions` validates the DB session | **HIGH** — IC-008, IC-009, IC-015 |
| **Voice-provider credentials** | Leak → unlicensed synthesis | Env var / process inspection | Server-side only, never serialised; rights gate returns 451 without an attestation; `ELEVENLABS_API_KEY` absent ⇒ 503 | **LOW** |
| **Database** | Exfiltration | SQL injection; leaked backup | Parameterised queries throughout (verified: `' OR 1=1--` → 0 results, 200); `?`→`$N` rebinding is comment-aware and tested; **but** MFA secrets are plaintext and `security_events` holds IPs | **MEDIUM** — IC-010, IC-007, IC-034 |
| **User activity / analytics** | Poisoning | Unauthenticated `POST /analytics/batch` with arbitrary `user_id` | 3-key PII strip list; 100-event cap | **HIGH** — IC-005 |
| **Session tokens** | Replay after logout/revocation | Present a rotated or revoked token | Signature + DB session check + family revocation on reuse + security email | **LOW** on `authed` routes; **HIGH** on the 8 unguarded routes (IC-004) |
| **Availability** | DoS | Unthrottled public `/search` with wildcard scans; unthrottled `/analytics/batch`; no `ReadTimeout`/`WriteTimeout`; unbounded admin lists | Rate limits on auth/email/avatar/deletion only; 1 MiB body cap; `ReadHeaderTimeout` 10s | **MEDIUM-HIGH** — IC-020, IC-023 |
| **JWT** | Algorithm confusion / forgery | `alg=none`, RS256 substitution, weak secret | Keyfunc rejects non-HMAC; boot refuses the published default secret outside dev/test; TTL capped at 24h and validated | **LOW** |

### Privacy threat actors

| Actor | Capability | Best available path | Blocked by |
|---|---|---|---|
| Anonymous attacker | Unauthenticated HTTP | `/community/feed` author ids; `/search` wildcard enumeration; `/metrics` counters; `/openapi.json` full surface map | Partially — IC-006, IC-020, IC-027 open |
| Authenticated attacker | Valid low-privilege session | Forge analytics for other users; unauthenticated-post attempts; enumerate via search | Weakly — IC-005 |
| Malicious user (spam) | Many accounts | Register 5/10min/IP with disposable email; flood reports (8/10min); flood moderation queue | IP throttle only — IC-017 |
| Scraper | Automated | `GET /search?q=%` repeatedly; walk 39 categories → 78 confessions (all public by design) | Nothing — IC-020 |
| Compromised admin | Super-admin token | Read all UGC text via moderation queue; erase any user; grant roles; change voice rights | Audit trail exists but is **empty for these actions** (IC-019/G-41); no MFA on the console path (IC-015) |
| Compromised employee (DB access) | Direct SQL | Read MFA secrets in plaintext, IPs, user agents, all confession text | Nothing — IC-010 |
| Third-party provider compromise | Postmark/ElevenLabs/APNs/FCM | Email content includes one-time links (mitigated: `EMAIL_PROVIDER=log` refused in prod); push payloads contain only a label and duration | Good payload minimisation |
| Database attacker (backup theft) | Offline copy | Plaintext TOTP secrets; IP/UA history; full UGC | Passwords and one-time tokens are hashed; **MFA secrets are not** |
| Insider (log access) | Log stream | Recipient email addresses on the unconfigured-mail path; user ids on reuse detection; erased account ids | Access log itself is clean (no IP/UA/body) — IC-031 |

---

## 12. TOP LAUNCH BLOCKERS

**24 verified launch blockers were found.** Ranked by severity × exploitability × blast radius ×
likelihood × production impact. (Fewer than 25 genuine blockers exist; the list is not padded.)

| # | ID | Blocker | Why it ranks here |
|---|---|---|---|
| 1 | IC-001 | `POST /sessions` → 500 outside dev: no voices, no audio | 100% of users, 100% of the time, primary write path, undetectable by health checks |
| 2 | IC-002 | Mobile app cannot play audio | The product's only output; primary client; also makes completion metrics forgeable |
| 3 | IC-003 | No real purchase verification; stub grants premium in dev/test/**staging** | Revenue integrity and analytics integrity; two mutually exclusive failures |
| 4 | IC-004 | 8 routes documented as authenticated are unauthenticated | Broken access control (OWASP A01) + session-revocation bypass + contract lie |
| 5 | IC-008 | `SEED=1` creates a super-admin with a repo-published password | Total admin takeover from one env var on a fresh DB |
| 6 | IC-009 | No CSP/XFO/nosniff/HSTS; admin token in `localStorage` | Clickjacking + XSS → full admin takeover |
| 7 | IC-007 | Erasure leaves `security_events` (IP+UA), `audit_logs.actor`, `user_confession_audio` | Legal non-compliance on a special-category-data product; the covering test cannot see it |
| 8 | IC-006 | Public feed returns `author_id` with confession text | Deanonymization — the product's core privacy promise |
| 9 | IC-034 | No backups, no restore procedure, no RPO/RTO | Unrecoverable data loss; a religious-confession database |
| 10 | IC-011 | Production audio delivery path undefined (no CDN, `MEDIA_BASE_URL` unvalidated, `/media` off) | Audio 404s or uncontrolled S3 egress on day one |
| 11 | IC-016 | No licence, privacy policy, terms or consent capture | Cannot lawfully launch |
| 12 | IC-019 | 78 unreviewed texts, fabricated author, empty audit trail | Content governance for a faith product |
| 13 | IC-012 | Mobile never registers a push token | The habit loop — scheduled reminders — cannot close |
| 14 | IC-013 | Offline downloads store licences, never bytes | Premium entitlement sold but not delivered |
| 15 | IC-015 | Admin console cannot complete MFA login; no moderation UI | Admins lock themselves out; UGC unmoderatable in practice |
| 16 | IC-010 | TOTP secrets stored plaintext | DB read silently downgrades every enrolled 2FA |
| 17 | IC-018 | CI is backend-only, no scanning, no artefacts, gate not enforced (PR #40 merged red) | Broken code reaches `main`; clients are never verified |
| 18 | IC-017 | Session issued before verification; IP-only signup throttle | Account farms, report/queue flooding at launch |
| 19 | IC-014 | `DisallowUnknownFields` on every body | Guaranteed client/server incompatibility on the next release |
| 20 | IC-005 | Unauth, unthrottled analytics with attacker-chosen `user_id` | Telemetry forgery + free abuse path |
| 21 | IC-023 | `PurgeExpired` never called; no read/write timeouts; no retention; per-process cache | Unbounded table growth, connection exhaustion, stale retracted content across replicas |
| 22 | IC-020 | Public unthrottled `/search` with unescaped `LIKE` wildcards and 4 unindexed scans | Cheapest scraping/DoS vector in the system |
| 23 | IC-022 | Inconsistent response shapes (PascalCase vs snake_case, `null` vs `[]`, 500 vs 422) | Predictable client bugs across three consumers |
| 24 | IC-025 | Fabricated streak metric with dead code | User-facing false data; erodes the one thing a ritual product needs — trust |

---

## 13. TOP ENGINEERING IMPROVEMENTS

### Must fix (before any external user)

1. Couple route **auth level** to the installed middleware in `h.route`; add a sweep test asserting
   every non-public route rejects an unauthenticated and a revoked-token caller (IC-004).
2. Map `engine.ErrNoVoice`/`ErrNoContent` to 422 with stable codes; add a boot inventory assertion
   and surface it in `/health/ready` (IC-001).
3. Ship real audio playback + background audio + interruption handling on mobile; drive server
   state from player events (IC-002).
4. Implement Apple/Google verification, signed webhooks with idempotency, and an entitlement record
   with provider/transaction/expiry (IC-003).
5. Refuse `Seed` outside dev/test; move demo accounts to an explicit operator command (IC-008).
6. Add a global security-header middleware; move admin auth off `localStorage`; replace inline
   `onclick` so a real CSP is possible (IC-009).
7. Extend the deletion policy to every identity-bearing column and replace the string-match
   coverage test with schema introspection + an end-to-end erasure assertion (IC-007).
8. Remove `author_id` from the public feed DTO; make `Store.Feed` honour `shared` **and** `public`;
   delete or wire the dead policy functions (IC-006).
9. Stand up backups + PITR + a tested restore runbook with committed evidence (IC-034).
10. Validate `MEDIA_BASE_URL` and rewrite presigned hosts to the CDN domain; add an end-to-end
    audio-fetch smoke test (IC-011).
11. Encrypt `mfa_secrets.secret` with an envelope key (IC-010).
12. Add a `flutter analyze && flutter test` + `dart test` CI job and make the check required
    (IC-018).
13. Drop `DisallowUnknownFields`; name the offending field in 400s (IC-014).
14. Gate `/metrics`; require auth on analytics and take `user_id` only from the session (IC-005,
    IC-027).
15. Add push registration + notification permissions + deep-link handling on mobile (IC-012).

### Should fix (before or immediately after launch)

16. Implement offline download: fetch bytes, store under app-private storage, `Seal` metadata with
    a device key, verify licence expiry before playback, handle interrupted downloads and storage
    exhaustion (IC-013).
17. Add moderation/queue/reports/audio-QA/rights/deletion views and `mfa_required` handling to the
    admin console; put the console behind SSO and a network boundary (IC-015).
18. Escape `LIKE` metacharacters, cap query length, rate-limit `/search`, and add a `tsvector` GIN
    index (IC-020).
19. Wire `IdempotencyStore.PurgeExpired` into the existing ticker; add `ReadTimeout`,
    `WriteTimeout`, `IdleTimeout`; add retention jobs for `jobs`, `scheduled_deliveries`,
    `security_events`, `idempotency_keys` (IC-023).
20. Move the content cache to Redis or add pub/sub invalidation so retraction is consistent across
    replicas (IC-023).
21. One response envelope, one casing convention, `[]` instead of `null`, single error-mapping
    function for engine sentinels (IC-022).
22. Compute streaks server-side from completion dates, or remove the card (IC-025).
23. Use `PUBLIC_BASE_URL` for `share_url` (IC-024).
24. Register `/admin/users/{id}/erase`, `/metrics`, `/openapi.json`, `/media/` through `h.route` so
    the spec and the authz sweep are complete (IC-026).
25. Migrate timestamps to `TIMESTAMPTZ`; paginate admin lists; make `SetSubscription`
    transactional with an existence check (IC-028).
26. Write `audit_logs` entries for content, moderation, user-management and deletion actions;
    surface them in the console (IC-019/G-41).
27. Replace `catch (_) {}` in the player with rollback + surfaced errors; send the real
    `queue_item_id` (IC-021).
28. Add a stuck-job reaper for `status='running'` past a lease deadline (IC-023/Failure matrix).
29. Add CAPTCHA/device attestation on register and login; gate capabilities on `email_verified`
    (IC-017).
30. Fix the Dockerfile `USER`, image tag, NetworkPolicy egress, and add an Ingress + TLS + migration
    Job to the K8s manifest (IC-018).

### Future improvement

31. Split `internal/api` into `api/auth`, `api/content`, `api/sessions`, `api/admin`; break
    `handlers.go` (1,524 lines) into per-domain files (IC-030).
32. Adopt `pgx`; reconsider `lib/pq` maintenance status; run `go mod tidy` to fix `// indirect`
    mislabels; pin actions by SHA; publish an SBOM (IC-029).
33. Wire structured JSON logging with request-id/trace-id correlation and a real sink; add Sentry
    or equivalent with **PII scrubbing for confession text** before it leaves the process (IC-031).
34. Move workers to a separate deployment so TTS bursts do not compete with request serving.
35. Replace the 24-entry password blocklist with a k-anonymity breach-corpus check (G-33).
36. Decide and document the anonymous-confession product question: today an account is always
    email-keyed, so "anonymous" means *pseudonymous to other users*, not *unknown to the operator*.
    If true anonymity is a product promise, it needs a separate design (no email, no IP retention,
    no device ids, per-post unlinkability) — none of which exists.
37. Delete `apps/web` and `apps/admin`, or finish them; delete `README-COMPLETE.md` and
    `MASTER-PROMPT-COMPLETION.md`, or rewrite them to match reality (IC-032).
38. Add an `.gitignore` fix for the bare `.md` line before it silently swallows a new document.
39. Add contract tests generated from `contracts/openapi.json` and run them against a live server
    in CI, so the spec becomes a checked artifact rather than an output.
40. Add a chaos suite: kill PostgreSQL mid-request, kill Redis, kill a worker mid-job, expire the
    ElevenLabs key — and assert the documented degradation for each.

---

## 14. 1K-DAU VERDICT

**Analytical model, not a measurement.** No load generator was obtainable in this sandbox, and a
single-node run would measure the sandbox rather than the system. The inputs below are measured
per-request latencies from the live server and the actual schema.

### Assumptions

* 1,000 DAU; 60% open a session daily → 600 sessions/day; 2 sessions/day for 10% → +120.
  **≈ 720 sessions/day.**
* Per session: 1 create, 1 start, ~12 progress syncs (one per 30s of a 6-min item × 2 items),
  1 complete, 2 GETs (session re-read for fresh signed URLs) → **≈ 17 API calls/session**.
* Plus cold start: `GET /me/bootstrap`, `/categories`, `/voices`, `/home` → ~6 calls/user/day.
* Audio: 720 sessions × 600s = **120 audio-hours/day** = 7,200 audio-minutes.
* Peak factor 8× on a 6-hour evening window → ~40% of daily traffic in 2 hours.

### Traffic model

| Quantity | Per day | Peak (2h, 40%) | Peak req/s |
|---|---|---|---|
| API requests (sessions) | 720 × 17 = 12,240 | 4,896 | **0.68** |
| API requests (cold start + misc) | 1,000 × 8 = 8,000 | 3,200 | **0.44** |
| Analytics batches | ~1,000 | 400 | 0.06 |
| **Total API** | ~21,000 | ~8,500 | **≈ 1.2 req/s** |
| Audio segment fetches | 720 sessions × ~20 range requests | — | bursty |
| Audio egress @ 128 kbps AAC | 120 h × 57.6 MB/h = **6.9 GB/day** | — | **≈ 208 GB/month** |
| Job volume | audio generation ~0 at steady state (assets are pre-generated); notifications 720/day | — | 0.01/s |

### Measured latencies (this hardware, PostgreSQL 17.10, single node, no load)

| Operation | Observed |
|---|---|
| `GET /healthz` | 13.7 µs |
| `GET /categories` (cached SWR) | 87–277 µs |
| `GET /categories/{id}/confessions` | 29–675 µs |
| `GET /confessions/{id}` | 1.55 ms |
| `POST /sessions` (engine build + persist + sign) | ~2.4 ms (preview) |
| `POST /me/confessions` | 2.1 ms |
| `POST /reports` | 3.6 ms |
| `POST /auth/register` (bcrypt cost 10) | **73–75 ms** |
| `POST /auth/login` (bcrypt) | **70–73 ms** |
| `GET /openapi.json` | 27–29 ms |
| `GET /search` | 1.9–2.3 ms (78 rows; will not hold at 100k) |

### Verdict

> **Yes — 1,000 DAU is comfortably within reach of this architecture on a single 2-vCPU / 2 GiB
> pod plus managed PostgreSQL, *provided IC-001, IC-002 and IC-011 are fixed first.*** At ~1.2
> peak req/s and p95 well under 10 ms for reads, the API is not the constraint. The K8s manifest's
> 3-replica minimum with an HPA to 20 is **grossly over-provisioned** for 1K DAU and would cost more
> than the database.

### The first bottleneck, in order

1. **bcrypt on login/registration (70–75 ms of CPU each).** At 1K DAU this is trivial (~100 logins
   at peak). At 10K DAU with a morning ritual spike — *the product is explicitly a scheduled
   6-a.m. habit* — a large fraction of DAU authenticates within the same 15 minutes. 1,000 logins in
   900s = 1.1/s × 75 ms = **8% of one core just hashing**, and the rate limiter allows only
   20/min/IP, so NAT'd users (universities, churches, mobile carriers) will collide. **This is the
   first real bottleneck and it is a product-shaped one: synchronised morning traffic.**
2. **`/search` (IC-020):** four unindexed `LIKE '%%'` scans per request, public and unthrottled. At
   78 confessions it is 2 ms; at 100k it is a table scan per keystroke if the client searches as
   you type.
3. **Per-process cache with no invalidation:** at 3+ replicas, publish/retract latency becomes
   replica-count-dependent.
4. **PostgreSQL connection pool:** 25 max open per pod × N pods against a managed instance whose
   `max_connections` is typically 100–200. At 3 pods this is fine; at the HPA's 20 pods it
   **exhausts the database**. No `pgbouncer`/transaction pooling exists.
5. **Audio egress without a CDN (IC-011):** 208 GB/month direct from S3 at ~$0.09/GB ≈ **$19/month**
   at 1K DAU — cheap, but it grows linearly and has no edge caching, so tail latency for
   range-request seeking is S3's, not a POP's.
6. **In-process workers:** a nightly regeneration batch competes with request serving for the same
   CPU and the same 25-connection pool.

---

## 15. SCALE ROADMAP

| | **1K DAU** | **10K DAU** | **100K DAU** | **1M DAU** |
|---|---|---|---|---|
| **Database** | Single managed PostgreSQL 17, 2 vCPU/4 GiB, PITR on. Migrate `TEXT`→`TIMESTAMPTZ` first (IC-028) — cheaper now than at 100M rows | Add a read replica for `/search`, admin lists and analytics reads; `pgbouncer` in transaction mode; partition `session_progress`, `playback_history`, `scheduled_deliveries`, `security_events` by month | Partition `jobs`, `audio_events`, `idempotency_keys`; add `tsvector` GIN + trigram indexes; consider Citus or shard `sessions`/`session_items` by `user_id` hash; archive >90-day telemetry to object storage | Shard by user; move analytics/telemetry out of PostgreSQL entirely (ClickHouse/BigQuery); logical replication per region; read-your-writes routing |
| **Caching** | Per-process SWR cache is fine (1–3 pods). Add Redis for rate limiting (already required in prod) | Move content cache to Redis with pub/sub invalidation so publish/retract is consistent (IC-023); cache entitlements per user for ≤60s | Edge-cache `/categories`, `/voices`, `/confessions/{id}` at the CDN with `stale-while-revalidate`; cache bootstrap payloads per user | Full CDN for all public reads; per-user edge auth via signed cookies; entitlement cache with explicit invalidation on purchase |
| **Workers** | In-process pool of 4 is adequate | **Split workers into a separate deployment** — this is the first architectural change actually justified by evidence; audio generation must not compete with request serving | Dedicated worker fleet with per-queue concurrency limits, a stuck-job reaper, and DLQ alerting; move email/push to their own queues | Regional worker fleets; provider-side batch APIs; cost-aware scheduling (TTS is the dominant variable cost) |
| **CDN** | Fix the delivery path (IC-011): CloudFront or Cloudflare R2 with signed URLs, `Range` support verified, 1-year immutable caching on audio keys | Add image/preview variants; enable HTTP/3; hotlink protection via signed cookies | Multi-POP with regional origin shields; per-territory rights enforcement at the edge (voice_rights has a `territories` column that nothing currently enforces at delivery) | Global multi-region origins, edge auth, per-market audio variants |
| **Storage** | S3 with lifecycle: `audio/` immutable + IA after 90d; `avatars/` standard | Versioning + object-lock for masters; separate bucket for user-generated audio with its own encryption key | Per-region replication for masters; tiering policy driven by play counts | Multi-region active-active for delivery, single-writer for masters |
| **Observability** | Structured JSON logs (IC-031), `/metrics` gated, one dashboard, alert on 5xx and login-failure spike | Add tracing with a real OTel SDK (`internal/tracing` is explicitly a stub), RED metrics per route, SLOs, on-call rotation | Per-tenant/per-route cost attribution; audio QoE pipeline (`audio_qoe_metrics` table exists and is unused); anomaly detection on auth counters | Federated dashboards, capacity forecasting, chaos programme, error budgets |
| **Rate limiting** | Redis-backed distributed limiter (implemented, needs Redis); add limits to `/search`, `/analytics/batch`, community writes | Per-account + per-device-fingerprint limits, not just per-IP (IC-017); sliding window instead of fixed | Adaptive throttling; challenge tier (CAPTCHA/attestation) before hard blocks; abuse scoring | Edge-level bot management; WAF rules; per-market abuse policies |
| **Infrastructure** | 1–2 pods (the manifest's `minReplicas: 3` is over-provisioned), HPA max 4, fix `USER`, image tag by SHA, add Ingress+TLS, add a migration Job, fix the NetworkPolicy egress allowlist | Separate API/worker deployments, PDBs per component, staging that mirrors prod, blue-green or canary with automatic rollback on SLO burn | Multi-AZ, multi-region warm standby, IaC (nothing here is IaC today — one hand-written manifest), automated DR drills | Multi-region active-active, cell-based architecture, per-cell blast-radius limits |
| **Architectural changes actually justified** | **None.** Fix the blockers. | Split workers; add Redis cache; add a read replica. | Partitioning; CDN edge auth; real OTel. | Sharding/cells; telemetry out of PostgreSQL; multi-region. |

**Explicitly not recommended:** microservices. The evidence shows a single binary at ~1.2 peak
req/s for 1K DAU with p95 < 10 ms. Decomposing now would multiply the failure modes this audit
already found (inconsistent state, per-process cache, unrecorded routes) across network boundaries.
The only split with evidence behind it is **workers out of the API process**.

---

## 16. COST MODEL (where measurable)

| Driver | Model | 1K DAU estimate | Notes |
|---|---|---|---|
| **Audio bandwidth** | 720 sessions/day × 600s @ 128 kbps = 6.9 GB/day | **≈ 208 GB/month** | Without a CDN: S3 egress ~$0.09/GB ≈ **$19/mo**. With a CDN: ~$0.02–0.05/GB but better tail latency. `FormatVariants` also defines 256/320 kbps and FLAC tiers — a "high" default would **double or triple** this. |
| **Audio storage** | 78 confessions × 4 variants × 1 voice × ~60–300s | < 1 GB today | Scales with voices × languages × variants, not with users. 5 voices × 3 languages ⇒ ~15 GB. Negligible. |
| **Voice generation** | ElevenLabs per-character | 78 confessions × ~700 chars × 4 variants ≈ 219k chars **per voice** | One-off per voice/language, not per user. This is the dominant *fixed* cost and the reason `AudioKeyFor` is deterministic ("never generate duplicate audio", §60) — a genuinely good cost control. |
| **Database** | ~21k requests/day, ~720 sessions, ~12k progress writes | 2 vCPU / 4 GiB managed ≈ **$30–70/mo** | `session_progress` and `idempotency_keys` are the growth tables; the latter never purges (IC-023). |
| **Redis** | Rate limiting only | Smallest managed tier ≈ **$10–20/mo** | Required in production by `config.Validate`. |
| **Push** | 720 reminders/day | APNs free; FCM free | No cost; currently zero deliveries (IC-012). |
| **Email** | ~1 signup + security events | Postmark free tier → ~$15/mo at scale | Async queue, 512 in-process (IC-023/Failure matrix). |
| **Compute** | 1–2 API pods + 1 worker pod | ≈ **$40–80/mo** | The K8s manifest's 3–20 replicas would cost 3–10× this for no benefit at 1K DAU. |
| **Analytics / monitoring / Sentry / APM** | — | **$0 today** | None are wired (IC-031). This is a saving that is actually a gap. |
| **App-store fees** | 15–30% of premium revenue | — | **$0 today: no purchase can complete** (IC-003). |
| **Cost per active user** | ≈ $100–200/mo ÷ 1,000 | **≈ $0.10–0.20/MAU/month** | Excluding voice generation amortisation and staff. |
| **Cost per session** | bandwidth + write amplification | **≈ $0.0003 + ~17 DB writes** | The 17 writes/session (mostly progress syncs) are the thing to optimise: batch progress, or sync every 60s rather than every 30s. |
| **Cost per audio minute** | 128 kbps ≈ 0.96 MB/min | **≈ $0.00009/min via CDN, $0.00009–0.0001 direct** | Bandwidth is not the risk; **unrestricted hotlinking is** — a signed URL is bearer-accessible for its TTL, and TTLs are 1h free / 6h premium (`playback_ttl_seconds` observed live: 3600 free, 21600 premium). A 6-hour premium URL shared publicly serves unlimited listeners at the platform's cost. Consider per-session binding and shorter TTLs with transparent re-minting (the client already re-reads the session to refresh URLs — the mechanism exists). |

---

## 17. "AI-ISH" SCORECARD

Evaluated on the **artifact**, not on whether AI assisted in producing it.

| Dimension | Finding |
|---|---|
| **Generic visual patterns** | **Absent.** No gradients, no glassmorphism, no decorative blobs, no generic SaaS dashboard. `rg 'gradient|blob|glass'` over `design/tokens.json` returns only a 5-step shadow ramp. The palette is a deliberate dark-green/brass/paper system with a *documented* rejection of a previous indigo brand (`test_design.py` asserts "primary is not the retired indigo"). Scripture is set in a serif and interface text in a sans — a real typographic decision, mechanically enforced. |
| **Generic copy** | **Mostly absent.** Error strings are product-voiced and specific: "a session can only be completed after playback has started", "that password appears in known breach lists; choose something less common", "Save these recovery codes now. They are shown once and cannot be retrieved later." The mobile error mapper has 20+ contextual recoveries ("That code has expired" vs "That didn't match" depending on `AuthSurface`). Exceptions: the admin console and `apps/web` pages narrate their own implementation ("Live from `GET /v1/categories` — cached SWR 5m (§7.1). 39 — not hard-coded."), which is **spec-speak rendered as user-facing copy** — a clear generation artifact. |
| **Component repetition** | **Controlled.** One `AppScaffold`, one `EmptyState`, one `Skeleton`, one `auth_form` reused across five auth surfaces via an `AuthSurface` enum. The web SPA has one `h()` escaper, one `guard()` wrapper, one `render()`. Not copy-paste-driven. |
| **Design-system consistency** | **Strong, and enforced.** 120 tokens in one JSON; Dart/CSS/TS generated from it; CI fails if generated code is stale; spacing scale validated as multiples of 4 within 4–64; radius set is exactly {8,12,16,20,999}; 30/30 text pairings pass WCAG 2.1 AA (verified live, 4.64:1 worst case). Very few repositories have a *machine-checked* design system. |
| **Product differentiation** | **Real.** The five-tab bar with a filled circular centre action instead of a FAB is argued in code comments on product grounds ("a FAB would cover content, sit outside the bar's rhythm, and read as a platform default rather than a decision"). The IA test asserts "labels are explore and confess, not discover and create", that a session is reachable from home in ≤2 taps, and that a free user can reach a session **without** passing the paywall. That is product thinking encoded as tests. |
| **UX intentionality** | **High, with one glaring exception.** Loading/empty/error states are distinct components; the home rail degrades independently per section; the builder enforces engine-derived duration bounds with a drift-guard test that **reads `planner.go` source** so client constants cannot rot. The exception is IC-025: a fabricated streak with dead code beside it. |
| **Code-generation artifacts** | **Present but minority.** (a) IC-025's `DateTime.parse(s.items.isNotEmpty ? '' : '')` + unused `completedDates`; (b) the dead policy layer `FilterFeed`/`FilterPublic`/`IsValidVisibility` — an abstraction with zero callers; (c) `ai/llm_adapter.go:20`'s `TODO: wire LLM SDK in /tmp`; (d) `metrics.go:58`'s `TODO: update p99 from histogram`; (e) `log.go`'s `JSONLogger` that doesn't emit JSON; (f) `ARCHITECTURE.md`'s list of 20 providers that are never imported; (g) the Flutter `pubspec.yaml` still carrying the entire default template comment block; (h) Android `build.gradle.kts` still carrying the template `TODO: Specify your own unique Application ID` and signing release builds with the **debug** keystore; (i) `app_shell.dart:160`'s `color: selected ? surfaces.primary : surfaces.primary` — both branches identical. |
| **Placeholder content** | **Honest about it.** `PlaceholderScreen`'s own doc comment says: *"A placeholder that pretends to be finished is worse than one that says so."* `/player` (no id) renders one. No lorem ipsum anywhere. The placeholder **audio** (`media/wav.go` tone generator) is explicitly dev-only and refused in production by the storage validation gate. |
| **Over-engineering** | **Mild.** The dead community-policy layer; a hand-written RESP client (defensible and argued); a hand-written `itoa64` "to avoid fmt for hot path" in a metrics handler that runs once per scrape; 286 routes where 143 would do (the `/v1/` duplication doubles the surface for exactly the drift found in IC-004). |
| **Under-engineering** | **Concentrated where it matters.** No audio playback, no push registration, no offline bytes, no payments, no CDN path, no backups, no client test execution, no security headers. |
| **Overall authenticity** | **Deliberately engineered, incompletely built.** |

> **Does this feel deliberately engineered for iCONFESS, or assembled from generic AI-generated
> patterns?**
>
> **Deliberately engineered.** The evidence is not stylistic, it is structural: a machine-checked
> design system with a documented brand migration; an IA test that encodes product rules (≤2 taps to
> a session, free users bypass the paywall); drift-guard tests that **read the Go source** to keep
> client constants honest; a session state machine whose stated invariant ("no path to COMPLETED
> that does not pass through playback") is proved by walking the whole graph; a deletion policy
> where every retention must carry a written reason; a route table that generates its own OpenAPI
> spec; and phase documents that record defects *found and fixed* with reproductions.
>
> Could this interface belong to 500 other AI-generated apps? **No.** The dark-green/brass/paper
> system, the serif-for-scripture rule, the centre-action tab bar and the confession-builder flow
> are specific to this product.
>
> The failure mode here is **not** AI slop. It is **documentation velocity outrunning delivery
> velocity**: 31 phase documents marked PASS, a README claiming Spotify-grade reliability, and a
> "100% complete" audio platform — attached to an app that cannot play sound, on a backend that
> 500s outside dev, with payments that do not exist. The generated artifacts (IC-025, the dead
> policy layer, the provider list in `ARCHITECTURE.md`) are symptoms of that same pressure: things
> written to *look* complete rather than to *be* complete.

---

## 18. PRODUCT AUTHENTICITY AUDIT

| Dimension | Assessment |
|---|---|
| **Product identity** | **Coherent and specific.** "Daily Confession Ritual. Scripture. Audio. Habit." The loop Discover → Choose → Create/Schedule → Listen → Confess → Complete → Return is implemented in the IA, enforced by tests, and reflected in the tab structure (Home / Explore / **Confess** / Activity / Me). The centre action is the builder, not a feed — correct for a ritual product. |
| **Emotional hierarchy** | **Present in the design system, absent in the runtime.** Scripture in serif, declarations in a "verse" panel with an "Now declaring" eyebrow, intensity 1–5 per confession, brass accent on the primary action. But the mobile app never renders a playing verse because nothing plays. |
| **Interaction model** | **Real and opinionated.** Builder walk: duration (engine-derived ladder + bounds-enforced custom) → strategy (BALANCED default) → voice (filtered to `status == 'active'`, "no preference" as a first-class choice) → review (live `POST /sessions/preview`, locked items shown with an upgrade affordance rather than silently skipped). |
| **Content experience** | **Structured, unreviewed.** 39 categories, 78 confessions, four duration variants each, `scripture_references` with book/chapter/verse/translation/`is_direct_quote`. The quote-vs-paraphrase distinction the audit brief asks for **exists in the schema and is populated** — a genuinely good provenance model. No review, no translation licence, fabricated `author`. |
| **Audio experience** | **Does not exist.** IC-001, IC-002, IC-011. |
| **Session experience** | **Correct on the server, hollow on the client.** The state machine, entitlement re-evaluation on every read, signed-URL refresh, locked-item signalling and soft-delete-after-cancel are all real and verified live. |
| **Visual language** | **Distinctive.** Dark green/brass/paper, 120 tokens, WCAG AA verified, no gradients or glass. The **admin console and `apps/web` are a different product** — indigo/violet `#7c8cf8`/`#b7a1ff` on `#0f1220`, which is exactly the palette the design system's own test asserts was *retired*. Two visual languages ship in one repository. |
| **Copywriting** | **Strong in the app, self-referential in the consoles.** Compare "You have been signed out on all other devices." (good) with "Live from `GET /v1/categories` — cached SWR 5m (§7.1). 39 — not hard-coded." (a spec paragraph rendered as UI copy). |
| **Navigation** | **Enforced.** 37 screens, 8 entry points, no orphans, no dead ends, 102 endpoints wired to screens — all validated by `test_ia.py` in CI. |
| **Product loop** | **Broken at Listen.** Discover ✓ Choose ✓ Create ✓ **Listen ✗** Confess (UGC) ✓ Complete ✓ Return (schedule) ✗ (no push). |

> **Does iCONFESS feel like iCONFESS?** The *design* of it does — unmistakably. The *running
> product* does not yet exist: there is no sound, no reminder, no purchase and no offline listening.
> What is present is an unusually well-specified and well-tested **skeleton of the right product**,
> with the four things that make it a product (audio, playback, delivery, monetisation) missing.

---

## 19. TESTING FORENSICS

Not a count — an assessment of what the tests actually prove.

| Suite | Files / lines | Executed? | Quality assessment |
|---|---|---|---|
| `internal/api` | 36 files / 8,342 lines | ✅ live, `-race`, vs PostgreSQL 17.10 | **Good.** 55.4% statements. Real adversarial tests: IDOR (`profile_test.go:368` "Bob supplies Alice's user id"), admin authorization sweep over the route table, session lifecycle, entitlement gating, audio-lifecycle archiving, email-flow end-to-end, route parity between prefixes, OpenAPI-vs-route-table. **But:** the authz sweep iterates `RouteTable()`, so IC-004's 8 unguarded routes and IC-026's unrecorded erase route are structurally invisible to it. |
| `internal/auth` | 3 / 381 | ✅ | 37.5% — low for the highest-value package. Password policy and token hashing covered; middleware paths partly. |
| `internal/sessions` | 2 / 441 | ✅ | **Excellent.** `TestCompletionIsUnreachableWithoutPlayback` walks the whole transition graph rather than asserting one input. 59.6% but the *property* is what matters. |
| `internal/deletion` | 3 / 557 + community | ✅ | **Good design, false guarantee.** 15 tests including "erasure is scoped to one account" and "tombstones are unique". `TestEveryUserTableHasAPolicy` is defeated by tables without a literal FK (IC-007). |
| `internal/mfa` | 1 / 245 | ✅ | **Excellent** — 94.2%, tested against RFC 6238 published vectors. |
| `internal/rights` | 2 / 377 | ✅ | 92.1%. The 451 gate is real. |
| `internal/scheduler` | 2 / 609 | ✅ | 89.0%, including "a week of downtime released 0 notifications at once" (burst suppression) and duplicate-tick idempotency. |
| `internal/billing` | 1 / 67 | ✅ | **20.9%.** Four tests, all about the fail-closed guard. Nothing tests a real receipt, expiry, renewal, cancellation, refund or webhook — because none exist (IC-003). |
| `internal/store` | 5 / 1,720 | ✅ | **26.6%** for 5,101 lines across 17 files. The data-access layer is the least-covered substantial package. |
| `internal/ratelimit` | 2 / 307 | ✅ | 32.6%. The hand-written RESP client has **never met a real Redis in automation** — CI has no Redis service. |
| `internal/models` | **0** | ✅ (nothing to run) | 956 lines, **0% coverage, no test file**. |
| `internal/storage` | 2 / 595 | ✅ | 52.3%; the one failing test (`TestContentTypeMapping`) is environmental — this image's `/etc/mime.types` maps `xyz`→`chemical/x-xyz`. Present in the pre-change baseline and passes on the CI runner. Correctly recorded rather than "fixed" by weakening the assertion. |
| `internal/arch` | 1 / 216 | ✅ | **Rare and valuable** — a parser-based layering test that fails when a handler imports a repository directly. |
| `internal/db` | 5 / 942 | ✅ | Migration checksum immutability, `?`→`$N` rebinding (including comment-awareness), schema-coverage and status-constraint parity tests. |
| **Mobile** (`apps/mobile/test`) | 15 files | ❌ **never executed by anyone** | Cannot be assessed for pass/fail. By reading: they use a `FakeApiClient`, a `DebugAnalytics` recorder with a PII-violation callback, a golden test for the welcome screen, and a validator-drift test that **reads `password_policy.go`**. The design is good. The execution gap is the finding (IC-018). |
| **Dart client** (`clients/dart/test`) | 2 files | ❌ never executed | Comments state they run "against a real local HTTP server rather than a mocked transport" — the right instinct, unverified. |
| **Python design** | 4 scripts | ✅ live | All pass. |
| **Contract tests** | — | — | **Absent.** `contracts/openapi.json` is generated but never validated against live responses, which is why IC-004 and IC-022 survived. |
| **Load / stress / soak / chaos** | — | — | **Absent entirely.** No k6/vegeta/locust artefacts, no chaos experiments, no failure-injection suite beyond three fault injections mentioned in `docs/31-MODERATION.md`. |
| **Security tests** | `internal/security/checks.go` (25 lines) | ✅ | `docs/SECURITY.md` describes authz-401, forged-premium-402, IDOR, rate-limit-429, token-reuse-401, sqli-200, xss-200 "cases CI must enforce". The file is 25 lines and `rg` finds no `Test` function in it — the described suite is **not present in that file**; equivalent coverage exists scattered in `internal/api`. Documented ≠ implemented, again. |

**Negative-path coverage verdict:** authorization ✓ (except IC-004), IDOR ✓, expired/revoked
session ✓, token reuse ✓, duplicate request ✓ (idempotency), provider failure ✓ (push/email/TTS),
database failure partial (fail-closed in auth only), concurrency ✓ (`-race`, `SKIP LOCKED`,
rotation races), stale state ✓ (asset-status gate), deleted resource ✓ (soft delete → 404).
**Missing:** load, chaos, contract, client execution, backup/restore.

---

## 20. CI/CD, RELEASE ENGINEERING AND INCIDENT RESPONSE

**CI** (`.github/workflows/ci.yml`, one job, `working-directory: server`): `go mod verify` →
`go build` → `go vet` → `go test -race` (against a real `postgres:17` service; `TEST_DATABASE_URL`
is set so tests fail rather than skip) → design scripts (with a documented override of the
job-level working directory, and a comment recording that PRs 24–31 all merged while that step was
silently failing) → golangci-lint v2.13 with `new-from-merge-base: main` → `gofmt -l`. The
workflow's comments are unusually good: each one records a specific past failure and why the step
is shaped the way it is. `permissions: contents: read` is set.

**What is missing:** required-check enforcement (PR #40 merged red), client jobs, `govulncheck`,
secret scanning, Dependabot/Renovate, container build/push/scan, SBOM, coverage gate, deploy,
rollback, and any staging environment.

**Release engineering:** `pubspec.yaml` is `1.0.0+1` and has never moved; there is no changelog, no
release notes, no build-number automation, no feature-flag *service* (a `feature_flags` table
exists and is seeded with `new_player`/`ai_session_builder` at 0%, but nothing reads it — verified:
`rg 'feature_flags' server/internal` finds only the migration and a conn test), no minimum-version
enforcement (referenced in a router comment as future work), and no environment separation beyond
`ENV`.

**Rollback:** migrations are forward-only with checksum enforcement (a good, deliberate choice) and
**no down path at all**. Combined with `image: iconfess:latest` and `imagePullPolicy: IfNotPresent`,
a bad deploy cannot be rolled back to a known artefact, and a bad migration cannot be rolled back
at all. `docs/DEPLOYMENT.md` exists; no rollback procedure was found in it or anywhere else.

**Incident response:** **no incident process exists.** No runbook, no on-call, no severity
definitions, no escalation path, no communication template, no postmortem directory, no alerting
configuration (the `AuthMetrics` counters and `Distributed.Degraded()` are never exported to an
alerting system). `docs/OBSERVABILITY.md` describes intent. The detection story for IC-001
illustrates the gap concretely: a deployment with zero playable content reports
`{"status":"ready"}` forever.

---

## 21. DOCUMENTATION AND DEVELOPER EXPERIENCE

**A new engineer following the docs cannot run the system.** Attempted from a clean tree:

| Step | Documented | Actual | Result |
|---|---|---|---|
| 1 | `git clone … && cd i-confess` | ✓ | ok |
| 2 | `cp .env.example .env` | no root `.env.example`; it is `server/.env.example`, and it is a **markdown document with `export` lines**, not a dotenv file | **fails** |
| 3 | `docker-compose up -d` | file is `server/docker-compose.yml`; no Docker in this environment | **fails** (and compose ships `postgres:16` while CI tests `postgres:17`) |
| 4 | `go run ./cmd/server` | the module is in `server/`; from the root this errors `go.mod file not found` | **fails** |
| 5 | "PostgreSQL 16+ (production) or **SQLite** (development)" | SQLite was removed; `db.Open` supports only `lib/pq`; `DATABASE_URL` defaults to a `lib/pq` keyword string | **misleading** |
| 6 | `make verify` | ✓ works, and the root `Makefile` is excellent — it handles the `server/` module boundary, exports `TEST_DATABASE_URL`, and documents why there is no skip fallback | **ok**, except `mobile-check` silently skips and `golangci-lint` is usually absent |
| 7 | `make test` | requires a live PostgreSQL 17 at `TEST_DATABASE_URL`; fails rather than skips (correct and documented) | ok once PostgreSQL exists |

The **root `Makefile` and `docs/ENGINEERING-PIPELINE.md` are the good path**; `README-COMPLETE.md`
is the bad one and should be deleted. `README.md` is short and accurate but does not mention
`make`. Environment variables are documented in three places that disagree:
`server/.env.example` (lists `DB_PATH`, `MEDIA_DIR`, `SENTRY_DSN`, `NEWRELIC_LICENSE_KEY`,
`DATADOG_API_KEY` — none of which the Go code reads except `MEDIA_DIR`), `config.go` (the truth:
26 variables), and `docs/DEPLOYMENT.md`. **`config.go` is the only accurate list, and it is not
presented as documentation.**

Undocumented dependencies for a clean-machine run: Go 1.25+, PostgreSQL 17 reachable at
`TEST_DATABASE_URL`, Python 3, `golangci-lint` v2.13, Flutter ≥3.44 (for `make mobile-check`), and
— for any real audio — an ElevenLabs key, S3 credentials, Postmark token, APNs `.p8` key and FCM
service account. None of these have a setup guide.

---

## 22. ACCESSIBILITY, CROSS-PLATFORM AND MOBILE STORAGE

**Accessibility** (assessed from tokens, semantics and the contrast checker; not runtime-verified
with a screen reader):
* 30/30 text pairings pass WCAG 2.1 AA, mechanically checked in CI; worst case 4.64:1.
* Typography roles are semantic (`display/heading/subheading/body/bodySm/caption/label/metric`)
  with a serif reserved for scripture — a real hierarchy, not one font size ladder.
* Nav targets are 64pt tall with 44pt minimum hit areas; every tab carries
  `Semantics(button: true, selected: …, label: …)`; the centre action has an explicit
  `label: 'Create a confession session'`.
* The web SPA uses `aria-live="polite"` on the root, `role="slider"` + `tabindex="0"` on the seek
  bar, `aria-pressed` on toggles, `role="status"` on toasts, `aria-current="page"` on nav, and a
  keyboard handler for Space/Escape.
* **Gaps:** no reduced-motion token or check (`motion` tokens exist but nothing asserts a
  `prefers-reduced-motion` path); no dynamic-type/font-scaling test; no captions or transcripts for
  audio (which does not yet exist); the admin console has no focus management, no skip link and
  label-less `<input>`s in places.

**Cross-platform:** iOS `Info.plist` declares portrait + both landscapes (a ritual audio product is
portrait-first; landscape support is unearned complexity) and has **no `UIBackgroundModes: audio`**,
so background playback would be killed even if an engine existed. `SceneDelegate.swift` is present
(unusual for a Flutter template — indicates deliberate iOS work). Android `AndroidManifest.xml` has
**no `POST_NOTIFICATIONS`, no `FOREGROUND_SERVICE`, no `WAKE_LOCK`, no deep-link `intent-filter`**
for the `iconfess://` scheme the server already emits, and `android:exported="true"` on the launcher
activity with `taskAffinity=""` (the latter is a good hardening detail). Release builds sign with
the **debug** keystore. Nothing here has been run on a device or emulator by anyone
(`NOT VERIFIED`).

**Mobile storage/secrets:** tokens go to Keychain/Keystore via `flutter_secure_storage` wrapped in a
`SecureTokenStore` that caches in memory, treats a keystore failure as "no token" rather than
crashing, and — per the code comment — is configured so a token is **not migrated to a new device in
a backup**, because "a token that outlives the device it was issued to is a token that can be
replayed". That is a genuinely sophisticated mobile-security decision. Non-sensitive prefs in
`shared_preferences`. Analytics has a PII blocklist with a debug-mode violation callback. **No
clipboard writes, no screenshot blocking** (relevant for private confessions on Android —
`FLAG_SECURE` is not set), and **no downloaded-content encryption** because nothing is downloaded
(IC-013). `internal/offline/crypto.go`'s AES-256-GCM `Seal`/`Open` has **no client counterpart and no
caller** — dead code.

---

## 23. SENIOR ENGINEER REVIEW

**Principal Engineer** would challenge: *"You have 31 phase documents marked PASS and a README
claiming Spotify-grade reliability, but the product cannot play audio and `POST /sessions` 500s
outside dev. What does PASS mean here? Show me the definition of done that a phase must satisfy, and
show me one phase whose user-visible outcome was verified by a human being on a device. Also: your
OpenAPI spec is generated from the route table so it 'cannot drift' — and it publishes `bearerAuth`
for four endpoints that accept no token. Generation is not verification. Where is the contract test?"*

**Staff Backend Engineer** would challenge: *"The session state machine, the rotation-with-reuse-
detection, and the deletion policy are better than most production systems I've reviewed. So why is
`internal/store` at 26.6% and `internal/models` at 0%? Why is `SetSubscription` a non-transactional
UPDATE with no existence check on the one write that determines revenue? Why are all your timestamps
TEXT — you've already documented that it costs you sub-second ordering in the job queue — and why is
`PurgeExpired` written, commented 'called from the periodic cleanup job', and never called? And your
cache is per-process while your manifest runs 3 to 20 replicas: what happens when an admin retracts
a confession?"*

**Senior Security Engineer** would challenge: *"Four things stop me cold. One: eight routes where the
auth string says `user` and the wrapper says `nil` — how did a route-parity test, an OpenAPI
generator, an IA test and an admin-authz sweep all fail to notice? Because every one of them reads
the *declared* value. Two: `SEED=1` creates a super-admin with a password that is in this
repository, and a design doc calls that 'not a concern'. Three: TOTP secrets in plaintext while
recovery codes next to them are hashed — you knew the pattern. Four: no CSP, no X-Frame-Options and
a bearer token in `localStorage` on an admin console that can't complete an MFA login. Fix the
mechanism, not the instances: make it impossible to register a route without choosing an enforcement
level."*

**Senior Mobile Engineer** would challenge: *"Your Riverpod architecture, error mapper, secure token
store and drift-guard tests are strong. But there is no audio engine, no background mode, no
interruption handling, no push registration and no offline byte storage — for an audio product whose
core loop is a scheduled daily reminder. And your player sets local state *before* the network call
and swallows every failure with `catch (_) {}`, which means your server's carefully-proven
'completion requires playback' invariant is satisfied by a client that never plays anything. Your
streak card computes `DateTime.parse('')`, catches, and returns `now()` — then ignores the result.
None of your 15 test files has ever been executed by CI."*

**Senior Product Engineer** would challenge: *"The Subscribe button is `onPressed: () {}`. The
premium screen shows five currencies for a plan nobody can buy. The Activity screen shows a streak
that is a capped session count. The web profile says 'Subscriptions are verified server-side —
coming in V1.' You have built a paywall, a trial journey, an offline entitlement and a scheduling
loop, and none of the four can complete. Which one is the product? Ship one loop end to end before
building the fourth surface."*

---

## 24. HIRING / FELLOWSHIP REVIEW

Assessed strictly from repository evidence; no inference about the person beyond what the artefacts
support.

### Strong engineering signals

* **Defect-driven documentation.** The phase docs and code comments record *specific* past failures
  with reproductions: an inverted production gate that let `ENV=prod` boot with a published JWT
  secret; an MFA endpoint that accepted a client-supplied secret; `mErr == nil && enrolment.Enabled`
  turning a database outage into an authentication bypass; an audio gate asserting the literal
  `"ready"` so archiving assets changed nothing; two schema sources of truth that had drifted to 25
  tables against a live 64; a CI step that failed on every branch for eight PRs while the real checks
  never ran. **Recognising, fixing, and writing down the mechanism of a bug is the single strongest
  signal in this repository.**
* **Properties over assertions.** `TestCompletionIsUnreachableWithoutPlayback` walks the entire
  transition graph. `arch_test.go` parses the source tree to enforce layering. Drift-guard tests read
  `planner.go` and `password_policy.go` so client constants cannot rot. `test_ia.py` validates
  reachability, tap depth and endpoint↔screen coverage. This is testing maturity well above typical.
* **Fail-closed instincts.** Auth session validation fails closed on a database error; entitlements
  degrade to Free, never Premium; an unparseable `TOKEN_TTL` falls back to 15 minutes, not 30 days;
  unimplemented storage providers are refused at boot rather than failing at first upload;
  `EMAIL_PROVIDER=log` is refused in production because it writes reset links to logs; billing fails
  closed in production. Each of these has a comment explaining the *direction* of the failure.
* **Security fundamentals correct without being asked.** HMAC keyfunc rejects non-HMAC algorithms;
  constant-time comparison for TOTP and token hashes; only hashes of one-time tokens stored; recovery
  codes hashed with a transcribable alphabet; newline separator in the signing MAC with a comment
  explaining the boundary-shift attack it prevents; EXIF stripped from avatars; bidirectional/zero-
  width characters rejected in display names with the full U+202A–202E and U+2066–2069 ranges
  covered; username homograph defence; refresh-token reuse revokes the whole family and emails the
  owner.
* **Operational thinking.** Liveness deliberately checks nothing while readiness checks the database,
  with the crash-loop reasoning written down. `FOR UPDATE SKIP LOCKED` for the queue. Occurrence-key
  uniqueness so a double scheduler tick is harmless. Burst suppression so a week of downtime doesn't
  release a week of notifications. Graceful shutdown with `sync.Once`. Redis-optional rate limiting
  that degrades rather than taking the site down.
* **Restraint.** The Dart client has **zero runtime dependencies**, with the reason documented
  ("cannot pull a transitive package into a codebase that handles credentials"). The Redis client is
  hand-written because two commands are needed. `PlaceholderScreen` refuses to fake finished content.
* **A machine-checked design system** with a documented brand migration and WCAG enforcement — rare
  at any seniority.

### Weak engineering signals

* **Verification theatre.** The gap between "PASS" and "works" is the central weakness: a player with
  no audio engine, a paywall with a no-op button, a streak that is a count, an analytics endpoint
  with no sink, an `ARCHITECTURE.md` naming twenty providers that are never imported, a
  `docs/SECURITY.md` describing a test file that contains no tests. Producing artefacts that *assert*
  completion without *demonstrating* it is the habit most worth unlearning.
* **Guarantees that don't hold at the edges.** "The spec cannot drift" — it misstates authorization.
  "Every table holding user data has a deletion policy" — the test only sees literal foreign keys.
  "All 44 admin routes reject a non-admin" — the most destructive one isn't in the table. A declared
  value is not an observed behaviour; this pattern recurs four times.
* **Dead abstractions.** `FilterFeed`, `FilterPublic`, `IsValidVisibility`, `offline.Seal`/`Open`,
  `log.JSONLogger`, `metrics.p99`, `Distributed.Degraded()`, `feature_flags` — all written, none
  called. Writing an abstraction and not wiring it is worse than not writing it, because it reads as
  a control that isn't.
* **Unfinished hygiene.** Release builds signed with the debug keystore and the template TODO left in
  place; template comment blocks throughout `pubspec.yaml`; `// indirect` mislabels; a bare `.md` in
  `.gitignore` that would silently swallow new documentation; two visual languages (retired indigo in
  the consoles, green/brass in the design system).
* **Client testing never executed.** 15 well-designed mobile test files that no pipeline has ever run.
* **No operational ownership.** No backups, no restore drill, no incident process, no alerting, no
  runbook, no deployment automation, no rollback. The system stops at "it runs on my machine".

**Net:** the repository demonstrates **real strength in security reasoning, property-based testing,
defect analysis and design discipline** — and a **real weakness in end-to-end verification,
operational ownership, and the honesty of status reporting**. The engineering instinct is sound; the
completion discipline is not.

---

## 25. END-TO-END JOURNEY RESULTS

| Journey | Result | Evidence |
|---|---|---|
| **A** Install → Onboarding → Signup → Preferences → Home → Discover → Choose → **Listen** → Complete → History | **FAILS at Listen** | Signup ✓ (200 + token, verification mail queued). Discover ✓ (39 categories, 78 confessions live). Choose ✓. **Listen ✗** — no audio engine (IC-002); and on a non-dev server the session cannot even be created (IC-001, 500). Complete ✓ server-side (200, `items_completed:1/2`). History: mobile reads `GET /sessions` ✓; `GET /me/history` returns `null` unless the client explicitly POSTs it. |
| **B** Create session → Schedule → Notification → Start → Pause → Resume → Complete | **PARTIAL** | Create ✓ 201 `READY`. Schedule endpoints ✓. **Notification ✗** — no client push registration (IC-012); server dispatcher would log `no registered devices`. Start ✓ 200. Pause ✓. Resume ✓. Complete ✓. Illegal moves correctly refused: complete-from-READY → **409** "a session can only be completed after playback has started"; start-after-complete → **409** "this session has already finished"; PATCH to ACTIVE from terminal → **409**. Double complete → 200 (idempotent no-op). Soft delete → 204, then GET → 404. |
| **C** Trial → Premium → Purchase → Entitlement → Premium content → Renewal | **FAILS** | Trial journey ✓ (`GET /subscriptions/trial`, deterministic Day 1–7). Plans ✓ with 5 currencies from the DB. **Purchase ✗** — mobile Subscribe is `onPressed: () {}`; no store SDK. Entitlement ✓ *server-side* but grantable by a stub string outside production (IC-003, live-verified free→premium). Premium gating ✓ (`POST /me/downloads` as free → **402 `ENTITLEMENT_REQUIRED`**). **Renewal/cancel/refund/grace ✗** — `ends_at` is never written or read. |
| **D** Offline → Download → Listen → Progress → Reconnect → Sync | **FAILS** | Server licences ✓ (entitlement-gated, count-limited, expiring, revocable, refresh endpoint). **Client ✗** — no bytes fetched, no storage, no `Seal`, no playback (IC-013). No conflict-resolution rules exist because there is no offline write path. |
| **E** Create content → Moderation → Approval → Audio → Publication → Discovery → Playback | **PARTIAL** | Author submission ✓ (create UGC 201 with `is_private:true`, `visibility:"private"`, `status:"draft"`; submit → `status:"submitted"`). Report ✓ 201 with per-entity dedupe. Admin queue ✓ returns UGC + reports. **Approval ✗ in practice** — the console has no moderation UI (IC-015) and the API rejects undocumented fields (IC-014). **Audio ✗** — no generation without an ElevenLabs key; rights records absent (G-42). Publication ✓ mechanically (`content_moderation_history` written, `published_at` preserved, phantom id → 404). Discovery ✓ for system content. **Playback ✗** (IC-001/IC-002). Also: approved UGC has no public reader (G-40), and `GET /admin/audit` returned `[]` after all of it (G-41). |

---

# iCONFESS PRODUCTION CERTIFICATION

```
ARCHITECTURE:        PASS
BACKEND:             FAIL
MOBILE:              FAIL
WEB:                 PASS
DATABASE:            PASS
API:                 FAIL
AUTHENTICATION:      PASS
AUTHORIZATION:       FAIL
ANONYMITY:           FAIL
PRIVACY:             FAIL
SECURITY:            FAIL
AUDIO:               FAIL
SESSION ENGINE:      PASS
OFFLINE:             FAIL
SCHEDULING:          FAIL
MODERATION:          FAIL
PAYMENTS:            FAIL
SUBSCRIPTIONS:       FAIL
INFRASTRUCTURE:      FAIL
CI/CD:               FAIL
TESTING:             FAIL
PERFORMANCE:         PASS
ACCESSIBILITY:       PASS
UX:                  PASS
CONTENT:             FAIL
DOCUMENTATION:       FAIL
AI-ISH AUDIT:        PASS
```

```
FINAL STATUS:

NO-GO
```

### The decision, strictly from verified evidence

**Four P0 findings, each independently sufficient:**

1. **The product does not function on a real deployment.** A server booted from this exact tree with
   `ENV=test` serves 39 categories and 78 confessions, returns `null` for `GET /voices`, answers
   `POST /sessions/preview` with 422 and **`POST /sessions` with HTTP 500** — while
   `GET /health/ready` reports `{"status":"ready"}`. `seed/ensure.go:25-30` documents that audio and
   voices are deliberately excluded from non-dev bootstrapping, and no readiness gate, error mapping
   or runbook covers the consequence.

2. **The primary client cannot produce the product's only output.** `apps/mobile/pubspec.yaml`
   contains no audio dependency of any kind; `player_providers.dart:31` and `player_screen.dart:16`
   both state that the audio engine is absent. There is no background mode, no interruption
   handling, no lock-screen control. Because the controller mutates local state before the network
   call and swallows every failure, the server's proven invariant "COMPLETED is unreachable without
   playback" is satisfied by a client that never plays anything — the completion metric is
   structurally forgeable.

3. **Payments are unverifiable in both directions.** Live reproduction in `ENV=test`:
   `POST /subscriptions/verify` with `{"provider":"apple","receipt":"valid_monthly"}` flipped a
   fresh free account to full premium (max session 900s→10,800s, downloads 0→5, offline hours 168).
   The fail-closed guard fires only for the exact string `production`, so **staging is exposed**. In
   production nothing can ever be purchased. There is no webhook route among the 286 registered
   routes, no provider or transaction column on `subscriptions`, no `ends_at` write or read, and the
   mobile Subscribe button is `onPressed: () {}`.

4. **Broken access control on eight endpoints.** `router.go:281-285` and `:419-423` register
   `/community/posts`, `/community/posts/{id}/react`, `/ai/parse` and `/analytics/batch` (both
   prefixes) with the auth string `"user"` and the wrapper `nil`. Live: a **revoked** session token
   returns 202 on `/analytics/batch` and 200 on `/ai/parse` where `/me` correctly returns 401;
   **no token at all** returns 202 with an attacker-chosen `user_id`; 30 unauthenticated rapid POSTs
   all return 202. `contracts/openapi.json`, the live `/openapi.json`, `design/routes.json` and
   `design/test_ia.py` all assert these routes require `bearerAuth`.

**Nineteen P1 findings compound them**, including: `SEED=1` creating a super-admin whose password is
published in this repository; no CSP, no `X-Frame-Options` and a bearer token in `localStorage` on an
admin console that cannot complete an MFA login; account erasure leaving `security_events`
(user id + IP + user agent) behind because the covering test only matches literal
`REFERENCES users(id)`; a public community feed that serialises `author_id` beside confession text;
plaintext TOTP secrets; no backup or restore capability of any kind; no privacy policy, terms,
licence or consent capture for a product whose entire payload is special-category religious data; and
a CI pipeline that runs one language, executes no client test, performs no security or dependency
scanning, builds no artefact, and was demonstrably not enforced when **PR #40 merged with a
`FAILURE` check**.

**What is genuinely good must be stated plainly,** because it is the reason this is a *fixable*
NO-GO rather than a rewrite: the session state machine, refresh-token rotation with reuse detection
and family revocation, the RFC-6238 MFA implementation tested against published vectors, the
policy-driven deletion design with written retention justifications, the voice-rights 451 gate,
HMAC-signed expiring audio URLs that resisted tampering, expiry, traversal and directory listing in
live testing, the machine-checked design system and information architecture, the layering test, the
migration-checksum immutability, the fail-closed configuration gate, and above all the phase
documents that record real defects with reproductions. Those are the artefacts of an engineer who
thinks about failure directions.

**The gap is not skill. It is verification and honesty of status.** Thirty-one phases are marked
PASS; a README claims Spotify-grade reliability; a document declares the audio platform 100%
complete. Attached to an app that cannot play sound, on a backend that 500s outside development, with
payments that do not exist, reminders that cannot be delivered, and offline listening that stores
nothing.

> **If real people start using iCONFESS tomorrow:** every one of them fails to create a session
> (HTTP 500) on any correctly-configured deployment; anyone who does reach a session hears nothing;
> anyone in staging gets premium for free with a 14-character string; anyone who discovers the eight
> unguarded routes can write forged telemetry for arbitrary users without a credential; an operator
> who sets `SEED=1` to populate the catalogue hands `super_admin` to a password published in this
> repository; an administrator who enrols MFA locks themselves out of the console; and a user who
> exercises their right to erasure leaves their IP address, user agent and event metadata in
> `security_events` forever — with no backup to restore from if any of it goes wrong.
>
> **Fix IC-001 through IC-004, IC-008, IC-009, IC-007, IC-034 and IC-011, then re-audit.** With the
> backend quality already present, that is a matter of weeks, not a rewrite.

---

*End of audit. Every quantitative claim above is reproducible from the commands recorded in §2 and
the file:line citations in §5. Items that could not be verified are marked `NOT VERIFIED` and listed
in §2.*
