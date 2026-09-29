# Web design-token parity guard (G-57)

**Date:** 2026-09-29  
**Scope:** design-token source and test; no CSS values or page behavior changed.

## Finding

`web/app/globals.css` has 27 distinct custom properties with hex values (35
hex-valued declarations including theme overrides). The consolidated web app
hand-copied these values from `design/tokens.json` without an automated check.
The brand ramp was in sync, but inspection found 12 distinct CSS hex values
absent from the source token file, including the web warning/danger and dark
surface palette. A check added without addressing those current misses would
have made CI fail immediately.

## Change

- Added the existing web palette values to an explicit `color.web` group in
  `design/tokens.json`; this records the current site colors and changes no CSS
  value or rendering.
- Extended `design/test_design.py` to parse every `--name: #hex` custom-property
  declaration in `web/app/globals.css` and require the exact value (case
  insensitive) to occur as a token color in `design/tokens.json`.
- Regenerated the token outputs, including the mobile generated token file.
  The corpus now contains 132 tokens rather than 120.

The guard checks presence of each color value as requested; it does not claim
that every CSS variable is semantically mapped to a particular token key. The
CSS intentionally remains hand-authored, and a future redesign can still choose
new values by updating the source tokens first.

## Evidence

```text
python3 design/generate.py --check
# generated code up to date (132 tokens, 3 files)

python3 design/test_design.py
# DESIGN SYSTEM CHECK PASSED: 132 tokens, all Section 12 rules hold.
# Web token parity: PASS

python3 design/test_ia.py
# IA CHECK PASSED: 39 screens, 8 entry points, 162 endpoints wired.

python3 scripts/check_dart_symbols.py
# PASSED: 133/133
```

A negative control appended `--token-drift-probe: #010203` to the CSS and ran
`python3 design/test_design.py`; the web parity assertion failed and named the
unrecognized property/value. The temporary probe was then removed. The test is
part of `.github/workflows/ci.yml`'s existing design-test step.

## Gap status

G-57 is closed: changes to a CSS custom property's hex value must be represented
in `design/tokens.json` for the design test to pass. This is the check-based fix;
the generated-token symlink was not restored.
