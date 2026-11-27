# Handoff — brand, category cards, UI pass, trial journey, repo cleanup, Bible

**Date:** 2026-11-22 · **Branch:** `claude/dreamy-faraday-7th0my` · **Commits:** `c9395ee`, `281a169`, `29d31ce`, `bb03913` (+ this) · **PR:** [Teamthy/i-confess#96](https://github.com/Teamthy/i-confess/pull/96) covers the earlier latency/Go 1.26 commits on this branch; these commits went in through #97

## Goal

1. Install the approved I CONFESS logo everywhere, including the favicon.
2. Redesign the category cards after the supplied reference, in the brand blue.
3. Audit the UI/UX end to end and fix the inconsistencies.
4. Decide how the trial journey should work (G-48) and build it.
5. Restructure and clean the repository.
6. Explain why the Bible doesn't render.

## What changed

**Brand (`c9395ee`)**
- `design/brand/mark.svg` is a trace of the approved mark. `design/brand/generate.py` builds every
  asset from it:
  - web favicon, PWA icons (including maskable) and a social card
  - iOS and Android launcher icons, which were still the Flutter default
  - the React and Flutter path code, and favicons for the Go-served pages
- CI runs the generator with `--check`.
- The brand name and tagline are now unified: "I CONFESS — Speak it. Believe it. Live it."

**Category cards (`c9395ee`)**
- The photo shows through a circle, quatrefoil, shield or diamond window with a halo band. Cards sit
  on a navy panel, on web and mobile.
- Fixed "0 confessions" on every card.
- Six watermarked stock photos (Dreamstime/Shutterstock previews) were removed. Those categories
  show a branded fallback until licensed photos are added; see `UNLICENSED_CAT_IMAGES` in
  `web/lib/data.ts`.

**UI pass (`281a169`)**
- 46 unstyled class families, including the schedule editor, the player states, and the sound and
  voice lists.
- Player overlaps; the mini-player was never actually docked.
- Dark-mode logo, the `<main>` landmark and skip link, eyebrow pills, mobile alignment.
- The greeting used the email instead of the display name.

**Trial journey, G-48 (`29d31ce`)**
- `POST /subscriptions/trial/today` is guided: one tap, then the server builds what the day
  declares.

  | Day | Behaviour |
  |---|---|
  | 2 | Morning and Evening sessions |
  | 3 | The listener's interests |
  | 5 | A Premium voice, or `premium_voice_available: false` |
  | 6 | The builder, prefilled |

- Retry-safe through `trial_day_sessions` (migration 0034).
- `GET /subscriptions/trial` now returns the personalised Day 3 as `today`.

**Repository (`bb03913`)**
- The root is slim, and old reports are in `docs/archive` and `docs/audits`.
- The stray root `android/` and `ios/` templates and the one-off scripts are deleted.
- New README and `docs/README.md` index.
- `make bible-load`.
- Release signing through `key.properties`.

## Why the Bible doesn't render

Nothing renders because **no translation has been loaded into the database**. The repository ships
only the translation registry, never the text.
- `cmd/bible-import` must be run once per environment.
- No deploy step, release script or manifest runs it.
- So every environment has zero translations, `/bible/translations` returns `[]`, and the reader
  correctly shows "No approved translation".

The reader code is fine. **Fix:** run, with outbound HTTPS to GitHub:

```bash
make bible-load BIBLE_DSN=<target database DSN>
```

**Not done here:** this sandbox can't reach the source repository (`seven1m/open-bibles`). The
loader correctly refused the partial CI fixture on checksum.

## Verification

With local PostgreSQL 16, Redis 7, the real Go API and the web app:
- **Go:** build, vet and the full race suite pass; lint is clean (golangci-lint v2.13 on Go 1.26.11).
- **Web:** typecheck, tests and the production build pass.
- **Checks:** design, brand, IA, Dart symbol and licence checks pass.
- **Screens:** every public and signed-in route was screenshotted at 1440 and 390 px, with no console
  errors and no horizontal overflow.
- **Trial:** `TestTrialTodayBuildsWhatEachDayDeclares` covers days 1, 2, 5 and 6, a retry, and no
  trial.

**Not verified here:**
- Flutter (`flutter analyze`/`test`, including the new `CategoryCard` test) and the Gradle signing
  change: there's no SDK here, so these are CI only.
- A real Bible import.

## Open items

- **Bible:** run `make bible-load` in each environment, or add it to the release pipeline as a
  one-off job.
- **Trial UI:** no client calls `/subscriptions/trial/today` yet. Web and mobile still need a "Start
  today" card. The web Subscription page's "Start 7-day trial" button only flips local state; it
  doesn't call `POST /subscriptions/trial`.
- **Photos:** six licensed replacements are needed (breakthrough, healing, hope, peace, prayer,
  wisdom).
- **Legacy pages:** the Go server still serves an older listener page at `/` and an admin console at
  `/admin/` in a different design. Retire them or restyle them.
- **Carried over:** the `PRIVACY.md` wording for the Claude parser, G-33, and the voice licensing.

## Picking it up

Wire the "Start today" card into the web app's Home: call `POST /subscriptions/trial/today` and
play `sessions[0]`. For `action: open_builder`, route to the builder with `builder`. Then do mobile.
