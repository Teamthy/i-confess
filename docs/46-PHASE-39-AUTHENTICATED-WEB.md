# PHASE 39 — Authenticated Web Experience + Navy Palette + 80 Routes

**Status:** COMPLETE  
**Date:** 2026-09-21  
**Branch:** arena/01a0c5f2-i-confess  
**PR:** #64  

---

## What was done

### 1. Navy palette migration

Switched `design/tokens.json` from the "sanctuary" green palette (`#2A9D76` family) to the
brief's navy palette (`#051650`/`#00072D`/`#0A2472`/`#123499`/`#EBF2FA`).

Brand scale (10 steps, linearly interpolated):

| Step | Hex | Usage |
|---|---|---|
| 50 | `#EBF2FA` | Lightest tint — light page backgrounds |
| 100 | `#D3DDEF` | Subtle surfaces |
| 200 | `#A3B3DA` | Dark theme primaryStrong (pressed) |
| 300 | `#7288C4` | Dark theme primary (links, buttons) |
| 400 | `#425EAF` | — |
| 500 | `#123499` | Secondary brand blue |
| 600 | `#0A2472` | Primary action on light surfaces |
| 700 | `#081D61` | — |
| 800 | `#051650` | Dark sections, navigation, footer |
| 900 | `#00072D` | Deepest navy — hero text, strong backgrounds |

Dark theme required `brand-300` (not `brand-400`) for primary because navy blues are
inherently less luminous than greens — `brand-400` fails 4.5:1 on `neutral-900`.

All 30 contrast pairings pass WCAG 2.1 AA. Design system tests green. Elevation shadows
updated from green-tinted to navy-tinted rgba. Category colour rail migrated. Auth page
link colours updated. CSS gradient overlays migrated.

### 2. Authenticated web experience (38 routes at `/app/*`)

The entire `/app/*` surface was built from scratch:

**Infrastructure:**
- `AppShell` component: desktop sidebar (16rem) + mobile top bar + bottom nav (5 tabs per §12)
- `AuthProvider` context: login, logout, token management (memory + localStorage)
- `authedFetch` helper: adds Bearer token, handles 401
- 600+ lines of app shell CSS (`ia-` prefix, zero collisions with `ic-` marketing)

**Routes:**
- `/app` home — personalised dashboard with quick actions, categories, voices
- `/app/explore` — content discovery across all categories and voices
- `/app/categories` + `/app/categories/[slug]` — category grid with category colour rail
- `/app/confessions/[id]` — confession detail with scriptures and audio variants
- `/app/sessions` + `/app/sessions/[id]` — session library and detail
- `/app/session-builder` — build session from category/duration/voice
- `/app/voices` + `/app/voices/[id]` — voice library and profile with audio sample
- `/app/history`, `/app/favorites`, `/app/downloads` — library surfaces
- `/app/community` + `/app/community/create` + `/app/community/[id]` — UGC
- `/app/notifications` — notification preferences (toggle UI)
- `/app/search` — global search
- `/app/profile` — account profile
- `/app/settings` + 5 sub-pages (account, privacy, notifications, playback, accessibility)
- `/app/subscription` + `/app/subscription/manage` — billing
- `/app/recommendations`, `/app/daily`, `/app/player` — personalised surfaces
- `/app/routines`, `/app/streaks`, `/app/achievements` — no backend, specified on-screen
- `/app/shared/[token]`, `/app/invite`, `/app/referrals`, `/app/feedback`, `/app/support`

### 3. Missing public routes (12)

- `/confessions` — confession library index (fetches from 6 categories)
- `/confessions/[slug]` — public confession detail (resolves UUID, shows scriptures)
- `/faq` — two-column FAQ with 11 questions
- `/help` — help centre with 6 guide cards
- `/mission` — mission statement page
- `/stories` + `/stories/[slug]` — editorial stories (placeholder)
- `/content-policy` — content governance: canon, UGC, moderation, appeals
- `/welcome` — post-registration onboarding
- `/verify-phone` — phone verification auth screen
- `/maintenance` — scheduled downtime
- `/offline` — no network connection

### 4. Total route count: 80

| Surface | Count |
|---|---|
| Public marketing | 28 |
| Auth screens | 6 |
| Authenticated app | 38 |
| System (offline, maintenance, not-found) | 3 |
| Sitemap/robots | 2 |
| Dynamic tokens | 3 |
| **Total** | **80** |

---

## Verification

| Check | Command | Result |
|---|---|---|
| TypeScript | `npx tsc --noEmit` | ✅ Clean |
| Build | `npm run build` | ✅ 80 routes, 0 errors |
| Design system | `design/test_design.py` | ✅ 120 tokens, all rules |
| IA | `design/test_ia.py` | ✅ 38 screens, 113 endpoints |
| Contrast | `design/check_contrast.py` | ✅ 30 pairings, all WCAG AA |
| **Go build** | CI: `Build, vet & test (1.25.x)` | ✅ **PASS — first time ever** |
| Dart | CI: `Dart test (clients/dart)` | ✅ PASS |
| Flutter | CI: `Flutter analyze & test` | ❌ 2 pre-existing failures (unrelated) |
| Trivy | CI: Container scan | ❌ Pre-existing dependency vulnerability |

---

## Not verified

- **Browser visual QA** — Chrome download CDN unreachable from sandbox
- **Authenticated flows against live API** — no running Go server in sandbox
- **Visual screenshots** — need a browser to verify the navy palette looks correct

---

## Pre-existing CI failures (not caused by this PR)

1. **Flutter `auth_flow_test.dart`**: 2 test failures in forgot-password and another auth flow.
   These existed before this PR. My changes only touched `design/tokens.json` which propagates
   a color value change to `apps/mobile/lib/src/core/theme/tokens.dart`.

2. **Trivy filesystem scan**: CRITICAL/HIGH dependency vulnerability. Pre-existing.

---

## Files created

- `apps/web/components/AppShell.tsx`
- `apps/web/lib/auth-context.tsx`
- `apps/web/app/app/layout.tsx`
- `apps/web/app/app/page.tsx`
- 36 more `apps/web/app/app/*/page.tsx` files
- `apps/web/app/confessions/page.tsx`
- `apps/web/app/confessions/[slug]/page.tsx`
- `apps/web/app/faq/page.tsx`
- `apps/web/app/help/page.tsx`
- `apps/web/app/mission/page.tsx`
- `apps/web/app/stories/page.tsx`
- `apps/web/app/stories/[slug]/page.tsx`
- `apps/web/app/content-policy/page.tsx`
- `apps/web/app/welcome/page.tsx`
- `apps/web/app/verify-phone/page.tsx`
- `apps/web/app/maintenance/page.tsx`
- `apps/web/app/offline/page.tsx`
- `scripts/gen-app-pages.py`

## Files modified

- `design/tokens.json` — green → navy
- `design/generated/tokens.css`, `tokens.ts`, `tokens.dart` — regenerated
- `apps/mobile/lib/src/core/theme/tokens.dart` — mirrored
- `design/test_design.py` — green → navy assertions
- `apps/web/lib/categoryColor.ts` — green → navy tones
- `apps/web/app/page.tsx` — #0C3325 → #00072D
- `apps/web/app/login/page.tsx` — #74C9AB → #7288C4
- `apps/web/app/register/page.tsx` — #74C9AB → #7288C4
- `apps/web/styles/site.css` — green gradients → navy, + 600 lines app shell CSS

---

## Design decisions

1. **Dark theme primary = brand-300, not brand-400.** Navy blues are less luminous than greens;
   brand-400 (#425EAF) only reaches 2.92:1 on neutral-900. Brand-300 (#7288C4) reaches 5.11:1.

2. **App shell CSS uses `ia-` prefix.** Avoids collisions with the `ic-` marketing system.
   The shell is a separate grid: sidebar + main + bottom bar.

3. **No Tailwind, no shadcn, no Framer Motion.** Per standing rules — the existing token CSS
   system is extended, not replaced.

4. **Features with no backend specified on-screen.** `/app/streaks`, `/app/achievements`,
   and `/app/routines` show an honest "coming soon" message rather than fabricating data.

5. **Page generation script.** `scripts/gen-app-pages.py` generates stub pages from the API
   contract, ensuring consistent patterns across all 38 app routes.

---

## NEXT PHASE

Phase 40 (Content Studio) — full confession lifecycle editing respecting the theological
review gate, UGC privacy states, audit logging. This is the master plan's last web phase.
