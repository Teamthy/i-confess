# Handoff — review fixes for the server-as-source-of-truth work (#97)

**Date:** 2026-11-27 · **Branch:** `claude/project-thread-80z95m` (from `claude/dreamy-faraday-7th0my` at `6378f80`) · **PR:** supersedes #97; see the PR this file ships in

## Goal

Review PR #97 (221 files, no reviewer), fix what the review found, and merge
it. This is step 1 of the fix-everything plan from the 2026-11-27 critical
review.

## What changed

A code review of #97 found ten problems. All ten are fixed here:

| # | Problem | Fix | Where |
|---|---|---|---|
| 1 | A second person signing in over a synced account could see the first person's journal when one fetch failed | Switching accounts wipes the previous account's data before loading | `web/lib/account-sync.tsx` |
| 2 | Sign-out left notes, searches, settings and downloads on the device; settings were then pushed into the next account | Sign-out resets every personal field (`wipePersonalData`) | `web/lib/account-sync.tsx`, `web/lib/store.ts` |
| 3 | Deleting a journal entry or verse kept its full text forever | Delete blanks the text in the same statement; migration 0038 blanks existing tombstones; PRIVACY.md says so | `server/internal/store/study.go`, `0038_erase_deleted_study_text.sql`, `docs/legal/PRIVACY.md` |
| 4 | First sign-in silently dropped voice notes and entries that failed to upload | They stay on the device, tagged with their owner, and the user is told | `web/lib/account-sync.tsx` |
| 5 | Journal list stopped at 100; the export stopped at 200 journal entries, 100 plays, 100 posts and 20 sessions, and hid read errors | Cursor paging on `GET /me/journal`; the web follows pages; export reads everything (`store.ExportAll`) and fails loudly | `server/internal/api/study.go`, `library.go`, stores |
| 6 | Favourite, history, interest and settings writes could fail while the UI said "Saved" | Each write is checked; a failed favourite is rolled back; every failure is toasted | `web/lib/account-sync.tsx` |
| 7 | Sign-up with the API unreachable walked into the app as if registered | Only a build with `NEXT_PUBLIC_STATIC_DEMO=1` may do that | `web/components/auth.tsx` |
| 8 | A trial day could be stuck on "still being prepared" forever | Release on a non-cancelled context, release when attach fails, take over claims older than 2 minutes | `server/internal/api/trial_today.go`, `store/trial_day_sessions.go` |
| 9 | Anyone could undo someone's newsletter unsubscribe and kill their old links | A known address is left untouched; no new token is mailed | `server/internal/store/public_inbox.go`, `api/public_inbox.go` |
| 10 | `?next=/\evil.com` was an open redirect after sign-in | `lib/safe-next.ts` rejects backslashes and resolves against the origin | `web/lib/safe-next.ts`, `web/tests/safe-next.test.cjs` |

Also: the handoffs from 2026-11-22 said the work was PR #96. They now say #97.

## Verification

Run here on go1.26.11 with PostgreSQL 16 and Redis 7:

| Check | Result |
|---|---|
| `go build ./...`, `go vet ./...`, `gofmt -l` | Clean |
| `go test -race ./... -count=1` | All packages pass |
| New Go tests | `TestJournalPagesThroughEveryEntry`, `TestDeletedJournalTextIsErased`, `TestExportIsNotCappedAtAPage`, `TestStaleTrialDayClaimCanBeTakenOver`, and the rewritten end of `TestNewsletterUnsubscribeLink` |
| `npm run typecheck` (web) | Clean |
| `npm run test:safe-next` (new, also added to CI) | 3 pass |

**Not verified here:** the web sync changes were type-checked, not clicked
through in a browser; there is still no end-to-end test of the web journey
(fix-everything plan row 10). Flutter, Docker and the deploy checks run only
in CI.

## Open items

- Newsletter: returning after an unsubscribe needs a confirmed (double)
  opt-in, which is not built. Until then, a person who left cannot rejoin from
  the form.
- The rest of the fix-everything plan: security notes out of the tree, admin
  MFA, analytics allowlist, fake features, review gates, end-to-end CI,
  staging runbook, hygiene.

## Picking it up

Read the "I CONFESS fix-everything master plan" doc and take the first row
that is not Done.
