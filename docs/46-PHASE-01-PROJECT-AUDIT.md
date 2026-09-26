# Ledger 46 — PHASE 01: Project Audit (web platform + marketing website)

**Date:** 2026-09-22
**Branch:** `arena/01a0c6ca-i-confess` (base `a5b0cce`)
**Scope:** audit before implementing the 79-route iCONFESS web platform +
marketing website against the reference wireframe spec.
**Method:** every claim below was produced by reading this checkout (no code
modified during this phase).

---

## 1. Repository shape

| Area | State |
|---|---|
| `server/` | Go 1.25 modular monolith (`github.com/Teamthy/i-confess`), 320 API routes in `design/routes.json`, 250 paths in `contracts/openapi.json`. No Go toolchain in this sandbox — verified via docs + route table, not by running. |
| `apps/web` | Next.js 15.5.25 + React 19 + TypeScript. **127 files, 70 page routes.** Rebuilt in PHASE 38 (ledger 45) against the PHASE 05 design system. `npm ci` + `next build` runnable here (baseline build in progress at time of writing). |
| `apps/mobile` | Flutter app (auth, home, explore, confess, activity, profile…). Out of scope for this phase except as the UX reference for `/app/*` parity. |
| `apps/admin` | Retired in-tree (`RETIREMENT.md`). Out of scope. |
| `design/` | `tokens.json` (single source of truth) → `design/generated/{tokens.css,tokens.ts,tokens.dart}`. Brand ramp `#EBF2FA → #00072D` matches the brief's palette exactly (`--brand-deep #00072D`, `--brand-navy #051650`, `--brand-blue #0A2472`, `--brand-blue-secondary #123499`, `--brand-light #EBF2FA`). |
| `contracts/` | `openapi.json` — typed web client is written against it + `design/routes.json`. |
| `scripts/dev-api.mjs` | Dependency-free API fixture serving the REAL canonical seed (`scripts/dev-api-corpus.py` extracts it from `server/internal/seed/*_canonical.go`). Preview/dev use only. |

## 2. Content sources of record (do not re-invent)

- **39 categories:** `server/internal/seed/categories_canonical.go` — exactly 39
  entries, test-enforced (`categories_canonical_test.go`). Served by
  `GET /categories` (name/slug/description/icon only — taglines NOT persisted;
  recorded backend gap, ledger 44 §5).
- **78 confessions:** `server/internal/seed/confessions_canonical.go`.
- **3 voices:** served by `GET /voices` (David, Grace, Faith; Faith premium).
  No `GET /voices/{id}` — detail pages resolve from the list payload.
- **Journal:** no articles API; typed local model in `apps/web/content/articles.ts`
  (5 launch essays, author `"iCONFESS"` — no invented writers).
- **Legal:** typed local model in `apps/web/content/legal.ts`.
- **Testimonials:** NONE exist in the repo. Per the brief's critical rules, the
  site must use editorial product statements, never fabricated quotes —
  the current homepage already does this correctly.

## 3. Existing `apps/web` route inventory (70 pages)

Public marketing (29): `/`, `/explore`, `/categories`, `/categories/[slug]`,
`/confessions`, `/confessions/[slug]`, `/sessions` (index only),
`/voices`, `/voices/[id]`, `/how-it-works`, `/premium`, `/pricing` (extra, live
plans), `/community`, `/about`, `/mission`, `/journal`, `/journal/[slug]`,
`/stories`, `/stories/[slug]`, `/download`, `/contact`, `/help`, `/faq`,
`/privacy`, `/terms`, `/cookies`, `/community-guidelines`, `/content-policy`,
`/t/[token]` (legacy share links).

Auth (7): `/login`, `/register`, `/forgot-password`, `/reset-password`,
`/verify-email`, `/verify-phone`, `/welcome`.

Authenticated `/app` (30): `/app`, `/app/explore`, `/app/categories`,
`/app/categories/[slug]`, `/app/confessions/[id]` (detail only),
`/app/sessions`, `/app/sessions/[id]`, `/app/session-builder`, `/app/player`,
`/app/voices`, `/app/voices/[id]`, `/app/history`, `/app/favorites`,
`/app/downloads`, `/app/routines`, `/app/notifications`, `/app/community`,
`/app/community/create`, `/app/community/[slug→id]`, `/app/profile`,
`/app/settings` + `account/privacy/notifications/playback/accessibility`,
`/app/subscription`, `/app/subscription/manage`, `/app/search`,
`/app/recommendations`, `/app/daily`, `/app/streaks`, `/app/achievements`,
`/app/shared/[token]`, `/app/invite`, `/app/referrals`, `/app/feedback`,
`/app/support`.

System (4): `not-found.tsx` (404), `error.tsx` (500 boundary),
`/maintenance`, `/offline`.

Plus: `/api/*` + `/media/*` rewrites proxying to the Go API, `sitemap.ts`,
`robots.ts`, JSON-LD structured data, per-page metadata/OpenGraph/canonical.

## 4. Gap analysis vs the 79-route brief

### 4a. Missing routes (3)

| # | Route | Evidence of need |
|---|---|---|
| 08 | `/sessions/[slug]` (public session detail) | Only the index exists. The brief requires a public session detail page. Backend: sessions are user-scoped (`POST /sessions` + `GET /sessions/{id}`); a public detail page must therefore be an honest preview/join wall, not a fake public session. |
| 40 | `/app/confessions` (library index) | Only `/app/confessions/[id]` exists. **The mobile bottom tab "Create" links to `/app/confessions` today → 404 in production.** P0 broken link. The page must combine library + "write your own" entry. |
| 51 | `/app/schedule` (scheduled sessions) | No page. Backend has `GET/POST /schedules`. Routines (`/app/routines`) exists; schedule is the time-based companion. |

79 − 3 missing + 2 extras (`/pricing`, `/t/[token]`) = 70 + 3 = **73 brief routes + 2 extras = 75 pages** after this phase. (The brief double-counts `/app/settings/*` structure; all six settings routes exist.)

### 4b. Reference-fidelity gaps (homepage + visual system)

The current homepage is a solid 16-section editorial page, but measured
against the brief's textual reconstruction of the reference:

| Reference motif (§54) | Current state | Verdict |
|---|---|---|
| Minimal header | `SiteHeader`: transparent→solid, drawer, restrained | ✅ keep |
| Editorial hero + large image + compact CTAs | Ink hero, text + confession card, **no image** | ❌ rebuild |
| Floating cards overlapping hero | None | ❌ build |
| Story, asymmetric | "Idea" section is 3-column, symmetric | ❌ rebuild |
| Dark rounded category section + horizontal cards | Bookcase rail on light bg; ink band is separate | ❌ rebuild |
| Principles, four cards | None | ❌ build |
| Large image + floating card | None — **zero photography on the whole site** (`public/` does not exist, no `next/image` usage) | ❌ build + image system |
| People/voices horizontal cards | 3-col grid, no audio preview | ⚠️ strengthen |
| Community block | Exists, simple split | ⚠️ strengthen |
| Testimonial / editorial statement | Honest library quotation, no fabrication | ✅ keep, restyle to two-column |
| Journal cards | **Absent from homepage** | ❌ build |
| FAQ two-column | Single column | ❌ rebuild |
| Newsletter dark block + image | Absent | ❌ build |
| Download/app CTA | Exists | ✅ keep, restyle |
| Final CTA + structured footer | Exist | ✅ keep |

### 4c. Deliberate non-gaps (brief asks, repo already satisfies)

- Design tokens (§34): implemented + generated + tested. Radius scale is
  `8/12/16/20/999` — the brief's `24/32` steps do not exist; large sections
  use `xl (20px)`. Documented exception, not a second system.
- Audio (§31): `PlayerClient` (583 lines) with loading/playing/paused/
  completed/error states, seek/speed/volume/save/share; signed `/media/*`
  proxy; `dev-api` serves generated silence so real code paths execute.
- Auth (§16/§50): login/register/reset/verify wired to real endpoints via the
  same-origin proxy; MFA-aware; no account enumeration; secure headers;
  server-side RBAC.
- SEO (§47), a11y (§48), analytics events (§51 — mobile + API batch endpoint;
  web fires page/CTA events), error states (§52 — every data page has
  loading/empty/error), motion + `prefers-reduced-motion` (§38).
- iCONFESS content only; no reference-brand copying possible — there is no
  reference brand in the repo (§61).

## 5. Dependencies & deployment

- Web deps: `next`, `react`, `react-dom`, `@fontsource-variable/inter`,
  `@fontsource/source-serif-4` only. No Tailwind, no framer-motion, no
  shadcn — the site uses hand-written token-driven CSS (`styles/site.css`,
  ~2,300 lines). The brief's stack is "preferred", not mandatory; introducing
  Tailwind now would fork the visual system. **Decision: keep the token CSS
  architecture.**
- Deployment: `Dockerfile` + `docker-compose.yml` (server), Vercel-ready
  Next.js app (`IC_API_URL` for the proxy target). No microservices added.

## 6. Risks

1. **Imagery.** The reference language is photography-led; the repo has no
   image pipeline or assets. Mitigation: generate a small set of editorial
   images (hero, immersive, community, newsletter, download) + CSS/SVG motif
   system for the 39 category cards. No hotlinked stock (no external
   dependency, no licensing risk).
2. **Public session detail.** Sessions are private by design; page 08 must be
   an honest preview, not a leak. Mitigation: preview + auth wall.
3. **Build time.** Full `next build` + 39-category SSG in sandbox takes
   minutes; verify incrementally (`tsc`, then build once per milestone).

---

## PHASE REPORT — PHASE 01

**PHASE:** 01 — PROJECT AUDIT
**STATUS:** COMPLETE

**COMPLETED:** Full inventory of routes (70), components (24), styles, tokens,
content sources, API surface, auth, gaps vs the 79-route brief.

**FILES CREATED:** `docs/46-PHASE-01-PROJECT-AUDIT.md` (this file)

**FILES MODIFIED:** none

**DESIGN DECISIONS:** none (audit only)

**TECHNICAL DECISIONS:**
- Keep token-driven hand-written CSS; do not introduce Tailwind/shadcn (would
  fork the tested design system).
- Imagery will be locally generated editorial images + deterministic
  CSS/SVG category motifs (no external stock dependency).

**TESTS:** `npm ci` clean; baseline `next build` running (result recorded in
Phase 02 report).

**KNOWN ISSUES:**
- P0: `/app/confessions` bottom-tab link 404s (page does not exist).
- P1: homepage does not follow the reference visual rhythm (no imagery,
  no overlapping cards, no dark category section, no principles/journal/
  newsletter sections, single-column FAQ).

**NEXT PHASE:** 02 — REFERENCE DECOMPOSITION + 03 IA + 04 JOURNEYS + 05 TOKENS
(combined into one planning document, since tokens already exist), then build:
missing routes → homepage rebuild → image system → QA.
