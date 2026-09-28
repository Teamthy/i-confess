"use client";

/* ============================================================================
   iCONFESS web — Bible home, and the pieces every Bible route shares.

   The structure (books, chapters, sections, verse bounds) is local and always
   available: it comes from lib/canon.json, generated from the Go canon. The
   text is not — it is fetched from the iCONFESS API, which is the only place
   translation rights are evaluated. So this page can always draw the whole
   Bible, and says plainly when the words behind it cannot be served.
   ========================================================================= */

import React, { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { Icon } from "./ui";
import {
  BOOKS,
  CANON,
  TESTAMENTS,
  BibleUnavailable,
  bible,
  bookByID,
  chapterComplete,
  citedChapters,
  confessionsForChapter,
  displayRef,
  parseReference,
  readerHref,
  readingStats,
  study,
  useBible,
  type APIPassage,
  type CanonBook,
  type Translation,
} from "@/lib/bible";

export const CATALOG_UNAVAILABLE =
  "The Scripture catalogue is served rights-gated from the iCONFESS API and is not reachable from this deployment. The structure below is the canon itself; no verse text is bundled with the website.";

/* ------------------------------------------------------------- catalogue */

export type Catalog = {
  translations: Translation[] | null;
  current: Translation | null;
  currentID: string;
  setTranslation: (id: string) => void;
  error: boolean;
  loading: boolean;
};

/** Loads the approved translations once and remembers the reader's choice.
 *  Preference order: the reader's saved translation, then the one the page
 *  asked for, then an English public-domain edition, then whatever the server
 *  approved first. The server decides what is in the list; this only chooses
 *  within it. */
export function useCatalog(preferred?: string): Catalog {
  const { prefs } = useBible();
  const [translations, setTranslations] = useState<Translation[] | null>(null);
  const [error, setError] = useState(false);
  const [chosen, setChosen] = useState("");

  useEffect(() => {
    const controller = new AbortController();
    bible
      .translations(undefined, undefined, controller.signal)
      .then((data) => {
        const list = data.translations || [];
        setTranslations(list);
        setError(list.length === 0);
      })
      .catch((err) => {
        if ((err as Error)?.name === "AbortError") return;
        setTranslations([]);
        setError(true);
      });
    return () => controller.abort();
  }, []);

  useEffect(() => {
    if (!translations || translations.length === 0) return;
    const wanted = [preferred, prefs.translation].filter(Boolean) as string[];
    const match =
      translations.find((t) => wanted.some((w) => w.toLowerCase() === t.id.toLowerCase())) ||
      translations.find((t) => wanted.some((w) => w.toLowerCase() === (t.abbreviation || "").toLowerCase())) ||
      translations.find((t) => /kjv|web|asv/i.test(t.abbreviation || t.id)) ||
      translations[0];
    setChosen(match.id);
  }, [translations, preferred, prefs.translation]);

  const setTranslation = useCallback((id: string) => {
    setChosen(id);
    study.setPrefs({ translation: id });
  }, []);

  const current = useMemo(
    () => (translations || []).find((t) => t.id === chosen) || null,
    [translations, chosen]
  );

  return { translations, current, currentID: chosen, setTranslation, error, loading: translations === null };
}

export function TranslationSelect({
  catalog,
  id = "bible-translation",
  label = "Translation",
}: {
  catalog: Catalog;
  id?: string;
  label?: string;
}) {
  return (
    <>
      <label className="bt-label" htmlFor={id}>
        {label}
      </label>
      <select
        id={id}
        value={catalog.currentID}
        disabled={!catalog.translations || catalog.translations.length === 0}
        onChange={(e) => catalog.setTranslation(e.target.value)}
      >
        {(catalog.translations || []).length === 0 && <option value="">No approved translation</option>}
        {(catalog.translations || []).map((t) => (
          <option key={t.id} value={t.id}>
            {t.abbreviation || t.id} · {t.language_name || t.language_code || "—"}
          </option>
        ))}
      </select>
    </>
  );
}

export function RightsChips({ t }: { t: Translation }) {
  const chips: string[] = [];
  if (t.coverage) chips.push(t.coverage === "new_testament" ? "New Testament" : "Full Bible");
  chips.push(t.public_domain ? "Public domain" : t.license || "Licensed");
  if (t.audio_allowed) chips.push("Audio");
  if (t.offline_allowed) chips.push("Offline");
  return (
    <div className="bt-chips">
      {chips.map((c) => (
        <span className="rights-chip" key={c}>
          {c}
        </span>
      ))}
    </div>
  );
}

/** One honest sentence per failure, never a status code. */
export function Unavailable({ error, children }: { error: unknown; children?: React.ReactNode }) {
  const notFound = error instanceof BibleUnavailable && error.notFound;
  return (
    <div className="bible-note" role="status">
      {notFound
        ? "That passage is not available in this translation. Try another translation, or another reference."
        : CATALOG_UNAVAILABLE}
      {children}
    </div>
  );
}

/* --------------------------------------------------------- reference bar */

export function ReferenceSearch({
  translation,
  placeholder = "Search Scripture or type a reference — John 3:16, Psalm 23, 1 Cor 13:4-7",
  autoFocus,
}: {
  translation?: string;
  placeholder?: string;
  autoFocus?: boolean;
}) {
  const [value, setValue] = useState("");
  const parsed = useMemo(() => parseReference(value), [value]);

  return (
    <form
      className="bible-search-bar"
      role="search"
      onSubmit={(e) => {
        e.preventDefault();
        const query = value.trim();
        if (!query) return;
        study.rememberSearch(query);
        if (parsed) {
          window.location.href = readerHref(translation || "-", parsed.bookID, parsed.chapter, parsed.startVerse);
          return;
        }
        window.location.href = `/bible/search?q=${encodeURIComponent(query)}${
          translation ? `&translation=${encodeURIComponent(translation)}` : ""
        }`;
      }}
    >
      <span className="bsb-icon">
        <Icon n="search" s={16} />
      </span>
      <input
        aria-label="Search Scripture or enter a reference"
        value={value}
        autoFocus={autoFocus}
        placeholder={placeholder}
        onChange={(e) => setValue(e.target.value)}
      />
      {parsed && (
        <span className="bsb-hint" aria-live="polite">
          Open {displayRef(parsed.bookID, parsed.chapter, parsed.startVerse, parsed.endVerse)}
        </span>
      )}
      <button className="btn btn-primary btn-sm" type="submit">
        Search
      </button>
    </form>
  );
}

/* ------------------------------------------------------ structure browser */

/** The canon as navigation: two testaments, ten sections, 66 books, every
 *  chapter. Rendered from local data, so it is complete before — and without —
 *  any request. `available` marks the books the chosen translation carries. */
export function StructureBrowser({
  translation,
  available,
  onPick,
  compact,
}: {
  translation: string;
  available?: Set<string> | null;
  onPick?: (book: CanonBook, chapter: number) => void;
  compact?: boolean;
}) {
  const state = useBible();
  const [testament, setTestament] = useState<"old" | "new">("old");
  const [openBook, setOpenBook] = useState<string | null>(null);
  const [query, setQuery] = useState("");

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return null;
    return BOOKS.filter(
      (b) =>
        b.name.toLowerCase().includes(needle) ||
        b.abbreviation.toLowerCase().includes(needle) ||
        b.id.toLowerCase().includes(needle) ||
        (b.aliases || []).some((a) => a.startsWith(needle))
    );
  }, [query]);

  const testamentData = TESTAMENTS.find((t) => t.id === testament)!;
  const isAvailable = (book: CanonBook) => !available || available.has(book.id);

  const bookButton = (book: CanonBook) => {
    const open = openBook === book.id;
    const unavailable = !isAvailable(book);
    return (
      <div className={"bible-book" + (open ? " open" : "") + (unavailable ? " unavailable" : "")} key={book.id}>
        <button
          className="bb-head"
          aria-expanded={open}
          onClick={() => setOpenBook(open ? null : book.id)}
          title={unavailable ? `${book.name} is not in this translation` : `${book.name} — ${book.chapter_count} chapters`}
        >
          <span className="bb-name">{book.name}</span>
          <span className="bb-meta">{book.chapter_count}</span>
        </button>
        {open && (
          <div className="bible-chapters" role="group" aria-label={`${book.name} chapters`}>
            {Array.from({ length: book.chapter_count }, (_, i) => i + 1).map((n) =>
              onPick ? (
                <button
                  key={n}
                  className={chapterComplete(state, book.id, n) ? "read" : undefined}
                  onClick={() => onPick(book, n)}
                >
                  {n}
                </button>
              ) : (
                <Link
                  key={n}
                  href={readerHref(translation || "-", book.id, n)}
                  className={chapterComplete(state, book.id, n) ? "read" : undefined}
                >
                  {n}
                </Link>
              )
            )}
          </div>
        )}
      </div>
    );
  };

  return (
    <div className={"bible-structure" + (compact ? " compact" : "")}>
      <div className="bs-controls">
        <div className="bs-tabs" role="tablist" aria-label="Testament">
          {TESTAMENTS.map((t) => (
            <button
              key={t.id}
              role="tab"
              aria-selected={testament === t.id && !filtered}
              onClick={() => {
                setTestament(t.id);
                setQuery("");
              }}
            >
              {t.name}
              <span>{t.book_count}</span>
            </button>
          ))}
        </div>
        <input
          className="bs-filter"
          value={query}
          placeholder="Find a book"
          aria-label="Find a book"
          onChange={(e) => setQuery(e.target.value)}
        />
      </div>

      {filtered ? (
        <div className="bs-section">
          <h4>
            {filtered.length} book{filtered.length === 1 ? "" : "s"}
          </h4>
          <div className="bs-books">{filtered.map(bookButton)}</div>
        </div>
      ) : (
        testamentData.sections.map((section) => (
          <div className="bs-section" key={section.id}>
            <h4>
              {section.name}
              <span>
                {section.book_count} books · {section.chapter_count} chapters
              </span>
            </h4>
            <div className="bs-books">
              {section.book_ids.map((bookID) => {
                const book = bookByID(bookID);
                return book ? bookButton(book) : null;
              })}
            </div>
          </div>
        ))
      )}
    </div>
  );
}

/* -------------------------------------------------------------- the home */

export function BiblePage() {
  const catalog = useCatalog();
  const state = useBible();
  const stats = readingStats(state);

  const [available, setAvailable] = useState<Set<string> | null>(null);
  const [votd, setVotd] = useState<APIPassage | null>(null);
  const [votdRef, setVotdRef] = useState<string>("");
  const [votdError, setVotdError] = useState(false);

  /* Which books this translation carries — the structure endpoint answers it
     in one request and the reader greys out the rest. */
  useEffect(() => {
    if (!catalog.currentID) return;
    const controller = new AbortController();
    bible
      .structure(catalog.currentID, controller.signal)
      .then((data) => setAvailable(new Set(data.translation?.available_book_ids || BOOKS.map((b) => b.id))))
      .catch(() => setAvailable(null));
    return () => controller.abort();
  }, [catalog.currentID]);

  useEffect(() => {
    if (!catalog.currentID) return;
    const controller = new AbortController();
    setVotdError(false);
    bible
      .verseOfDay(catalog.currentID, controller.signal)
      .then((data) => {
        setVotd(data.passage || null);
        setVotdRef(data.reference || data.passage?.reference || "");
      })
      .catch(() => setVotdError(true));
    return () => controller.abort();
  }, [catalog.currentID]);

  const resume = state.history[0];
  const resumeBook = resume ? bookByID(resume.bookID) : undefined;

  return (
    <>
      <section className="bible-hero container-wide">
        <div>
          <span className="scripture-eyebrow">Scripture</span>
          <h1 className="h-display" style={{ marginTop: 22, maxWidth: "12ch" }}>
            The verses behind the words.
          </h1>
          <p className="lede" style={{ marginTop: 22, maxWidth: "48ch" }}>
            The whole canon — {CANON.book_count} books, {CANON.chapter_count.toLocaleString()} chapters — open to read,
            mark and keep. Every confession on iCONFESS stands on Scripture you can follow back to its source.
          </p>
          <div style={{ marginTop: 26 }}>
            <ReferenceSearch translation={catalog.currentID} />
          </div>
          <div className="bible-quick">
            <Link href="/bible/search">Search</Link>
            <Link href="/bible/translations">Translations</Link>
            <Link href="/bible/plans">Reading plans</Link>
            <Link href="/bible/highlights">Highlights</Link>
            <Link href="/bible/notes">Notes</Link>
            <Link href="/bible/settings">Reader settings</Link>
          </div>
        </div>

        <aside className="bible-translation-card" aria-label="Translation">
          <TranslationSelect catalog={catalog} />
          {catalog.error ? (
            <p className="bt-meta" style={{ marginTop: 14 }}>
              {CATALOG_UNAVAILABLE}
            </p>
          ) : catalog.current ? (
            <>
              <div className="bt-name">{catalog.current.name}</div>
              <RightsChips t={catalog.current} />
              <div className="bt-meta">
                {(catalog.current.verse_count || 0).toLocaleString()} verses ·{" "}
                {catalog.current.book_count || available?.size || CANON.book_count} books
                {catalog.current.source_url && (
                  <>
                    {" · "}
                    <a href={catalog.current.source_url} target="_blank" rel="noopener noreferrer">
                      source
                    </a>
                  </>
                )}
              </div>
              <Link className="btn btn-primary btn-sm" style={{ marginTop: 16 }} href="/bible/translations">
                All translations <Icon n="arrow" s={12} />
              </Link>
            </>
          ) : (
            <p className="bt-meta" style={{ marginTop: 14 }}>
              Loading the catalogue…
            </p>
          )}
        </aside>
      </section>

      <section className="container-wide bible-home-grid">
        <div className="bible-card continue">
          <span className="bc-eyebrow">Continue reading</span>
          {resume && resumeBook ? (
            <>
              <h3>
                {resumeBook.name} {resume.chapter}
              </h3>
              <p className="small">
                Last opened {new Date(resume.at).toLocaleDateString(undefined, { month: "long", day: "numeric" })} ·{" "}
                {resume.translation || catalog.currentID}
              </p>
              <Link
                className="btn btn-primary btn-sm"
                href={readerHref(resume.translation || catalog.currentID, resume.bookID, resume.chapter)}
              >
                Keep reading <Icon n="arrow" s={12} />
              </Link>
            </>
          ) : (
            <>
              <h3>Genesis 1</h3>
              <p className="small">Start at the beginning, or open any book below.</p>
              <Link className="btn btn-primary btn-sm" href={readerHref(catalog.currentID, "Gen", 1)}>
                Begin <Icon n="arrow" s={12} />
              </Link>
            </>
          )}
        </div>

        <div className="bible-card votd">
          <span className="bc-eyebrow">Verse of the day</span>
          {votd && votd.verses?.length ? (
            <>
              <blockquote>
                {votd.verses.map((v) => (
                  <span key={v.id || v.number}>
                    <sup>{v.number}</sup>
                    {v.text}{" "}
                  </span>
                ))}
              </blockquote>
              <p className="small">
                {votd.reference || votdRef} · {votd.translation?.abbreviation || catalog.current?.abbreviation}
              </p>
            </>
          ) : (
            <p className="small">
              {votdError || catalog.error
                ? "The reviewed verse of the day is served by the API and is not reachable from this deployment. Nothing is substituted for it."
                : "Loading today's reviewed verse…"}
            </p>
          )}
        </div>

        <div className="bible-card stats">
          <span className="bc-eyebrow">Your reading</span>
          <div className="bc-stats">
            <div>
              <b>{stats.chapters}</b>
              <span>chapters read</span>
            </div>
            <div>
              <b>{stats.booksComplete}</b>
              <span>books complete</span>
            </div>
            <div>
              <b>{stats.highlights}</b>
              <span>highlights</span>
            </div>
            <div>
              <b>{stats.notes}</b>
              <span>notes</span>
            </div>
          </div>
          <div className="bc-progress" aria-hidden="true">
            <span style={{ width: `${Math.min(100, stats.percent)}%` }} />
          </div>
          <p className="small">
            {stats.percent}% of {CANON.chapter_count.toLocaleString()} chapters. Private to this browser until you sign
            in.
          </p>
        </div>
      </section>

      <section className="container-wide" style={{ paddingBottom: 24 }}>
        <div className="bible-section-head">
          <h2 className="h2">The whole Bible</h2>
          <p className="small">
            {CANON.book_count} books · {CANON.chapter_count.toLocaleString()} chapters ·{" "}
            {CANON.verse_count.toLocaleString()} verses in the reference distribution. Grey books are not in the
            selected translation.
          </p>
        </div>
        <StructureBrowser translation={catalog.currentID} available={available} />
      </section>

      {citedChapters.length > 0 && (
        <section className="container-wide" style={{ paddingBottom: 96 }}>
          <div className="bible-section-head">
            <h2 className="h2">Chapters iCONFESS stands on</h2>
            <p className="small">
              The passages the reviewed confession corpus cites most. Open one and every confession written from it is
              listed beside the text.
            </p>
          </div>
          <div className="bible-cited">
            {citedChapters.slice(0, 12).map(({ bookID, chapter, count }) => {
              const book = bookByID(bookID);
              if (!book) return null;
              const confessions = confessionsForChapter(bookID, chapter);
              return (
                <Link className="bcited" key={`${bookID}.${chapter}`} href={readerHref(catalog.currentID, bookID, chapter)}>
                  <b>
                    {book.name} {chapter}
                  </b>
                  <span>
                    {count} confession{count === 1 ? "" : "s"}
                  </span>
                  <em>{confessions[0]?.confession.title}</em>
                </Link>
              );
            })}
          </div>
        </section>
      )}
    </>
  );
}
