# RETIRED — do not extend

**Status:** marked for replacement, not yet deleted.
**Decision:** recorded in `docs/03-TECHNOLOGY-DECISIONS.md`.
**Replaced by:** PHASE 18.
**Size at retirement:** 749 lines.

## Why

D-4: a fresh Flutter app will consume clients/dart (iconfess_api) instead. This shell cannot play audio — just_audio and audio_service are commented out in pubspec.yaml — and it is smaller than the API client it would depend on.

## Why it is still here

The replacement is sequenced to PHASE 18. Deleting this directory now would leave the
repository without the surface it provides for every phase in between, and would
destroy the only working reference for how a client talks to the API.

Nothing here should be extended. If a piece is worth keeping, lift it
deliberately into the replacement, file by file, with a reason recorded in the
pull request.
