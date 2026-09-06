#!/usr/bin/env python3
"""Generate platform code from design/tokens.json.

One source of truth, three consumers. Hand-editing anything under
design/generated/ is a bug: the next run overwrites it and the platforms drift
apart, which is the failure this file exists to prevent.

Usage: python3 design/generate.py [--check]
  --check   verify generated files are up to date; exit non-zero if not.
"""
import json
import re
import sys
from pathlib import Path

HERE = Path(__file__).parent
OUT = HERE / "generated"
REF = re.compile(r"\{([^}]+)\}")

HEADER = "Generated from design/tokens.json by design/generate.py - DO NOT EDIT.\n"


def load():
    data = json.loads((HERE / "tokens.json").read_text())
    flat, order = {}, []

    def walk(node, prefix):
        for key, val in node.items():
            if key.startswith("$"):
                continue
            path = f"{prefix}.{key}" if prefix else key
            if isinstance(val, dict) and "$value" in val:
                flat[path] = val["$value"]
                order.append(path)
            elif isinstance(val, dict):
                walk(val, path)

    walk(data, "")

    for _ in range(10):
        changed = False
        for path in order:
            val = flat[path]
            if not isinstance(val, str):
                continue

            def sub(m, _p=path):
                target = m.group(1)
                if target not in flat:
                    raise KeyError(f"{_p} references unknown token {{{target}}}")
                return str(flat[target])

            new = REF.sub(sub, val)
            if new != val:
                flat[path] = new
                changed = True
        if not changed:
            break

    for path in order:
        if isinstance(flat[path], str) and "{" in flat[path]:
            raise ValueError(f"unresolved reference in {path}: {flat[path]}")
    return data, flat, order


def camel(path: str) -> str:
    parts = re.split(r"[._]", path)
    return parts[0] + "".join(p[:1].upper() + p[1:] for p in parts[1:])


def css_name(path: str) -> str:
    return "--ic-" + path.replace(".", "-")


def dart_color(hexstr: str) -> str:
    h = hexstr.lstrip("#").upper()
    return f"Color(0xFF{h})"


def gen_css(flat, order):
    lines = [f"/* {HEADER} */", ":root {"]
    for path in order:
        lines.append(f"  {css_name(path)}: {flat[path]};")
    lines += ["}", ""]
    for theme in ("dark", "light"):
        lines.append(f'[data-theme="{theme}"] {{')
        for path in order:
            if path.startswith(f"theme.{theme}."):
                role = path.split(".")[-1]
                lines.append(f"  --ic-{role}: var({css_name(path)});")
        lines += ["}", ""]
    return "\n".join(lines)


def gen_ts(flat, order):
    lines = [f"// {HEADER}", "// Values are strings as written in tokens.json.",
             "export const tokens = {"]
    for path in order:
        val = flat[path]
        rendered = val if isinstance(val, (int, float)) else json.dumps(val)
        lines.append(f"  {json.dumps(path)}: {rendered},")
    lines += ["} as const;", ""]
    lines.append("export type TokenPath = keyof typeof tokens;")
    for theme in ("dark", "light"):
        lines.append(f"export const {theme}Theme = {{")
        for path in order:
            if path.startswith(f"theme.{theme}."):
                lines.append(f"  {path.split('.')[-1]}: tokens[{json.dumps(path)}],")
        lines.append("} as const;")
    lines.append("")
    return "\n".join(lines)


def gen_dart(data, flat, order):
    L = [f"// {HEADER}", "// ignore_for_file: constant_identifier_names",
         "import 'package:flutter/material.dart';", "",
         "/// I CONFESS design tokens. Section 12 of the build directive.",
         "/// Values are generated; edit design/tokens.json instead.",
         "abstract final class IConfess {"]

    L.append("  // --- spacing (4..64) ---")
    for path in order:
        if path.startswith("spacing."):
            px = float(flat[path].replace("px", ""))
            num = int(px) if px == int(px) else px
            L.append(f"  static const double space{camel(path)[7:]} = {num};")

    L.append("")
    L.append("  // --- radius ---")
    for path in order:
        if path.startswith("radius."):
            px = float(flat[path].replace("px", ""))
            num = int(px) if px == int(px) else px
            L.append(f"  static const double radius{camel(path)[6:].upper()[:1]}"
                     f"{camel(path)[7:]} = {num};")

    L.append("")
    L.append("  // --- colour ---")
    for path in order:
        v = flat[path]
        if isinstance(v, str) and re.fullmatch(r"#[0-9A-Fa-f]{6}", v):
            L.append(f"  static const Color {camel(path)} = {dart_color(v)};")

    L.append("")
    L.append("  // --- type scale ---")
    size = {p.split(".")[-1]: float(flat[p].replace("px", ""))
            for p in order if p.startswith("font.size.")}
    weight = {p.split(".")[-1]: int(flat[p])
              for p in order if p.startswith("font.weight.")}
    lh = {p.split(".")[-1]: float(flat[p])
          for p in order if p.startswith("font.lineHeight.")}
    ls = {p.split(".")[-1]: float(flat[p].replace("em", ""))
          for p in order if p.startswith("font.letterSpacing.")}
    fam = {p: flat[p] for p in order if p.startswith("font.family.")}

    def family_expr(path):
        """First quoted family in the CSS stack. Flutter resolves one name;
        the stack exists for the web. Falling back to a default here would
        silently erase the serif/sans split, so an unknown family is an error."""
        stack = fam.get(path)
        if stack is None:
            raise KeyError(f"typography references unknown font family {{{path}}}")
        m = re.search(r"'([^']+)'", stack)
        if not m:
            raise ValueError(f"font family {{{path}}} has no quoted primary: {stack}")
        return "'" + m.group(1) + "'"

    for role, spec in data["typography"].items():
        if role.startswith("$"):
            continue
        s = spec["fontSize"].strip("{}").split(".")[-1]
        w = spec["fontWeight"].strip("{}").split(".")[-1]
        h = spec["lineHeight"].strip("{}").split(".")[-1]
        k = spec["letterSpacing"].strip("{}").split(".")[-1]
        f = spec["fontFamily"].strip("{}")
        L.append(
            f"  static const TextStyle {role} = TextStyle("
            f"fontFamily: {family_expr(f)}, "
            f"fontSize: {size[s]:g}, fontWeight: FontWeight.w{weight[w]}, "
            f"height: {lh[h]:g}, letterSpacing: {ls[k]:g});")

    L.append("")
    L.append("  // --- motion ---")
    for path in order:
        if path.startswith("motion.duration."):
            ms = int(flat[path].replace("ms", ""))
            L.append(f"  static const Duration {camel(path)} = "
                     f"Duration(milliseconds: {ms});")

    L += ["}", ""]

    for theme in ("dark", "light"):
        cls = "IConfessDark" if theme == "dark" else "IConfessLight"
        L += [f"abstract final class {cls} {{"]
        for path in order:
            if path.startswith(f"theme.{theme}."):
                v = flat[path]
                if isinstance(v, str) and re.fullmatch(r"#[0-9A-Fa-f]{6}", v):
                    role = path.split(".")[-1]
                    L.append(f"  static const Color {role[0].lower()}{role[1:]} = "
                             f"{dart_color(v)};")
        L += ["}", ""]
    return "\n".join(L)


# The Flutter app consumes the Dart tokens directly, and a Dart package cannot
# import across the repository from design/generated. So the same bytes are
# mirrored into the app's own source tree.
#
# Mirroring from the generator rather than copying by hand is what keeps the two
# from drifting, and --check covers the mirror as well as the original: an app
# theme built on a stale palette is exactly the failure the check exists to
# catch. A mirror whose app directory does not exist is skipped, so this does not
# couple the design build to an app that has not been created yet.
MIRRORS = {
    "tokens.dart": [
        HERE.parent / "apps" / "mobile" / "lib" / "src" / "core" / "theme" / "tokens.dart",
    ],
}


def _mirror_targets(name: str):
    """Yields the mirror paths whose app is present.

    The test is whether the *app root* exists, not the destination directory.
    Testing the destination would make --check skip a mirror that is merely
    missing, so an app whose tokens were never generated would pass the
    staleness check and then fail to compile.
    """
    for target in MIRRORS.get(name, []):
        if _app_root(target).exists():
            yield target


def _app_root(target: Path) -> Path:
    """Walks up from a mirror path to the app it belongs to (the apps/<name> dir)."""
    for parent in target.parents:
        if parent.parent.name == "apps":
            return parent
    return target.parent


def main() -> int:
    check = "--check" in sys.argv
    data, flat, order = load()
    OUT.mkdir(exist_ok=True)

    files = {
        "tokens.css": gen_css(flat, order),
        "tokens.ts": gen_ts(flat, order),
        "tokens.dart": gen_dart(data, flat, order),
    }

    stale = []
    for name, content in files.items():
        targets = [OUT / name] + list(_mirror_targets(name))
        for target in targets:
            existing = target.read_text() if target.exists() else None
            if check:
                if existing != content:
                    stale.append(str(target.relative_to(HERE.parent)))
            else:
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_text(content)
                print(f"  wrote {target.relative_to(HERE.parent)} ({len(content)} bytes)")

    if check:
        if stale:
            print("GENERATED CODE IS STALE: " + ", ".join(stale))
            print("Run: python3 design/generate.py")
            return 1
        print(f"generated code up to date ({len(flat)} tokens, {len(files)} files)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
