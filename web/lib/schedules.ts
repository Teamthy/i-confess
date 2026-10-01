/** ISO weekdays and IANA wall-clock scheduling, shared by the editor and the
 * foreground reminder dispatcher. Policy matches Go: skip spring gaps; use the
 * FIRST instant in a fall overlap. No stored UTC offsets. */
export type ScheduleRecord = {
  id: string;
  label: string;
  time: string;
  days_of_week: number[];
  timezone: string;
  duration_seconds: number;
  voice_id?: string;
  category_ids?: string[];
  enabled: boolean;
  next_run_at?: string;
  /** Local-only records belong to this browser account, never an API user. */
  localOwner?: string;
};
export const WEEKDAYS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];
export const ALL_DAYS = [1, 2, 3, 4, 5, 6, 7];
export const DAYS_PRESETS = { Daily: ALL_DAYS, Weekdays: [1, 2, 3, 4, 5], Weekends: [6, 7] };
export function daysLabel(days: number[]): string {
  const d = [...new Set(days)].sort((a, b) => a - b);
  for (const [name, preset] of Object.entries(DAYS_PRESETS)) if (d.join() === preset.join()) return name;
  return d.map((day) => WEEKDAYS[day - 1]).filter(Boolean).join(" · ") || "No days";
}
export function deviceTimezone(): string {
  try { return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC"; } catch { return "UTC"; }
}
export function validTimezone(tz: string): boolean {
  if (!tz || tz === "Local") return false;
  try { new Intl.DateTimeFormat("en", { timeZone: tz }).format(0); return true; } catch { return false; }
}
export function validateSchedule(s: Pick<ScheduleRecord, "label" | "time" | "days_of_week" | "timezone" | "duration_seconds">): string | null {
  if (!s.label.trim() || [...s.label.trim()].length > 120) return "Give your routine a name of 1–120 characters.";
  if (!/^(?:[01]\d|2[0-3]):[0-5]\d$/.test(s.time)) return "Choose a valid time.";
  if (!validTimezone(s.timezone)) return "Choose a valid IANA timezone, such as Africa/Lagos.";
  if (!s.days_of_week.length || s.days_of_week.some((d) => !Number.isInteger(d) || d < 1 || d > 7)) return "Choose at least one day.";
  if (!Number.isInteger(s.duration_seconds) || s.duration_seconds < 60 || s.duration_seconds > 10800) return "Choose a duration between 1 and 180 minutes.";
  return null;
}

type Wall = { year: number; month: number; day: number; hour: number; minute: number; second: number };
const formatters = new Map<string, Intl.DateTimeFormat>();
function wall(at: Date, timezone: string): Wall {
  let fmt = formatters.get(timezone);
  if (!fmt) {
    fmt = new Intl.DateTimeFormat("en-GB", {
      timeZone: timezone, year: "numeric", month: "2-digit", day: "2-digit",
      hour: "2-digit", minute: "2-digit", second: "2-digit", hourCycle: "h23",
    });
    formatters.set(timezone, fmt);
  }
  const parts: Record<string, number> = {};
  for (const p of fmt.formatToParts(at)) if (p.type !== "literal") parts[p.type] = Number(p.value);
  return parts as Wall;
}
function epoch(p: Wall): number { return Date.UTC(p.year, p.month - 1, p.day, p.hour, p.minute, p.second); }
function resolveWallTime(w: Wall, timezone: string): Date | null {
  const nominal = epoch(w);
  const offsets = new Set<number>();
  // Both sides of any contemporary DST boundary. Sampling, not an iterative
  // offset guess, handles ambiguous and non-existent times without oscillation.
  for (const hours of [-24, -12, 0, 12, 24]) {
    const at = new Date(nominal + hours * 3600000);
    offsets.add(epoch(wall(at, timezone)) - at.getTime());
  }
  const candidates = [...offsets].map((offset) => new Date(nominal - offset))
    .filter((at) => epoch(wall(at, timezone)) === nominal)
    .sort((a, b) => a.getTime() - b.getTime());
  return candidates[0] || null;
}
export function nextOccurrence(s: ScheduleRecord, from = new Date()): Date | null {
  if (!s.enabled || validateSchedule(s)) return null;
  try {
    const local = wall(from, s.timezone);
    const [hour, minute] = s.time.split(":").map(Number);
    for (let i = 0; i < 8; i++) {
      const day = new Date(Date.UTC(local.year, local.month - 1, local.day + i));
      const weekday = day.getUTCDay() || 7;
      if (!s.days_of_week.includes(weekday)) continue;
      const at = resolveWallTime({ year: day.getUTCFullYear(), month: day.getUTCMonth() + 1, day: day.getUTCDate(), hour, minute, second: 0 }, s.timezone);
      if (at && at.getTime() > from.getTime()) return at;
    }
  } catch { /* malformed imported schedule: never guess UTC */ }
  return null;
}
export function occurrenceKey(s: ScheduleRecord, at: Date): string {
  const p = wall(at, s.timezone);
  return `${p.year}-${String(p.month).padStart(2, "0")}-${String(p.day).padStart(2, "0")}T${s.time}`;
}
export function dueOccurrence(s: ScheduleRecord, after: Date, now: Date): { key: string; at: Date } | null {
  const at = nextOccurrence(s, after);
  return at && at <= now ? { key: occurrenceKey(s, at), at } : null;
}
export function inQuietHours(at: Date, start: string, end: string): boolean {
  const minutes = (t: string) => Number(t.slice(0, 2)) * 60 + Number(t.slice(3));
  if (!/^[0-2]\d:[0-5]\d$/.test(start) || !/^[0-2]\d:[0-5]\d$/.test(end) || start === end) return false;
  const m = at.getHours() * 60 + at.getMinutes(), a = minutes(start), b = minutes(end);
  return a < b ? m >= a && m < b : m >= a || m < b;
}

/** Upgrade browser-only schedules from before scheduling was connected. */
export function migrateLocalSchedules(value: unknown, owner = ""): ScheduleRecord[] {
  if (!Array.isArray(value)) return [];
  return value.filter((v) => v && typeof v === "object").map((v) => {
    const legacy = typeof v.category === "string";
    const time = /^(?:[01]\d|2[0-3]):[0-5]\d$/.test(v.time) ? v.time : v.time === "Evening" ? "21:00" : "07:00";
    const days = Array.isArray(v.days_of_week) ? v.days_of_week : DAYS_PRESETS[v.days as keyof typeof DAYS_PRESETS] || ALL_DAYS;
    return {
      id: String(v.id || `local-${time}-${v.category}`),
      label: typeof v.label === "string" ? v.label : `${v.category || "Daily"} practice`,
      time, days_of_week: [...new Set<number>(days.filter((d: number) => Number.isInteger(d) && d >= 1 && d <= 7))],
      timezone: validTimezone(v.timezone) ? v.timezone : deviceTimezone(),
      duration_seconds: Number.isInteger(v.duration_seconds) ? v.duration_seconds : 300,
      voice_id: typeof v.voice_id === "string" ? v.voice_id : "",
      category_ids: Array.isArray(v.category_ids) ? v.category_ids.filter((c: unknown) => typeof c === "string") : legacy ? [v.category] : [],
      enabled: v.enabled !== false, localOwner: typeof v.localOwner === "string" ? v.localOwner : owner,
    };
  });
}
