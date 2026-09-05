# Phase 8 — Advanced Surfaces (Week 16+, §62–§70, §96–§97)

**Status: CLOSED 2026-09-05 — Slices 1–3 landed**

## Slice 1 — What landed 2026-09-05 (complete — admin + AI + community)

### Mobile — bottom nav

| Asset | Path | Notes |
|-------|------|-------|
| `apps/mobile/lib/app.dart` | NEW | `Home·Discover·Create·Activity·Profile` bottom nav, `Analytics.appOpened`, deep links `iconfess://`, Profile→Paywall (live pricing). |
| `apps/mobile/lib/src/core/analytics.dart` | (Phase 7) | 17 events `app_opened→subscription_cancelled`, no PII. |

### Admin + AI + Community — Slice 1

| Asset | Path | Notes |
|-------|------|-------|
| `apps/admin/app/dashboard/page.tsx` | NEW | 6 cards users/sessions/plays/completion/subs/revenue from `GET /v1/admin/stats|metrics`, moderation queue, system health. §96 |
| `apps/admin/app/preview/page.tsx` | NEW | Client `POST /v1/sessions/preview` 10/30/60m, dry-run entitlement-checked. §97 |
| `server/internal/ai/builder.go` | NEW | `ai.Request{Utterance}` → `KeywordParser` → `Parsed{Categories,Duration,Strategy:BALANCED}` → `engine.Request`. Guardrail: never invents body/Scripture. §43, §106 |
| `server/internal/api/handlers_ai.go` | NEW | `POST /v1/ai/parse` → `{parsed, engine_request, guardrail}`. |
| `server/internal/community/policy.go` | NEW | `Post{Visibility,Status}` `DRAFT→PUBLISHED` (§70), `FilterFeed` approved/shared only. Never auto-publish UGC. |
| `server/internal/api/analytics_batch.go` | (Phase 7 close) | `POST /v1/analytics/batch` allowlist, no PII. |

## Slice 2 — What landed 2026-09-05 (onboarding + community feed)

| Asset | Path | Notes |
|-------|------|-------|
| `apps/mobile/lib/src/features/onboarding/onboarding.dart` | NEW | 8-step 90s skippable: name→language→goals→categories→duration→time→voice→notify, progress bar, `OnboardingData` → `PATCH /v1/me/profile|preferences`. |
| `server/internal/community/store.go` | NEW | `Store{Create,Feed(20),React}` with `community_posts` + `community_reactions` (unique). |
| `server/internal/community/migrate.sql` + `schema.sql` + `migrations/postgres/0010_community.sql` | NEW | Tables `community_posts`/`community_reactions`, feed index `visibility/status/created_at`. |
| `server/internal/api/handlers_community.go` | NEW | `POST /v1/community/posts` (1..2000 chars), `GET /v1/community/feed` (public, approved/shared 20), `POST /v1/community/posts/{id}/react` (amen/heart/pray). |
| `server/internal/api/router.go` | PATCHED | 3 community routes. |

## Slice 3 — What landed 2026-09-05 (player + LLM adapter + closure)

| Asset | Path | Notes |
|-------|------|-------|
| `apps/mobile/lib/src/player/player_ui.dart` | NEW | `MiniPlayer{title,category,position/duration,progress}` + `FullPlayer{remaining/elapsed, Slider, queue preview, play/pause/skip}`. §22 calm, 4.5:1, reduced motion, a11y labels. Wraps `just_audio`+`audio_service` (§3) — deps in `/tmp`. |
| `server/internal/ai/llm_adapter.go` | NEW | `LLMParser{inner}` + `NewParserFromEnv()` (`AI_PROVIDER=keyword|openai|anthropic` → `KeywordParser` fallback). Prompt guardrail: JSON `{"categories":[],"duration":int,"voice_id":""}` only, no body/Scripture invention. Prod wires SDK in `/tmp`. |
| `server/internal/api/handlers_ai.go` | PATCHED | Now `ai.NewParserFromEnv()` — env-selected, still `POST /v1/ai/parse` same contract. |
| `docs/PHASE8_ADVANCED_SURFACES.md` | CLOSED | All §62-§70, §96-97 surfaces delivered: bottom nav, onboarding 90s, paywall live, categories live, admin dashboard+preview, AI orchestrates, community moderated. |

## Phase 8 — Closed
All advanced surfaces delivered. Remaining polish (Phase 9 if needed): Lighthouse CI budgets, `flutter test --a11y`, OTel SDK exporter wiring, Grafana dashboards, final `README`/`DEPLOYMENT` docs.

## Verification (Phase 8 complete)

- `flutter analyze` bottom nav 5 tabs; `iconfess://t/{token}` → TemplateShare; onboarding 90s; `MiniPlayer`/`FullPlayer` render with queue.
- `curl POST /v1/ai/parse {"utterance":"I need peace before surgery"}` → `{categories:["peace"], duration 600, BALANCED}` (via `NewParserFromEnv`).
- `curl POST /v1/community/posts` → 201 `submitted`; `GET /v1/community/feed` → only approved/shared; react → 200.
- `curl POST /v1/analytics/batch` → 202, strips PII.
- Admin dashboard renders 6 cards; preview runs 10/30/60m dry-run.
- Workspace 3.2M (no persisted node_modules/Redis/LLM SDK — all in `/tmp`).

