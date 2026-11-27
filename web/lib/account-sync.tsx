"use client";
/* ============================================================
   I CONFESS web — the account is the source of truth.

   Signed in, everything that belongs to a person is loaded from the API into
   the app state, and every change is written back:

     favourites      GET/POST/DELETE /me/favorites
     history         GET/POST /me/history          (streak derives from it)
     plan, interests GET /me/bootstrap, PUT /me/interests
     settings        GET/PATCH /me/preferences  (client_settings)
     journal         GET/POST/PATCH/DELETE /me/journal
     memory verses   GET/POST/DELETE /me/memory, POST /me/memory/{id}/review

   Favourites, history and settings are pushed by observing state changes, so
   the many buttons that change them stay as they are. The journal and memory
   need server ids and versions, so they use the explicit actions below.

   Signed out (or with no API at all) the state is this device's own, as
   before. Signing out of a synced account wipes its data from the device,
   and switching accounts wipes the previous account's data before the new
   one loads, so a failed fetch can never show one person another's journal.

   A write that the server refuses is rolled back on screen and announced,
   so "Saved" always means saved.
   ============================================================ */
import { useEffect, useRef } from "react";
import { useToast } from "./ui";
import { useAuth } from "./auth-context";
import { appApi, newIdempotencyKey, type AppApiResult } from "./app-api";
import { defaults, getAppState, mutate, subscribeApp, type AppState } from "./store";
import { hydrateSettings, type AppSettings } from "./preferences";
import { CATEGORIES } from "./data";

/* ---------------- library index: slug ↔ server id ---------------- */

export type LibraryEntry = { id: string; slug: string; title: string; category: string; category_slug: string; short_text: string; medium_text: string; long_text: string; intensity: number };
type Index = { bySlug: Map<string, LibraryEntry>; byId: Map<string, LibraryEntry> };
let indexPromise: Promise<Index> | null = null;

/** The published library from GET /confessions, fetched once per page load. */
export function libraryIndex(): Promise<Index> {
  if (!indexPromise) {
    indexPromise = appApi<{ confessions: LibraryEntry[] }>("/confessions").then((r) => {
      if (!r.ok) { indexPromise = null; throw new Error(r.message); }
      const bySlug = new Map<string, LibraryEntry>(), byId = new Map<string, LibraryEntry>();
      for (const c of r.data.confessions) { bySlug.set(c.slug, c); byId.set(c.id, c); }
      return { bySlug, byId };
    });
  }
  return indexPromise;
}

/* ---------------- shapes the API returns ---------------- */

type Fav = { entity_type: string; entity_id: string };
type Play = { confession_id?: string; listened_at: string; completed: boolean };
type Journal = { id: string; title: string; body: string; mood: string; created_at: string; row_version: number };
type Verse = { id: string; reference: string; text: string; title: string; box: number; due_at: string };

const toJournal = (j: Journal): AppState["journals"][number] => ({ id: j.id, date: j.created_at, title: j.title, text: j.body, mood: j.mood, rowVersion: j.row_version });
const toVerse = (v: Verse): AppState["memory"][number] => ({ id: v.id, ref: v.reference, text: v.text, title: v.title, box: v.box, due: Date.parse(v.due_at) });

/** Consecutive days with a completed play, ending today or yesterday. */
export function streakFrom(history: AppState["history"]): AppState["streak"] {
  const days = new Set(history.map((h) => new Date(h.at).toDateString()));
  const d = new Date();
  if (!days.has(d.toDateString())) d.setDate(d.getDate() - 1);
  if (!days.has(d.toDateString())) return { count: 0, last: history[0] ? new Date(history[0].at).toDateString() : null };
  const last = d.toDateString();
  let count = 0;
  while (days.has(d.toDateString())) { count++; d.setDate(d.getDate() - 1); }
  return { count, last };
}

/** Settings that describe this device, not the person: never synced. */
const DEVICE_ONLY: (keyof AppSettings)[] = ["deviceVoice"];
const portable = (s: AppSettings) => { const o: Record<string, unknown> = { ...s }; for (const k of DEVICE_ONLY) delete o[k]; return o; };

/**
 * Everything personal goes; what describes the device stays: its voice, the
 * dismissed tour card, and journal entries written here while signed out
 * that no account has claimed yet. Entries kept for an account (`owner`) go
 * with that account.
 */
export function wipePersonalData(s: AppState): void {
  const fresh = defaults();
  const keepJournals = s.journals.filter((j) => typeof j.id === "number" && !j.owner);
  const deviceVoice = s.settings.deviceVoice;
  const promoHidden = s.promoHidden;
  Object.assign(s, fresh);
  s.journals = keepJournals;
  s.settings.deviceVoice = deviceVoice;
  s.promoHidden = promoHidden;
  s.syncedAs = null;
}

/** Every journal entry, following the API's pages (it answers 100 at a time). */
async function allJournal(token: string): Promise<AppApiResult<{ entries: Journal[] }>> {
  const entries: Journal[] = [];
  let cursor = "";
  for (let page = 0; page < 100; page++) {
    const r = await appApi<{ entries: Journal[]; next_cursor?: string }>(
      "/me/journal?limit=200" + (cursor ? "&cursor=" + encodeURIComponent(cursor) : ""), { token });
    if (!r.ok) return r;
    entries.push(...(r.data.entries || []));
    cursor = r.data.next_cursor || "";
    if (!cursor) break;
  }
  return { ok: true, data: { entries } };
}

/* ---------------- the sync ---------------- */

/** Mounted once in the root layout. */
export function AccountSync() {
  const auth = useAuth();
  const userId = auth.user?.id || null;
  const hydrating = useRef(false);
  const toast = useToast();
  const toastRef = useRef(toast); toastRef.current = toast;
  const token = useRef(auth.token); token.current = auth.token;

  // Load the account's data when a session appears.
  useEffect(() => {
    if (auth.loading) return;
    if (!auth.token || !userId) {
      // Signed out of a synced account: its data must not stay on the device.
      if (getAppState().syncedAs) { hydrating.current = true; mutate(wipePersonalData); hydrating.current = false; }
      return;
    }
    // Another account was loaded here and nobody signed out (a second person
    // signed in over it). Clear it now, before any fetch can fail and leave a
    // field holding the previous person's data under the new person's name.
    const prior = getAppState().syncedAs;
    if (prior && prior !== userId) { hydrating.current = true; mutate(wipePersonalData); hydrating.current = false; }
    let cancelled = false;
    void (async () => {
      const t = auth.token!;
      const local = getAppState();
      const firstTimeHere = local.syncedAs !== userId;
      const [favs, hist, boot, journal, memory, prefs, cats] = await Promise.all([
        appApi<Fav[]>("/me/favorites?type=confession", { token: t }),
        appApi<Play[]>("/me/history", { token: t }),
        appApi<{ subscription: { plan: string }; interests: string[] }>("/me/bootstrap", { token: t }),
        allJournal(t),
        appApi<{ verses: Verse[] }>("/me/memory", { token: t }),
        appApi<{ client_settings?: Record<string, unknown> }>("/me/preferences", { token: t }),
        appApi<{ id: string; slug: string }[]>("/categories"),
      ]);
      const idx = await libraryIndex().catch(() => null);
      if (cancelled || token.current !== t) return;

      // Data this device collected while signed out joins the account the
      // first time it signs in here; another account's data never does.
      const adopt = firstTimeHere && !local.syncedAs;
      if (adopt && idx) {
        const serverFavs = new Set((favs.ok ? favs.data || [] : []).map((f) => f.entity_id));
        for (const slug of local.favs) {
          const id = idx.bySlug.get(slug)?.id;
          if (id && !serverFavs.has(id)) { await appApi("/me/favorites", { token: t, method: "POST", body: { entity_type: "confession", entity_id: id } }); serverFavs.add(id); }
        }
        if (favs.ok) favs.data = [...serverFavs].map((id) => ({ entity_type: "confession", entity_id: id }));
      }
      // Entries that cannot join the account stay on this device rather than
      // vanish: voice notes (audio is not uploaded) and any upload that failed.
      const keptJournals: AppState["journals"] = [];
      const keptMemory: AppState["memory"] = [];
      if (adopt) {
        for (const j of local.journals.filter((x) => typeof x.id === "number")) {
          if (j.audio) { keptJournals.push({ ...j, owner: userId }); continue; }
          const r = await appApi<Journal>("/me/journal", { token: t, method: "POST", idempotencyKey: newIdempotencyKey(), body: { title: j.title, body: j.text, mood: j.mood } });
          if (r.ok && journal.ok) journal.data.entries.unshift(r.data);
          else keptJournals.push({ ...j, owner: userId });
        }
        for (const m of local.memory.filter((x) => !x.id)) {
          const r = await appApi<{ verse: Verse }>("/me/memory", { token: t, method: "POST", body: { reference: m.ref, text: m.text, title: m.title } });
          if (r.ok && memory.ok) { if (!memory.data.verses.some((v) => v.id === r.data.verse.id)) memory.data.verses.push(r.data.verse); }
          else keptMemory.push({ ...m, owner: userId });
        }
        if (keptJournals.length || keptMemory.length) {
          toastRef.current("Some entries stayed on this device only. Voice notes and anything that failed to upload were kept here.");
        }
      }

      const catSlug = new Map((cats.ok && Array.isArray(cats.data) ? cats.data : []).map((c) => [c.id, c.slug]));
      const serverSettings = prefs.ok ? prefs.data.client_settings : undefined;
      const hasServerSettings = !!serverSettings && Object.keys(serverSettings).length > 0;
      if (prefs.ok && !hasServerSettings) {
        // First device for this account: its current settings become the account's.
        void appApi("/me/preferences", { token: t, method: "PATCH", body: { client_settings: portable(local.settings) } });
      }

      // Device-only entries this account kept on an earlier visit.
      const current = getAppState();
      keptJournals.push(...current.journals.filter((j) => j.owner === userId && !keptJournals.some((k) => k.id === j.id)));
      keptMemory.push(...current.memory.filter((m) => m.owner === userId && !keptMemory.some((k) => k.ref === m.ref)));

      hydrating.current = true;
      mutate((s) => {
        s.syncedAs = userId;
        if (favs.ok && idx) s.favs = (favs.data || []).map((f) => idx.byId.get(f.entity_id)?.slug).filter((x): x is string => !!x);
        if (hist.ok && idx) {
          s.history = (hist.data || []).filter((h) => h.confession_id && idx.byId.has(h.confession_id)).map((h) => {
            const c = idx.byId.get(h.confession_id!)!;
            return { slug: c.slug, title: c.title, category: c.category, at: Date.parse(h.listened_at) };
          });
          s.streak = streakFrom(s.history);
        }
        const me = auth.user!;
        s.user = { name: me.display_name || me.email.split("@")[0], email: me.email, plan: s.user?.email === me.email ? s.user.plan : "free", interests: s.user?.email === me.email ? s.user.interests : [] };
        if (boot.ok) {
          s.user.plan = boot.data.subscription?.plan === "premium" ? "premium" : boot.data.subscription?.plan === "trial" ? "trial" : "free";
          s.user.interests = (boot.data.interests || []).map((id) => catSlug.get(id)).filter((x): x is string => !!x && CATEGORIES.some((c) => c.slug === x));
        }
        if (journal.ok) s.journals = [...(journal.data.entries || []).map(toJournal), ...keptJournals];
        if (memory.ok) s.memory = [...(memory.data.verses || []).map(toVerse), ...keptMemory.filter((m) => !(memory.data.verses || []).some((v) => v.reference === m.ref))];
        if (hasServerSettings) s.settings = hydrateSettings({ ...serverSettings, deviceVoice: s.settings.deviceVoice });
      });
      hydrating.current = false;
    })();
    return () => { cancelled = true; };
  }, [auth.loading, auth.token, userId]);

  // Push favourites, plays and settings as they change.
  useEffect(() => {
    let prev = getAppState();
    let settingsTimer: ReturnType<typeof setTimeout> | undefined;
    const failed = (message: string) => toastRef.current(message);
    const unsub = subscribeApp(() => {
      const next = getAppState();
      const before = prev; prev = next;
      const t = token.current;
      if (!t || hydrating.current || !next.syncedAs || before.syncedAs !== next.syncedAs) return;

      if (next.favs !== before.favs) {
        const added = next.favs.filter((x) => !before.favs.includes(x));
        const removed = before.favs.filter((x) => !next.favs.includes(x));
        // A refused write is undone on screen; the toast that said "Saved"
        // is corrected rather than left standing.
        const undo = (slug: string, wasAdded: boolean) => {
          hydrating.current = true;
          mutate((s) => { s.favs = wasAdded ? s.favs.filter((x) => x !== slug) : s.favs.includes(slug) ? s.favs : [...s.favs, slug]; });
          hydrating.current = false;
          failed(wasAdded ? "We couldn't save that favourite. Please try again." : "We couldn't remove that favourite. Please try again.");
        };
        void libraryIndex().then(async (idx) => {
          for (const slug of added) {
            const id = idx.bySlug.get(slug)?.id; if (!id) continue;
            const r = await appApi("/me/favorites", { token: t, method: "POST", body: { entity_type: "confession", entity_id: id } });
            if (!r.ok) undo(slug, true);
          }
          for (const slug of removed) {
            const id = idx.bySlug.get(slug)?.id; if (!id) continue;
            const r = await appApi("/me/favorites", { token: t, method: "DELETE", body: { entity_type: "confession", entity_id: id } });
            if (!r.ok && r.status !== 404) undo(slug, false);
          }
        }).catch(() => failed("We couldn't reach the library to save that. Please try again."));
      }

      // A completed play is prepended to history by the player.
      if (next.history !== before.history && next.history.length && next.history[0] !== before.history[0] && next.history[0].at > (before.history[0]?.at || 0)) {
        const play = next.history[0];
        void libraryIndex().then(async (idx) => {
          const id = idx.bySlug.get(play.slug)?.id;
          if (!id) return;
          const r = await appApi("/me/history", { token: t, method: "POST", idempotencyKey: newIdempotencyKey(), body: { confession_id: id, completed: true } });
          if (!r.ok) failed("This listen wasn't recorded, so it may not count toward your streak.");
        }).catch(() => failed("This listen wasn't recorded, so it may not count toward your streak."));
        mutate((s) => { s.streak = streakFrom(s.history); });
      }

      const ni = next.user?.interests, bi = before.user?.interests;
      if (ni && bi && ni !== bi && ni.join() !== bi.join()) {
        void appApi<{ id: string; slug: string }[]>("/categories").then((cats) => {
          const rows = cats.ok && Array.isArray(cats.data) ? cats.data : [];
          const ids = ni.map((slug) => rows.find((c) => c.slug === slug)?.id).filter((x): x is string => !!x);
          return appApi("/me/interests", { token: t, method: "PUT", body: { category_ids: ids, source: "EXPLICIT_SELECTION" } });
        }).then((r) => { if (!r.ok) failed("We couldn't save your interests. Please try again."); });
      }

      if (next.settings !== before.settings && JSON.stringify(portable(next.settings)) !== JSON.stringify(portable(before.settings))) {
        clearTimeout(settingsTimer);
        settingsTimer = setTimeout(() => {
          void appApi("/me/preferences", { token: t, method: "PATCH", body: { client_settings: portable(getAppState().settings) } })
            .then((r) => { if (!r.ok) failed("Your settings changed on this device but weren't saved to your account."); });
        }, 800);
      }
    });
    return () => { unsub(); clearTimeout(settingsTimer); };
  }, []);

  return null;
}

/* ---------------- explicit actions: journal and memory ---------------- */

type Result = { ok: true } | { ok: false; message: string };

export async function addJournal(token: string | null, entry: { title: string; text: string; mood: string; audio?: string }): Promise<Result> {
  if (!token) {
    mutate((s) => { s.journals.unshift({ id: Date.now(), date: new Date().toISOString(), ...entry }); });
    return { ok: true };
  }
  const r = await appApi<Journal>("/me/journal", { token, method: "POST", idempotencyKey: newIdempotencyKey(), body: { title: entry.title, body: entry.text, mood: entry.mood } });
  if (!r.ok) return { ok: false, message: r.message };
  mutate((s) => { s.journals.unshift(toJournal(r.data)); });
  return { ok: true };
}

export async function deleteJournal(token: string | null, id: number | string): Promise<Result> {
  if (token && typeof id === "string") {
    const r = await appApi(`/me/journal/${encodeURIComponent(id)}`, { token, method: "DELETE" });
    if (!r.ok && r.status !== 404) return { ok: false, message: r.message };
  }
  mutate((s) => { s.journals = s.journals.filter((x) => x.id !== id); });
  return { ok: true };
}

export async function addMemoryVerse(token: string | null, v: { ref: string; text: string; title: string }): Promise<Result & { existed?: boolean }> {
  if (!token) {
    if (getAppState().memory.some((m) => m.ref === v.ref)) return { ok: true, existed: true };
    mutate((s) => { s.memory.push({ ...v, box: 0, due: Date.now() }); });
    return { ok: true };
  }
  const r = await appApi<{ verse: Verse; created: boolean }>("/me/memory", { token, method: "POST", body: { reference: v.ref, text: v.text, title: v.title } });
  if (!r.ok) return { ok: false, message: r.message };
  mutate((s) => { s.memory = [...s.memory.filter((m) => m.ref !== v.ref), toVerse(r.data.verse)]; });
  return { ok: true, existed: !r.data.created };
}

export async function reviewMemoryVerse(token: string | null, ref: string, remembered: boolean): Promise<Result> {
  const m = getAppState().memory.find((x) => x.ref === ref);
  if (!m) return { ok: false, message: "That verse is no longer in practice." };
  if (token && m.id) {
    const r = await appApi<Verse>(`/me/memory/${encodeURIComponent(m.id)}/review`, { token, method: "POST", body: { remembered } });
    if (!r.ok) return { ok: false, message: r.message };
    mutate((s) => { s.memory = s.memory.map((x) => x.ref === ref ? toVerse(r.data) : x); });
    return { ok: true };
  }
  const days = [1, 1, 3, 7, 14, 30];
  mutate((s) => { const x = s.memory.find((y) => y.ref === ref); if (!x) return; x.box = remembered ? Math.min(x.box + 1, 5) : 0; x.due = Date.now() + days[x.box] * 864e5; });
  return { ok: true };
}
