# Handoff — the account and library come from the server

**Date:** 2026-11-22 · **Branch:** `claude/dreamy-faraday-7th0my` · **Commits:** `03eefb8`, `46caeec` (+ this) · **PR:** [Teamthy/i-confess#96](https://github.com/Teamthy/i-confess/pull/96)

## Goal

"Wire favourites and history to the server. All major resources must serve from the server, not just
the frontend."

## What changed

**What lives where now (signed in)**

| Resource | Before | Now |
|---|---|---|
| Confession library (public pages) | Bundled `lib/confs.json` | `GET /confessions` (new) via `lib/library-server.ts`, 60 s revalidate; bundled copy only if the API is unreachable |
| Favourites | localStorage | `/me/favorites` (read, add, remove) |
| Listening history | localStorage | `GET/POST /me/history` |
| Streak | Counter in localStorage | Derived from server history |
| Plan, interests | Local guesses | `/me/bootstrap`, `PUT /me/interests` |
| Settings (speed, EQ, ambient, quiet hours…) | localStorage | `user_preferences.client_settings` (new, ≤16 KB JSON). `deviceVoice` stays per device |
| Journal | localStorage | `/me/journal` (new, row_version protected) |
| Scripture memory | localStorage | `/me/memory` (new). The server schedules reviews |
| Routines | Separate local list | They are schedules (server-backed) |
| Export | Missing history, posts | Adds `listening_history`, `community_posts`, `journal`, `memory_verses` |

**How the web does it: `web/lib/account-sync.tsx` (`<AccountSync />` in the root layout)**
- **On sign-in:** loads everything above into the existing app state, so the UI code is unchanged.
- **Pushing changes:**
  - Favourites, plays, interests and settings are pushed by observing state changes (`subscribeApp`
    in `lib/store.ts`).
  - The journal and memory use explicit actions (`addJournal`, `deleteJournal`, `addMemoryVerse`,
    `reviewMemoryVerse`) because they need server ids and versions.
- **First sign-in on a device:** data collected there while signed out is uploaded to the account.
  Data from another account is never uploaded.
- **Sign-out:** wipes the synced copy from the device (`syncedAs`).
- **Slug ↔ id:** the library index maps them. Server slugs use the same rule as `web/lib/data.ts`; all
  78 match.

**Fakes removed instead of faked further**
- **Share:** created a link that only worked on the sharer's device. It now shares the public
  `/confessions/{slug}` URL, and old `/app/shared/sh-*` links redirect there.
- **"Save for offline" and pack saves:** set a flag; nothing was cached. Offline is the mobile app's
  (the pricing already says so). Downloads now lists the account's real `/me/downloads` licences.
- **Partners and Family:** stored fake pairings in the browser. No invitation system exists, so both
  pages now say "Not available yet" and are gone from Home.
- **In-app feedback:** said "recorded" and sent nothing. It now goes to the contact inbox
  (`POST /contact`).

**Server fix:** empty favourites and history returned `null`, which crashed the sync. They now return `[]`.

## Verification

**Run here:**
- **Go:** the race suite passes; golangci-lint reports 0 issues.
- **New Go tests:**
  - journal privacy and conflicts
  - memory scheduling
  - export contents
  - the library index (unpublished excluded, slugs)
  - client_settings round trip
  - empty lists are `[]`
- **Design checks:** IA (exceptions for the web-only endpoints) and the design check pass.
- **Web:** `tsc` passes.
- **Browser, two separate browser contexts on one account:**
  - device A saved a favourite, a journal entry, a memory verse and speed 1.5; the server had all four
  - device B loaded all four, and its Favorites page showed the confession
  - logging out on B wiped favs, journal, memory and user
  - a play reached `/me/history` and the streak became 1. Headless Chromium has no speech engine, so I
    triggered this through the store's cross-tab event, which runs the same observer a completed play
    does
- **Library from the server:** editing a confession's text in the DB showed on `/confessions`,
  `/categories/healing` and, after the 60 s revalidate, the detail page. The edit was reverted.

**Not run here:**
- **A real audio play completing in a browser:** headless has no speech engine.
- **Mobile:** not touched.

## Open items

- **Still device-only, by choice or for now:**
  - recent searches (a device convenience)
  - per-confession annotations (small; no API yet)
  - the `seasons` state (unused)
- **Client islands** (Play, snippets) still read text from the bundled library for browser speech.
  Pages render server text; playback of a server session already uses server audio.
- **Partners and Family:** need a real invitation and consent design before they can exist.
- **Mobile:** has no journal or memory screens yet. The endpoints are ready.
- **Carried over:**
  - `/voice-sessions` decision
  - legal review of `PRIVACY.md`
  - `load-bible` in staging and production
  - G-33
  - minister-voice licensing

## Picking it up

Move the client islands to the server library: pass `library` from the page into PlayBtn, Snippet and
Radio. Then delete `lib/confs.json` once production has its content loaded.
