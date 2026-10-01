"use client";
import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "./auth-context";
import { appApi, useApiData, type AppApiResult, type AppApiFail } from "./app-api";
import { mutate, useApp } from "./store";
import { usePlayer } from "./player";
import { buildQueue, catBySlug, CONFESSIONS } from "./data";
import { dueOccurrence, inQuietHours, validateSchedule, type ScheduleRecord } from "./schedules";
import type { ApiSession } from "./session-playback";

export type ScheduleDraft = Omit<ScheduleRecord, "id" | "localOwner" | "next_run_at">;
type Notice = { schedule: ScheduleRecord; key: string };
type ScheduleContextValue = {
  schedules: ScheduleRecord[];
  mode: "account" | "local";
  loading: boolean;
  error: AppApiFail | null;
  preferencesError: AppApiFail | null;
  reminders: boolean;
  remindersLoading: boolean;
  permission: NotificationPermission | "unsupported";
  reload: () => void;
  save: (draft: ScheduleDraft, id?: string) => Promise<AppApiResult<ScheduleRecord>>;
  remove: (id: string) => Promise<AppApiResult<null>>;
  setEnabled: (schedule: ScheduleRecord, enabled: boolean) => Promise<AppApiResult<ScheduleRecord>>;
  setReminders: (enabled: boolean) => Promise<AppApiResult<unknown>>;
  enableAlerts: () => Promise<string | null>;
  start: (schedule: ScheduleRecord) => Promise<AppApiResult<unknown>>;
  notices: Notice[];
  dismiss: (key: string) => void;
};
const Ctx = createContext<ScheduleContextValue | null>(null);
export function useSchedules() {
  const value = useContext(Ctx);
  if (!value) throw new Error("useSchedules requires ScheduleProvider");
  return value;
}
const deliveriesInMemory = new Set<string>();
async function claimReminder(owner: string, schedule: ScheduleRecord, occurrence: string): Promise<boolean> {
  const key = JSON.stringify([owner, schedule.id, occurrence]);
  const claim = () => {
    if (deliveriesInMemory.has(key)) return false;
    try {
      const storeKey = "ic-schedule-deliveries:v1";
      const value = JSON.parse(localStorage.getItem(storeKey) || "{}");
      const records: Record<string, number> = value && typeof value === "object" && !Array.isArray(value) ? value : {};
      if (records[key]) return false;
      const now = Date.now();
      for (const [id, at] of Object.entries(records)) if (!Number.isFinite(at) || at < now - 8 * 86400000) delete records[id];
      records[key] = now;
      localStorage.setItem(storeKey, JSON.stringify(records));
    } catch { /* storage unavailable: still dedupe within this tab */ }
    deliveriesInMemory.add(key);
    return true;
  };
  // Web Locks makes the read/claim atomic across browser tabs. Older browsers
  // fall back to best-effort storage + in-memory deduplication.
  if (navigator.locks) return navigator.locks.request(`ic-schedule:${key}`, () => claim());
  return claim();
}

export function ScheduleProvider({ children }: { children: ReactNode }) {
  const auth = useAuth();
  const app = useApp();
  const player = usePlayer();
  const router = useRouter();
  const records = useApiData<ScheduleRecord[]>(auth.token, "/schedules");
  const prefs = useApiData<{ preferences: { scheduled_sessions: boolean } }>(auth.token, "/me/notifications");
  const [permission, setPermission] = useState<NotificationPermission | "unsupported">("unsupported");
  const [notices, setNotices] = useState<Notice[]>([]);
  const [changingPrefs, setChangingPrefs] = useState(false);
  const owner = auth.user?.id || app.user?.email || "";
  const tokenRef = useRef(auth.token); tokenRef.current = auth.token;
  const ownerRef = useRef(owner); ownerRef.current = owner;
  const schedules = auth.token ? records.data || [] : app.schedule.filter((s) => s.localOwner === (app.user?.email || ""));
  const reminders = auth.token ? prefs.data?.preferences.scheduled_sessions === true : app.settings.notifications.reminders;
  const runtime = useRef({ schedules, reminders, notifications: app.settings.notifications, owner });
  runtime.current = { schedules, reminders, notifications: app.settings.notifications, owner };

  const reload = useCallback(() => { records.reload(); prefs.reload(); }, [records.reload, prefs.reload]);
  useEffect(() => {
    const checkPermission = () => setPermission("Notification" in window ? Notification.permission : "unsupported");
    checkPermission();
    const onFocus = () => { checkPermission(); reload(); };
    window.addEventListener("focus", onFocus);
    const refresh = setInterval(() => { if (tokenRef.current && document.visibilityState === "visible") reload(); }, 60000);
    return () => { clearInterval(refresh); window.removeEventListener("focus", onFocus); };
  }, [reload]);

  const save = useCallback(async (draft: ScheduleDraft, id?: string): Promise<AppApiResult<ScheduleRecord>> => {
    const error = validateSchedule(draft);
    if (error) return { ok: false, status: 400, code: "SCHEDULE_INVALID", message: error };
    const token = tokenRef.current, currentOwner = ownerRef.current;
    if (token) {
      const result = await appApi<ScheduleRecord>(id ? `/schedules/${encodeURIComponent(id)}` : "/schedules", { token, method: id ? "PATCH" : "POST", body: draft, idempotencyKey: id ? undefined : crypto.randomUUID() });
      if (currentOwner !== ownerRef.current || token !== tokenRef.current) return { ok: false, status: 0, code: "ABORTED", message: "Account changed. Please try again." };
      if (result.ok) reload();
      return result;
    }
    const result: ScheduleRecord = { ...draft, label: draft.label.trim(), id: id || `local-${crypto.randomUUID()}`, localOwner: currentOwner };
    mutate((s) => { s.schedule = id ? s.schedule.map((sc) => sc.id === id && sc.localOwner === currentOwner ? result : sc) : [...s.schedule, result]; });
    return { ok: true, data: result };
  }, [reload]);
  const remove = useCallback(async (id: string): Promise<AppApiResult<null>> => {
    if (tokenRef.current) {
      const result = await appApi<null>(`/schedules/${encodeURIComponent(id)}`, { token: tokenRef.current, method: "DELETE" });
      if (result.ok) { reload(); setNotices((list) => list.filter((n) => n.schedule.id !== id)); }
      return result;
    }
    mutate((s) => { s.schedule = s.schedule.filter((sc) => sc.id !== id || sc.localOwner !== ownerRef.current); });
    setNotices((list) => list.filter((n) => n.schedule.id !== id));
    return { ok: true, data: null };
  }, [reload]);
  const setEnabled = useCallback(async (sc: ScheduleRecord, enabled: boolean): Promise<AppApiResult<ScheduleRecord>> => {
    if (tokenRef.current) {
      const result = await appApi<ScheduleRecord>(`/schedules/${encodeURIComponent(sc.id)}`, { token: tokenRef.current, method: "PATCH", body: { enabled } });
      if (result.ok) reload();
      return result;
    }
    const updated = { ...sc, enabled };
    mutate((s) => { s.schedule = s.schedule.map((record) => record.id === sc.id && record.localOwner === ownerRef.current ? updated : record); });
    return { ok: true, data: updated };
  }, [reload]);
  const setReminders = useCallback(async (enabled: boolean): Promise<AppApiResult<unknown>> => {
    if (!tokenRef.current) {
      mutate((s) => { s.settings.notifications.reminders = enabled; });
      return { ok: true, data: null };
    }
    setChangingPrefs(true);
    const result = await appApi<unknown>("/me/notifications", { token: tokenRef.current, method: "PATCH", body: { scheduled_sessions: enabled } });
    if (result.ok) prefs.reload();
    setChangingPrefs(false);
    return result;
  }, [prefs.reload]);
  const enableAlerts = useCallback(async (): Promise<string | null> => {
    if (!("Notification" in window)) return "Browser alerts aren't supported here. In-app reminders still work while this page is open.";
    try {
      const result = await Notification.requestPermission();
      setPermission(result);
      if (result !== "granted") return "Browser alerts were not allowed. You can change permission in your browser settings; in-app reminders still work.";
      const saved = await setReminders(true);
      return saved.ok ? null : saved.message;
    } catch { return "Browser alerts couldn't be enabled. Try your browser's notification settings."; }
  }, [setReminders]);
  const start = useCallback(async (sc: ScheduleRecord): Promise<AppApiResult<unknown>> => {
    const token = tokenRef.current;
    if (token) {
      const built = await appApi<ApiSession>(`/schedules/${encodeURIComponent(sc.id)}/start`, { token, method: "POST", idempotencyKey: crypto.randomUUID() });
      if (!built.ok) return built;
      if (token !== tokenRef.current) return { ok: false, status: 0, code: "ABORTED", message: "Account changed. Playback cancelled." };
      const result = await player.playSession(built.data, token);
      if (result.ok) router.push(`/app/player?session=${encodeURIComponent(built.data.id)}`);
      return result;
    }
    const selected = new Set((sc.category_ids || []).map((id) => catBySlug(id)?.name));
    const items = CONFESSIONS.filter((c) => selected.has(c.category));
    if (!items.length) return { ok: false, status: 422, code: "CONTENT_UNAVAILABLE", message: "Choose a category with reviewed confessions before starting this routine." };
    player.playQueue(buildQueue({ slug: sc.id, title: sc.label, description: "Local device-speech routine", category: sc.category_ids?.[0] || "peace", minutes: sc.duration_seconds / 60, items }));
    router.push("/app/player");
    return { ok: true, data: null };
  }, [player.playSession, player.playQueue, router]);

  useEffect(() => {
    setNotices([]);
    let disposed = false;
    let after = new Date(Date.now() - 60000);
    const tick = async () => {
      const now = new Date();
      const windowStart = new Date(Math.max(after.getTime(), now.getTime() - 90000));
      after = now; // don't flood someone with hours of stale reminders
      const current = runtime.current;
      if (!current.owner || !current.reminders) return;
      for (const sc of current.schedules) {
        const due = dueOccurrence(sc, windowStart, now);
        if (!due) continue;
        const claimed = await claimReminder(current.owner, sc, due.key);
        if (disposed || current.owner !== ownerRef.current || !runtime.current.reminders) return;
        if (!claimed || inQuietHours(now, current.notifications.quietStart, current.notifications.quietEnd)) continue;
        const key = `${sc.id}:${due.key}`;
        setNotices((list) => [...list.filter((n) => n.key !== key), { schedule: sc, key }].slice(-3));
        if ("Notification" in window && Notification.permission === "granted") {
          const options = { body: "Your scheduled practice is ready. Open iCONFESS when you're ready to begin.", tag: key, silent: true, icon: "/assets/logo-mark.jpg", data: { url: "/app/schedule" } };
          try {
            const registration = await navigator.serviceWorker?.getRegistration();
            if (registration) await registration.showNotification("Time for your practice", options);
            else {
              const notification = new Notification("Time for your practice", options);
              notification.onclick = () => { window.focus(); router.push("/app/schedule"); notification.close(); };
            }
          } catch { /* OS alerts fail independently; the in-app notice remains */ }
        }
      }
    };
    const id = setInterval(() => { void tick(); }, 15000);
    const onVisible = () => { if (document.visibilityState === "visible") { reload(); void tick(); } };
    document.addEventListener("visibilitychange", onVisible);
    return () => { disposed = true; clearInterval(id); document.removeEventListener("visibilitychange", onVisible); };
  }, [owner, router, reload]);

  // An edited/paused/deleted routine must disappear from the reminder queue.
  useEffect(() => { setNotices((list) => list.filter((n) => schedules.some((s) => s.id === n.schedule.id && s.enabled))); }, [records.data, app.schedule]);
  const value: ScheduleContextValue = {
    schedules, mode: auth.token ? "account" : "local", loading: auth.loading || (Boolean(auth.token) && records.loading),
    error: auth.token ? records.error : null, preferencesError: auth.token ? prefs.error : null,
    reminders, remindersLoading: changingPrefs || (Boolean(auth.token) && prefs.loading), permission,
    reload, save, remove, setEnabled, setReminders, enableAlerts, start, notices,
    dismiss: (key) => setNotices((list) => list.filter((n) => n.key !== key)),
  };
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}
