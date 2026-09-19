# PHASE 24 — Activity (History, Streak, Schedules)

**Status:** PASS
**Date:** 2026-09-19
**Depends on:** PHASE 20 (home), PHASE 15 (session engine)

## Objective

Activity tab: continue listening, history, schedules, streak. History and streak are derived from sessions, not separate tables.

## Implementation

- `activity_providers.dart`: `activitySessionsProvider` (mySessions), `activitySchedulesProvider` (schedules), `streakProvider` (simple count of completed sessions last 7 days, capped), `activityGroupedProvider` (continue/completed/scheduled grouped by status).
- `activity_screen.dart`:
  - Streak card: fire icon, day count, "Keep speaking life daily".
  - TabBar with 3 tabs: Continue, History, Schedules.
  - Continue tab: horizontal or vertical list of sessions with ACTIVE/PAUSED/INTERRUPTED/READY/STARTING, play icon, tap -> player, empty state with CTA to builder.
  - History tab: COMPLETED sessions, check icon.
  - Schedules tab: list of schedules with label, time, days count, enabled switch.
  - _SessionTile: Material + InkWell, 48x48 icon, title from first item, duration and status.

### Router

- `/activity` -> `ActivityScreen`, `/history` -> `ActivityScreen`.

## Exit Criteria

- Activity shows real sessions grouped by status ✔
- Streak displayed ✔
- Schedules listed ✔
- Continue navigates to player ✔
- Empty states ✔

## Verdict — PASS
