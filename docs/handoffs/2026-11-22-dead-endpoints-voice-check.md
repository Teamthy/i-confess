# Handoff — dead endpoints, real saves, unsubscribe, voice engine check

**Date:** 2026-11-22 · **Branch:** `claude/dreamy-faraday-7th0my` · **Commits:** `74345bb`, `2ff0c8d`, `6b080b4` (+ this) · **PR:** [Teamthy/i-confess#97](https://github.com/Teamthy/i-confess/pull/97), merged through #98 with review fixes (this line said #96 until 2026-11-27)

## Goal

Close the three open items from `2026-11-22-home-sections-and-real-signup.md`, find dead endpoints and
route them properly, re-verify the home page routing, and check the voice engine and orchestrator.

## What changed

**The three open items**
- **Onboarding practice:** now saved on the account. It creates one schedule with `POST /schedules`
  (time, length, up to 3 categories) and sets the reminder with `PATCH /me/notifications`. Local-only
  storage is kept only when there is no API.
- **Newsletter unsubscribe:**
  - Migration `0036` adds `unsubscribe_token_hash`. Only the SHA-256 of the token is stored, and it
    rotates whenever the address subscribes again.
  - Signing up sends a welcome mail with the link. `POST /newsletter/unsubscribe` (+ `/v1`) handles it.
  - The page `/newsletter/unsubscribe` unsubscribes only on a button press, so a mail scanner
    prefetching the link does nothing.
  - `email.Message` has no custom-header support, so there is no `List-Unsubscribe` header yet.
- **Privacy:** `docs/legal/PRIVACY.md` and the web privacy page now cover:
  - newsletter and contact data, and how they are retained
  - the session-builder text sent to Anthropic. Checked in `internal/ai/llm_adapter.go`: only the
    utterance and the category list are sent.

  I removed a claim about Anthropic's training terms that I couldn't verify here. Legal should review
  the wording.

**Dead endpoints.** I compared every path the web and Dart clients call with `design/routes.json`.
Every client call matches a server route. The reverse found server features the web app faked locally:

| Feature | Before | Now |
|---|---|---|
| Write a confession (`/app/community/create`) | Saved to localStorage and said "Submitted for human review" | `POST /community/posts`. Shared/public needs a verified email (the 403 is explained to the person). The unbacked "Mentor" option is removed |
| Your confessions list and detail | localStorage | New `GET /me/community/posts`: the author's own posts in every status, no author id. There was no way to read your own posts back |
| Session builder | Local queue only | "Describe what you need" → `POST /ai/parse`, shown for confirmation before building; signed-in build → `POST /sessions` → real player |
| Export | Toast saying "(demo)" | Downloads `GET /me/export` as JSON |

**Remaining endpoints with no client**
- Infrastructure (expected): health, metrics, `openapi.json`, store webhooks.
- `POST/GET /voice-sessions`: a multi-section audio session API that nothing calls. It needs a product
  decision: wire it into the voice studio or remove it.
- `POST /sessions/preview`: listed in IA for the builder, but the builder builds directly.
- Several admin routes the matcher can't confirm, because the admin UI builds their paths from variables.

**Voice engine and orchestrator** (`internal/voiceengine`, `internal/voice`, `internal/voicegov`)
- **Reviewed:**
  - `orchestrator.go`: rights preflight, candidate and fallback selection, the content hash, and the
    per-handoff grant re-check
  - the `HTTPProvider` worker client
  - the wiring in `cmd/server/main.go`
- Every API use goes through `requestOrchestrator`, which loads the pronunciation dictionary and the
  live grant check. A nil `Dict` is safe.
- **Bug fixed:** `HTTPProvider.Generate` read at most 512 MiB and returned anything longer truncated,
  as a success. It now fails with a permanent error. There is a test.
- **No other defects found.** The voice package tests pass.

**AI parser bug (found while testing the builder):** the keyword parser, used whenever no model is
configured, read only "30" and "long". It now reads stated lengths: digits, number words, hours,
"half an hour". It ignores bare numbers ("Psalm 30") and "I am" ("a m"). There is a table test.

**Home routing:** unchanged since the previous handoff, where every link was verified (all 200, one
`<h1>` per page).

## Verification

**Run here:**
- **Go:** the race suite passes; golangci-lint v2.13 reports 0 issues.
- **Design checks:** IA (170 endpoints wired) and the design check pass.
- **Web:** `tsc` passes.
- **Browser (Playwright against the local API and Next dev):**
  - register → interests → practice created the schedule (07:15, 600 s, 3 categories)
  - a private confession was saved, listed and opened
  - a shared one was refused with the verification message
  - "Fifteen minutes on fear…" → Overcoming Fear, 15 minutes → built → the player showed 3 confessions
  - Export downloaded the JSON (account, favorites, interests, schedules, …)
  - the unsubscribe page set `unsubscribed_at`; a wrong token is refused (Go test)

**Not run here:**
- **The welcome mail:** no mail provider here. It is covered by a Go test with the memory sender.
- **Claude parsing:** not run live (no key). The keyword path was exercised.

## Open items

- `/voice-sessions`: wire it or remove it (needs a product decision).
- Add `List-Unsubscribe` headers when `email.Message` gains header support.
- The Dart client and the mobile app have no "my community posts" yet (`GET /me/community/posts`).
- Favourites and history in the web app are still local only; `/me/favorites` and `/me/history` exist.
- Legal review of the new `PRIVACY.md` sections.
- **Carried over:**
  - run `load-bible` in staging and production
  - mobile Day 6 builder prefill
  - G-33 (breached passwords)
  - minister-voice licensing

## Picking it up

Wire web favourites and history to `/me/favorites` and `/me/history` the same way community was done
(signed in → API, otherwise local), then decide on `/voice-sessions`.
