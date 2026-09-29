# Mobile color-token cleanup (G-16 re-audit)

**Date:** 2026-09-29

**Scope:** mobile color references and a design regression guard. No route,
schema, API contract, or generated token value changed.

## Reconciliation

The reported 92 `Color(0xFF...)` matches counted generated constants along with
application source. On the base checkout, the generated `tokens.dart` alone had
91 such declarations (`git show 251f906:apps/mobile/lib/src/core/theme/tokens.dart
| grep -c 'Color(0xFF'`), and there was one application literal:
`background_playback_service.dart`'s purple notification default. Thus 92 was
the total grep count, not 92 bypasses of the token system.

A source-only scan (`grep ... --exclude=tokens.dart`) found that one literal.
The surrounding app code also used Flutter's named `Colors` palette directly
for white, black, status, and warning colors; those references were not part of
the 92 count.

## Change

- Replaced the notification purple and named Material palette references with
  the existing generated `IConfess` tokens for brand, neutral, success, warning,
  and danger colors. `Colors.transparent` remains for a transparent UI state,
  not a color choice.
- Extended `design/test_design.py` to fail on handwritten `Color(0xFF...)`
  constructors or named Material palette colors in mobile library source,
  excluding only the generated `tokens.dart` and `Colors.transparent`.
- Added negative controls: a temporary raw hex and `Colors.purple` source probe
  made both assertions fail with the offending values. The probe was removed.

## Evidence

```text
grep -RohE 'Color\(0xFF[0-9A-Fa-f]{6}\)' apps/mobile/lib --include='*.dart' --exclude='tokens.dart' | wc -l
# 0

grep -RohE 'Colors\.[A-Za-z_][A-Za-z0-9_]*(\.shade[0-9]+)?' apps/mobile/lib --include='*.dart' --exclude='tokens.dart' | grep -v '^Colors\.transparent$' | wc -l
# 0

python3 design/test_design.py
# DESIGN SYSTEM CHECK PASSED: 132 tokens, all Section 12 rules hold.

python3 scripts/check_dart_symbols.py
# PASSED: 133/133
```

Flutter/Dart SDKs are not installed in this sandbox, so `flutter analyze` and
`flutter test` remain CI-only. The design checks are wired into the existing CI
design step.

## Gap status

The specific G-16 finding—handwritten color choices bypassing generated design
tokens—is closed by the source scan and recurrence guard. Flutter rendering and
analyzer/test validation are still owed to CI; this ledger does not claim those
commands ran locally.
