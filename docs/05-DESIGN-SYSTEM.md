# PHASE 05 — Design System

**Status: PASS (8/10)**
**Date:** 2026-09-06
**Depends on:** PHASE 03 (T-4 Flutter, T-5 Next.js)
**Blocks:** PHASE 38 (web), PHASE 40 (admin), and every UI phase in between

## Objective

One design source of truth that mobile, web and admin all consume, so the three
surfaces cannot drift apart and so section 12's rules are enforced by a test
rather than remembered by a person.

## What existed

Nothing. This phase built from zero, and the inspection found why that matters:

| Finding | Evidence |
|---|---|
| No token file, no CSS, no theme, no Tailwind config anywhere | `find` across the repo returned zero style files |
| 13 distinct hardcoded hex colours, 42 inline occurrences across 4 files | `grep -rho "0xFF..."` |
| 9 unrelated font sizes: 10, 11, 12, 13, 16, 18, 22, 24, 28 | `grep -rho "fontSize: [0-9.]*"` |
| **The seed colour was indigo, not green** | `apps/mobile/lib/app.dart:40` — `ColorScheme.fromSeed(seedColor: Color(0xFF7C8CF8))` |

That last row is a direct violation of section 12, which mandates a green
primary. It was never going to be caught by a test suite, because no test
asserted it. It is asserted now — `design/test_design.py` checks the hue of the
primary in both themes and explicitly fails if it drifts back toward the retired
indigo.

Also worth recording: an earlier working note in this project claimed a
`packages/ui` directory containing 679 lines of UI code. **That directory does
not exist.** The claim was wrong, and nothing was built on it here.

## Implementation

```
design/
  tokens.json          120 tokens, the single source of truth
  generate.py          emits Dart, CSS and TypeScript
  check_contrast.py    WCAG 2.1 AA verification
  test_design.py       section 12 compliance
  generated/
    tokens.dart        Flutter  — consumed by the PHASE 18 app
    tokens.css         CSS custom properties — web and admin
    tokens.ts          TypeScript — typed access from Next.js
```

`design/generated/` is committed, not gitignored. Flutter cannot run a codegen
step without a build-runner dependency, and a committed generated file means the
token values are reviewable in a diff. `generate.py --check` fails CI if anyone
edits tokens.json and forgets to regenerate, or hand-edits the output.

### Section 12 compliance

| Requirement | Delivered | Enforced by |
|---|---|---|
| Spacing 4–64 | 4, 8, 12, 16, 20, 24, 32, 40, 48, 64 | test asserts range and 4px steps |
| Radius 8/12/16/20/999 | exactly those five | test asserts the exact set |
| Green primary | brand 500 `#2A9D76`, hue 160° | test asserts hue 100–170° in both themes |
| Seven type roles | display, heading, subheading, body, caption, label, metric | test asserts all seven present |
| Bottom nav, five items | home, explore, confess, activity, profile | test asserts count and order |

One addition beyond the seven roles: `bodySm` (14px sans). The serif `body`
exists for scripture and reads deliberately; interface copy needs a smaller
sans, and inventing one per screen is how the scale fragments. The test allows
exactly one extra role and fails on a second, so the exception cannot quietly
become the rule.

### Colour

Green primary as mandated, with gold reserved for premium and liturgical
accents and never for a primary action — a prayer app that puts a buy button in
the same colour as its scripture has confused its own priorities.

Dark is the default theme. This is a night-time product.

### Typography

Scripture and confession text use a serif at 1.7 line height; interface text
uses a sans. That distinction is the point — the words being spoken over
someone's life should not be typographically identical to a settings label. A
bug in the generator's first version silently collapsed `body` back to the
sans; the test now asserts the serif survives generation.

## Accessibility

`check_contrast.py` verifies 30 text/background pairings against WCAG 2.1 AA.
**All 30 pass.** The first run did not:

```
FAIL   3.84:1  dark: semantic danger on background   #C0453C on #0A0E0D
FAIL   3.66:1  dark: semantic info on background     #3A6EA5 on #0A0E0D
FAIL   3.40:1  light: semantic success on background #2A9D76 on #FFFFFF
FAIL   3.43:1  light: semantic warning on background #B5811F on #FFFFFF
```

The cause was architectural rather than a wrong hex value: a single mid-tone
cannot reach 4.5:1 on both near-black and white. Every semantic colour now has
a light and a dark variant, and each theme references the one that works on it.
Choosing one value per theme by hand later is exactly how contrast regresses a
screen at a time, so the variants are in the tokens.

## Section 81 — AI-looking design audit

Scored honestly, against the usual tells:

| Tell | Present? |
|---|---|
| Purple or indigo gradient hero | No — green, no gradients defined anywhere |
| Glassmorphism / blur everywhere | No — no blur tokens exist |
| Emoji used as icons | No — no emoji in tokens; icons are PHASE 18's job |
| Rainbow of semantic colours | No — four, all contrast-verified |
| Uniform oversized rounding | No — five radii with distinct jobs |
| Gratuitous shadow on every card | No — five elevations, `0` is `none` |
| Generic 3-up card grid | Not applicable at token level |

**Score: 8/10.** Two points held back: the palette is untested against real
screens, and no imagery, iconography or motion has been applied to an actual
layout yet. A token set can be disciplined and still produce a bland product.

**This does not discharge section 81.** The audit must be re-run on built
screens in PHASE 18, 38 and 40, where it can actually fail. Anything scoring
below 8 gets redesigned, per the directive.

## Testing

```
make design-check
```

Runs `generate.py --check` then `test_design.py`: 30 assertions covering the
spacing scale, the radius set, primary hue in both themes, the seven type roles,
the serif/sans split, the five-tab navigation, all 30 contrast pairings,
generator determinism, and that every token reached all three outputs.

One assertion in the first draft was tautological — it ended in `or True` and
could not fail. It was replaced with a real check for unresolved `{token.path}`
references leaking into generated code, and verified to fail by injecting one.

## Security review

Design tokens carry no secrets and no user data. Two notes:

- `tokens.css` and `tokens.ts` are committed and shipped to the browser. Nothing
  in them may ever be environment-specific or sensitive.
- The gold accent marks premium. Colour alone must never be the only signal of
  entitlement — that is server-determined per section 35. The token exists for
  presentation; the check stays in the API.

## Exit criteria

- [x] Single token source of truth exists
- [x] Dart, CSS and TypeScript all generated from it, deterministically
- [x] Section 12 values delivered and asserted by test
- [x] 30/30 contrast pairings meet WCAG 2.1 AA
- [x] `make verify` and CI both gate it
- [x] Section 81 audit run and scored, with its limits stated
- [ ] Consumed by a real surface — deferred to PHASE 18, which is the correct
      place for it

## Carried forward

- **G-14** — no iconography defined. Icons are drawn or licensed in PHASE 18.
  Emoji are prohibited as a substitute.
- **G-15** — motion tokens exist but are unexercised. Breathing and session
  transitions need review against a real screen; the 700ms `deliberate` duration
  is a guess until someone watches it.
- **G-16** — `apps/mobile` still hardcodes 13 colours and violates the green
  mandate. It is retired under D-4, so this is fixed by replacement rather than
  by editing it.
