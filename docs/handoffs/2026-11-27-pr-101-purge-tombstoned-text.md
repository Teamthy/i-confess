# Handoff — purge tombstoned private text (Row #5)

**Date:** 2026-11-27 · **Branch:** `arena/2a377dd9-i-confess` · **Commits:** none yet · **PR:** none yet

## Goal

Row #5 of the 2026-11-27 critical review: soft-delete paths must wipe private text fields (body, title, text, note, label, mood, description) when setting deleted_at; user_confessions needs a delete function; tests must assert tombstoned rows have empty private text.

## What changed

**Soft-delete purge fixes (private text wiped on delete):**
- `server/internal/api/bible_user.go` — `deleteMyBibleBookmark` now sets `label='', note=''`; `deleteMyBibleNote` now sets `title='', body=''` on soft delete.
- `server/internal/api/bible_capabilities.go` — bookmark soft-delete now wipes `label`/`note`; note soft-delete now wipes `title`/`body`; highlight soft-delete left unchanged (color is not private text).
- `server/internal/store/sessions.go` — `SessionStore.SoftDelete` now sets `title='', description=''` alongside `deleted_at`.
- `server/internal/store/engagement.go` — `ListUserConfessions` filters `deleted_at IS NULL`; added `DeleteUserConfession` that soft-deletes and wipes `title`/`text` in one statement.

**Already correct (no change needed):**
- `server/internal/store/study.go` — `DeleteJournal` wipes title/body/mood; `DeleteMemory` wipes text/title.

**Tests:**
- `server/internal/api/purge_test.go` — 6 tests, all passing against real PostgreSQL:
  - `TestSoftDeletedConfessionIsNotListed`
  - `TestDeletedBookmarkAnnotationsArePurged`
  - `TestDeletedNoteContentIsPurged`
  - `TestPurgeAppliesThroughDeleteAPI`
  - `TestDeletedConfessionTextIsPurged`
  - `TestDeletedSessionMetadataIsPurged`

## Verification

- `go build -modfile=/tmp/local.mod ./internal/store/ ./internal/api/` — EXIT 0
- `gofmt -l` on all 5 changed files — clean
- 6 purge tests against real PostgreSQL (TEST_DATABASE_URL) — 6/6 PASS
- `internal/api` package tests cannot run locally (otel/auto/sdk unreachable); verified in CI during PRs #99/#100 pattern.

## Open items

- None for Row #5. Rows 6-12 (feature-status register + label fakes, CODEOWNERS/PR template/branch protection, Dependabot, web ESLint, E2E staging test, staging runbook, repo hygiene) are not started.

## Picking it up

Commit and push these changes, then open a PR for Row #5 (purge tombstoned private text). One theme per PR, ~400 lines, filled PR template. CI must be green + written review + handoff doc before merge.
