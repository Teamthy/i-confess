/**
 * Deterministic per-category colour.
 *
 * A pure function of the slug over the design system's navy ramp and neutral
 * ink tones, so every category is visually identifiable without inventing a
 * second palette. Server-safe: shared by the category rail (client) and the
 * card components (server).
 */

const TONES = [
  "#00072D", // brand-900 deepest navy
  "#051650", // brand-800 dark navy
  "#081D61", // brand-700
  "#0A2472", // brand-600 primary blue
  "#123499", // brand-500 secondary blue
  "#141918", // neutral-900
  "#0A0E0D", // neutral-950
  "#051650", // brand-800 repeat for better distribution
];

export function railColor(slug: string): string {
  let h = 0;
  for (const ch of slug) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  return TONES[h % TONES.length];
}

export function railStyle(slug: string): React.CSSProperties {
  return { ["--rail" as string]: railColor(slug) };
}

/**
 * Category card motif: a two-stop navy gradient derived from the same tone.
 * One visual system for all 39 categories — the ramp is the token ramp, so
 * a card can never drift into a second palette.
 */
const MOTIF_PAIRS: Record<string, [string, string]> = {
  "#00072D": ["#123499", "#00072D"],
  "#051650": ["#425EAF", "#051650"],
  "#081D61": ["#123499", "#081D61"],
  "#0A2472": ["#7288C4", "#0A2472"],
  "#123499": ["#A3B3DA", "#123499"],
  "#141918": ["#425EAF", "#141918"],
  "#0A0E0D": ["#123499", "#0A0E0D"],
};

export function motifStyle(slug: string): React.CSSProperties {
  const base = railColor(slug);
  const [from, to] = MOTIF_PAIRS[base] ?? ["#123499", "#00072D"];
  return { background: `linear-gradient(135deg, ${from} 0%, ${to} 78%)` };
}
