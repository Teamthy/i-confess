#!/usr/bin/env python3
"""Generate every brand asset from the single traced mark.

Source of truth: design/brand/mark.svg, a trace of the approved mark
(design/brand/source-mark.jpg; the full lockup is source-lockup.jpg). Edit the
mark there and re-run this script; never edit an output by hand, the next run
overwrites it.

Outputs
  web/app/icon.svg                    favicon (modern browsers)
  web/app/favicon.ico                 favicon (16/32/48, legacy browsers)
  web/app/apple-icon.png              iOS home-screen bookmark (180)
  web/public/brand/mark.svg           mark in currentColor, for <img>/CSS use
  web/public/brand/icon-192.png       PWA icons
  web/public/brand/icon-512.png
  web/public/brand/icon-maskable-512.png
  web/public/brand/og-mark.png        1200x630 social card fallback
  web/lib/brand-mark.ts               path data for the React <Mark/>
  apps/mobile/lib/src/core/brand/brand_mark_path.dart   Flutter Path
  apps/mobile/ios/.../AppIcon.appiconset/*.png          iOS launcher icons
  apps/mobile/android/.../mipmap-*/ic_launcher.png      Android launcher icons
  server/internal/{webapp,adminui}/favicon.svg   favicons for the Go-served pages

Usage
  python3 design/brand/generate.py           write everything
  python3 design/brand/generate.py --check   fail if a text output is stale
                                             (PNGs are not compared: their
                                             bytes depend on the Pillow build)
"""
from __future__ import annotations

import io
import re
import sys
from pathlib import Path


HERE = Path(__file__).resolve().parent
ROOT = HERE.parent.parent

# Brand colours (design/tokens.json): brand-800 is the icon field, brand-900
# the deepest navy used for the mark on light backgrounds.
NAVY = (0x05, 0x16, 0x50)  # brand-800
DEEP = (0x00, 0x07, 0x2D)  # brand-900
WHITE = (255, 255, 255)

HEADER = "Generated from design/brand/mark.svg by design/brand/generate.py - DO NOT EDIT."


# --------------------------------------------------------------------------
# Path parsing and rasterising
# --------------------------------------------------------------------------

def read_mark() -> tuple[str, float, float]:
    svg = (HERE / "mark.svg").read_text()
    vb = re.search(r'viewBox="0 0 ([\d.]+) ([\d.]+)"', svg)
    d = re.search(r' d="([^"]+)"', svg)
    if not vb or not d:
        sys.exit("design/brand/mark.svg: expected a viewBox and one path")
    return d.group(1), float(vb.group(1)), float(vb.group(2))


def parse(d: str) -> list[list[tuple]]:
    """Split absolute M/L/C/Z path data into subpaths of commands."""
    tokens = re.findall(r"[MLCZ]|-?\d+(?:\.\d+)?", d)
    subpaths, cur, i = [], None, 0
    while i < len(tokens):
        t = tokens[i]
        if t == "M":
            cur = [("M", float(tokens[i + 1]), float(tokens[i + 2]))]
            subpaths.append(cur)
            i += 3
        elif t == "L":
            cur.append(("L", float(tokens[i + 1]), float(tokens[i + 2])))
            i += 3
        elif t == "C":
            cur.append(("C", *map(float, tokens[i + 1:i + 7])))
            i += 7
        elif t == "Z":
            cur.append(("Z",))
            i += 1
        else:
            sys.exit(f"mark.svg: unsupported path token {t!r} (only absolute M/L/C/Z)")
    return subpaths


def flatten(sub: list[tuple], steps: int = 24) -> list[tuple[float, float]]:
    pts: list[tuple[float, float]] = []
    x = y = 0.0
    for c in sub:
        if c[0] in ("M", "L"):
            x, y = c[1], c[2]
            pts.append((x, y))
        elif c[0] == "C":
            x1, y1, x2, y2, x3, y3 = c[1:]
            for k in range(1, steps + 1):
                t = k / steps
                u = 1 - t
                pts.append((u**3 * x + 3 * u * u * t * x1 + 3 * u * t * t * x2 + t**3 * x3,
                            u**3 * y + 3 * u * u * t * y1 + 3 * u * t * t * y2 + t**3 * y3))
            x, y = x3, y3
    return pts


def mark_mask(size_px: float, subpaths, vb_w: float, vb_h: float, ss: int = 4):
    """Anti-aliased even-odd mask of the mark, size_px wide."""
    from PIL import Image, ImageChops, ImageDraw
    scale = size_px * ss / vb_w
    W, H = round(vb_w * scale), round(vb_h * scale)
    acc = Image.new("1", (W, H), 0)
    for sub in subpaths:
        layer = Image.new("1", (W, H), 0)
        ImageDraw.Draw(layer).polygon([(px * scale, py * scale) for px, py in flatten(sub)], fill=1)
        acc = ImageChops.logical_xor(acc, layer)
    return acc.convert("L").resize((max(1, round(W / ss)), max(1, round(H / ss))), Image.LANCZOS)


def icon(size: int, subpaths, vb, *, fill=NAVY, ink=WHITE, mark_ratio=0.6,
         radius_ratio=0.0, alpha=False):
    """A square icon: the mark centred on a brand field."""
    from PIL import Image, ImageDraw
    ss = 4
    big = size * ss
    mode = "RGBA" if alpha else "RGB"
    bg = Image.new(mode, (big, big), (0, 0, 0, 0) if alpha else fill)
    if alpha:
        field = Image.new("L", (big, big), 0)
        ImageDraw.Draw(field).rounded_rectangle([0, 0, big - 1, big - 1], radius=round(big * radius_ratio), fill=255)
        bg.paste(Image.new("RGBA", (big, big), fill + (255,)), (0, 0), field)
    m = mark_mask(big * mark_ratio, subpaths, *vb, ss=2)
    ox, oy = (big - m.width) // 2, (big - m.height) // 2
    bg.paste(Image.new(mode, m.size, ink + ((255,) if alpha else ())), (ox, oy), m)
    return bg.resize((size, size), Image.LANCZOS)


# --------------------------------------------------------------------------
# Text outputs
# --------------------------------------------------------------------------

def icon_svg(d: str, vb_w: float, vb_h: float) -> str:
    """Favicon SVG: white mark on a rounded navy field (readable on any tab bar)."""
    size = 64
    mark_w = size * 0.68
    s = mark_w / vb_w
    tx = (size - mark_w) / 2
    ty = (size - vb_h * s) / 2
    return (f'<!-- {HEADER} -->\n'
            f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {size} {size}">'
            f'<rect width="{size}" height="{size}" rx="14" fill="#051650"/>'
            f'<path fill="#FFFFFF" fill-rule="evenodd" transform="translate({tx:.2f} {ty:.2f}) scale({s:.4f})" d="{d}"/>'
            f'</svg>\n')


def mark_svg_public(d: str, vb_w: float, vb_h: float) -> str:
    return (f'<!-- {HEADER} -->\n'
            f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {vb_w:g} {vb_h:g}" role="img" aria-label="I CONFESS">'
            f'<path fill="#00072D" fill-rule="evenodd" d="{d}"/></svg>\n')


def ts_module(d: str, vb_w: float, vb_h: float) -> str:
    return (f"// {HEADER}\n\n"
            f"/** viewBox of the I CONFESS mark. */\n"
            f'export const MARK_VIEWBOX = "0 0 {vb_w:g} {vb_h:g}";\n'
            f"export const MARK_ASPECT = {vb_h / vb_w:.4f};\n\n"
            f"/** Even-odd path data of the mark; fill with currentColor. */\n"
            f'export const MARK_PATH =\n  "{d}";\n')


def dart_module(subpaths, vb_w: float, vb_h: float) -> str:
    lines = [f"// {HEADER}", "",
             "import 'dart:ui';", "",
             "/// Width and height of the coordinate space [brandMarkPath] is drawn in.",
             f"const double brandMarkWidth = {vb_w:g};",
             f"const double brandMarkHeight = {vb_h:g};", "",
             "/// The I CONFESS mark as an even-odd [Path] in a",
             "/// [brandMarkWidth] x [brandMarkHeight] box.",
             "Path brandMarkPath() {",
             "  final p = Path()..fillType = PathFillType.evenOdd;"]
    for sub in subpaths:
        for c in sub:
            if c[0] == "M":
                lines.append(f"  p.moveTo({c[1]:g}, {c[2]:g});")
            elif c[0] == "L":
                lines.append(f"  p.lineTo({c[1]:g}, {c[2]:g});")
            elif c[0] == "C":
                lines.append("  p.cubicTo(" + ", ".join(f"{v:g}" for v in c[1:]) + ");")
            else:
                lines.append("  p.close();")
    lines += ["  return p;", "}", ""]
    return "\n".join(lines)


# --------------------------------------------------------------------------

def png_bytes(img, **kw) -> bytes:
    buf = io.BytesIO()
    img.save(buf, format=kw.pop("format", "PNG"), **kw)
    return buf.getvalue()


def main() -> int:
    check = "--check" in sys.argv[1:]
    d, vb_w, vb_h = read_mark()
    subpaths = parse(d)
    vb = (vb_w, vb_h)

    text_outputs = {
        ROOT / "web/app/icon.svg": icon_svg(d, vb_w, vb_h),
        ROOT / "web/public/brand/mark.svg": mark_svg_public(d, vb_w, vb_h),
        ROOT / "web/lib/brand-mark.ts": ts_module(d, vb_w, vb_h),
        # The Go binary serves two embedded pages (listener site at /, admin
        # console at /admin/); each embeds its own copy of the favicon.
        ROOT / "server/internal/webapp/favicon.svg": icon_svg(d, vb_w, vb_h),
        ROOT / "server/internal/adminui/favicon.svg": icon_svg(d, vb_w, vb_h),
        ROOT / "apps/mobile/lib/src/core/brand/brand_mark_path.dart": dart_module(subpaths, vb_w, vb_h),
    }

    if check:
        stale = [str(p.relative_to(ROOT)) for p, body in text_outputs.items()
                 if not p.exists() or p.read_text() != body]
        if stale:
            print("brand assets are stale; run python3 design/brand/generate.py:\n  " + "\n  ".join(stale))
            return 1
        print(f"brand assets up to date ({len(text_outputs)} text outputs)")
        return 0

    for path, body in text_outputs.items():
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(body)

    # Pillow is needed only to draw the images; --check (run in CI without
    # it) never reaches this point.
    from PIL import Image

    binary: dict[Path, bytes] = {}
    # Favicon (.ico) carries a rounded navy field so it reads on light and dark tabs.
    fav = icon(256, subpaths, vb, mark_ratio=0.68, radius_ratio=0.22, alpha=True)
    binary[ROOT / "web/app/favicon.ico"] = png_bytes(fav, format="ICO", sizes=[(16, 16), (32, 32), (48, 48)])
    # Apple and PWA icons are full-bleed: the platform applies its own mask.
    binary[ROOT / "web/app/apple-icon.png"] = png_bytes(icon(180, subpaths, vb, mark_ratio=0.6))
    binary[ROOT / "web/public/brand/icon-192.png"] = png_bytes(icon(192, subpaths, vb, mark_ratio=0.6, radius_ratio=0.22, alpha=True))
    binary[ROOT / "web/public/brand/icon-512.png"] = png_bytes(icon(512, subpaths, vb, mark_ratio=0.6, radius_ratio=0.22, alpha=True))
    # Maskable: the mark stays inside the 80% safe circle.
    binary[ROOT / "web/public/brand/icon-maskable-512.png"] = png_bytes(icon(512, subpaths, vb, mark_ratio=0.46))

    # Social card fallback: white mark on deep navy, 1200x630.
    og = Image.new("RGB", (1200, 630), DEEP)
    m = mark_mask(260, subpaths, *vb)
    og.paste(Image.new("RGB", m.size, WHITE), ((1200 - m.width) // 2, (630 - m.height) // 2), m)
    binary[ROOT / "web/public/brand/og-mark.png"] = png_bytes(og)

    # iOS launcher icons: opaque (App Store rejects alpha), full bleed.
    ios = ROOT / "apps/mobile/ios/Runner/Assets.xcassets/AppIcon.appiconset"
    for f in sorted(ios.glob("Icon-App-*.png")):
        size_pt, scale = re.match(r"Icon-App-([\d.]+)x[\d.]+@(\d)x\.png", f.name).groups()
        px = round(float(size_pt) * int(scale))
        binary[f] = png_bytes(icon(px, subpaths, vb, mark_ratio=0.58))

    # Android legacy launcher icons: full bleed; launchers apply their mask.
    android = ROOT / "apps/mobile/android/app/src/main/res"
    for density, px in {"mdpi": 48, "hdpi": 72, "xhdpi": 96, "xxhdpi": 144, "xxxhdpi": 192}.items():
        binary[android / f"mipmap-{density}/ic_launcher.png"] = png_bytes(icon(px, subpaths, vb, mark_ratio=0.56))

    for path, body in binary.items():
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(body)

    print(f"wrote {len(text_outputs)} text and {len(binary)} image assets")
    return 0


if __name__ == "__main__":
    sys.exit(main())
