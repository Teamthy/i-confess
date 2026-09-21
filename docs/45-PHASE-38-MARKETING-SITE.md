# Ledger 45 — PHASE 38: the public marketing website

**Date:** 2026-09-21
**Branch:** `arena/01a0c442-i-confess`
**Depends on:** Ledger 44 (`docs/44-WEB-PLATFORM-AUDIT.md`), PHASE 05 design
system, the PHASE 43 API surface (320 routes, unchanged here).
**Build order mapping:** implementation steps 1–22 plus the auth public
screens of step 23 (login/register/forgot/reset/verify), all under master-plan
PHASE 38. The authenticated `/app` experience (steps 24–33) is PHASE 39 /
ledger 46.

---

## 1. What was built

`apps/web` was rebuilt in place against the PHASE 05 design system and the
live Go API. The retired scaffold (694 lines, `RETIREMENT.md`, five dead
links, an off-token indigo visual system) is replaced by a 30-file App Router
application in which every colour, radius, spacing step, duration and font
family comes from `design/generated/tokens.css`.

### The homepage journey (PAUSE → FEEL → UNDERSTAND → EXPERIENCE → BELIEVE → BEGIN)

All 12 required sections exist, plus the supporting sections from the longer
brief, in one editorial flow:

| # | Section | Source of content |
|---|---|---|
| 01 | Navigation (transparent→solid, mobile drawer) | component |
| 02 | Hero with a real confession card | `GET /categories/{id}/confessions` (Peace) — real copy, never invented |
| 03 | The idea — Speak / Hear / Repeat | editorial |
| 04 | Product experience frame | mirrors a real session shape; clearly a preview, not a fake screenshot |
| 05 | 39 areas of life — the signature bookcase rail | `GET /categories` |
| 06 | Featured category band (Peace) | `GET /categories` + confessions |
| 07 | Daily ritual (Morning/Midday/Night) | editorial |
| 08 | Voice experience | `GET /voices` — the three real voices |
| 09 | How it works (01–04) | editorial |
| 10 | Personalization | capability description; no fabricated AI claims |
| 11 | Community (Private/Shared/Community) | describes the real UGC states |
| 12 | Premium (ink band) | links to live pricing |
| 13 | Social proof — a real confession quotation + library facts; **no fabricated testimonials, counts or ratings** | `GET /confessions` |
| 14 | App download | honest state: store listings pending, no fake badges |
| 15 | FAQ (10 questions) | editorial, consistent with real product behaviour |
| 16 | Final CTA | — |
| 17 | Footer (product/company/legal) | all 22 links resolve |

### The signature category interaction

Desktop: an editorial *bookcase* — one panel open at full height, neighbours
standing as vertical spines. Activation is click, Enter/Space, or focus-through
(focus moves → panel opens); arrow keys walk, Home/End jump. Hover *previews*
by opening but every action is reachable without hover. The open panel carries
the category name, the API's description, a Premium flag where true, and a link
to `/categories/{slug}`.

Mobile/tablet (<64rem): the bookcase becomes a horizontal snap carousel with
the next card peeking, `-webkit-overflow-scrolling: touch`, and visible
scrollbar affordance.

Per-category colour is a deterministic pure function of the slug over the
token ramp (`lib/categoryColor.ts`) — identifiable, never a second palette.

### Public route inventory (all 20 + auth + system)

`/`, `/explore`, `/categories`, `/categories/[slug]` (39 SSG pages from the
live API), `/confessions/[id]` (404s correctly for unknown/private IDs —
the public endpoint can only serve published canonical content), `/sessions`,
`/voices`, `/voices/[id]`, `/how-it-works`, `/premium`, `/pricing` (live
plans), `/community` (real published-UGC reader with empty state), `/about`,
`/journal` + `/journal/[slug]` (5 launch essays), `/download`, `/contact`,
`/privacy`, `/terms`, `/cookies`, `/community-guidelines`, `/login`,
`/register`, `/forgot-password`, `/reset-password`, `/verify-email`,
`t/[token]` (rebuilt), `not-found` (404), `error` (500 boundary).

### Auth screens

Wired to the real endpoints through the same-origin `/api` proxy:
`POST /auth/login` (with the server's MFA-aware behaviour surfaced honestly),
`POST /auth/register` (verification state), `POST /auth/request-password-reset`
(identical response whether or not the account exists — no account
enumeration), `POST /auth/reset-password`, `POST /auth/verify-email`. Full
loading/disabled/error/success states on every form.

## 2. Design system

- `styles/tokens.css` is a **symlink** to `design/generated/tokens.css` — the
  site consumes the generated tokens in place; drift is impossible by
  construction.
- `styles/globals.css`: reset, focus-visible rings (adjusted on ink surfaces),
  skip link, visually-hidden, global `prefers-reduced-motion` suppression.
- `styles/site.css`: the marketing component library (~1,100 lines) — layout
  containers, ink/mist/paper surfaces, type roles, buttons (primary /
  on-dark / secondary / text / small), header + drawer, hero, bookcase rail,
  carousel + category cards, confession/voice cards, steps, ritual slots,
  product frame, FAQ, forms with error/hint/disabled states, skeletons,
  empty/error states, prose, footer, page heroes.
- Web type ramp (`--ic-web-display-hero` etc.) extends the seven token roles
  for a large canvas using the same families — documented exception, not a
  second system. Gold (`--ic-color-accent-gold`) is used only for the premium
  chip, per the token's own rule.

## 3. Brand assets — recorded finding

The repository contains **no approved iCONFESS logo**. Both mobile app icons
are the unmodified Flutter template icon (Google's Flutter trademark; verified
by inspection). Using them would place another company's mark on the site, so
per the build rules the site ships a restrained, intentional fallback: a
typographic `I CONFESS` wordmark with a three-bar "spoken word" glyph drawn
from the tokens (`components/Wordmark.tsx`). This is explicitly a fallback,
not a redesign; the component is the single file to replace when the approved
logo lands.

## 4. SEO, accessibility, performance, security

- Per-page metadata, canonical URLs, Open Graph and Twitter cards; JSON-LD
  (WebSite, CollectionPage per category, Article per confession, BlogPosting
  per journal entry).
- `robots.txt` disallows `/app/`, `/admin/`, `/api/`, `/t/`, verify/reset
  pages; `sitemap.xml` is generated from the live category list plus static
  routes. Confession detail pages are indexable *only* if the public API
  serves them — private/UGC content is unreachable by construction.
- Dynamic routes return real HTTP 404s (the root `loading.tsx` was removed
  after QA showed it streamed 200 shells before `notFound()` resolved — the
  failure mode the IA tests warn about, caught live).
- DOM-level QA across 20 pages: exactly one `h1` each, semantic landmarks,
  skip link, `lang`, alt/aria text on all interactive elements, no dead
  headings.
- First Load JS ≈ 103–109 kB across the site; most routes are static or ISR;
  fonts are self-hosted (`@fontsource`, no external requests); no image assets
  shipped (the visual system is typographic and token-colour based).
- Security: API origin stays server-side (`IC_API_URL`, never
  `NEXT_PUBLIC_`); browser code uses same-origin `/api` + `/media` rewrites;
  security headers (nosniff, DENY, referrer, permissions) retained from the
  retired config; no `dangerouslySetInnerHTML` on user data (JSON-LD only,
  server-controlled shapes).

## 5. Proving commands

| Check | Result |
|---|---|
| `npx tsc --noEmit` | clean |
| `npm run build` (against live API) | 71 routes built, 0 errors |
| DOM audit script (20 pages × 12 checks) | all pass |
| HTTP matrix incl. 404s | correct statuses |
| `python3 design/test_ia.py` | PASSED (38 screens, 113 endpoints) |
| `python3 design/test_design.py` | PASSED (120 tokens, §12 holds) |
| `design/generate.py --check` | up to date |
| `gofmt -l` + `go vet ./...` (sandbox modfile) | clean |
| `go test ./internal/api ./internal/seed` | ok |
| `scripts/check_dart_symbols.py` | 133/133 |
| Same-origin auth proxy (`POST /api/auth/login`) | 200 + token |
| Live preview desktop/mobile | served on 0.0.0.0:3000; browser screenshot QA **not runnable** — Chrome CDNs unreachable from this sandbox; DOM audit stands in, visual browser QA owed to CI/local |

## 6. Integration boundaries recorded

1. Voice detail resolves from `GET /voices` (no `GET /voices/{id}` yet).
2. Category `tagline` (seed of record) is not served by the API; the site
   renders the served `description` and hardcodes nothing.
3. Journal + legal content are typed local models (`content/`), shaped for
   the future articles API.
4. Store listings pending → `/download` shows honest placeholders, driven by
   `NEXT_PUBLIC_APP_STORE_URL` / `NEXT_PUBLIC_PLAY_STORE_URL` when configured.
5. Contact posts to `/api/support/contact`; the endpoint does not exist yet,
   and the form's error state says exactly that.

## 7. Phase report

**PHASE:** 38 — Public marketing website
**STATUS:** COMPLETE (verified; browser screenshot QA owed to CI)

**COMPLETED:** audit (ledger 44); design system consumption via token
symlink; header/drawer/footer/buttons/cards/forms/feedback components;
signature category bookcase + mobile carousel; 16-section homepage on the
PAUSE→BEGIN journey; all 20 public pages; 5 auth screens on the real API;
SEO (metadata/OG/Twitter/canonical/robots/sitemap/JSON-LD); real 404/500
states; DOM a11y QA; live preview serving on 0.0.0.0.

**FILES CREATED:** see commit — `apps/web/{app,components,content,lib,styles}`,
`docs/44-WEB-PLATFORM-AUDIT.md`, this document, ledger entry, `.gitignore`
web rules.

**FILES MODIFIED:** `apps/web/{package.json,tsconfig.json,next.config.mjs}`,
`apps/web/app/{page,robots,sitemap}.*`, `apps/web/app/pricing/page.tsx`,
`apps/web/app/t/[token]/page.tsx`, `.gitignore`, `docs/PROJECT-STATUS.md`.

**DESIGN DECISIONS:** ink/mist/paper surface rhythm from the token neutrals;
green reserved for action and identity, gold strictly premium; serif display
type for editorial voice; bookcase rail over card grid; wordmark fallback for
the missing logo; no fabricated imagery anywhere.

**TECHNICAL DECISIONS:** App Router server components fetch the API directly
(ISR); same-origin proxy for browser code; symlinked generated tokens; typed
`ApiResult` envelope so pages render real error states; removed root
`loading.tsx` to preserve true 404 statuses.

**TESTS:** build + typecheck + DOM audit + repository design/IA/Go/dart-symbol
gates as tabled above.

**ISSUES:** browser screenshot QA not runnable (CDN-blocked sandbox); store
badges pending listings; voice detail/tagline/contact-endpoint boundaries
recorded.

**NEXT PHASE:** ledger 46 — PHASE 39, the authenticated web experience
(`/app` shell, player, builder, history, favourites, schedules, profile,
settings, subscription with server-side cancellation).
