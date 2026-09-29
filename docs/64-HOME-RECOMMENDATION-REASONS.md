# 64 — Explain home recommendations (G-56)

**Status: implemented; static/design gates passed, Flutter widget test awaits CI.**

Home now renders the recommendation evidence the server already returns instead
of presenting ranked content as an unexplained feed.

## Listener experience

- The **Picked for you** rail includes category and confession cards. Each card
  renders a short plain-language explanation translated from the server's
  reason codes (chosen interest, saved item, repeated listening, category
  history, time-of-day match, or a different direction from recent listening).
- When no listener signal exists, the copy says the cards are starting points;
  it does not call the feed personalized or imply history that is not present.
- The server's suggested session duration is shown only when it is positive.
- A **listen again** item explains how often the listener returned to it.
- The rail loads independently; if recommendation loading fails, the rest of
  Home remains available.

## Verification

- Added a Flutter widget test for the rendered reasons, repeat count, and
  suggested duration. It could not be run locally because Flutter/Dart are not
  installed; it remains a CI gate.
- `scripts/check_dart_symbols.py` passed 140/140 after adding Home-specific
  provider and rendering checks.
- `design/test_ia.py` passed (40 screens, 8 entry points, 163 endpoints).
- The Go build, vet and race suite passed in ledger 63; this phase changes only
  the mobile presentation of the existing typed recommendation payload.

G-52 remains absent and unverified. Do not open a PR until the G-52 change and
its tests are restored and the full suite passes.
