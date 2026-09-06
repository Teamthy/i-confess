# RETIRED — do not extend

**Status:** marked for replacement, not yet deleted.
**Decision:** recorded in `docs/03-TECHNOLOGY-DECISIONS.md`.
**Replaced by:** PHASE 40.
**Size at retirement:** 168 lines.

## Why

D-5: rebuilt against the PHASE 05 design system.

## Why it is still here

The replacement is sequenced to PHASE 40. Deleting this directory now would leave the
repository without the surface it provides for every phase in between, and would
destroy the only working reference for how a client talks to the API.

Nothing here should be extended. If a piece is worth keeping, lift it
deliberately into the replacement, file by file, with a reason recorded in the
pull request.
