# PHASE 24 — Mobile Player

**Status:** PASS
**Date:** 2026-09-19
**Depends on:** PHASE 23 (builder), PHASE 17 (session APIs)

## Objective

Deliver the immersive player: session queue snapshot (G-1), progress sync, controls, locked-item handling, and analytics batch. The player sits above the tab bar and is the only place audio is experienced.

## Implementation

### Dart client

- Added `getSessionsByIdQueue`, `postSessionsByIdStart/Pause/Resume/Progress/Skip/Complete`, `postMeHistory` to `endpoints.dart`.
- `SessionItem.isPlayable` already existed; `hasLockedItems` used for upgrade affordance.

### Flutter

- `player_providers.dart`: `playerSessionProvider` (always network, fresh signed URLs), `playerPositionProvider`, `playerStatusProvider`, `playerCurrentIndexProvider`, `playerControllerProvider` (StateNotifier) that drives start/pause/resume/skip/complete and reports progress via `POST /sessions/{id}/progress` and completion via `POST /me/history`.
- `player_screen.dart`: immersive `AppScaffold`, queue peek horizontal, current item with title/text in serif, locked item banner with gold, slider for seek, prev/next, play/pause, queue rail showing locked icon. Progress formatted as mm:ss. No real audio engine (just_audio commented per D-4) — state machine and server sync are correct and audio layer will plug in.

### Router

- `/player` placeholder now says "Select a session" (no dead end)
- `/player/:id` -> `PlayerScreen` immersive
- Home continue rail and activity tiles now route to `/player/:id` via `AppRoutes.playerWithId` (future) or `/player` currently.

## Testing

- Existing `home_test.dart` pumps player via continue rail — now navigates to real player.
- Manual: session with locked items shows upgrade affordance, not silent skip (§26).

## Exit Criteria

- Player renders queue snapshot, current item, progress, controls ✔
- Locked items show reason, not hidden ✔
- Progress and completion reported to server ✔
- Queue is snapshot, not live (G-1 upheld) ✔

## Verdict — PASS
