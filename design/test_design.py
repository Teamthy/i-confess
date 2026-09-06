#!/usr/bin/env python3
"""Section 12 compliance tests for the design system.

Section 12 of the build directive specifies exact values: spacing 4 to 64,
radius 8/12/16/20/999, a green primary, seven type roles, and a five-item
bottom navigation. Written down, those rules decay - a sixth tab appears, a
28px radius creeps in, the primary drifts teal. As tests, they cannot.

Usage: python3 design/test_design.py
"""
import colorsys
import json
import re
import subprocess
import sys
from pathlib import Path

HERE = Path(__file__).parent
sys.path.insert(0, str(HERE))

import check_contrast  # noqa: E402
import generate  # noqa: E402

TOKENS = json.loads((HERE / "tokens.json").read_text())
failures = []


def check(name, condition, detail=""):
    status = "PASS" if condition else "FAIL"
    print(f"  {status}  {name}" + (f"  -- {detail}" if detail and not condition else ""))
    if not condition:
        failures.append(name)


def hue_degrees(hexstr: str) -> float:
    r, g, b = check_contrast.hex_to_rgb(hexstr)
    h, _, _ = colorsys.rgb_to_hsv(r / 255, g / 255, b / 255)
    return h * 360


print("Section 12 - spacing")
spacing = {k: float(v["$value"].replace("px", ""))
           for k, v in TOKENS["spacing"].items() if not k.startswith("$")}
steps = sorted(spacing.values())
check("scale runs 4 to 64", steps[1] == 4 and steps[-1] == 64, f"got {steps}")
check("every step is a multiple of 4",
      all(s % 4 == 0 for s in steps), f"got {steps}")
check("no value outside 4..64 except zero",
      all(s == 0 or 4 <= s <= 64 for s in steps), f"got {steps}")
check("scale is monotonically increasing", steps == sorted(set(steps)))

print("\nSection 12 - radius")
radii = sorted(float(v["$value"].replace("px", ""))
               for k, v in TOKENS["radius"].items() if not k.startswith("$"))
check("exactly 8, 12, 16, 20, 999", radii == [8, 12, 16, 20, 999], f"got {radii}")

print("\nSection 12 - green primary")
brand500 = TOKENS["color"]["brand"]["500"]["$value"]
hue = hue_degrees(brand500)
check("brand 500 hue is green (100-170 deg)", 100 <= hue <= 170,
      f"{brand500} is {hue:.0f} deg")
for theme in ("dark", "light"):
    prim = check_contrast.load_resolved()[f"theme.{theme}.primary"]
    check(f"{theme} theme primary is green", 100 <= hue_degrees(prim) <= 170,
          f"{prim} is {hue_degrees(prim):.0f} deg")
# The mobile shell that this replaces seeds Material from indigo (0xFF7C8CF8,
# about 231 deg). That is the regression this assertion exists to block.
indigo = hue_degrees("#7C8CF8")
check("primary is not the retired indigo", abs(hue - indigo) > 40,
      f"brand hue {hue:.0f} is too close to indigo {indigo:.0f}")

print("\nSection 12 - typography roles")
roles = {k for k in TOKENS["typography"] if not k.startswith("$")}
required = {"display", "heading", "subheading", "body", "caption", "label", "metric"}
check("all seven required roles present", required <= roles,
      f"missing {required - roles}")
check("only bodySm is added beyond the seven", roles - required == {"bodySm"},
      f"unexpected extra roles: {roles - required}")
for role in roles:
    spec = TOKENS["typography"][role]
    check(f"{role} defines all five properties",
          set(spec) == {"fontFamily", "fontSize", "fontWeight", "lineHeight", "letterSpacing"},
          f"got {sorted(spec)}")
check("scripture body is set apart in a serif",
      "serif" in TOKENS["typography"]["body"]["fontFamily"].lower(),
      TOKENS["typography"]["body"]["fontFamily"])
check("interface roles are sans",
      all("serif" not in TOKENS["typography"][r]["fontFamily"].lower()
          for r in roles - {"body"}))

print("\nSection 12 - bottom navigation")
nav = [n.strip() for n in
       TOKENS["navigation"]["bottom"]["$value"].split(",")]
check("exactly five destinations", len(nav) == 5, f"got {len(nav)}: {nav}")
check("in the mandated order",
      nav == ["home", "explore", "confess", "activity", "profile"], f"got {nav}")

print("\nAccessibility")
rc = subprocess.run([sys.executable, str(HERE / "check_contrast.py")],
                    capture_output=True, text=True)
check("all text pairings meet WCAG 2.1 AA", rc.returncode == 0,
      rc.stdout.strip().splitlines()[-1] if rc.stdout else rc.stderr[-300:])

print("\nGenerated code")
_, flat, order = generate.load()
rc = subprocess.run([sys.executable, str(HERE / "generate.py"), "--check"],
                    capture_output=True, text=True)
check("generated Dart/CSS/TS match tokens.json", rc.returncode == 0,
      rc.stdout.strip() + rc.stderr.strip())

dart = (HERE / "generated" / "tokens.dart").read_text()
css = (HERE / "generated" / "tokens.css").read_text()
ts = (HERE / "generated" / "tokens.ts").read_text()
check("every colour token reached Dart",
      all(flat[p].lstrip("#").upper() in dart
          for p in order if isinstance(flat[p], str) and flat[p].startswith("#")))
check("every colour token reached CSS",
      all(flat[p] in css for p in order
          if isinstance(flat[p], str) and flat[p].startswith("#")))
check("every token reached TypeScript",
      all(p in ts for p in order))
# An unresolved {path.to.token} surviving into generated code renders as a
# literal string at runtime. This pattern matches a reference and nothing that
# legitimately appears in Dart, CSS or TypeScript.
unresolved = re.compile(r"\{[a-zA-Z_][\w]*(?:\.[\w]+)+\}")
leaked = {name: unresolved.findall(text)
          for name, text in (("dart", dart), ("css", css), ("ts", ts))}
leaked = {k: v for k, v in leaked.items() if v}
check("no unresolved references leaked into output", not leaked, f"{leaked}")
check("body renders serif in generated Dart",
      "body = TextStyle(fontFamily: 'Source Serif 4'" in dart)

print()
if failures:
    print(f"DESIGN SYSTEM CHECK FAILED: {len(failures)} assertion(s)")
    for f in failures:
        print(f"  - {f}")
    sys.exit(1)
print(f"DESIGN SYSTEM CHECK PASSED: {len(order)} tokens, all Section 12 rules hold.")
