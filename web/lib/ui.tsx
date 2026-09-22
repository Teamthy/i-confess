"use client";
import React, { createContext, useContext, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { CATEGORIES, CONFESSIONS, ARTICLES, VOICES, ALL_SESSIONS, catBySlug, motifStyle, queueItem, type QueueItem } from "./data";
import { useApp, mutate, track } from "./store";
import { Icon } from "@/components/ui";

/* ---------------- audio engine (Web Speech — real audio, zero files) ---------------- */
type AudioState = { queue: QueueItem[]; idx: number; status: "idle" | "playing" | "paused" | "completed" | "error"; progress: number };
type AudioApi = AudioState & {
  play: (q: QueueItem[], start?: number) => void; pause: () => void; resume: () => void; stop: () => void;
  next: () => void; prev: () => void; current: () => QueueItem | null;
  updateQueue: (q: QueueItem[], idx: number) => void;
};
const AudioCtx = createContext<AudioApi | null>(null);
export const useAudio = () => useContext(AudioCtx)!;
const ToastCtx = createContext<(m: string) => void>(() => { });
export const useToast = () => useContext(ToastCtx);
const SearchCtx = createContext<() => void>(() => { });
export const useSearch = () => useContext(SearchCtx);

export function UiProvider({ children }: { children: React.ReactNode }) {
  React.useEffect(() => {
    if (process.env.NODE_ENV === "production" && "serviceWorker" in navigator) {
      navigator.serviceWorker.register("/sw.js").catch(() => { });
    }
  }, []);
  const router = useRouter();
  const [audio, setAudio] = useState<AudioState>({ queue: [], idx: 0, status: "idle", progress: 0 });
  const ref = useRef(audio); ref.current = audio;
  const synthRef = useRef<any>(typeof window !== "undefined" ? window.speechSynthesis || null : null);
  const [toastMsg, setToastMsg] = useState<string | null>(null);
  const [mpHidden, setMpHidden] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const st = useApp();

  const toast = (m: string) => { setToastMsg(m); window.setTimeout(() => setToastMsg(null), 2600); };

  const pickVoice = () => {
    const synth = synthRef.current; if (!synth) return null;
    const vs = synth.getVoices();
    return vs.find((v: any) => v.lang.startsWith("en") && /female|zira|samantha|aria|moira|tessa/i.test(v.name)) || vs.find((v: any) => v.lang.startsWith("en")) || vs[0] || null;
  };
  const set = (p: Partial<AudioState>) => setAudio((a) => ({ ...a, ...p }));

  const record = (item: QueueItem) => {
    if (st.settings.privacy.pauseHistory) return;
    mutate((s) => {
      s.history = [{ slug: item.slug, title: item.title, category: item.category, at: Date.now() }, ...s.history.filter((h) => !(h.slug === item.slug && h.at > Date.now() - 60000))].slice(0, 60);
      const today = new Date().toDateString();
      if (s.streak.last !== today) {
        const yest = new Date(Date.now() - 864e5).toDateString();
        s.streak = { count: s.streak.last === yest ? s.streak.count + 1 : 1, last: today };
      }
    });
    track("confession_played", { slug: item.slug });
  };

  const api: AudioApi = {
    ...audio,
    current: () => ref.current.queue[ref.current.idx] || null,
    play(q, start = 0) {
      if (!synthRef.current) { set({ queue: q, idx: start, status: "error" }); return; }
      set({ queue: q, idx: start, status: "playing", progress: 0 });
      speak(q, start);
    },
    pause() { synthRef.current?.pause(); set({ status: "paused" }); },
    resume() { synthRef.current?.resume(); set({ status: "playing" }); },
    stop() { synthRef.current?.cancel(); set({ queue: [], idx: 0, status: "idle", progress: 0 }); },
    next() { const i = ref.current.idx; if (i < ref.current.queue.length - 1) { set({ idx: i + 1, status: "playing" }); speak(ref.current.queue, i + 1); } },
    prev() { const i = ref.current.idx; if (i > 0) { set({ idx: i - 1, status: "playing" }); speak(ref.current.queue, i - 1); } },
    updateQueue(q, idx) { repeatLeft.current = 0; set({ queue: q, idx, status: "playing", progress: 0 }); speak(q, idx); },
  };
  const repeatLeft = useRef(0);

  function speak(q: QueueItem[], idx: number) {
    const synth = synthRef.current; const item = q[idx];
    if (!synth) return;
    if (!item) { set({ status: "completed", progress: 0 }); track("session_completed", { items: q.length }); return; }
    synth.cancel();
    const u = new SpeechSynthesisUtterance(item.text);
    const v = pickVoice(); if (v) u.voice = v;
    const conf = CONFESSIONS.find((c) => c.slug === item.slug);
    const inten = conf?.intensity ?? 2;
    u.rate = (st.settings.rate || 1) * (st.settings.eq === "bright" ? 1.08 : st.settings.eq === "calm" ? 0.92 : 1);
    u.pitch = (st.settings.pitch ?? 1) * (st.settings.eq === "bright" ? 1.1 : st.settings.eq === "calm" ? 0.95 : 1);
    u.volume = st.settings.normalize ? (st.settings.volume ?? 1) : Math.min(1, (st.settings.volume ?? 1) * (0.8 + inten * 0.07));
    if (idx !== ref.current.idx || repeatLeft.current === 0) repeatLeft.current = Math.max(0, (st.settings.repeat || 1) - 1);
    u.onboundary = (e: any) => set({ progress: Math.min(1, (e.charIndex || 0) / Math.max(1, item.text.length)) });
    u.onend = () => { if (ref.current.status === "playing") { if (repeatLeft.current > 0) { repeatLeft.current -= 1; speak(q, idx); return; } record(item); speak(q, idx + 1); set({ idx: idx + 1, progress: 0 }); } };
    u.onerror = () => set({ status: "error" });
    record(item);
    synth.speak(u);
  }

  useEffect(() => { synthRef.current?.getVoices(); }, []);

  const item = audio.queue[audio.idx] || null;
  const playerOn = !!item && audio.status !== "idle";

  return (
    <AudioCtx.Provider value={api}>
      <ToastCtx.Provider value={toast}>
        <SearchCtx.Provider value={() => setSearchOpen(true)}>
          {children}
          {/* mini player */}
          <div className={"mini-player" + (playerOn ? " on" : "") + (mpHidden ? " hid" : "")} aria-hidden={!playerOn}>
            <button className="mp-hide" aria-label={mpHidden ? "Show player" : "Hide player"} onClick={() => setMpHidden(!mpHidden)}>{mpHidden ? "▲" : "▼"}</button>
            {item && (
              <div className="mp-in">
                <div className="mp-art" style={motifStyle(catBySlug(item.category)?.slug || "peace")}>{<Icon n="wave" s={16} />}</div>
                <div className="mp-main">
                  <div className="mp-title">{item.title}</div>
                  <div className="mp-sub">{item.category} · Voice: Grace · {audio.status}</div>
                  <div className="progress"><i style={{ width: Math.round(audio.progress * 100) + "%" }} /></div>
                </div>
                <button className="icon-btn ghost-dark" onClick={api.prev} aria-label="Previous"><Icon n="prev" s={16} /></button>
                <button className="icon-btn accent" onClick={() => (audio.status === "playing" ? api.pause() : audio.status === "paused" ? api.resume() : null)} aria-label="Play or pause"><Icon n={audio.status === "playing" ? "pause" : "play"} s={16} /></button>
                <button className="icon-btn ghost-dark" onClick={api.next} aria-label="Next"><Icon n="next" s={16} /></button>
                <button className="icon-btn ghost-dark" onClick={() => router.push("/app/player")} aria-label="Open player"><Icon n="arrow" s={14} /></button>
              </div>
            )}
          </div>
          {/* toast */}
          <div className={"toast" + (toastMsg ? " on" : "")} role="status">{toastMsg}</div>
          {/* search overlay */}
          <div className={"search-ovl" + (searchOpen ? " open" : "")} onClick={(e) => e.target === e.currentTarget && setSearchOpen(false)}>
            <SearchPanel close={() => setSearchOpen(false)} />
          </div>
        </SearchCtx.Provider>
      </ToastCtx.Provider>
    </AudioCtx.Provider>
  );
}

function scriptureRows(q: string) {
  const m = q.trim().match(/^((?:[1-3]\s)?[a-z]+)\.?\s+(\d+)/);
  if (!m) return [];
  const book = m[1].toLowerCase(); const ch = Number(m[2]);
  return CONFESSIONS.filter((c) => c.scriptures.some((sc) => sc.book.toLowerCase().startsWith(book) && sc.chapter === ch)).slice(0, 5)
    .map((c) => ({ kind: "Stands on " + m![1] + " " + m![2], title: c.title, sub: c.scriptures.map((sc) => sc.book + " " + sc.chapter + ":" + sc.verse).join(", "), href: `/confessions/${c.slug}` }));
}
function SearchPanel({ close }: { close: () => void }) {
  const router = useRouter();
  const [q, setQ] = useState("");
  const st = useApp();
  const rows: { kind: string; title: string; sub: string; href: string }[] = !q.trim()
    ? st.recentSearches.slice(0, 5).map((r) => ({ kind: "Recent", title: r, sub: "", href: "/app/search?q=" + encodeURIComponent(r) }))
    : [
      ...CATEGORIES.filter((c) => (c.name + c.tagline).toLowerCase().includes(q)).slice(0, 4).map((c) => ({ kind: "Category", title: c.name, sub: c.tagline, href: `/categories/${c.slug}` })),
      ...CONFESSIONS.filter((c) => (c.title + c.short).toLowerCase().includes(q)).slice(0, 6).map((c) => ({ kind: "Confession", title: c.title, sub: c.category, href: `/confessions/${c.slug}` })),
      ...scriptureRows(q),
      ...ALL_SESSIONS.filter((s) => s.title.toLowerCase().includes(q)).slice(0, 4).map((s) => ({ kind: "Session", title: s.title, sub: s.description, href: `/sessions/${s.slug}` })),
      ...VOICES.filter((v) => (v.name + v.description).toLowerCase().includes(q)).map((v) => ({ kind: "Voice", title: v.name, sub: v.description, href: `/voices/${v.slug}` })),
      ...ARTICLES.filter((a) => (a.title + a.excerpt).toLowerCase().includes(q)).slice(0, 4).map((a) => ({ kind: "Journal", title: a.title, sub: a.category, href: `/journal/${a.slug}` })),
    ];
  return (
    <div className="search-panel" role="dialog" aria-modal="true" aria-label="Search iCONFESS">
      <input autoFocus type="search" placeholder="Search confessions, categories, sessions, voices, articles…" aria-label="Search"
        value={q} onChange={(e) => setQ(e.target.value.toLowerCase())} onKeyDown={(e) => e.key === "Escape" && close()} />
      <div className="search-results">
        {rows.length ? rows.map((r, i) => (
          <div className="sr-row" key={i} onClick={() => { mutate((s) => { s.recentSearches = [q, ...s.recentSearches.filter((x) => x !== q)].slice(0, 6); }); close(); router.push(r.href); }}>
            <span className="sr-kind">{r.kind}</span><div><b>{r.title}</b>{r.sub && <span>{r.sub}</span>}</div>
          </div>
        )) : <div style={{ padding: "28px 24px" }} className="small">{q ? "No results. Try “peace” or “healed”." : `Type to search ${CONFESSIONS.length} confessions, ${CATEGORIES.length} categories, sessions, voices and the journal.`}</div>}
      </div>
    </div>
  );
}
