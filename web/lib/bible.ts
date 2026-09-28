"use client";

/* ============================================================================
   iCONFESS web — the Bible domain layer.

   Three things live here, and nothing else does:

   1. THE STRUCTURE, re-exported from ./canon — the generated canon, with no
      scripture text in it. It is local so the reader can draw every book and
      chapter before its first request resolves, and keep drawing them when
      the catalogue is empty or unreachable.

   2. THE CLIENT. Every call goes to the iCONFESS API over the same-origin
      /api proxy. The browser never talks to a Bible provider: rights are
      evaluated server-side, and a translation the server has not approved is
      simply absent from the catalogue.

   3. THE STUDY STORE. Highlights, bookmarks, notes, collections, history,
      progress and reading preferences. They are private, they are stored
      locally under one key, and when the browser holds a real API session
      token they are pushed to /api/v1/me/bible/* — the same endpoints the
      mobile app syncs through. Without a token the reader still works and
      nothing is silently lost; it is local until an account claims it.
   ========================================================================= */

import { useSyncExternalStore } from "react";
import { CONFESSIONS, type Confession } from "./data";
import { BOOKS, CANON, bookByID, resolveBook, type Canon } from "./canon";

/* The canon lives in ./canon so server components (page metadata, canonical
   URLs) resolve references through the same code the reader does. It is
   re-exported here so a client component has one import for the domain. */
export * from "./canon";

/* ------------------------------------------------- confessions ↔ scripture */

/** The link that makes this a Bible inside iCONFESS rather than a Bible beside
 *  it: which confessions in the reviewed corpus stand on a given verse. Built
 *  once from the same corpus the rest of the site renders. */
export type VerseConfession = { confession: Confession; reference: string; direct: boolean };

const CONFESSION_INDEX: Map<string, VerseConfession[]> = (() => {
  const index = new Map<string, VerseConfession[]>();
  const add = (key: string, entry: VerseConfession) => {
    const list = index.get(key);
    if (list) {
      if (!list.some((x) => x.confession.slug === entry.confession.slug)) list.push(entry);
      return;
    }
    index.set(key, [entry]);
  };
  for (const confession of CONFESSIONS) {
    for (const scripture of confession.scriptures || []) {
      const bookID = resolveBook(scripture.book);
      if (!bookID) continue;
      const book = bookByID(bookID)!;
      const chapter = Number(scripture.chapter);
      if (!chapter || chapter < 1 || chapter > book.chapter_count) continue;
      const reference = `${book.name} ${chapter}:${scripture.verse}`;
      const entry: VerseConfession = { confession, reference, direct: !!scripture.direct };
      // Expand "22-24" and "2,3" so every verse a confession cites links back.
      const spec = String(scripture.verse || "").replace(/[–—]/g, "-");
      const parts = spec.split(/[-,;]/).map((p) => parseInt(p.trim(), 10)).filter((n) => !Number.isNaN(n));
      if (parts.length === 2 && parts[1] >= parts[0]) {
        for (let v = parts[0]; v <= parts[1]; v++) add(`${bookID}.${chapter}.${v}`, entry);
      } else if (parts.length > 0) {
        for (const v of parts) add(`${bookID}.${chapter}.${v}`, entry);
      }
      add(`${bookID}.${chapter}`, entry);
    }
  }
  return index;
})();

export const confessionsForVerse = (bookID: string, chapter: number, verse: number): VerseConfession[] =>
  CONFESSION_INDEX.get(`${bookID}.${chapter}.${verse}`) || [];

export const confessionsForChapter = (bookID: string, chapter: number): VerseConfession[] =>
  CONFESSION_INDEX.get(`${bookID}.${chapter}`) || [];

/** Books the corpus cites at all — used to surface "passages iCONFESS stands
 *  on" without inventing a popularity metric the platform does not measure. */
export const citedChapters: { bookID: string; chapter: number; count: number }[] = (() => {
  const counts = new Map<string, number>();
  for (const [key, list] of CONFESSION_INDEX) {
    const parts = key.split(".");
    if (parts.length !== 2) continue;
    counts.set(key, list.length);
  }
  return [...counts.entries()]
    .map(([key, count]) => {
      const [bookID, chapter] = key.split(".");
      return { bookID, chapter: Number(chapter), count };
    })
    .sort((a, b) => b.count - a.count || a.bookID.localeCompare(b.bookID));
})();

/* ---------------------------------------------------------------- the API */

export type Translation = {
  id: string;
  name: string;
  abbreviation: string;
  language_code?: string;
  language_name?: string;
  locale?: string;
  country?: string;
  dialect?: string;
  direction?: string;
  publisher?: string;
  description?: string;
  copyright?: string;
  license?: string;
  license_url?: string;
  public_domain?: boolean;
  commercial_use?: boolean;
  redistribution_allowed?: boolean;
  audio_allowed?: boolean;
  offline_allowed?: boolean;
  copy_allowed?: boolean;
  share_allowed?: boolean;
  search_index_allowed?: boolean;
  api_exposure_allowed?: boolean;
  attribution_required?: boolean;
  attribution_text?: string;
  source_url?: string;
  status?: string;
  coverage?: string;
  book_count?: number;
  chapter_count?: number;
  verse_count?: number;
};
export type Language = {
  id: string;
  name: string;
  native_name?: string;
  bcp47?: string;
  direction?: string;
  region?: string;
  script?: string;
  status?: string;
};
export type APIBook = { id: string; name: string; testament: string; canonical_order: number; chapter_count: number };
export type APIVerse = { id: string; number: number; text: string; book_id?: string; chapter?: number };
export type APIChapter = { translation: Translation; book: APIBook; chapter: number; verses: APIVerse[] };
export type APIPassage = { reference: string; translation: Translation; verses: APIVerse[] };
export type SearchHit = { translation: Translation; book: APIBook; chapter: number; verse: number; text: string };
export type StructureResponse = Canon & {
  translation?: {
    id: string;
    name: string;
    abbreviation: string;
    coverage?: string;
    available_book_ids: string[];
    missing_book_ids: string[];
    extra_book_ids?: string[];
  };
};

/** A failed Bible read is never shown as a status code. The reader turns this
 *  into one of two honest sentences: the catalogue is unreachable, or this
 *  passage is not available in this translation. */
export class BibleUnavailable extends Error {
  constructor(readonly status: number, readonly code?: string) {
    super(code || `bible_unavailable_${status}`);
    this.name = "BibleUnavailable";
  }
  get notFound() {
    return this.status === 404;
  }
}

const TRANSIENT_BIBLE_STATUSES = new Set([408, 425, 429]);
const RETRY_DELAYS_MS = [250, 750];

async function get<T>(path: string, signal?: AbortSignal): Promise<T> {
  for (let attempt = 0; attempt <= RETRY_DELAYS_MS.length; attempt += 1) {
    if (signal?.aborted) throw new DOMException("The request was aborted", "AbortError");

    let response: Response;
    try {
      response = await fetch(`/api/v1/bible${path}`, {
        cache: "no-store",
        signal,
        headers: { accept: "application/json" },
      });
    } catch (error) {
      if (signal?.aborted || (error instanceof Error && error.name === "AbortError")) throw error;
      if (attempt === RETRY_DELAYS_MS.length) throw new BibleUnavailable(0);
      await new Promise((resolve) => setTimeout(resolve, RETRY_DELAYS_MS[attempt]));
      continue;
    }

    if (response.ok) return (await response.json()) as T;

    const body = await response.json().catch(() => ({}));
    const transient = TRANSIENT_BIBLE_STATUSES.has(response.status) || response.status >= 500;
    if (transient && attempt < RETRY_DELAYS_MS.length) {
      await new Promise((resolve) => setTimeout(resolve, RETRY_DELAYS_MS[attempt]));
      continue;
    }
    throw new BibleUnavailable(response.status, typeof body?.code === "string" ? body.code : undefined);
  }

  // The loop always returns or throws. Keep a typed fail-closed fallback in
  // case a future edit changes the retry condition.
  throw new BibleUnavailable(0);
}

const q = (params: Record<string, string | number | undefined>) => {
  const search = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== "") search.set(k, String(v));
  const s = search.toString();
  return s ? `?${s}` : "";
};

export const bible = {
  structure: (translation?: string, signal?: AbortSignal) =>
    get<StructureResponse>(`/structure${q({ translation })}`, signal),
  languages: (signal?: AbortSignal) => get<{ languages: Language[] }>("/languages", signal),
  translations: (language?: string, query?: string, signal?: AbortSignal) =>
    get<{ translations: Translation[] }>(`/translations${q({ language, q: query })}`, signal),
  translation: (id: string, signal?: AbortSignal) => get<Translation>(`/translations/${encodeURIComponent(id)}`, signal),
  books: (translation: string, signal?: AbortSignal) => get<{ books: APIBook[] }>(`/books${q({ translation })}`, signal),
  chapter: (translation: string, book: string, chapter: number, signal?: AbortSignal) =>
    get<APIChapter>(`/${encodeURIComponent(translation)}/${encodeURIComponent(book)}/${chapter}`, signal),
  verse: (translation: string, book: string, chapter: number, verse: number, signal?: AbortSignal) =>
    get<APIVerse>(`/${encodeURIComponent(translation)}/${encodeURIComponent(book)}/${chapter}/${verse}`, signal),
  passage: (translation: string, reference: string, signal?: AbortSignal) =>
    get<APIPassage>(`/passage${q({ translation, reference })}`, signal),
  search: (query: string, translation?: string, book?: string, limit = 25, signal?: AbortSignal) =>
    get<{ kind: "text" | "reference"; results: (SearchHit | APIPassage)[] }>(
      `/search${q({ q: query, translation, book, limit })}`,
      signal
    ),
  /** The handler requires two to four named translations and refuses
   *  anything else, so the client names them rather than hoping for a
   *  default it would only discover was missing in production. */
  compare: (reference: string, translations: string[], signal?: AbortSignal) =>
    get<{ reference: string; passages: APIPassage[] }>(
      `/compare${q({ reference, translations: translations.slice(0, 4).join(",") })}`,
      signal
    ),
  crossReferences: (reference: string, signal?: AbortSignal) =>
    get<{ reference: string; references: string[] }>(`/cross-references${q({ reference })}`, signal),
  verseOfDay: (translation: string, signal?: AbortSignal) =>
    get<VerseOfDay>(`/verse-of-day${q({ translation })}`, signal),
  topics: (signal?: AbortSignal) => get<{ topics: BibleTopic[] }>("/topics", signal),
  topic: (slug: string, signal?: AbortSignal) =>
    get<{ topic: BibleTopic }>(`/topics/${encodeURIComponent(slug)}`, signal),
  random: (translation: string, signal?: AbortSignal) =>
    get<{ reference: string; passage: APIPassage; topic?: { slug: string; name: string } }>(
      `/random${q({ translation })}`,
      signal
    ),
  plans: (signal?: AbortSignal) => get<{ plans: BiblePlan[] }>("/plans", signal),
  plan: (slug: string, signal?: AbortSignal) => get<{ plan: BiblePlan }>(`/plans/${encodeURIComponent(slug)}`, signal),
  audio: (translation: string, reference: string, signal?: AbortSignal) =>
    get<BibleAudio>(`/audio${q({ translation, reference })}`, signal),
};

/** The reading-plan shapes the Go handlers serve: a list carries counts, the
 *  detail carries days, and a day carries canon-validated references. */
export type BiblePlan = {
  id?: string;
  slug?: string;
  title?: string;
  description?: string;
  language?: string;
  duration_days?: number;
  reading_count?: number;
  source_note?: string;
  days?: { day_number: number; title?: string; references: string[] }[];
};

export type BibleTopicPassage = {
  reference: string;
  book_id: string;
  book_name?: string;
  chapter: number;
  verses?: string;
  canonical_id?: string;
  confession_count: number;
};
export type BibleTopic = {
  id: string;
  slug: string;
  name: string;
  description?: string;
  passage_count: number;
  passages?: BibleTopicPassage[];
};

/** Verse of the day. The API answers with a single verse and the canonical
 *  verse ID as its reference — not a passage — so that is what is typed. */
export type VerseOfDay = {
  date?: string;
  editor_note?: string;
  translation?: Translation;
  verse?: APIVerse;
  reference?: string;
};

export type BibleAudio = {
  id?: string;
  reference?: string;
  translation?: Translation;
  voice_id?: string;
  duration_ms?: number;
  checksum_sha256?: string;
  alignment?: { verse: number; start_ms: number; end_ms: number }[];
  audio_url?: string;
  expires_in_seconds?: number;
};

/* ------------------------------------------------------- the study store */

export type HighlightColor = "hope" | "faith" | "prayer" | "wisdom" | "love" | "promise" | "personal";

export const HIGHLIGHT_COLORS: { id: HighlightColor; name: string; swatch: string }[] = [
  { id: "hope", name: "Hope", swatch: "#D3DDEF" },
  { id: "faith", name: "Faith", swatch: "#CFE7DC" },
  { id: "prayer", name: "Prayer", swatch: "#E4DAF2" },
  { id: "wisdom", name: "Wisdom", swatch: "#F5E4C3" },
  { id: "love", name: "Love", swatch: "#F6D8D8" },
  { id: "promise", name: "Promise", swatch: "#D9EAF7" },
  { id: "personal", name: "Personal", swatch: "#E9EDEC" },
];

export type Highlight = { id: string; verseID: string; reference: string; color: HighlightColor; at: number };
export type Bookmark = { id: string; verseID: string; reference: string; title?: string; note?: string; at: number };
export type Note = { id: string; verseID: string; reference: string; text: string; at: number; updatedAt: number };
export type Collection = { id: string; name: string; items: { verseID: string; reference: string; at: number }[]; at: number };
export type HistoryEntry = { translation: string; bookID: string; chapter: number; verse?: number; at: number };
export type ReaderMode = "standard" | "focus" | "study" | "audio";

export type BiblePreferences = {
  translation: string;
  secondaryTranslation: string;
  compareWith: string[];
  fontSize: number; // px, applied to verse text
  lineHeight: number;
  fontFamily: "sans" | "serif";
  layout: "verse" | "paragraph";
  showVerseNumbers: boolean;
  theme: "light" | "dark";
  mode: ReaderMode;
  audioSpeed: number;
};

export type BibleState = {
  prefs: BiblePreferences;
  highlights: Highlight[];
  bookmarks: Bookmark[];
  notes: Note[];
  collections: Collection[];
  history: HistoryEntry[];
  progress: Record<string, number>; // `${bookID}.${chapter}` -> completed at
  plans: Record<string, { enrolledAt: number; days: Record<number, number> }>;
  searches: string[];
};

const KEY = "iconfess:bible:v1";

const defaults = (): BibleState => ({
  prefs: {
    translation: "",
    secondaryTranslation: "",
    compareWith: [],
    fontSize: 19,
    lineHeight: 1.85,
    fontFamily: "serif",
    layout: "verse",
    showVerseNumbers: true,
    theme: "light",
    mode: "standard",
    audioSpeed: 1,
  },
  highlights: [],
  bookmarks: [],
  notes: [],
  collections: [],
  history: [],
  progress: {},
  plans: {},
  searches: [],
});

function load(): BibleState {
  if (typeof window === "undefined") return defaults();
  try {
    const stored = JSON.parse(window.localStorage.getItem(KEY) || "{}");
    const base = defaults();
    const merged = { ...base, ...stored, prefs: { ...base.prefs, ...(stored.prefs || {}) } };
    // Retired palette names from older builds resolve into the two that remain.
    if (merged.prefs.theme !== "dark" && merged.prefs.theme !== "light") merged.prefs.theme = "light";
    return merged;
  } catch {
    return defaults();
  }
}

let state: BibleState = typeof window === "undefined" ? defaults() : load();
const listeners = new Set<() => void>();
const server = defaults();

function persist() {
  try {
    window.localStorage.setItem(KEY, JSON.stringify(state));
  } catch {
    /* private mode: the reader keeps working, the marks just do not survive */
  }
}

export function mutateBible(fn: (s: BibleState) => void) {
  fn(state);
  state = { ...state };
  persist();
  listeners.forEach((l) => l());
}

export function useBible(): BibleState {
  return useSyncExternalStore(
    (cb) => {
      listeners.add(cb);
      return () => listeners.delete(cb);
    },
    () => state,
    () => server
  );
}

const id = () => Math.random().toString(36).slice(2, 10) + Date.now().toString(36);

/* Every mutation below is local-first and idempotent on the verse identity, so
   the same verse can never collect two highlights or two bookmarks. When a
   real API session exists the change is mirrored to /api/v1/me/bible/*; a
   failed mirror is not an error the reader shows, because the local write
   already succeeded and the sync outbox is the mobile client's job. */

function mirror(path: string, method: string, body?: unknown) {
  if (typeof window === "undefined") return;
  let token = "";
  try {
    token = window.localStorage.getItem("ic_token") || "";
  } catch {
    return;
  }
  if (!token) return;
  void fetch(`/api/v1/me/bible${path}`, {
    method,
    headers: { authorization: `Bearer ${token}`, ...(body ? { "content-type": "application/json" } : {}) },
    ...(body ? { body: JSON.stringify(body) } : {}),
  }).catch(() => undefined);
}

export const study = {
  highlight(verseID: string, reference: string, color: HighlightColor) {
    mutateBible((s) => {
      const existing = s.highlights.find((h) => h.verseID === verseID);
      if (existing) {
        if (existing.color === color) {
          s.highlights = s.highlights.filter((h) => h.verseID !== verseID);
          return;
        }
        existing.color = color;
        existing.at = Date.now();
        return;
      }
      s.highlights = [{ id: id(), verseID, reference, color, at: Date.now() }, ...s.highlights];
    });
    mirror("/highlights", "POST", { verse_id: verseID, reference, color });
  },
  clearHighlight(verseID: string) {
    mutateBible((s) => {
      s.highlights = s.highlights.filter((h) => h.verseID !== verseID);
    });
  },
  bookmark(verseID: string, reference: string, title?: string) {
    let added = false;
    mutateBible((s) => {
      if (s.bookmarks.some((b) => b.verseID === verseID)) {
        s.bookmarks = s.bookmarks.filter((b) => b.verseID !== verseID);
        return;
      }
      added = true;
      s.bookmarks = [{ id: id(), verseID, reference, title, at: Date.now() }, ...s.bookmarks];
    });
    if (added) mirror("/bookmarks", "POST", { verse_id: verseID, reference, title });
  },
  removeBookmark(bookmarkID: string) {
    mutateBible((s) => {
      s.bookmarks = s.bookmarks.filter((b) => b.id !== bookmarkID);
    });
  },
  note(verseID: string, reference: string, text: string) {
    mutateBible((s) => {
      const existing = s.notes.find((n) => n.verseID === verseID);
      if (existing) {
        existing.text = text;
        existing.updatedAt = Date.now();
        return;
      }
      s.notes = [{ id: id(), verseID, reference, text, at: Date.now(), updatedAt: Date.now() }, ...s.notes];
    });
    mirror("/notes", "POST", { verse_id: verseID, reference, body: text });
  },
  removeNote(noteID: string) {
    mutateBible((s) => {
      s.notes = s.notes.filter((n) => n.id !== noteID);
    });
  },
  createCollection(name: string): string {
    const collectionID = id();
    mutateBible((s) => {
      s.collections = [{ id: collectionID, name, items: [], at: Date.now() }, ...s.collections];
    });
    mirror("/collections", "POST", { name });
    return collectionID;
  },
  addToCollection(collectionID: string, verseID: string, reference: string) {
    mutateBible((s) => {
      const collection = s.collections.find((c) => c.id === collectionID);
      if (!collection || collection.items.some((i) => i.verseID === verseID)) return;
      collection.items = [{ verseID, reference, at: Date.now() }, ...collection.items];
    });
  },
  removeFromCollection(collectionID: string, verseID: string) {
    mutateBible((s) => {
      const collection = s.collections.find((c) => c.id === collectionID);
      if (collection) collection.items = collection.items.filter((i) => i.verseID !== verseID);
    });
  },
  deleteCollection(collectionID: string) {
    mutateBible((s) => {
      s.collections = s.collections.filter((c) => c.id !== collectionID);
    });
  },
  recordRead(translation: string, bookID: string, chapter: number, verse?: number) {
    mutateBible((s) => {
      s.history = [
        { translation, bookID, chapter, verse, at: Date.now() },
        ...s.history.filter((h) => !(h.bookID === bookID && h.chapter === chapter && h.translation === translation)),
      ].slice(0, 120);
    });
    mirror("/history", "POST", { translation_id: translation, book_id: bookID, chapter, verse });
  },
  completeChapter(bookID: string, chapter: number) {
    mutateBible((s) => {
      const key = `${bookID}.${chapter}`;
      if (s.progress[key]) delete s.progress[key];
      else s.progress[key] = Date.now();
    });
    mirror("/progress", "POST", { book_id: bookID, chapter });
  },
  rememberSearch(query: string) {
    mutateBible((s) => {
      s.searches = [query, ...s.searches.filter((x) => x !== query)].slice(0, 12);
    });
  },
  setPrefs(patch: Partial<BiblePreferences>) {
    mutateBible((s) => {
      s.prefs = { ...s.prefs, ...patch };
    });
    mirror("/preferences", "PUT", patch);
  },
  enrollPlan(planID: string) {
    mutateBible((s) => {
      if (!s.plans[planID]) s.plans[planID] = { enrolledAt: Date.now(), days: {} };
    });
  },
  completePlanDay(planID: string, day: number) {
    mutateBible((s) => {
      const plan = s.plans[planID] || { enrolledAt: Date.now(), days: {} };
      if (plan.days[day]) delete plan.days[day];
      else plan.days[day] = Date.now();
      s.plans[planID] = plan;
    });
  },
};

/* --------------------------------------------------------------- helpers */

export const highlightFor = (s: BibleState, verseID: string) => s.highlights.find((h) => h.verseID === verseID);
export const bookmarkFor = (s: BibleState, verseID: string) => s.bookmarks.find((b) => b.verseID === verseID);
export const noteFor = (s: BibleState, verseID: string) => s.notes.find((n) => n.verseID === verseID);
export const chapterComplete = (s: BibleState, bookID: string, chapter: number) => !!s.progress[`${bookID}.${chapter}`];

export const readingStats = (s: BibleState) => {
  const chapters = Object.keys(s.progress).length;
  const booksComplete = BOOKS.filter((b) =>
    Array.from({ length: b.chapter_count }, (_, i) => i + 1).every((c) => !!s.progress[`${b.id}.${c}`])
  ).length;
  return {
    chapters,
    booksComplete,
    percent: Math.round((chapters / CANON.chapter_count) * 1000) / 10,
    highlights: s.highlights.length,
    bookmarks: s.bookmarks.length,
    notes: s.notes.length,
  };
};

/** Deep-link path for the reader: /bible/{translation}/{book}/{chapter}. */
export const readerHref = (translation: string, bookID: string, chapter: number, verse?: number) =>
  `/bible/${encodeURIComponent(translation || "-")}/${encodeURIComponent(bookID)}/${chapter}${verse ? `#v${verse}` : ""}`;

/** The appearance applies to the whole surface — header toggle and reader
 *  settings write the same light/dark state, so the two can never disagree. */
export function applyTheme(theme: BiblePreferences["theme"]) {
  if (typeof document === "undefined") return;
  document.documentElement.setAttribute("data-theme", theme);
  try { window.localStorage.setItem("ic-theme", theme); } catch { }
}
export function currentTheme(): BiblePreferences["theme"] {
  if (typeof document === "undefined") return "light";
  return document.documentElement.getAttribute("data-theme") === "dark" ? "dark" : "light";
}
