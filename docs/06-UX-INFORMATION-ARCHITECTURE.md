# PHASE 06 — UX & Information Architecture

**Status: PASS (8/10)**
**Date:** 2026-09-06
**Depends on:** PHASE 05 (design tokens), PHASE 02 (architecture)
**Blocks:** PHASE 18 (mobile app), PHASE 38 (web), PHASE 40 (admin)

## Objective

Define the screen inventory and navigation graph for the product, and make it
checkable — so that a screen nobody can reach, a screen nobody can leave, and a
screen that calls an endpoint the backend does not have are all caught by a test
rather than by a user.

## What existed

Inspection of the three surfaces found the following. All of it is measured, not
estimated:

| Surface | Finding |
|---|---|
| `apps/mobile` | 5 nav destinations, but only 2 are real screens — the rest render `_Placeholder`. Labels are **`Discover`** and **`Create`**, not the `EXPLORE` and `CONFESS` that §12 mandates. |
| `apps/web` | **5 dead links.** The footer and CTAs point at `/signup`, `/privacy`, `/terms`, `/faq`, `/contact`. None of those pages exist; only `/`, `/categories`, `/plans`, `/pricing` and `/t/[token]` do. |
| `apps/admin` | **1 dead link.** Links to `/admin/preview`; the page is at `/preview`. |

That is six dead links across two small surfaces. They are the ordinary failure
mode of a nav graph — nothing errors, the link just goes nowhere — and neither
surface had a test that could have noticed.

The label mismatch is worth more attention than it looks. "Discover" and "Create"
describe a content app and a document editor. The product is neither: you are
not browsing, and you are not making a file. §12's `EXPLORE` and `CONFESS` are
load-bearing words, and `design/test_ia.py` now asserts them.

## Implementation

```
design/ia.json       37 screens, 8 entry points, the navigation graph
design/routes.json   262 routes, exported from the running handler
design/test_ia.py    graph and endpoint assertions
```

`routes.json` is **exported, not transcribed**. `TestExportRouteTable` builds the
real `Handler`, calls `Routes()`, and dumps `RouteTable()`. `make routes`
regenerates it. A hand-copied endpoint list would drift the first time someone
renamed a route, and the drift would only show up as a spinner in production.

The export test refuses to write an empty table. The first version of it wrote
one and passed — `RouteTable()` only fills after `Routes()` runs, and nothing
called it. A test that exports nothing and reports success is worse than no
test, because it reads as coverage.

### The graph

Eight entry points: the five tabs, plus `onboarding`, `auth/sign-in` and
`template/share` (a deep link that must render for a signed-out viewer).

Five tabs, sourced from the `navigation.bottom` token rather than restated, so
the IA and the design system cannot disagree:

```
home · explore · confess · activity · profile
```

The core loop, `confess → confess/confession → confess/duration → confess/voice
→ confess/create → player`, is asserted to reach a playing session within four
taps, and within two from home.

`community` is defined but deliberately one level down and not a tab. G-4 stands:
there is no `PUBLIC` UGC visibility state, so that surface has nothing to publish
to. It must not appear in navigation until PHASE 30 gives it somewhere to put
things.

## What the tests caught

`design/test_ia.py` failed three assertions on its first run against my own graph:

**1. `GET /auth/security-events` does not exist.** The route is `POST`. I had
guessed the method. This is exactly the class of error the endpoint check exists
for — a plausible-looking string that no handler serves.

**2. Five endpoints had no screen at all:**

| Endpoint | Where it belongs |
|---|---|
| `DELETE /me/deletion` | account/deletion — cancelling a pending deletion |
| `GET /me` | profile |
| `GET /subscription` | profile, paywall |
| `POST /ai/parse` | **nowhere** |
| `POST /auth/security-events` | auth/security |

`POST /ai/parse` turns "45 minutes on fear" into a session spec, and the IA had
no place for it. That is a backend feature with no front door — either dead code
or a missing screen, and it was the second. It is now `confess/describe`, one tap
from the builder, with the parsed result always shown for confirmation before
anything is created. The user is responsible for what they speak over their
life, so the app does not get to guess silently.

**3. `template/share` had `depth: 1` and no parents** — a screen that claims to be
one level deep but has nothing above it. Corrected to a root.

## Two checks that were not checks

The first draft contained:

```python
check("every tab is one tap from any other tab", True)
check("the paywall is reachable but never the only way forward",
      from_home.get("paywall", 99) >= 1 and from_home.get("player", 99) >= 0)
```

The first is a literal. The second's `>= 0` cannot fail. Both were deleted and
replaced with the property they were pretending to test: **a free user must be
able to reach a session without passing through the paywall.** That is now a real
reachability computation with the paywall removed from the graph, and it was
verified to fail when the player's only parent is set to `paywall`.

Tab-to-tab distance is one tap by construction in a bottom nav, so asserting it
would only ever test the test.

## Security review

- **`account/deletion` is destructive and irreversible.** It requires typed
  confirmation and is two levels down, never on a tab. `DELETE /me/deletion` is
  wired so a pending deletion can be cancelled — the route existed with no UI,
  which means the only way to stop a deletion was an API call.
- **The paywall is never on the critical path to playback.** Asserted, not
  assumed. Gating a session behind purchase by accident is the fastest way to
  lose the free tier that the trial exists to convert.
- **`template/share` renders for signed-out viewers.** A shared prayer that
  demands an account before showing its own text will not be shared twice.
- **Entitlement stays server-side (§35).** The paywall reads `GET /entitlements`;
  it never decides anything itself.

## Exit criteria

- [x] Screen inventory defined — 37 screens
- [x] Navigation graph defined with no orphans and no dead ends
- [x] Five tabs, matching the §12 token exactly
- [x] Every screen's endpoints verified against the live route table
- [x] Every user-facing endpoint has a screen
- [x] Core loop depth asserted
- [x] Gated in `make verify` and CI
- [ ] Wireframes and real screens — PHASE 18

## Carried forward

- **G-17** — six dead links in `apps/web` and `apps/admin`. Both surfaces are
  replaced under D-5, so this is fixed by deletion, not repair. Recorded so the
  replacements do not repeat it.
- **G-18** — the mobile nav labels `Discover` and `Create` violate §12. Fixed by
  the D-4 replacement; `design/test_ia.py` prevents recurrence.
- **G-19** — the IA describes mobile only. Web and admin need their own graphs in
  PHASE 38 and 40, and `design/ia.json` is currently single-platform by design
  rather than by omission.
- **G-20** — `community` is defined but unreachable in practice until G-4 is
  closed in PHASE 30. If PHASE 30 slips, the screen must come out of the graph
  rather than ship empty.
