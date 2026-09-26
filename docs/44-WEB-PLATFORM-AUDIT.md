# Ledger 44 — Web Platform Audit (master-plan PHASE 38 entry gate)

**Date:** 2026-09-21
**Branch:** `arena/01a0c442-i-confess` (base `48dc2af`)
**Scope:** audit before implementation of master-plan PHASE 38 (public marketing
website), PHASE 39 (authenticated web), PHASE 40 (admin command centre), and
PHASE 41 (content studio).

Every claim below was produced by running something against this checkout: the
backend test suite, a live server, and the live route table.

---

## 1. What already works (verified)

| Claim | Evidence |
|---|---|
| Backend builds and the full suite passes | `go build ./...`; `go test -count=1 ./...` → 35 packages ok, zero FAIL |
| API serves the real catalogue | `GET /categories` → 39 published categories with slugs; `GET /voices` → 3 active voices (David, Grace, Faith; Faith premium) |
| Canonical corpus is real | seed log: `created 39 categories, 78 confessions, 1 voice, demo admin + user`, 78 theological reviews, 312 audio assets |
| Audio delivery is signed | `POST /sessions` returns `audio_url` as a relative `/media/...?exp=…&sig=…`; `HTTP 200` when fetched |
| Session engine works end to end | built a 5-minute Peace session for the demo user; queue items carry text, variant, voice, signed audio |
| Auth works, MFA-ready | `POST /auth/login` returns token + user; MFA, refresh, sessions, password reset endpoints all present |
| RBAC is server-side | demo user → `GET /admin/stats` = **403**; `RequireRoleWithSessions` re-reads role from the DB per request; roles: `super_admin`, `admin`, `audio_producer`, `voice_manager` |
| Admin surface exists | `/admin/stats`, `/admin/queue`, `/admin/audit`, `/admin/moderation/queue`, `/admin/confessions*`, `/admin/categories*`, `/admin/voices*`, `/admin/audio*`, `/admin/plans`, `/admin/users/*` — 38 admin routes |
| Design system exists and is tested | `design/tokens.json` (120 tokens), `make design-check` (generate + section-12 tests + IA tests) passes |
| Content lifecycle is enforced | `internal/content` forward-only graph `draft → content_review → theological_review → audio_production → audio_qa → approved → published → deprecated|archived`; illegal PATCH = 409 |
| Moderation is real | UGC states draft→submitted→approved/rejected; appeals; audit sink; published UGC public reader `GET /community/confessions` |
| Trial journey exists | `GET /subscriptions/trial` Day 1–7 with intents; engagement reader |
| Plans are config-driven | `GET /subscriptions/plans` returns 5 currencies; no prices hardcoded in clients |

Demo accounts (dev seed): `admin@iconfess.dev` / `Admin!ChangeMe-2026`
(super_admin), `demo@iconfess.dev` / `Demo!ChangeMe-2026`.

## 2. What is incomplete or absent

| Area | State |
|---|---|
| `apps/web` | 694 lines total, **retired in-tree** (`apps/web/RETIREMENT.md`). No `package.json` — it has never been installed or built in this checkout. 5 dead links documented in PHASE 06. Not the product. |
| `apps/admin` | 168 lines, retired in-tree (`apps/admin/RETIREMENT.md`). Same status. |
| Embedded SPAs | `server/internal/webapp` (listener SPA served at `/`) and `server/internal/adminui` (console at `/admin/`) — vanilla JS, own off-token visual system, no tests. The Go binary serves them; they remain the fallback surfaces and are **not** extended here. |
| PHASE 45 commit `0f6879e` | **Absent.** `git cat-file -t 0f6879e` fails; `docs/HANDOFF-AFTER-43.md` absent. No subscription cancellation endpoint exists (`grep cancel design/routes.json` → nothing). Verified, not assumed. |
| Public voices detail | `GET /voices` exists; **no** `GET /voices/{id}`. Voice profile pages must resolve from the list payload. |
| Confession slugs | Categories have slugs; confessions/voices are UUID-keyed only. Clean URLs limited to `/categories/{slug}` until the backend grows a slug registry. |
| Category taglines | The seed of record (`categories_canonical.go`) defines second-person taglines "shown on the website's category rail", but **only** name/slug/description/icon are persisted and served. The site uses API `description`; exposing `tagline` is recorded as a backend gap, not hardcoded client-side. |
| Journal / articles | No articles API, no articles table. Journal pages are served from typed local content models (§70: content separated from presentation) until a content API exists. |
| FAQs | Same: no FAQ API; FAQ content is a typed local content model. |
| Admin users list | No `GET /admin/users` (only `/admin/users/admins`). Users module needs a real list → backend endpoint added in PHASE 40. |
| Admin sessions/subscriptions/analytics surfaces | No admin routes for cross-account session insight, subscription ledger, or analytics aggregation (only `/admin/metrics` security counters + `/admin/stats` totals). Added in PHASE 40. |
| Scripture surface | Scripture refs exist on confessions (`scriptures` array, full model) but have no dedicated admin browse surface. Added in PHASE 40/41 on top of the confession endpoints. |
| Homepage content management | No CMS endpoints for homepage sections; not fabricated — documented boundary. |

## 3. What can be reused

- **Design tokens** (`design/generated/tokens.css|ts`) — consumed directly by the
  new web app. No second visual system.
- **The route table** (`design/routes.json`, 320 routes) and
  `contracts/openapi.json` — the typed web client is written against these.
- **The webapp SPA's API choreography** (login → /me → /home → sessions) — lifted
  as reference behaviour, reimplemented properly in TypeScript.
- **The Go server as API + dev origin** — signed audio, seeding, auth, RBAC all
  work today; the Next.js apps proxy `/api/*` and `/media/*` to it.
- **`scripts/sandbox-bootstrap.sh`** — reproducible Go 1.27 + PostgreSQL 17 in
  this sandbox; used to run the suite and the live server.
- **Demo accounts** for QA of auth, entitlement, and RBAC states.

## 4. What must be replaced

- `apps/web` page components (all of them; nothing survives except the
  security-headers config and the sitemap/robots idea, both rewritten).
- `apps/admin` page components (both files).
- Nothing in `server/` is replaced; the server only gains endpoints.

## 5. Missing API contracts (and the decision for each)

| Need | Decision |
|---|---|
| Admin users list | **Implement** `GET /admin/users` (paginated, search) in PHASE 40, audited surface, tests, route export, OpenAPI regen |
| Admin sessions insight | **Implement** `GET /admin/sessions` (live sessions by state) in PHASE 40 |
| Admin subscriptions ledger | **Implement** `GET /admin/subscriptions` in PHASE 40 |
| Admin analytics | **Implement** `GET /admin/analytics` (event rollups already persisted by `analytics_events`) in PHASE 40 |
| Subscription cancellation (PHASE 45 work, absent) | **Implement** `POST /subscriptions/cancel` in PHASE 39 with store semantics (end-of-period), audit + tests |
| Confession/voice slugs | **Boundary** — not added; UUID routes documented |
| Taglines on the wire | **Boundary** — site uses `description`; gap recorded |
| Articles/FAQs/homepage CMS | **Boundary** — typed local content models in the web app, marked as integration points |
| Public voice detail | **Boundary** — resolve from `GET /voices` list |

## 6. Missing pages (target inventory)

Public: `/`, `/explore`, `/categories`, `/categories/[slug]`,
`/confessions/[id]`, `/sessions`, `/voices`, `/voices/[id]`, `/how-it-works`,
`/premium`, `/community`, `/about`, `/journal`, `/journal/[slug]`, `/download`,
`/contact`, `/privacy`, `/terms`, `/cookies`, `/community-guidelines`,
`/login`, `/register`, `/forgot-password`, `/reset-password`, `/verify-email`.

Authenticated `/app`: home, explore, categories, category detail, confession,
builder, player, history, favorites, collections, schedules, notifications,
profile, settings, subscription, community.

Admin: dashboard, users, categories, confessions (studio), scriptures, voices,
audio, sessions, moderation, subscriptions, analytics, system.

System: `not-found` (404) and `error` (500) surfaces in both apps; offline
guidance on the player.

## 7. Missing states

The retired scaffolds render only success paths. The replacement builds, for
every data surface: skeleton loading matched to layout, empty states with a
next action, human-readable error states with retry, success feedback (toasts),
unauthorized/forbidden states that never reveal which role would suffice, and
offline/reduced-motion variants.

## 8. Accessibility / SEO / performance / security gaps

- Retired scaffolds: no focus management, no skip links, client-only fetching
  (spinner-first pages, no SSR content for SEO), metadata duplication, dead
  links. All replaced: server components fetch content directly, semantic
  landmarks, WCAG 2.2 AA contrast from the existing token pairs,
  `prefers-reduced-motion` respected, per-page metadata + OG/Twitter, canonical
  URLs, `robots.ts` (private/unpublished content never indexed — confession
  pages indexable only for published canonical items), `sitemap.ts` driven by
  the live category list, JSON-LD on home/category/confession.
- Security: tokens held client-side (the API is Bearer-based); the web app keeps
  tokens out of URLs, never renders raw HTML from data, proxies API/media
  same-origin, keeps the security headers from the retired config, and treats
  the server as the only authority on entitlement/roles.

## 9. Ordered implementation plan (master-plan mapping)

| Repo ledger | Master-plan | Deliverable |
|---|---|---|
| 45 | PHASE 38 | `apps/web` rebuilt: design system, header/footer/buttons/cards/forms/feedback components, category signature interaction, 16-section homepage, all 20 public pages, SEO, 404/500 |
| 46 | PHASE 39 | Auth pages + `/app` shell + home/explore/confession/builder/player/history/favorites/schedules/profile/settings/subscription/community, server-authoritative entitlements, `POST /subscriptions/cancel` |
| 47 | PHASE 40 | `apps/admin` command centre, 12 modules, backend `GET /admin/users|sessions|subscriptions|analytics`, RBAC against the live server |
| 48 | PHASE 41 | Content studio: lifecycle-aware confession editor, scripture, variants, audio production/QA, moderation review, versioning, visibility, featured |

Each phase: implement → `make verify` subset that can run here (build, vet,
design-check, IA tests, Go tests with the sandbox toolchain; Flutter/dart-analyze
not runnable — `check_dart_symbols.py` stands in per repo policy) → live preview
QA (desktop + mobile) → phase doc → `PROJECT-STATUS.md` entry → one commit →
push → CI watch.
