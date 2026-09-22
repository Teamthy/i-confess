# Ledger 48 — BUILD REPORT: missing routes + reference homepage

**Date:** 2026-09-22
**Branch:** `arena/01a0c6ca-i-confess`
**Depends on:** Ledgers 46 (audit) + 47 (planning).

---

## BUILD A — Missing routes (brief §26 gaps)

| Route | Implementation | Notes |
|---|---|---|
| `/app/confessions` | `app/app/confessions/page.tsx` + `components/ConfessionsClient.tsx` | Personal shelf: `GET /me/confessions` grouped inline (no detail endpoint exists — expansion, not links), submit-for-review action, review notes verbatim. **Fixes the P0**: bottom-tab "Create" + create-confirmation links 404'd before. |
| `/app/schedule` | `app/app/schedule/page.tsx` + `components/ScheduleClient.tsx` | Full schedule CRUD against `models.Schedule` shapes (label, HH:MM, days, duration, voice, categories), enable toggle, delete confirm, Start-now → player. 402 renders `PremiumOffer`. Browser timezone default. |
| `/sessions/[slug]` | `app/sessions/[slug]/page.tsx`, SSG × 4 starters | Sessions are user-scoped, so this is an honest preview: starter shape + live sample confession + builder deep-link (`?category=`). No fake session IDs. |

Supporting changes: `AppShell` sidebar gains Routines + Schedule (§28 order);
`/sessions` index gains a starters band; `sitemap.ts` gains `/sessions` + 4
starters; fixture gains in-memory schedules CRUD + `GET /me/confessions`
state fix (`scripts/dev-api.mjs`).

## BUILD B — Image system

`apps/web/public/images/`: `hero.jpg`, `story.jpg`, `immersive.jpg`,
`community.jpg`, `newsletter.jpg` — locally generated editorial photography
(120–240 KB each, AVIF/WebP via `next/image`). No external stock dependency.
Category cards use the deterministic token-ramp motif (`motifStyle`, one
system × 39) instead of 39 photos.

## BUILD C — Homepage rebuild (motifs M2–M12)

`app/page.tsx` rewritten to the 15-section reference rhythm: light editorial
hero (`Speak it. Hear it. Live it.`) → overlapping cards (auth-aware:
continue-listening when signed in, curated when not) → asymmetric philosophy
→ **dark rounded category panel** → 4 principles (Presence active) →
immersive image + floating card → voice rail with real `sample_url` previews
(`VoicePreview`, single-play, no-sample fallback) → community split →
two-column honest library statement (**no fabricated testimonials**) →
journal cards → two-column 11-question FAQ (brief §22 verbatim) → download
band → newsletter split (email carried into `/register?email=`, new prefill;
no fake subscribe endpoint) → final CTA → structured footer.

CSS: ~900 lines appended to `styles/site.css` (all token values), plus the
missing `.ic-input` rule the schedule form needs. Header uses `<SiteHeader
light />` over the light hero.

## BUILD D — QA

- `tsc --noEmit` clean.
- `next build` green (exit 0): 82 routes, `/sessions/[slug]` SSG × 4,
  First Load JS unchanged at 103 kB.
- Fixture-backed dev preview: `/`, `/sessions/morning-peace`, `/sessions`,
  `/app/confessions`, `/app/schedule`, `/register?email=`, `/categories`,
  `/app` all 200; homepage renders 8 category cards, 3 voice previews,
  11 FAQ items, 3 journal cards, 0 error states; schedule POST→GET
  round-trip verified; register prefill verified.

## Anti-AI + fidelity self-audit (§53/§54)

- Rhythm: light → overlap → light → DARK → light → image → light → mist →
  light → light → mist → split → light → DARK. Alternation structural. ✓
- No generic gradients (token navy only), no glassmorphism, no floating
  decoration, no badges, no three-column repetition (rails/carousels/splits
  alternate). ✓
- All numbers real (API counts) or editorial ("5-min starter" links the
  real starter). No invented testimonials/users/stats/pricing. ✓
- Motion: existing `Reveal` + CSS hover lifts + native scroll-snap;
  `prefers-reduced-motion` suppresses all. ✓

---

## PHASE REPORT — BUILD A/B/C/D

**STATUS:** COMPLETE

**FILES CREATED:** `app/sessions/[slug]/page.tsx`,
`app/app/confessions/page.tsx`, `app/app/schedule/page.tsx`,
`components/ConfessionsClient.tsx`, `components/ScheduleClient.tsx`,
`components/VoicePreview.tsx`, `components/HomeExperienceCards.tsx`,
`components/NewsletterForm.tsx`, `public/images/*.jpg` (5),
`docs/46-…`, `docs/47-…`, `docs/48-…` (this file)

**FILES MODIFIED:** `app/page.tsx` (rewrite), `styles/site.css` (+~900),
`lib/categoryColor.ts` (+motif), `components/AppShell.tsx` (sidebar),
`app/sessions/page.tsx` (starters), `app/sitemap.ts`,
`app/register/page.tsx` + `RegisterForm.tsx` (prefill),
`scripts/dev-api.mjs` (schedules)

**KNOWN ISSUES:** none P0/P1. Follow-ups for later phases: `/app` home
personalisation depth (§29 — continue/today/routines wiring), mini-player
global wiring (§31 — slot exists, player is page-scoped), admin/CMS (§46 —
no CMS endpoints exist; content models stay typed-local).

**NEXT:** user review of the preview; then `/app` home enrichment + player
persistence per §29/§31.
