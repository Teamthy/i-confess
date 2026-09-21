/**
 * Deterministic per-category colour.
 *
 * A pure function of the slug over the design system's green ramp and neutral
 * ink tones, so every category is visually identifiable without inventing a
 * second palette. Server-safe: shared by the category rail (client) and the
 * card components (server).
 */

const TONES = [
  "#0C3325", // brand-900
  "#124A37", // brand-800
  "#186248", // brand-700
  "#141918", // neutral-900
  "#0A0E0D", // neutral-950
  "#1F7E5E", // brand-600
  "#212927", // neutral-800
  "#333C3A", // neutral-700
];

export function railColor(slug: string): string {
  let h = 0;
  for (const ch of slug) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  return TONES[h % TONES.length];
}

export function railStyle(slug: string): React.CSSProperties {
  return { ["--rail" as string]: railColor(slug) };
}
