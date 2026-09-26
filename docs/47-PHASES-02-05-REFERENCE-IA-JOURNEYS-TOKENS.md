# Ledger 47 — PHASES 02–05: Reference decomposition, IA, journeys, tokens

**Date:** 2026-09-22
**Branch:** `arena/01a0c6ca-i-confess`
**Depends on:** Ledger 46 (Phase 01 audit). Phases 02–05 are combined here
because the design tokens already exist and are tested — what remains is
planning decisions, not token implementation.

---

## PHASE 02 — Reference decomposition

The brief's textual reconstruction resolves to 12 structural motifs. Each maps
to an iCONFESS section + a reusable class/component (all additive to
`styles/site.css`; no existing class is restyled into a different meaning):

| # | Motif | iCONFESS section | Implementation |
|---|---|---|---|
| M1 | Minimal header 64–76px, transparent→solid | keep `SiteHeader` | exists; no change |
| M2 | Editorial hero: eyebrow + 2-line headline + paragraph + 2 compact CTAs + large human image | `EditorialHero` | **new**: light hero, `Speak it. Hear it. Live it.`, `next/image` hero visual |
| M3 | Floating cards overlapping hero image | `FloatingCards` | **new**: Today's Confession / Continue Listening / Featured Voice; `-margin-top` overlap; public = curated, authed = real data where available |
| M4 | Asymmetric story block | Philosophy | **new**: left eyebrow/headline/para/CTA, right product metadata card |
| M5 | Large dark rounded feature section + horizontal cards | Category ecosystem `#051650` | **new**: `ic-dark-panel` (radius xl, inset margin) + snap carousel of `CategoryCard` w/ visual motif |
| M6 | Four principle cards, one active-blue | Our approach | **new**: Intention / Presence / Repetition / Personal experience |
| M7 | Full-width image + floating white card | Immersive moment | **new**: 60–75% image + overlapping card |
| M8 | People/voice horizontal cards + audio preview | Voices | strengthen existing: horizontal rail + `sample_url` HTMLAudio preview |
| M9 | Image + text community block, 3 states | Community | **new**: Private/Shared/Public cards + create CTA |
| M10 | Two-column testimonial | Editorial statement | restyle existing honest quotation; **no fabricated testimonials** |
| M11 | Editorial resource cards | Journal | **new**: 3 `ArticleCard`s from `content/articles.ts` |
| M12 | Two-column FAQ + split newsletter (dark+image) + final CTA + structured footer | Closing rhythm | **new** FAQ grid + `Newsletter` (mailto-free: POST /contact interest? No — newsletter has no endpoint; use honest "coming with account" + link to register. See decision D3) |

Visual rhythm enforced on `/`: light hero → overlap cards → light story →
**dark panel** → light principles → image → light voices → mist community →
light statement → light journal → mist FAQ → split newsletter → light final CTA
→ **dark footer**. Alternation is structural (section classes), not decorative.

## PHASE 03 — Information architecture (79 routes)

Sitemap = ledger 46 §3 + 3 new pages:

- `/sessions/[slug]` — public session preview. Sessions are user-scoped, so
  this page is an honest preview: what a session is, the format/length/voice
  shape, sample confession from a category, CTA to build one (auth wall).
  Static params: a small set of curated "session starters" (one per flagship
  category: peace, healing, faith…) linking `POST /sessions` presets. No fake
  session IDs, no fabricated content.
- `/app/confessions` — personal library: `GET /me/confessions` list grouped
  by status (draft/private/pending/approved) + "Write" entry → existing
  `/app/community/create`. Fixes the P0 bottom-tab + create-confirmation 404s.
- `/app/schedule` — `GET/POST/PATCH/DELETE /schedules` + `POST …/start`:
  list with enable toggle, create form (label, HH:MM, days, duration,
  voice, categories), delete. Timezone defaults to browser zone.

Full sitemap table (73 brief pages + `/pricing`, `/t/[token]` extras):

Public 28 + auth 7 + app 30 + additional 10 + system 4 — see ledger 46 §3 for
the per-route list; all resolve after this phase. No other new routes: the
brief's count is met exactly (brief §26 lists `/app/settings` once and its
five children separately; all six exist).

## PHASE 04 — User journeys (acceptance bar per journey)

1. **Visitor → understands → explores:** `/` → `/categories` → category →
   confession → plays sample (public previews only) → `/premium`.
2. **Visitor → account:** any CTA → `/register` → `/verify-email` →
   `/welcome` → `/app`. Every form: loading/disabled/error/success.
3. **Returning user:** `/login` → `/app` (continue listening, today's
   experience) → `/app/player` → complete → `/app/history` + `/app/streaks`.
4. **Premium user:** `/premium` → `/app/subscription` (plan, renewal,
   manage) → longer sessions + premium voices; 402 surfaces upgrade CTA,
   never a dead end.
5. **Creator:** `/app/confessions` → write → private saves always work →
   submit → moderation states visible → community.
6. **Scheduler:** `/app/schedule` → create 06:30 daily Peace → toggle/delete →
   notification prefs in `/app/settings/notifications`.

## PHASE 05 — Design tokens (verification, not implementation)

Tokens exist (`design/tokens.json` → generated CSS/TS/Dart, `make
design-check` green per ledger 44). This phase verifies web consumption:

- `apps/web/styles/tokens.css` re-exports generated tokens (checked: 160
  lines, `@import` of generated file — drift-impossible).
- Homepage rebuild uses ONLY token colors/spacing/radii/type + one documented
  exception: large editorial imagery (new asset class, no token impact).
- Radius: no `24/32` steps exist; dark panel + image containers use `xl
  (20px)` per token scale. Image aspect ratios are layout, not tokens.
- Category visuals: deterministic pure function of slug → token-ramp
  gradient + SVG motif (`lib/categoryColor.ts` extended, never a 2nd palette).
- Motion: existing `Reveal` (fade/translate, `prefers-reduced-motion` off
  switch) reused; carousels are native scroll-snap (no JS animation lib).

---

## PHASE REPORTS — 02/03/04/05

**PHASES:** 02 REFERENCE DECOMPOSITION, 03 IA, 04 JOURNEYS, 05 TOKENS
**STATUS:** COMPLETE (planning; implementation follows in build phases)

**COMPLETED:** 12-motif decomposition table; sitemap delta (3 pages);
6 acceptance journeys; token-consumption verification.

**FILES CREATED:** `docs/47-PHASES-02-05-REFERENCE-IA-JOURNEYS-TOKENS.md`

**FILES MODIFIED:** none

**DESIGN DECISIONS:**
- D1: Rebuild homepage in place (`app/page.tsx`) reusing `SiteHeader`,
  `SiteFooter`, `Reveal`, `SectionHead`, token classes; add ~600 lines of
  motif CSS. No framework change.
- D2: Photography = 5 locally generated editorial images in
  `apps/web/public/images/` (hero, immersive, community, newsletter, voices
  band). Category cards = deterministic token-ramp motif + icon key (one
  visual system for all 39, per brief §13).
- D3: Newsletter has no backend endpoint; the block collects interest via the
  existing contact channel copy + register CTA — no fake subscribe POST, no
  fabricated list claims.

**TECHNICAL DECISIONS:**
- D4: `next/image` for the 5 editorial images (AVIF/WebP, responsive sizes,
  blur placeholder via tiny inline SVG; `images.formats` already configured).
- D5: Voice preview uses `sample_url` from `GET /voices` with HTMLAudio and
  full no-sample fallback (never a dead play button).
- D6: `/sessions/[slug]` SSG from curated starter presets, not session IDs.

**TESTS:** none (planning). Build phases verify with `tsc`, `next build`,
fixture-backed dev preview.

**KNOWN ISSUES:** P0 `/app/confessions` 404 still open — fixed in build phase A.

**NEXT PHASE:** BUILD A — missing routes (`/app/confessions`,
`/app/schedule`, `/sessions/[slug]`).
