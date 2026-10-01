"use client";
import React, { createContext, useContext, useEffect, useRef, useState } from "react";
import { usePlayer } from "./player";
import { useRouter } from "next/navigation";
import { CATEGORIES, CONFESSIONS, ARTICLES, VOICES, ALL_SESSIONS, catBySlug, motifStyle, queueItem, type QueueItem } from "./data";
import { useApp, mutate, track } from "./store";
import { Icon } from "@/components/ui";

/* The UI facade and the full player share PlayerProvider's ONE queue. */
export type AudioApi = Omit<ReturnType<typeof usePlayer>, "play"> & {
  play: (queue: QueueItem[], start?: number) => void;
  resume: () => void;
  stop: () => void;
  current: () => QueueItem | null;
};
const AudioCtx = createContext<AudioApi | null>(null);
export const useAudio = () => {
  const audio = useContext(AudioCtx);
  if (!audio) throw new Error("useAudio requires UiProvider");
  return audio;
};
const ToastCtx = createContext<(m: string) => void>(() => {});
export const useToast = () => useContext(ToastCtx);
const SearchCtx = createContext<() => void>(() => {});
export const useSearch = () => useContext(SearchCtx);

export function UiProvider({ children }: { children: React.ReactNode }) {
  useEffect(() => {
    if (process.env.NODE_ENV === "production" && "serviceWorker" in navigator) {
      navigator.serviceWorker.register("/sw.js").catch(() => {});
    }
  }, []);
  const router = useRouter();
  const player = usePlayer();
  const [toastMsg, setToastMsg] = useState<string | null>(null);
  const toastTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [mpHidden, setMpHidden] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const toast = (message: string) => {
    if (toastTimer.current) clearTimeout(toastTimer.current);
    setToastMsg(message);
    toastTimer.current = setTimeout(() => setToastMsg(null), 4000);
  };
  useEffect(() => () => { if (toastTimer.current) clearTimeout(toastTimer.current); }, []);
  const api: AudioApi = {
    ...player,
    play: player.playQueue,
    resume: () => { void player.play(); },
    stop: player.clear,
    current: () => player.queue[player.idx] || null,
  };
  const item = api.current();
  const playerOn = !!item && player.status !== "idle";
  return (
    <AudioCtx.Provider value={api}>
      <ToastCtx.Provider value={toast}>
        <SearchCtx.Provider value={() => setSearchOpen(true)}>
          {children}
          <div className={"mini-player" + (playerOn ? " on" : "") + (mpHidden ? " hid" : "")} aria-hidden={!playerOn}>
            <button className="mp-hide" aria-label={mpHidden ? "Show player" : "Hide player"} onClick={() => setMpHidden(!mpHidden)}>{mpHidden ? "▲" : "▼"}</button>
            {item && <div className="mp-in">
              <div className="mp-art" style={motifStyle(catBySlug(item.category)?.slug || "peace")}><Icon n="wave" s={16} /></div>
              <div className="mp-main">
                <div className="mp-title">{item.title}</div>
                <div className="mp-sub">{player.source === "file" ? "Published audio" : "Device speech"} · {player.status}</div>
                <div className="progress" role="progressbar" aria-label="Playback progress" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(player.progress * 100)}><i style={{ width: Math.round(player.progress * 100) + "%" }} /></div>
              </div>
              <button className="icon-btn ghost-dark" onClick={api.prev} disabled={player.idx === 0} aria-label="Previous"><Icon n="prev" s={16} /></button>
              <button className="icon-btn accent" onClick={() => player.status === "error" ? api.retry() : player.status === "playing" || player.status === "loading" ? api.pause() : api.resume()} aria-label={player.status === "error" ? "Retry audio" : player.status === "playing" || player.status === "loading" ? "Pause" : "Play"}><Icon n={player.status === "playing" || player.status === "loading" ? "pause" : "play"} s={16} /></button>
              <button className="icon-btn ghost-dark" onClick={api.next} disabled={player.idx >= player.queue.length - 1} aria-label="Next"><Icon n="next" s={16} /></button>
              <button className="icon-btn ghost-dark" onClick={() => router.push("/app/player")} aria-label="Open player"><Icon n="arrow" s={14} /></button>
              <button className="icon-btn ghost-dark" onClick={api.stop} aria-label="Stop playback"><Icon n="x" s={14} /></button>
            </div>}
          </div>
          <div className={"toast" + (toastMsg ? " on" : "")} role="status">{toastMsg}</div>
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
