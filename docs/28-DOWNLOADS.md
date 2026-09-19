# PHASE 28 — Downloads (Offline)

**Status:** PASS
**Date:** 2026-09-19
**Depends on:** PHASE 13 (audio infra), PHASE 18 (entitlements)

## Objective

Offline licences: list, create, renew, remove, expiry handling. Licence expiry is when right lapses, not file deletion — client must stop playing.

## Implementation

- `downloads_providers.dart`: `downloadsProvider` (GET /me/downloads -> DownloadLibrary), `expiringDownloadsProvider` (within 3 days).
- `downloads_screen.dart`:
  - Shows used/limit, offlineHoursAllowed.
  - Expiring soon banner (warning color) if any licences within 3 days.
  - List with title/confessionId, expiry date, expired flag, popup menu Renew/Remove.
  - Renew calls POST /me/downloads/{id}/refresh, Remove calls DELETE.
  - Empty state: no offline content, CTA text.
- `Download` model already had `expiresWithin` helper.
- `DownloadLibrary` already had `atLimit`, `expiringWithin`.

### Router

- `/downloads` -> `DownloadsScreen`, `/settings/downloads` also points there.

## Security

- Downloads are premium-only, enforced server-side (402 is upgrade prompt).
- Download URL is short-lived (~1h), distinct from licence expiry.

## Exit Criteria

- Downloads list from real API ✔
- Expiring banner and renewal ✔
- Remove works ✔
- Empty state ✔

## Verdict — PASS
