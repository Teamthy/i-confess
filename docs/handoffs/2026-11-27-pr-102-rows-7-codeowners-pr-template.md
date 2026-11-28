# Handoff — Row #7 (CODEOWNERS + PR template)

**Date:** 2026-11-27 · **Branch:** `arena/2a377dd9-i-confess` · **Commits:** `a6fd8aa` · **PR:** #102 (to create)

## Goal

Row #7 of the 2026-11-27 critical review: CODEOWNERS file + PR template enforcing master prompt merge rules (one theme, ~400 lines, CI green, written review, handoff doc).

## What changed

- `.github/CODEOWNERS` — default owner `@olusanyatimothy` + per-directory owners for server/, web/, apps/mobile/, clients/dart/, voice-engine/, design/, .github/workflows/, infrastructure/, scripts/
- `.github/PULL_REQUEST_TEMPLATE.md` — added Scope section (one theme, ~400 lines), CI green, written review, and handoff doc requirements

## Verification

- Files created and committed
- Branch protection (required status checks, required reviews, conversation resolution, enforce admins, no force pushes/deletions on main) must be enabled in GitHub repository settings — not settable via the GitHub App API (403 when attempted)

## Open items

- Branch protection needs manual setup in GitHub repo settings
- PR #102 needs to be created, reviewed, and merged
- Rows 8-12 not started: Dependabot, web ESLint, E2E staging test, staging runbook, repo hygiene

## Picking it up

Create PR #102 from this branch, get CI green + written review, then merge. Branch protection setup is a manual GitHub settings step. Then proceed to Row #8 (Dependabot).
