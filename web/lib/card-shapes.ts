/* ============================================================
   I CONFESS web — the photo cut-outs on category cards.

   Each card shows its photo through one of four shapes — circle,
   quatrefoil, shield, diamond — with a lighter halo band around it.
   Shapes are built from two primitives (circle, rounded polygon) in a
   300×230 art box, then emitted twice:
     - in user units, for the halo each card draws in its own <svg>;
     - normalised to 0..1, for one shared <clipPath objectBoundingBox>
       per shape (CardShapeDefs, rendered once in the root layout), so
       no card needs a unique SVG id — the home rail renders each card
       twice for its loop.
   ============================================================ */

export const ART_W = 300;
export const ART_H = 230;

type Pt = [number, number];
type Prim = { kind: "circle"; cx: number; cy: number; r: number } | { kind: "poly"; pts: Pt[]; r: number };

const f = (n: number) => +n.toFixed(4);

function primPath(p: Prim, sx: number, sy: number): string {
  if (p.kind === "circle") {
    const { cx, cy, r } = p;
    return `M${f((cx - r) * sx)} ${f(cy * sy)}A${f(r * sx)} ${f(r * sy)} 0 1 0 ${f((cx + r) * sx)} ${f(cy * sy)}A${f(r * sx)} ${f(r * sy)} 0 1 0 ${f((cx - r) * sx)} ${f(cy * sy)}Z`;
  }
  // Rounded polygon: cut each corner at distance r along both edges and join
  // the cut points with a quadratic curve through the corner.
  const { pts, r } = p;
  const n = pts.length;
  let d = "";
  for (let i = 0; i < n; i++) {
    const [px, py] = pts[(i - 1 + n) % n];
    const [cx, cy] = pts[i];
    const [nx, ny] = pts[(i + 1) % n];
    const l1 = Math.hypot(px - cx, py - cy);
    const l2 = Math.hypot(nx - cx, ny - cy);
    const a: Pt = [cx + ((px - cx) / l1) * r, cy + ((py - cy) / l1) * r];
    const b: Pt = [cx + ((nx - cx) / l2) * r, cy + ((ny - cy) / l2) * r];
    d += `${i === 0 ? "M" : "L"}${f(a[0] * sx)} ${f(a[1] * sy)}Q${f(cx * sx)} ${f(cy * sy)} ${f(b[0] * sx)} ${f(b[1] * sy)}`;
  }
  return d + "Z";
}

// Shapes bleed past the top (and sometimes the side) of the card on purpose:
// the card clips them, which is what makes the cut-out read as a window.
const SHAPES: Record<string, Prim[]> = {
  circle: [{ kind: "circle", cx: 112, cy: 82, r: 108 }],
  flower: [
    { kind: "circle", cx: 150, cy: 30, r: 54 },
    { kind: "circle", cx: 94, cy: 90, r: 54 },
    { kind: "circle", cx: 206, cy: 90, r: 54 },
    { kind: "circle", cx: 150, cy: 148, r: 50 },
    { kind: "circle", cx: 150, cy: 90, r: 60 },
  ],
  shield: [{ kind: "poly", pts: [[24, -30], [236, -30], [236, 128], [130, 206], [24, 128]], r: 34 }],
  diamond: [{ kind: "poly", pts: [[136, -34], [262, 90], [136, 214], [10, 90]], r: 36 }],
};

export type ShapeKey = keyof typeof SHAPES;
export const SHAPE_KEYS = Object.keys(SHAPES) as ShapeKey[];

/** Visual centre of each shape inside the art box, for placing a glyph. */
const CENTRES: Record<string, Pt> = { circle: [112, 82], flower: [150, 90], shield: [130, 84], diamond: [136, 90] };
export function shapeCentre(key: ShapeKey): Pt {
  return CENTRES[key];
}

/** Halo/backing paths in art-box user units. */
export function shapePaths(key: ShapeKey): string[] {
  return SHAPES[key].map((p) => primPath(p, 1, 1));
}

/** Clip paths normalised to the art box, for clipPathUnits="objectBoundingBox". */
export function shapeClipPaths(key: ShapeKey): string[] {
  return SHAPES[key].map((p) => primPath(p, 1 / ART_W, 1 / ART_H));
}

/** A stable shape per category: the same category looks the same everywhere. */
export function shapeForIndex(i: number): ShapeKey {
  return SHAPE_KEYS[((i % SHAPE_KEYS.length) + SHAPE_KEYS.length) % SHAPE_KEYS.length];
}
