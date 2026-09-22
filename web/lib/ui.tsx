"use client";
import React, { createContext, useContext, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { CATEGORIES, CONFESSIONS, ARTICLES, VOICES, ALL_SESSIONS, catBySlug, motifStyle, queueItem, type QueueItem } from "./data";
import { useApp, mutate, track } from "./store";
import { resolveVoice, preprocessSpeech, logAE } from "./audio-engine";
import { startAmbience, stopAmbience, setAmbienceLevel, setDucked, toneToSpeech } from "./dsp";
import { Icon } from "@/components/ui";

/* ---------------- Audio + Voice Engine — global player (§38, §90) ----------------
   State machine: idle → playing ⇄ paused → completed | error.
   One engine, persists across navigation, drives mini + full player + ambience. */
type AudioState = { queue: QueueItem[]; idx: number; status: "idle" | "playing" | "paused" | "completed" | "error"; progress: number };
type AudioApi = AudioState & {
  play: (q: QueueItem[], start?: number) => void; pause: () => void; resume: () => void; stop: () => void;
  next: () => void; prev: () => void; current: () => QueueItem | null;
  updateQueue: (q: QueueItem[], idx: number) => void;
  seekRel: (deltaPct: number) => void;
  voiceName: () => string;
};
const AudioCtx = createContext<AudioApi | null>(null);
export const useAudio = () => useContext(AudioCtx)!;
const ToastCtx = createContext<(m: string) => void>(() => { });
export const useToast = () => useContext(ToastCtx);
const SearchCtx = createContext<() => void>(() => { });
export const useSearch = () => useContext(SearchCtx);

const sentences = (t: string) => t.split(/(?<=[.!?])\s+/).map((s) => s.trim()).filter(Boolean);

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
  const stRef = useRef(st); stRef.current = st;

  const toast = (m: string) => { setToastMsg(m); window.setTimeout(() => setToastMsg(null), 2600); };

  /* §64 ambience engine — follows the saved audio settings */
  const ambId = st.settings.ambienceId || "";
  const ambLevel = st.settings.ambienceLevel ?? 45;
  useEffect(() => {
    if (typeof window === "undefined") return;
    if (!ambId) stopAmbience(); else { startAmbience(ambId, ambLevel); setDucked(ref.current.status === "playing"); }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ambId]);
  useEffect(() => { if (ambId) setAmbienceLevel(ambLevel); }, [ambLevel, ambId]);

  const set = (p: Partial<AudioState>) => setAudio((a) => ({ ...a, ...p }));
  const savePosition = (slug: string, pct: number) => {
    if (pct > 0.03 && pct < 0.97) mutate((s) => { s.positions = { ...s.positions, [slug]: { pct, at: Date.now() } }; });
  };

  const record = (item: QueueItem) => {
    if (stRef.current.settings.privacy.pauseHistory) return;
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

  const repeatLeft = useRef(0);
  const api: AudioApi = {
    ...audio,
    current: () => ref.current.queue[ref.current.idx] || null,
    voiceName: () => { try { return resolveVoice(stRef.current.settings.voiceId).voice.displayName; } catch { return "Grace"; } },
    play(q, start = 0) {
      if (!synthRef.current) { set({ queue: q, idx: start, status: "error" }); return; }
      /* §42 resume — offer to continue from the saved position (§91 local sync) */
      let startPct = 0;
      const first = q[start];
      const saved = first ? stRef.current.positions[first.slug] : null;
      if (saved && saved.pct > 0.08) {
        startPct = saved.pct;
        toast("Continuing from " + Math.round(saved.pct * 100) + "%");
      }
      set({ queue: q, idx: start, status: "playing", progress: startPct });
      speak(q, start, startPct);
      logAE("audio_started", { items: q.length });
    },
    pause() {
      synthRef.current?.pause(); set({ status: "paused" }); setDucked(false);
      const it = ref.current.queue[ref.current.idx]; if (it) savePosition(it.slug, ref.current.progress);
      logAE("audio_paused", { slug: it?.slug || "" });
    },
    resume() { synthRef.current?.resume(); set({ status: "playing" }); setDucked(true); logAE("audio_resumed", {}); },
    stop() {
      const it = ref.current.queue[ref.current.idx]; if (it) savePosition(it.slug, ref.current.progress);
      synthRef.current?.cancel(); set({ queue: [], idx: 0, status: "idle", progress: 0 }); setDucked(false);
    },
    next() { const i = ref.current.idx; if (i < ref.current.queue.length - 1) { set({ idx: i + 1, status: "playing", progress: 0 }); speak(ref.current.queue, i + 1, 0); } },
    prev() { const i = ref.current.idx; if (i > 0) { set({ idx: i - 1, status: "playing", progress: 0 }); speak(ref.current.queue, i - 1, 0); } },
    updateQueue(q, idx) { repeatLeft.current = 0; set({ queue: q, idx, status: "playing", progress: 0 }); speak(q, idx, 0); },
    seekRel(d) {
      const it = ref.current.queue[ref.current.idx]; if (!it) return;
      const p = Math.min(0.98, Math.max(0, ref.current.progress + d));
      set({ progress: p }); speak(ref.current.queue, ref.current.idx, p);
      logAE("audio_seeked", { slug: it.slug });
    },
  };

  function speak(q: QueueItem[], idx: number, fromPct = 0) {
    const synth = synthRef.current; const base = q[idx];
    if (!synth) return;
    if (!base) { set({ status: "completed", progress: 0 }); setDucked(false); track("session_completed", { items: q.length }); return; }
    synth.cancel();
    /* §42/§36 resume mid-item: speak from the sentence nearest the saved position */
    let item = base;
    if (fromPct > 0.03) {
      const ss = sentences(base.text);
      const si = Math.min(ss.length - 1, Math.round(fromPct * (ss.length - 1)));
      if (si > 0) item = { ...base, text: ss.slice(si).join(" ") };
    }
    const u = new SpeechSynthesisUtterance(preprocessSpeech(item.text));
    /* provider abstraction (§07) + rights gate (§06) + voice tone mapping (§24) */
    const s = stRef.current.settings;
    const { voice: cv, resolved, gate } = resolveVoice(s.voiceId || "grace");
    const tone = toneToSpeech({ warmth: s.warmth, clarity: s.clarity, presence: s.presence });
    if (gate.ok && resolved) { if (resolved.synthVoice) u.voice = resolved.synthVoice; }
    else { const vs: any[] = synth.getVoices(); u.voice = vs.find((v) => v.lang.startsWith("en")) || vs[0] || null; }
    const conf = CONFESSIONS.find((c) => c.slug === base.slug);
    const inten = conf?.intensity ?? 2;
    const eqMul = s.eq === "bright" ? [1.08, 1.1] : s.eq === "calm" ? [0.92, 0.95] : [1, 1];
    u.rate = (s.rate || 1) * eqMul[0] * (resolved?.rateMul ?? 1) * tone.rateMul;
    u.pitch = (s.pitch ?? 1) * eqMul[1] * (resolved?.pitchMul ?? 1) * tone.pitchMul;
    u.volume = Math.min(1, (s.normalize ? (s.volume ?? 1) : Math.min(1, (s.volume ?? 1) * (0.8 + inten * 0.07))) * tone.volMul);
    /* §46 Media Session */
    try {
      if ("mediaSession" in navigator) {
        navigator.mediaSession.metadata = new MediaMetadata({ title: base.title, artist: "iCONFESS · " + cv.displayName, album: base.category });
        navigator.mediaSession.setActionHandler("play", () => api.resume());
        navigator.mediaSession.setActionHandler("pause", () => api.pause());
        navigator.mediaSession.setActionHandler("previoustrack", () => api.prev());
        navigator.mediaSession.setActionHandler("nexttrack", () => api.next());
      }
    } catch { }
    setDucked(true);
    if (idx !== ref.current.idx || repeatLeft.current === 0) repeatLeft.current = Math.max(0, (s.repeat || 1) - 1);
    const total = Math.max(1, item.text.length);
    u.onboundary = (e: any) => {
      const local = Math.min(1, (e.charIndex || 0) / total);
      const global = fromPct + local * (1 - fromPct);
      set({ progress: Math.min(1, global) });
    };
    u.onend = () => {
      if (ref.current.status === "playing") {
        if (repeatLeft.current > 0) { repeatLeft.current -= 1; speak(q, idx, 0); return; }
        record(base);
        mutate((x) => { delete x.positions[base.slug]; });
        if (idx + 1 < q.length) { speak(q, idx + 1, 0); set({ idx: idx + 1, progress: 0 }); }
        else { set({ status: "completed", progress: 1 }); setDucked(false); logAE("audio_completed", { slug: base.slug }); }
      }
    };
    u.onerror = () => { set({ status: "error" }); setDucked(false); logAE("audio_error", { slug: base.slug }); };
    record(base);
    synth.speak(u);
  }

  /* §93 keyboard shortcuts — space play/pause, arrows seek/volume */
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement;
      if (t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.isContentEditable)) return;
      const a = ref.current;
      if (!a.queue.length) return;
      if (e.code === "Space") { e.preventDefault(); a.status === "playing" ? api.pause() : a.status === "paused" ? api.resume() : null; }
      else if (e.key === "ArrowRight") { e.preventDefault(); api.seekRel(0.1); }
      else if (e.key === "ArrowLeft") { e.preventDefault(); api.seekRel(-0.1); }
      else if (e.key === "ArrowUp") { e.preventDefault(); mutate((s) => { s.settings.volume = Math.min(1, +(s.settings.volume + 0.1).toFixed(2)); }); logAE("volume_changed", {}); }
      else if (e.key === "ArrowDown") { e.preventDefault(); mutate((s) => { s.settings.volume = Math.max(0, +(s.settings.volume - 0.1).toFixed(2)); }); logAE("volume_changed", {}); }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  useEffect(() => { synthRef.current?.getVoices(); }, []);

  const item = audio.queue[audio.idx] || null;
  const playerOn = !!item && audio.status !== "idle";
  const vName = (() => { try { return resolveVoice(st.settings.voiceId || "grace").voice.displayName; } catch { return "Grace"; } })();

  return (
    <AudioCtx.Provider value={api}>
      <ToastCtx.Provider value={toast}>
        <SearchCtx.Provider value={() => setSearchOpen(true)}>
          {children}
          {/* mini player (§39) */}
          <div className={"mini-player" + (playerOn ? " on" : "") + (mpHidden ? " hid" : "")} aria-hidden={!playerOn}>
            <button className="mp-hide" aria-label={mpHidden ? "Show player" : "Hide player"} onClick={() => setMpHidden(!mpHidden)}>{mpHidden ? "▲" : "▼"}</button>
            {item && (
              <div className="mp-in">
                <div className="mp-art" style={motifStyle(catBySlug(item.category)?.slug || "peace")}>{<Icon n="wave" s={16} />}</div>
                <div className="mp-main">
                  <div className="mp-title">{item.title}</div>
                  <div className="mp-sub">{item.category} · Voice: {vName} · {audio.status}</div>
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
    .map((c) => ({ kind: "Stands on " + m![1] + " " + ch, title: c.title, sub: c.scriptures.map((sc) => sc.book + " " + sc.chapter + ":" + sc.verse).join(", "), href: `/confessions/${c.slug}` }));
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
