# PHASE 25 — Me / Profile

**Status:** PASS
**Date:** 2026-09-19
**Depends on:** PHASE 19 (auth), PHASE 18 (foundation)

## Objective

Replace Me placeholder with real profile: bootstrap, avatar, display name, bio, completion, entitlements, library entry points, premium, settings, sign-out.

## Implementation

- `me_providers.dart`: `bootstrapProvider` (GET /me/bootstrap), `profileProvider`, `preferencesProvider`, `interestsProvider`, `authSessionsProvider`.
- `me_screen.dart`: header with avatar (NetworkImage if present), display name fallback to email, completion progress bar, entitlements summary (plan, max minutes), sections: Your library (Library, Templates, Downloads), Account (Edit profile, Interests, Premium, Settings), Sign out (clears cache and token, routes to welcome).
- `ProfileEditScreen`: display name and bio fields, save via PATCH /me/profile, invalidates bootstrap and profile providers.
- Router: `/me` -> `MeScreen`, `/me/edit` -> `ProfileEditScreen`, `/me/settings` -> `SettingsScreen`, `/me/premium` -> `PremiumScreen`.

## Security

- Sign-out clears local cache and token even if server call fails.
- Avatar URL from server, no HTML rendering.

## Exit Criteria

- Me shows real bootstrap data, not placeholder ✔
- Edit profile updates server and reflects immediately ✔
- Sign-out works and routes to welcome ✔
- Library/templates/downloads/premium/settings reachable from Me ✔

## Verdict — PASS
