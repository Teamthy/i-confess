# Handoff — home page sections, real sign-up, newsletter and contact

**Date:** 2026-11-22 · **Branch:** `claude/dreamy-faraday-7th0my` · **Commits:** `8476f04`, `80cd492` (+ this) · **PR:** [Teamthy/i-confess#96](https://github.com/Teamthy/i-confess/pull/96)

## Goal

Make every home page section below the hero look professional and route correctly.

## What changed

**Home sections (`web/components/marketing.tsx`, styles at the end of `web/app/globals.css`, prefix `hm-`)**

| Section | Before | Now |
|---|---|---|
| Who we are | Heading, decorative orb, `StoryTabs` | Heading, lede, About / How it works, three numbered pillars (counts come from the library) |
| Approach | "Three commitments… Hover a card" with four venn cards | "Four commitments", icon cards: Intention, Presence, Repetition, Privacy |
| Image band | "You'll find what you need", "Explore" | "Five minutes or fifteen. Sized to your day.", "Explore the library" |
| Voice | Centered heading, stray orb link | Two columns: copy with three licensing checks + CTA, cards on the right |
| Sharing split | One paragraph, CTA to an auth-walled page | Private / Shared / Community list; "Write your confession" goes through sign-up when signed out; "How sharing works" |
| Product statement | Quote with a non-functional left arrow | "Our promise" quote with real counts and "See how a session works" |
| Journal / FAQ | "More Insights", "Learn More" → /help | "Read the journal"; "All questions" → /faq plus "Contact us" |
| Final CTA | "Take control of your mornings?", plain-text email | "Begin with five minutes tomorrow morning", Start / See plans, `mailto:`, contact form link |

- `AuthLink` (islands.tsx): signed-in → the app path; signed-out → `/register?next=<path>`. Used by the hero, sharing and final CTAs.
- `StoryTabs` was removed (no other users).
- Footer social icons were removed: all three pointed at `/community`, and there are no real accounts.
- `SectionHead` and `CategoryPanel` take `as="h1"`. Every marketing and auth page now has exactly one `<h1>` (checked on 24 routes); `/faq` and several others had none.

**Sign-up was fake, now real (`web/components/auth.tsx`)**
- Register called no API: a demo code `000000`, a local-only user and no session, so every "Begin" CTA ended at an auth wall.
- Now it calls `POST /auth/register`. With a session it continues to interests, saves them with `PUT /me/interests` (`source: ONBOARDING`), then goes to `?next=`.
  - An already-registered address gets `pending` with no session (no membership oracle), so the visitor is shown "Check your email".
  - With no API at all (status 0, the static demo), it continues locally, the same as login.
- Forgot password, reset password (reads `?token=`) and verify email (reads `?token=` on mount) call the real endpoints. Verify phone has no API, so the page says so.
- The password minimum is 8, matching the server policy (it said 6).
- Login carries `?next=` through to the register link.

**Backend (`8476f04`)**
- Migration `0035_public_inbox.sql` adds the `newsletter_subscriptions` and `contact_messages` tables (122 in total; the counts in the tests are updated).
- `POST /newsletter` and `POST /contact`, plus `/v1` mirrors, share a new `ratelimit.PublicForm` limit (5 per 15 minutes per IP, separate buckets).
- When mail is configured, contact messages are forwarded to `SupportAddress`.
- The OpenAPI spec and `design/routes.json` are regenerated; the IA coverage exceptions are marked `web-admin`.

## Verification

**Run here:**
- **Go:** the race suite passes, including the new `TestNewsletterAndContactAreStoredAndForwarded` and the route-parity test, which caught the missing `/v1` mirrors. golangci-lint v2.13 reports 0 issues.
- **Design checks:** `design/test_design.py` and `design/test_ia.py` pass.
- **Web:** `tsc` passes.
- **Browser (Playwright against the local API on :8080 and Next dev on :3000):**
  - Every home link returns 200 and lands on a page with an `<h1>`.
  - No horizontal overflow at 1440 or 390.
  - Registering with `?next=/app/community/create` created the user, stored 3 interests and landed on `/app/community/create`.
  - The newsletter form showed its confirmation.
  - `POST /api/contact` through the web proxy returned a reference.

**Not run here:**
- **Email delivery:** there's no mail provider here. Contact forwarding is covered by the unit test only.
- **The reset and verify links:** not clicked from a real email.
- **ESLint:** `next lint` has no config in this repo and prompts interactively.

## Open items

- Onboarding's practice step still keeps reminders in the local store (as before). Moving it to `POST /schedules` is the next real-data step.
- The newsletter has no unsubscribe endpoint yet (the column exists).
- **Carried over:**
  - run `load-bible` in staging and production
  - mobile Day 6 builder prefill
  - the `PRIVACY.md` wording for the Claude parser (it should also mention the newsletter and contact data now)
  - G-33 (breached passwords)
  - minister-voice licensing

## Picking it up

Wire onboarding's practice step to `POST /schedules`, add `POST /newsletter/unsubscribe` with a signed link, and update `docs/legal/PRIVACY.md` for the two new data stores.
