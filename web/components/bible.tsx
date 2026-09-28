"use client";

/* ============================================================================
   iCONFESS web — Bible home, and the pieces every Bible route shares.

   The structure (books, chapters, sections, verse bounds) is local and always
   available: it comes from lib/canon.json, generated from the Go canon. The
   text is not — it is fetched from the iCONFESS API, which is the only place
   translation rights are evaluated. So this page can always draw the whole
   Bible, and says plainly when the words behind it cannot be served.
   ========================================================================= */

import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { Icon } from "./ui";
import { track } from "@/lib/store";
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
  parseCanonicalVerseID,
  parseReference,
  readerHref,
  readingStats,
  study,
  useBible,
  type CanonBook,
  type Translation,
  type VerseOfDay,
} from "@/lib/bible";

export const CATALOG_UNAVAILABLE =
  "The approved Scripture catalogue is temporarily unreachable. Verse text is served by the iCONFESS API and is not bundled with this website.";
const VERSE_UNAVAILABLE =
  "Verse text is temporarily unavailable from the approved Scripture service. Nothing is substituted for it.";

/* ------------------------------------------------------------- catalogue */

export type CatalogStatus = "loading" | "ready" | "empty" | "unreachable";
export type Catalog = {
  translations: Translation[] | null;
  current: Translation | null;
  currentID: string;
  setTranslation: (id: string) => void;
  retry: () => void;
  status: CatalogStatus;
  /** Kept as a convenience for older Bible subpages. */
  error: boolean;
  empty: boolean;
  loading: boolean;
};

/** Load only the translations the API has approved. A successful empty list is
 *  different from an unreachable API; the former is an editorial/rights state,
 *  while the latter is a network/service failure. Transient failures get a
 *  short backoff and unresolved states are checked again when the reader
 *  returns to the tab. */
export function useFocusRetry(enabled: boolean, retry: () => void, cooldownMs = 1500) {
  const retryRef = useRef(retry);

  useEffect(() => {
    retryRef.current = retry;
  }, [retry]);

  useEffect(() => {
    if (!enabled) return;
    let lastRetry = 0;
    const recover = () => {
      if (document.visibilityState === "hidden" || Date.now() - lastRetry < cooldownMs) return;
      lastRetry = Date.now();
      retryRef.current();
    };
    window.addEventListener("focus", recover);
    document.addEventListener("visibilitychange", recover);
    return () => {
      window.removeEventListener("focus", recover);
      document.removeEventListener("visibilitychange", recover);
    };
  }, [enabled, cooldownMs]);
}

export function useCatalog(preferred?: string): Catalog {
  const { prefs } = useBible();
  const [translations, setTranslations] = useState<Translation[] | null>(null);
  const [status, setStatus] = useState<CatalogStatus>("loading");
  const [chosen, setChosen] = useState("");
  const [refreshKey, setRefreshKey] = useState(0);
  const retryCount = useRef(0);
  const retryTimer = useRef<number | null>(null);

  const retry = useCallback(() => {
    retryCount.current = 0;
    if (retryTimer.current !== null) window.clearTimeout(retryTimer.current);
    retryTimer.current = null;
    setRefreshKey((key) => key + 1);
  }, []);

  useEffect(() => {
    let alive = true;
    const controller = new AbortController();
    if (retryTimer.current !== null) window.clearTimeout(retryTimer.current);
    retryTimer.current = null;
    setStatus((current) => current === "ready" ? current : "loading");

    bible
      .translations(undefined, undefined, controller.signal)
      .then((data) => {
        if (!alive) return;
        const list = Array.isArray(data.translations) ? data.translations : [];
        setTranslations(list);
        setStatus(list.length ? "ready" : "empty");
        retryCount.current = 0;
      })
      .catch((err) => {
        if (!alive || (err as Error)?.name === "AbortError") return;
        setTranslations([]);
        setStatus("unreachable");
        const retryIndex = retryCount.current;
        const backoff = [1800, 5000, 15000][retryIndex];
        if (backoff !== undefined) {
          retryCount.current += 1;
          retryTimer.current = window.setTimeout(() => setRefreshKey((key) => key + 1), backoff);
        }
      });

    return () => {
      alive = false;
      controller.abort();
      if (retryTimer.current !== null) window.clearTimeout(retryTimer.current);
      retryTimer.current = null;
    };
  }, [refreshKey]);

  useFocusRetry(status === "empty" || status === "unreachable", retry);

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

  return {
    translations,
    current,
    currentID: chosen,
    setTranslation,
    retry,
    status,
    error: status === "unreachable",
    empty: status === "empty",
    loading: status === "loading",
  };
}

export function CatalogNotice({ catalog }: { catalog: Catalog }) {
  if (catalog.status !== "empty" && catalog.status !== "unreachable") return null;
  const message = catalog.status === "empty"
    ? "No approved translation is available right now. Verse text is not bundled with this website, so a passage can only appear after an edition is approved and served by the API."
    : `${CATALOG_UNAVAILABLE} We’ll keep checking when you return.`;
  return (
    <div className="bible-note bible-catalog-notice" role="status">
      <p>{message}</p>
      <button className="btn btn-ghost btn-sm" type="button" onClick={catalog.retry}>Check again</button>
    </div>
  );
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
  const emptyLabel = catalog.status === "unreachable"
    ? "Catalogue unavailable"
    : catalog.status === "empty"
      ? "No approved translation"
      : "Loading translations…";
  return (
    <>
      <label className="bt-label" htmlFor={id}>{label}</label>
      <select
        id={id}
        value={catalog.currentID}
        disabled={!catalog.translations || catalog.translations.length === 0}
        onChange={(e) => catalog.setTranslation(e.target.value)}
      >
        {(catalog.translations || []).length === 0 && <option value="">{emptyLabel}</option>}
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
export function Unavailable({ error, children, onRetry }: { error: unknown; children?: React.ReactNode; onRetry?: () => void }) {
  const apiError = error instanceof BibleUnavailable ? error : null;
  const notFound = !!apiError?.notFound;
  const transient = !apiError || apiError.status === 0 || apiError.status >= 500 || [408, 425, 429].includes(apiError.status);
  const message = notFound
    ? "That passage is not available in this translation. Try another approved translation, or another reference."
    : apiError && !transient
      ? "This translation cannot serve that passage. Choose another approved edition or check the reference."
      : `${VERSE_UNAVAILABLE} We’ll try again when you return.`;
  return (
    <div className="bible-note" role="status">
      <p>{message}</p>
      {transient && onRetry && <button className="btn btn-ghost btn-sm" type="button" onClick={onRetry}>Try again</button>}
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
  const [structureError, setStructureError] = useState(false);
  const [structureRetry, setStructureRetry] = useState(0);
  const [votd, setVotd] = useState<VerseOfDay | null>(null);
  const [votdError, setVotdError] = useState(false);
  const [votdRetry, setVotdRetry] = useState(0);
  const [random, setRandom] = useState<{ reference: string; text: string; topic?: string } | null>(null);
  const [randomError, setRandomError] = useState<unknown>(null);
  const randomRequest = useRef<AbortController | null>(null);

  const retryStructure = useCallback(() => setStructureRetry((value) => value + 1), []);

  /* Which books this translation carries — the structure endpoint answers it
     in one request and the reader greys out the rest. */
  useEffect(() => {
    if (!catalog.currentID) {
      setAvailable(null);
      setStructureError(false);
      return;
    }
    const controller = new AbortController();
    setAvailable(null);
    setStructureError(false);
    bible
      .structure(catalog.currentID, controller.signal)
      .then((data) => {
        if (controller.signal.aborted) return;
        setAvailable(new Set(data.translation?.available_book_ids || BOOKS.map((b) => b.id)));
      })
      .catch((error) => {
        if (!controller.signal.aborted && (error as Error)?.name !== "AbortError") setStructureError(true);
      });
    return () => controller.abort();
  }, [catalog.currentID, structureRetry]);

  useFocusRetry(structureError, retryStructure);

  useEffect(() => {
    if (!catalog.currentID) {
      setVotd(null);
      setVotdError(false);
      return;
    }
    const controller = new AbortController();
    setVotd(null);
    setVotdError(false);
    bible
      .verseOfDay(catalog.currentID, controller.signal)
      .then((verse) => {
        if (!controller.signal.aborted) {
          setVotd(verse);
          setVotdError(false);
        }
      })
      .catch((error) => {
        if (!controller.signal.aborted && (error as Error)?.name !== "AbortError") setVotdError(true);
      });
    return () => controller.abort();
  }, [catalog.currentID, votdRetry]);

  useFocusRetry(votdError, () => setVotdRetry((value) => value + 1));

  const showRandom = useCallback(() => {
    if (!catalog.currentID) return;
    randomRequest.current?.abort();
    const controller = new AbortController();
    randomRequest.current = controller;
    setRandom(null);
    setRandomError(null);
    track("bible_random_requested", { translation: catalog.currentID });
    bible
      .random(catalog.currentID, controller.signal)
      .then((data) => {
        if (controller.signal.aborted || randomRequest.current !== controller) return;
        setRandom({
          reference: data.reference,
          text: (data.passage?.verses || []).map((verse) => verse.text).join(" "),
          topic: data.topic?.name,
        });
      })
      .catch((error) => {
        if (!controller.signal.aborted && randomRequest.current === controller && (error as Error)?.name !== "AbortError") {
          setRandomError(error);
        }
      })
      .finally(() => {
        if (randomRequest.current === controller) randomRequest.current = null;
      });
  }, [catalog.currentID]);
  const randomErrorTransient = !!randomError && (
    !(randomError instanceof BibleUnavailable) ||
    randomError.status === 0 ||
    randomError.status >= 500 ||
    [408, 425, 429].includes(randomError.status)
  );
  useFocusRetry(randomErrorTransient, showRandom);

  useEffect(() => {
    randomRequest.current?.abort();
    randomRequest.current = null;
    setRandom(null);
    setRandomError(null);
    return () => {
      randomRequest.current?.abort();
      randomRequest.current = null;
    };
  }, [catalog.currentID]);

  useEffect(() => {
    track("bible_opened", { translation: catalog.currentID || "none" });
  }, [catalog.currentID]);

  /* The API answers with the canonical verse ID (JHN.3.16); the reader
     spells it the way a person reads it. */
  const votdParsed = votd?.reference ? parseCanonicalVerseID(votd.reference) : null;
  const votdReference = votdParsed
    ? displayRef(votdParsed.bookID, votdParsed.chapter, votdParsed.verse)
    : votd?.reference || "";

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
            <Link href="/bible/books">Books</Link>
            <Link href="/bible/translations">Translations</Link>
            <Link href="/bible/topics">Topics</Link>
            <Link href="/bible/verse-of-the-day">Verse of the day</Link>
            <Link href="/bible/compare">Compare</Link>
            <Link href="/bible/plans">Reading plans</Link>
            <Link href="/bible/highlights">Highlights</Link>
            <Link href="/bible/notes">Notes</Link>
            <Link href="/bible/settings">Reader settings</Link>
          </div>
        </div>

        <aside className="bible-translation-card" aria-label="Translation">
          <TranslationSelect catalog={catalog} />
          {catalog.current ? (
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
              {catalog.loading ? "Loading the approved catalogue…" : catalog.empty ? "No approved translation is available." : "The catalogue is unavailable. Try again shortly."}
            </p>
          )}
        </aside>
      </section>

      <div className="container-wide bible-status-wrap"><CatalogNotice catalog={catalog} /></div>

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
          {votd?.verse ? (
            <>
              <blockquote>
                <sup>{votd.verse.number}</sup>
                {votd.verse.text}
              </blockquote>
              <p className="small">
                {votdReference} · {votd.translation?.abbreviation || catalog.current?.abbreviation}
              </p>
              {votdParsed && (
                <Link
                  className="textlink"
                  href={readerHref(catalog.currentID, votdParsed.bookID, votdParsed.chapter, votdParsed.verse)}
                >
                  Read it in context <Icon n="arrow" s={12} />
                </Link>
              )}
            </>
          ) : (
            <>
              <p className="small">
                {catalog.status === "empty"
                  ? "No approved translation is available, so there is no verse to display yet. Text is not bundled with this site."
                  : catalog.status === "unreachable" || votdError
                    ? "The reviewed verse is temporarily unavailable from the API. Nothing is substituted; we will check again when you return."
                    : "Loading today's reviewed verse…"}
              </p>
              {votdError && catalog.currentID && <button className="btn btn-ghost btn-sm" style={{ marginTop: 12 }} onClick={() => setVotdRetry((value) => value + 1)}>Try again</button>}
            </>
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

      <section className="container-wide bible-random">
        <div>
          <span className="bc-eyebrow">Somewhere to start</span>
          {random ? (
            <>
              <p>{random.text}</p>
              <p className="small">
                {random.reference}
                {random.topic ? ` · ${random.topic}` : ""}
              </p>
            </>
          ) : randomError ? (
            <p className="small" role="status">
              {randomError instanceof BibleUnavailable && randomError.notFound
                ? "No reviewed passage is available from the citation pool yet. Nothing is substituted."
                : "The approved Scripture service is temporarily unavailable. Nothing is substituted; we’ll check again when you return."}
            </p>
          ) : (
            <p className="small">
              A passage drawn from the ones the reviewed corpus stands on — never an arbitrary verse.
            </p>
          )}
        </div>
        <button
          className="btn btn-ghost btn-sm"
          disabled={!catalog.currentID || (randomError instanceof BibleUnavailable && randomError.notFound)}
          onClick={showRandom}
        >
          {randomError && randomErrorTransient ? "Try again" : random ? "Show another passage" : "Show me a passage"}
        </button>
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
        {structureError && (
          <div className="bible-note bible-catalog-notice" role="status">
            <p>Translation coverage details are temporarily unavailable. The canon remains browsable, but book availability cannot be confirmed yet.</p>
            <button className="btn btn-ghost btn-sm" type="button" onClick={retryStructure}>Try again</button>
          </div>
        )}
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
