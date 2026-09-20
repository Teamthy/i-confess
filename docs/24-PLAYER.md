# PHASE 24 — Mobile Player & Session Engine Integration

**Status:** PASS
**Date:** 2026-09-20
**Depends on:** PHASE 23 (builder), PHASE 17 (session APIs), PR-B/PR-D (CloudFront signed audio URLs)

## Objective

Deliver the production immersive player with full Session Engine integration, real audio playback using server-issued signed URLs, deterministic local/cloud conflict resolution, progress persistence and resume, background/route interruption recovery, and server-authoritative completion validation.

## Implementation

### Backend (`server/`)

- Audio URL signing on all session lifecycle transitions:
  - `startSession`, `resumeSession`, `getSessionQueue`, and `skipSessionItem` sign session audio items via `AudioSigner` (CloudFront HMAC signed URLs with TTL).
  - Queue snapshots return freshly signed streaming URLs for active and queued items.
  - Added `interruptSession` endpoint (`POST /sessions/{id}/interrupt`) allowing clients to persist interrupted state on audio focus loss or route disconnect.
- Server-authoritative completion validation and anti-forgery guards:
  - `completeSession` updates session status to `completed` in the database and automatically writes verified completion to `playback_history`.
  - `recordPlayback` in `handlers.go` strictly validates session ownership against authenticated `userID` and rejects unstarted/non-existent sessions (`400 Bad Request` / `403 Forbidden`).
  - Added `sessions_playback_test.go` covering signed audio URLs, entitlement checks, queue skip transitions, deterministic timestamp conflict resolution, and anti-forgery guards (100% pass across all tests and `-race`).

### Typed Dart Client (`clients/dart/`)

- Models: added `SessionProgress`, `SessionQueueResponse`, `SyncProgressResponse`, and `SessionItem.status`, `category`, and `copyWith`.
- Endpoints: added `postSessionsByIdInterrupt`.
- Repository: added `sessionQueue`, `startSession`, `pauseSession`, `resumeSession`, `interruptSession`, `syncProgress`, `skipSessionItem`, and `completeSession`.
- Unit tests: verified serialization and client-side lifecycle flows in `repository_test.dart`.

### Flutter Mobile App (`apps/mobile/`)

- Audio Playback Service (`audio_playback_service.dart`):
  - Defined clean `AudioPlaybackService` abstraction.
  - `JustAudioPlaybackService`: production audio engine powered by `just_audio` and `audio_session`. Configures audio session for speech, listens to audio focus interruption events (`interruptionEventStream`), and detects headphone/Bluetooth disconnects (`becomingNoisyEventStream`).
  - `TestAudioPlaybackService`: deterministic test implementation for headless CI and unit/widget test stability.
- Session Engine (`player_providers.dart`):
  - `SessionEngine` (`StateNotifier<SessionPlaybackState>`) owns all playback and session lifecycle state (business logic decoupled from Flutter widgets).
  - State persistence: stores active item, playback position, completed/skipped items, session progress, and timestamp via `KeyValueStore` under `StoreKeys.sessionPlayback`.
  - Deterministic conflict resolution: compares local persisted timestamp vs server `updated_at` before loading, adopting newer state while avoiding backward progress jumps.
  - Transparent signed URL refresh: when audio fails to load or expired signed URLs return 403, engine queries `getSessionQueue` to obtain freshly signed URLs and seamlessly resumes without losing playback position.
  - Interruption and route changes: pauses playback and syncs status on audio focus loss or `becomingNoisy` disconnect events.
  - Entitlement loss / locked items: surfaces upgrade affordance (`/premium`) for locked items rather than silent skips.
- Player Screen UI (`player_screen.dart`):
  - Immersive `AppScaffold(immersive: true, scrollable: false)`.
  - Queue rail preview with status indicators and lock icons.
  - Active item confession card with title, scripture serif text, and metadata.
  - Paywall CTA button and banner for locked confession items.
  - Seek slider with elapsed/remaining mm:ss timestamps.
  - Full transport controls: previous, rew 15s, play/pause toggle, fwd 15s, skip next.
  - Actionable error card with retry button on network or playback failures.
  - Session completion screen with celebration graphic and "Return to Activity" CTA.

## Verification

- `go test -modfile=/tmp/local.mod -count=1 ./...`: 100% pass across all Go packages.
- `go vet -modfile=/tmp/local.mod ./...` and `gofmt -l .`: zero warnings, zero diffs.
- `go test -race -modfile=/tmp/local.mod -count=1 ./internal/api/ ./internal/sessions/`: clean, zero race conditions.
- Design token checks (`design/generate.py --check`, `test_design.py`, `test_ia.py`): 120 tokens, 37 screens, 103 endpoints wired.
- Mobile test suite: `session_engine_test.dart` and `player_screen_test.dart` verifying all 12 acceptance criteria.

## Verdict — PASS
