import { bounded } from "./audio-playback";
import { EQ_PRESETS, SOUNDS, type SoundName } from "./sound-engine";

export type AppSettings = {
  /** Platform voice ID (for built/scheduled API sessions), never a device URI. */
  voice: string;
  deviceVoice: string;
  rate: number;
  volume: number;
  preset: string;
  dlQuality: string;
  ambient: boolean;
  ambientSound: SoundName;
  ambientVolume: number;
  pitch: number;
  normalize: boolean;
  repeat: number;
  eq: string;
  chime: boolean;
  dataSaver: boolean;
  notifications: { daily: boolean; reminders: boolean; community: boolean; digest: boolean; quietStart: string; quietEnd: string; autoExport: boolean; autoExportEmail: string };
  privacy: { privateByDefault: boolean; showStreak: boolean; pauseHistory: boolean };
  accessibility: { captions: boolean; largeText: boolean };
};
export function defaultSettings(): AppSettings {
  return {
    voice: "", deviceVoice: "", rate: 1, volume: 1, preset: "steady", dlQuality: "std",
    ambient: false, ambientSound: "rain", ambientVolume: 0.2, pitch: 1,
    normalize: true, repeat: 1, eq: "flat", chime: false, dataSaver: false,
    notifications: { daily: true, reminders: false, community: true, digest: false, quietStart: "21:00", quietEnd: "07:00", autoExport: false, autoExportEmail: "" },
    privacy: { privateByDefault: true, showStreak: true, pauseHistory: false },
    accessibility: { captions: true, largeText: false },
  };
}
/** Deeply hydrate old/partial preferences; a shallow merge lost all new controls
 * for existing accounts and could leave nested notification/privacy paths null. */
export function hydrateSettings(value: unknown): AppSettings {
  const d = defaultSettings();
  if (!value || typeof value !== "object" || Array.isArray(value)) return d;
  const v = value as Partial<AppSettings>;
  const out = { ...d, ...v,
    notifications: { ...d.notifications, ...v.notifications },
    privacy: { ...d.privacy, ...v.privacy },
    accessibility: { ...d.accessibility, ...v.accessibility },
  };
  out.rate = bounded(Number(out.rate), 0.5, 2, 1);
  out.pitch = bounded(Number(out.pitch), 0.5, 1.5, 1);
  out.volume = bounded(Number(out.volume), 0, 1, 1);
  out.ambientVolume = bounded(Number(out.ambientVolume), 0, 0.6, 0.2);
  out.repeat = Math.round(bounded(Number(out.repeat), 1, 7, 1));
  out.eq = out.eq === "bright" ? "clear" : out.eq === "calm" ? "night" : out.eq;
  if (!(out.eq in EQ_PRESETS)) out.eq = "flat";
  if (!(out.ambientSound in SOUNDS)) out.ambientSound = "rain";
  for (const key of ["voice", "deviceVoice", "preset", "dlQuality"] as const) if (typeof out[key] !== "string") out[key] = d[key];
  for (const key of ["ambient", "normalize", "chime", "dataSaver"] as const) if (typeof out[key] !== "boolean") out[key] = d[key];
  for (const key of ["daily", "reminders", "community", "digest", "autoExport"] as const) if (typeof out.notifications[key] !== "boolean") out.notifications[key] = d.notifications[key];
  for (const key of ["privateByDefault", "showStreak", "pauseHistory"] as const) if (typeof out.privacy[key] !== "boolean") out.privacy[key] = d.privacy[key];
  for (const key of ["quietStart", "quietEnd"] as const) if (!/^(?:[01]\d|2[0-3]):[0-5]\d$/.test(out.notifications[key])) out.notifications[key] = d.notifications[key];
  return out;
}
