/* ============================================================================
   iCONFESS web — the canonical Bible structure, as data.

   canon.json is generated from server/internal/bible by
   `go run ./cmd/bible-structure ../web/lib/canon.json`, and a Go test fails
   when the two drift. It is the canon — 66 books, 1,189 chapters, the KJV
   reference verse distribution, the section grouping and the alias table the
   reference parser resolves against — and it contains NO scripture text.

   That distinction is the design. The website may ship the shape of the
   Bible, because the shape is a fact about the canon; it may never ship verse
   text, because the text is a licensed asset the API serves under
   per-translation rights. The structure being local is what lets the reader
   draw every book and chapter before the first request resolves, validate a
   typed reference without a round trip, and keep working when the catalogue
   is empty or unreachable.

   This module is deliberately NOT a client module: page metadata, canonical
   URLs and the reader all resolve references through the same code.
   ========================================================================= */

import canon from "./canon.json";

export type CanonBook = {
  id: string;
  usfm: string;
  name: string;
  abbreviation: string;
  testament: "old" | "new";
  section: string;
  canonical_order: number;
  chapter_count: number;
  verse_count: number;
  chapters: number[];
  aliases?: string[];
};
export type CanonSection = {
  id: string;
  name: string;
  testament: string;
  book_ids: string[];
  book_count: number;
  chapter_count: number;
};
export type CanonTestament = {
  id: "old" | "new";
  name: string;
  book_count: number;
  chapter_count: number;
  verse_count: number;
  sections: CanonSection[];
};
export type Canon = {
  canon: string;
  book_count: number;
  chapter_count: number;
  verse_count: number;
  testaments: CanonTestament[];
  books: CanonBook[];
  aliases: Record<string, string>;
};

export const CANON = canon as Canon;
export const BOOKS = CANON.books;
export const TESTAMENTS = CANON.testaments;

const BY_ID = new Map(BOOKS.map((b) => [b.id, b]));
const BY_USFM = new Map(BOOKS.map((b) => [b.usfm, b]));
const BY_ORDER = [...BOOKS].sort((a, b) => a.canonical_order - b.canonical_order);

export const bookByID = (id: string): CanonBook | undefined =>
  BY_ID.get(id) || BY_USFM.get(id.toUpperCase()) || BY_ID.get(resolveBook(id) || "");

export const sectionName = (id: string): string => {
  for (const t of TESTAMENTS) for (const s of t.sections) if (s.id === id) return s.name;
  return id;
};

/** Mirrors NormalizeBookKey in server/internal/bible: lowercase, drop
 *  punctuation, collapse spacing. A client that normalises differently from
 *  the server resolves references differently, which is a silent bug. */
export function normalizeBookKey(input: string): string {
  let out = "";
  let lastSpace = true;
  for (const ch of input.trim().toLowerCase()) {
    if ((ch >= "a" && ch <= "z") || (ch >= "0" && ch <= "9")) {
      out += ch;
      lastSpace = false;
    } else if (ch === " " || ch === "\t") {
      if (!lastSpace) {
        out += " ";
        lastSpace = true;
      }
    }
    /* "." "-" "'" and every other mark are dropped, as on the server */
  }
  return out.trim();
}

/** Resolve any spelling — "John", "jn", "JHN", "1 Cor", "1Pet.2.24" — to a
 *  canonical OSIS book ID, using the same alias table the server publishes. */
export function resolveBook(name: string): string | null {
  let raw = name.trim();
  if (!raw) return null;
  const dot = raw.indexOf(".");
  if (dot > 0 && /^\d+$/.test(raw.slice(dot + 1).trim().split(".")[0] || "")) raw = raw.slice(0, dot);
  return CANON.aliases[normalizeBookKey(raw)] ?? null;
}

export type ParsedReference = {
  bookID: string;
  book: CanonBook;
  chapter: number;
  startVerse?: number;
  endVerse?: number;
};

const REFERENCE = /^(.+?)\s+(\d+)(?::(\d+)(?:\s*[-–—]\s*(\d+))?)?$/i;
const CANONICAL_ID = /^([1-3]?[A-Za-z]{2,6})\.(\d+)(?:\.(\d+)(?:-(\d+))?)?$/;

/** Parse "John 3:16", "Jn 3:16-18", "Psalm 23", "JHN.3.16", "1 Cor 13:4-7".
 *  Returns null for anything the canon cannot hold — a chapter past the end of
 *  a book, a verse past the reference distribution — so the reader can say so
 *  before spending a request on it. */
export function parseReference(input: string): ParsedReference | null {
  let text = (input || "").trim();
  if (!text) return null;
  const canonical = CANONICAL_ID.exec(text);
  if (canonical) {
    text = canonical[3]
      ? `${canonical[1]} ${canonical[2]}:${canonical[3]}${canonical[4] ? `-${canonical[4]}` : ""}`
      : `${canonical[1]} ${canonical[2]}`;
  }
  const match = REFERENCE.exec(text);
  if (!match) {
    // A bare book name opens its first chapter.
    const bare = resolveBook(text);
    if (!bare) return null;
    return { bookID: bare, book: BY_ID.get(bare)!, chapter: 1 };
  }
  const bookID = resolveBook(match[1]);
  if (!bookID) return null;
  const book = BY_ID.get(bookID)!;
  const chapter = Number(match[2]);
  if (!chapter || chapter < 1 || chapter > book.chapter_count) return null;
  const parsed: ParsedReference = { bookID, book, chapter };
  if (!match[3]) return parsed;
  const start = Number(match[3]);
  const end = match[4] ? Number(match[4]) : start;
  const bound = book.chapters[chapter - 1];
  if (!start || start < 1 || end < start || start > bound || end > bound) return null;
  parsed.startVerse = start;
  parsed.endVerse = end;
  return parsed;
}

/** "John 3:16", "John 3:16-18", "John 3" — the single place the app spells a
 *  reference, so a share card, a bookmark and a heading always agree. */
export function displayRef(bookID: string, chapter: number, start?: number, end?: number): string {
  const book = BY_ID.get(bookID);
  const name = book ? book.name : bookID;
  if (!start) return `${name} ${chapter}`;
  if (!end || end === start) return `${name} ${chapter}:${start}`;
  return `${name} ${chapter}:${start}-${end}`;
}

/** The provider-independent verse identity, e.g. JHN.3.16. User data keys off
 *  this and never off a provider's row ID. */
export const canonicalVerseID = (bookID: string, chapter: number, verse: number): string =>
  `${BY_ID.get(bookID)?.usfm ?? bookID.toUpperCase()}.${chapter}.${verse}`;

export function parseCanonicalVerseID(id: string): { bookID: string; chapter: number; verse: number } | null {
  const m = /^([1-3A-Z]{3})\.(\d+)\.(\d+)$/.exec(id.toUpperCase());
  if (!m) return null;
  const book = BY_USFM.get(m[1]);
  if (!book) return null;
  return { bookID: book.id, chapter: Number(m[2]), verse: Number(m[3]) };
}

/** Continuous reading: the chapter after this one, crossing into the next
 *  book at the end of the last chapter, and stopping at Revelation 22. */
export function nextChapter(bookID: string, chapter: number): { bookID: string; chapter: number } | null {
  const book = BY_ID.get(bookID);
  if (!book) return null;
  if (chapter < book.chapter_count) return { bookID, chapter: chapter + 1 };
  const following = BY_ORDER[book.canonical_order]; // canonical_order is 1-based
  return following ? { bookID: following.id, chapter: 1 } : null;
}

export function previousChapter(bookID: string, chapter: number): { bookID: string; chapter: number } | null {
  const book = BY_ID.get(bookID);
  if (!book) return null;
  if (chapter > 1) return { bookID, chapter: chapter - 1 };
  const preceding = BY_ORDER[book.canonical_order - 2];
  return preceding ? { bookID: preceding.id, chapter: preceding.chapter_count } : null;
}

