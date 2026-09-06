#!/usr/bin/env python3
"""Verify that every text/background pairing in tokens.json meets WCAG 2.1 AA.

A design system that ships with failing contrast gets patched per-screen later,
which is how it ends up inconsistent. Checking it here means the tokens cannot
be merged in a state that is illegible.

Usage: python3 design/check_contrast.py
Exits non-zero if any required pairing fails.
"""
import json
import re
import sys
from pathlib import Path

TOKENS = Path(__file__).parent / "tokens.json"

# Minimum contrast required. 4.5 is WCAG AA for normal text; 3.0 is AA for
# large text (>= 18pt / 14pt bold) and for non-text UI boundaries.
NORMAL_TEXT = 4.5
LARGE_TEXT = 3.0
UI_BOUNDARY = 3.0


def hex_to_rgb(h: str):
    h = h.lstrip("#")
    return tuple(int(h[i:i + 2], 16) for i in (0, 2, 4))


def _channel(c: int) -> float:
    s = c / 255.0
    return s / 12.92 if s <= 0.04045 else ((s + 0.055) / 1.055) ** 2.4


def luminance(rgb) -> float:
    r, g, b = (_channel(c) for c in rgb)
    return 0.2126 * r + 0.7152 * g + 0.0722 * b


def contrast(fg: str, bg: str) -> float:
    l1, l2 = luminance(hex_to_rgb(fg)), luminance(hex_to_rgb(bg))
    hi, lo = max(l1, l2), min(l1, l2)
    return (hi + 0.05) / (lo + 0.05)


def load_resolved():
    """Flatten tokens.json and resolve {path.to.value} references."""
    data = json.loads(TOKENS.read_text())

    flat = {}

    def walk(node, prefix):
        for key, val in node.items():
            if key.startswith("$"):
                continue
            path = f"{prefix}.{key}" if prefix else key
            if isinstance(val, dict) and "$value" in val:
                flat[path] = val["$value"]
            elif isinstance(val, dict):
                walk(val, path)

    walk(data, "")

    # Resolve references, repeatedly, until nothing changes (handles chains).
    ref = re.compile(r"\{([^}]+)\}")
    for _ in range(10):
        changed = False
        for path, val in list(flat.items()):
            if not isinstance(val, str):
                continue

            def sub(m):
                target = m.group(1)
                if target not in flat:
                    raise KeyError(f"{path} references unknown token {{{target}}}")
                return str(flat[target])

            new = ref.sub(sub, val)
            if new != val:
                flat[path] = new
                changed = True
        if not changed:
            break

    unresolved = [p for p, v in flat.items() if isinstance(v, str) and "{" in v]
    if unresolved:
        raise ValueError(f"unresolved references: {unresolved}")
    return flat


# Pairings that must hold. (foreground, background, minimum, what it is)
def pairings(tok):
    out = []
    for theme in ("dark", "light"):
        def t(name, _theme=theme):
            return tok["theme." + _theme + "." + name]

        bgs = [("background", t("background")), ("surface", t("surface")),
               ("surfaceRaised", t("surfaceRaised"))]
        for bgname, bg in bgs:
            out += [
                (t("textPrimary"), bg, NORMAL_TEXT, theme + ": primary text on " + bgname),
                (t("textSecondary"), bg, NORMAL_TEXT, theme + ": secondary text on " + bgname),
            ]
        out += [
            (t("onPrimary"), t("primary"), NORMAL_TEXT, theme + ": label on primary button"),
            (t("onPrimary"), t("primaryStrong"), NORMAL_TEXT, theme + ": label on pressed button"),
            (t("primary"), t("background"), NORMAL_TEXT, theme + ": primary as link text"),
            (t("primary"), t("surface"), NORMAL_TEXT, theme + ": primary on surface"),
            (t("background"), t("primary"), UI_BOUNDARY, theme + ": primary against background"),
        ]
    # Semantic colours are used as text and as icons, on both themes' surfaces.
    for theme in ("dark", "light"):
        surf = tok["theme." + theme + ".background"]
        for name in ("success", "warning", "danger", "info"):
            out.append((tok["theme." + theme + "." + name], surf, NORMAL_TEXT,
                        theme + ": semantic " + name + " on background"))
    return out


def main() -> int:
    flat = load_resolved()
    checks = pairings(flat)

    failures, results = [], []
    for fg, bg, minimum, label in checks:
        ratio = contrast(fg, bg)
        ok = ratio >= minimum
        results.append((ok, ratio, minimum, label, fg, bg))
        if not ok:
            failures.append(results[-1])

    width = max(len(r[3]) for r in results)
    for ok, ratio, minimum, label, fg, bg in results:
        print(f"  {'PASS' if ok else 'FAIL'}  {ratio:5.2f}:1  (need {minimum:.1f})  {label:<{width}}  {fg} on {bg}")

    print()
    if failures:
        print(f"CONTRAST CHECK FAILED: {len(failures)} of {len(checks)} pairings below WCAG AA.")
        for _, ratio, minimum, label, fg, bg in failures:
            print(f"  {label}: {ratio:.2f}:1, needs {minimum:.1f}:1 ({fg} on {bg})")
        return 1

    print(f"CONTRAST CHECK PASSED: all {len(checks)} pairings meet WCAG 2.1 AA.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
