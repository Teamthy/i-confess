"use client";
import { useSyncExternalStore } from "react";

export type AppState = {
  user: null | { name: string; email: string; plan: string; interests: string[] };
  favs: string[];
  history: { slug: string; title: string; category: string; at: number }[];
  routines: { slot: string; cats: string[] }[];
  schedule: { id: number; category: string; time: string; days: string }[];
  community: { slug: string; title: string; text: string; visibility: string; status: string; category: string; at: number }[];
  shared: Record<string, { slug: string; at: number }>;
  settings: {
    voice: string; rate: number; volume: number; preset: string; dlQuality: string; ambient: boolean; pitch: number; normalize: boolean; repeat: number; eq: string; dataSaver: boolean;
    voiceId: string; bass: number; mid: number; treble: number; warmth: number; clarity: number; presence: number; ambienceId: string; ambienceLevel: number; reverb: boolean; profileId: string;
    notifications: { daily: boolean; reminders: boolean; community: boolean; digest: boolean; quietStart: string; quietEnd: string; autoExport: boolean; autoExportEmail: string };
    privacy: { privateByDefault: boolean; showStreak: boolean; pauseHistory: boolean };
    accessibility: { captions: boolean; largeText: boolean };
  };
  streak: { count: number; last: string | null };
  recentSearches: string[];
  journals: { id: number; date: string; title: string; text: string; mood: string; audio?: string }[];
  downloads: string[];
  memory: { ref: string; text: string; title: string; box: number; due: number }[];
  seasons: { id: number; name: string; cats: string[]; active: boolean; archived: boolean }[];
  partners: { name: string; code: string; since: number }[];
  family: { id: number; name: string; role: string; band: string }[];
  partnerQueue: string[];
  annotations: Record<string, string>;
  audioProfiles: { id: number; name: string; cfg: Record<string, number | string> }[];
  positions: Record<string, { pct: number; at: number }>;
};

const defaults = (): AppState => ({
  user: null, favs: [], history: [], routines: [], schedule: [], community: [], shared: {},
  settings: { voice: "grace", rate: 1, volume: 1, preset: "steady", dlQuality: "std", ambient: false, pitch: 1, normalize: true, repeat: 1, eq: "warm", dataSaver: false,
    /* Audio + Voice Engine: user DSP + ambience + voice selection (spec §64) */
    voiceId: "grace", bass: 0, mid: 0, treble: 0, warmth: 60, clarity: 55, presence: 60, ambienceId: "", ambienceLevel: 45, reverb: false, profileId: "default",
    notifications: { daily: true, reminders: false, community: true, digest: false, quietStart: "21:00", quietEnd: "07:00", autoExport: false, autoExportEmail: "" }, privacy: { privateByDefault: true, showStreak: true, pauseHistory: false }, accessibility: { captions: true, largeText: false } },
  streak: { count: 0, last: null }, recentSearches: [], journals: [], downloads: [], memory: [], seasons: [], partners: [], family: [], partnerQueue: [], annotations: {},
  audioProfiles: [], positions: {},
});

const KEY = "iconfess:v1";
function load(): AppState {
  if (typeof window === "undefined") return defaults();
  try {
    const saved = JSON.parse(localStorage.getItem(KEY) || "{}");
    const d = defaults();
    /* deep-merge settings so returning users gain new audio keys with defaults */
    if (saved.settings) saved.settings = Object.assign({}, d.settings, saved.settings, {
      notifications: Object.assign({}, d.settings.notifications, saved.settings.notifications || {}),
      privacy: Object.assign({}, d.settings.privacy, saved.settings.privacy || {}),
      accessibility: Object.assign({}, d.settings.accessibility, saved.settings.accessibility || {}),
    });
    return Object.assign(d, saved);
  } catch { return defaults(); }
}
let state: AppState = typeof window === "undefined" ? defaults() : load();
const listeners = new Set<() => void>();
const persist = () => { try { localStorage.setItem(KEY, JSON.stringify(state)); } catch { } };

export function mutate(fn: (s: AppState) => void) { fn(state); state = { ...state }; persist(); listeners.forEach((l) => l()); }
export function useApp(): AppState {
  return useSyncExternalStore(
    (cb) => { listeners.add(cb); return () => listeners.delete(cb); },
    () => state,
    () => state
  );
}
export function track(name: string, props?: Record<string, unknown>) {
  try {
    const evs = JSON.parse(localStorage.getItem("iconfess:events") || "[]");
    evs.push({ name, props: props || {}, at: Date.now() });
    localStorage.setItem("iconfess:events", JSON.stringify(evs.slice(-400)));
  } catch { }
  console.debug("[analytics]", name, props || {});
}
