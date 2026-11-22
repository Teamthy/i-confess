# Phase 5 — Personalization (Weeks 10–11, §23, §41)

**Status:** CLOSED 2026-09-05 — Search (tsvector), Home (feature-flagged), Recommendations v1, Builder preview (dry-run), Templates (shareable), History resume (deterministic merge).

---

## 1. Slice 1 — Search, Home, Recommendations

| Step | File | Change |
|------|------|--------|
| **5.2 Discover + Search** | `server/migrations/postgres/0007_search_tsvector.sql` **NEW** + `server/internal/search/search.go` **OVERWRITTEN** + `server/internal/api/handlers_extended.go` | PG `tsv` columns on `confessions/categories/voices` with `setweight(A/B/C)` + GIN indexes + sync triggers (`tsvectorupdate_*`). `SearchStore.Search` tries PG `tsv @@ plainto_tsquery + ts_rank` first (with `popular_score` tie-break), falls back to `LIKE` for SQLite/dev — never errors on driver mismatch. Handler now parses `?q&kind=scripture|theme|tag|confession|category|voice&category&voice&language&sort=popular|relevance&limit` and delegates to store; empty `q` returns `Trending` (popular). `SearchRequest` now carries `CategoryID/VoiceID/Language/Sort`. |
| **5.1 Home** | `server/internal/api/home.go` **NEW** + `router.go` | `GET /v1/home` + `GET /home` returns ordered `sections` (`continue_listening, for_you, trending, categories, new_content, scheduled`) with `Enabled/Position`. Consults `feature_flags` (`home_sections`, `home_trending`, `home_recommendations`) and `preferences.recommendations_enabled` to hide `for_you` when disabled. Fail-open to visible on flag DB error. Migration seeds `home_sections/trending/recommendations` flags. |
| **5.6 Recommendations v1** | `server/internal/recommendations/recommendations.go` **NEW** + `server/internal/api/recommendations.go` **NEW** + `router.go` | Deterministic weighted sum (no ML): `+30 preferred category, +20 favorite, -15 per skip (cap -30), -25 if completed <7d / -10 <30d, +15 duration within 20%, +8 time-of-day (morning Healing/Peace, evening Gratitude), +5 language match / -20 mismatch, +0.1*popular_score, +3 fresh<7d and not completed, -5 fresh<14d but recently completed`. Sorted `Score DESC, ID ASC` for determinism. Handler `GET /v1/recommendations?limit&language` loads `profile.PreferredCategoryIDs` from `interests`, `PreferredDuration` from `preferences`, language from profile, then `loadCandidates` (50 most popular published) enriches with `favorites, playback_history skips/completed, confession_categories`. Returns `{recommendations:[{id,type,category_ids,score}], profile}`. |

**API:**
* `GET /v1/search?q=healing&kind=confession&sort=popular` — PG `ts_rank` → `popular_score` ordering; `q=healing` now finds confession+category+scripture (tags/theme included).
* `GET /v1/search` with empty `q` → `Trending` (10 most popular).
* `GET /v1/home` — feature-flagged sections, `recommendations_enabled:false` hides `for_you`.
* `GET /v1/recommendations?limit=10` — deterministic v1.

---

## 2. Still in Phase 5 (next slices, tables already exist)

| Step | Status | Note |
|------|--------|------|
| **5.3 Category + Builder** | **DONE** | `POST /v1/sessions/preview` + `POST /sessions/preview` (dry-run `engine.Build` without `Create` or outbox). Returns `target_duration/actual_duration, total_items, items_preview[3], voice_downgraded, display: "30 MIN • 12 • Voices"`. Builder preview now matches actual pack (same engine, same weighting/diversity/favorite/recency, same Strategy). Entitlement `MaxSessionSeconds` enforced before build. |
| **5.4 Custom sessions + Templates** | **DONE** | `user_templates` table (0008) + `store/templates.go` + `api/handlers_templates.go`. CRUD `POST/GET/PATCH/DELETE /v1/templates` with `category_ids, weights JSON, voice_id, voice_rules, ordering{strategy}, is_public`. `share_token` 12-hex, `share_url: https://iconfess.app/t/{token}` + `deeplink: iconfess://t/{token}`, `GET /v1/t/{token}` public if `is_public`. `POST /v1/templates/{id}/start` builds a session from template (same pipeline as schedule). Web `app/t/[token]/page.tsx` + mobile `deep_link.dart` `t` kind. |
| **5.5 History + Resume** | **DONE (documented)** | `playback_progress` PK `(user_id, audio_asset_id, device_id)` with `last_updated_at`; UPSERT `ON CONFLICT ... DO UPDATE SET position=excluded.position WHERE excluded.last_updated_at > playback_progress.last_updated_at` — later wins, deterministic. `GET /v1/me/history` + `POST /v1/me/history` already wired; `POST /v1/sessions/{id}/progress` + `playback_progress` for `Continue listening`. Flutter `History` uses `GET /v1/sessions` + local `playback_progress` merge. |
| **5.1 Feature-flag rollout** | **DONE for Phase 5** | `home_sections/trending/recommendations` flags + `preferences.recommendations_enabled`; `GET /v1/home` respects both. `rollout_percent/platforms` columns exist for Phase 7 A/B bucketing. |

---

## 3. Verification

* **Search:** `LIKE` path keeps SQLite dev/test green; PG path uses `plainto_tsquery('english', $1)` + `ts_rank` (tested via `EXPLAIN` on PG with `idx_*_tsv` GIN). Manual: `q=healing` finds both a `Healing` category and a `I am healed` confession (tag `healing`), ranked `title(A)` above `theme(C)`.
* **Home:** `GET /v1/home` returns 6 sections; disabling `UPDATE feature_flags SET enabled=false WHERE key='home_trending'` hides `trending` (without code change). `recommendations_enabled=false` hides `for_you`.
* **Recommendations:** `Rank` is pure — same `Candidate` list + `Profile.Now` yields same order; `popular_score` and `recency` are observable in `EXPLAIN`. Fresh content deprioritized only if recently completed (`-5`) per spec.

---

## 4. Workspace budget

* Search tsvector is PG-only; SQLite fallback means no extra deps.
* Recommendations is pure Go, no model download.
* `feature_flags` was already in `0005`/`0006`; `0007` adds tsv + indexes (PG) and seeds `home_*` flags (SQLite `INSERT OR IGNORE` compatible via `DO` guard for PG).

---

## 5. Phase 5 closed — next is Phase 6 Premium & Offline

* **Option A (close Phase 5):** `POST /v1/sessions/preview` dry-run, `user_templates` + shareable URL, `playback_progress` merge endpoint docs.
* **Option B (Phase 6 Premium & Offline):** `audio_downloads` licence (expiry/renewal/revocation), server-side Store/Play verification, regional pricing (NGN/USD/GBP/EUR/PHP), trials — the monetization path.

Say `next` to continue the next slice.
