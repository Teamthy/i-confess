"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useAuth } from "@/lib/auth-context";

type Translation = {
  id: string;
  name: string;
  abbreviation: string;
  language_code: string;
  language_name: string;
  direction: "ltr" | "rtl";
  status: string;
  attribution_required: boolean;
  attribution_text?: string;
  license?: string;
  license_url?: string;
  offline_allowed: boolean;
  audio_allowed: boolean;
  copy_allowed: boolean;
  share_allowed: boolean;
};
type Language = { id: string; name: string; native_name: string; direction: "ltr" | "rtl" };
type Book = { id: string; name: string; testament: string; canonical_order: number; chapter_count: number };
type Verse = { id: string; number: number; text: string; book_id: string; chapter: number };
type Chapter = { translation: Translation; book: Book; chapter: number; verses: Verse[] };
type Passage = { reference: string; translation: Translation; verses: Verse[] };
type SearchResponse = { kind: "reference" | "text"; results: (Passage | { translation: Translation; book: Book; chapter: number; verse: number; text: string })[] };

async function getJSON<T>(url: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(url, { signal, headers: { accept: "application/json" } });
  if (!response.ok) {
    const body = await response.json().catch(() => ({})) as { error?: string };
    throw new Error(body.error ?? "Bible content is temporarily unavailable.");
  }
  return response.json() as Promise<T>;
}

type BibleReaderProps = { initialTranslation?: string; initialReference?: string };

export function BibleReader({ initialTranslation, initialReference }: BibleReaderProps = {}) {
  const { token } = useAuth();
  const [translations, setTranslations] = useState<Translation[]>([]);
  const [languages, setLanguages] = useState<Language[]>([]);
  const [translationID, setTranslationID] = useState("");
  const [books, setBooks] = useState<Book[]>([]);
  const [bookID, setBookID] = useState("Gen");
  const [chapterNumber, setChapterNumber] = useState(1);
  const [chapter, setChapter] = useState<Chapter | null>(null);
  const [selectedVerse, setSelectedVerse] = useState<Verse | null>(null);
  const [noteEditorOpen, setNoteEditorOpen] = useState(false);
  const [noteDraft, setNoteDraft] = useState("");
  const [savingStudyItem, setSavingStudyItem] = useState(false);
  const [pendingVerse, setPendingVerse] = useState<number | null>(null);
  const [deepLinkResolved, setDeepLinkResolved] = useState(false);
  const [query, setQuery] = useState("");
  const [searchText, setSearchText] = useState("");
  const [searchResults, setSearchResults] = useState<SearchResponse | null>(null);
  const [busy, setBusy] = useState(true);
  const [error, setError] = useState("");
  const [fontSize, setFontSize] = useState(21);
  const [readerMode, setReaderMode] = useState<"standard" | "focus">("standard");

  useEffect(() => {
    const controller = new AbortController();
    Promise.all([
      getJSON<{ translations: Translation[] }>("/api/v1/bible/translations", controller.signal),
      getJSON<{ languages: Language[] }>("/api/v1/bible/languages", controller.signal),
    ]).then(([translationData, languageData]) => {
      setTranslations(translationData.translations ?? []);
      setLanguages(languageData.languages ?? []);
      const preferred = translationData.translations?.find((item) => item.id.toLowerCase() === initialTranslation?.toLowerCase()) ?? translationData.translations?.find((item) => item.id === "web") ?? translationData.translations?.[0];
      if (preferred) setTranslationID(preferred.id);
    }).catch((reason: unknown) => {
      if (reason instanceof Error && reason.name !== "AbortError") setError(reason.message);
    }).finally(() => setBusy(false));
    return () => controller.abort();
  }, []);

  useEffect(() => {
    if (!translationID) return;
    const controller = new AbortController();
    getJSON<{ books: Book[] }>(`/api/v1/bible/books?translation=${encodeURIComponent(translationID)}`, controller.signal)
      .then((data) => {
        const nextBooks = data.books ?? [];
        setBooks(nextBooks);
        setBookID((current) => nextBooks.some((book) => book.id === current) ? current : nextBooks[0]?.id ?? "");
      })
      .catch((reason: unknown) => {
        if (reason instanceof Error && reason.name !== "AbortError") setError(reason.message);
      });
    return () => controller.abort();
  }, [translationID]);

  useEffect(() => {
    if (!translationID || !bookID) return;
    const controller = new AbortController();
    setBusy(true);
    setError("");
    getJSON<Chapter>(`/api/v1/bible/${encodeURIComponent(translationID)}/${encodeURIComponent(bookID)}/${chapterNumber}`, controller.signal)
      .then(setChapter)
      .catch((reason: unknown) => {
        if (reason instanceof Error && reason.name !== "AbortError") setError(reason.message);
      })
      .finally(() => setBusy(false));
    return () => controller.abort();
  }, [translationID, bookID, chapterNumber]);

  useEffect(() => {
    if (!initialReference || !translationID || deepLinkResolved) return;
    const controller = new AbortController();
    const params = new URLSearchParams({ reference: initialReference, translation: translationID });
    getJSON<Passage>(`/api/v1/bible/passage?${params.toString()}`, controller.signal)
      .then((passage) => {
        const verse = passage.verses[0];
        if (!verse) return;
        setBookID(verse.book_id);
        setChapterNumber(verse.chapter);
        setPendingVerse(verse.number);
        setDeepLinkResolved(true);
      })
      .catch((reason: unknown) => {
        if (reason instanceof Error && reason.name !== "AbortError") setError(reason.message);
        setDeepLinkResolved(true);
      });
    return () => controller.abort();
  }, [initialReference, translationID, deepLinkResolved]);

  useEffect(() => {
    if (!chapter || pendingVerse === null) return;
    const timer = window.setTimeout(() => document.getElementById(`verse-${pendingVerse}`)?.scrollIntoView({ behavior: "smooth", block: "center" }), 120);
    setPendingVerse(null);
    return () => window.clearTimeout(timer);
  }, [chapter, pendingVerse]);

  const selectedTranslation = useMemo(() => translations.find((item) => item.id === translationID), [translations, translationID]);
  const currentBookIndex = books.findIndex((book) => book.id === bookID);

  const changeChapter = useCallback((delta: number) => {
    const book = books[currentBookIndex];
    if (!book) return;
    if (chapterNumber + delta >= 1 && chapterNumber + delta <= book.chapter_count) {
      setChapterNumber((current) => current + delta);
      setSelectedVerse(null);
      window.scrollTo({ top: 0, behavior: "smooth" });
      return;
    }
    const nextBook = books[currentBookIndex + delta];
    if (nextBook) {
      setBookID(nextBook.id);
      setChapterNumber(delta > 0 ? 1 : nextBook.chapter_count);
      setSelectedVerse(null);
      window.scrollTo({ top: 0, behavior: "smooth" });
    }
  }, [books, chapterNumber, currentBookIndex]);

  async function runSearch(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!query.trim() || !translationID) return;
    setSearchText(query.trim());
    setSearchResults(null);
    setError("");
    try {
      const params = new URLSearchParams({ q: query.trim(), translation: translationID });
      setSearchResults(await getJSON<SearchResponse>(`/api/v1/bible/search?${params.toString()}`));
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Search is temporarily unavailable.");
    }
  }

  function openReference(book: string, chapter: number, verse?: number) {
    if (!books.some((item) => item.id === book)) return;
    setBookID(book);
    setChapterNumber(chapter);
    setSearchResults(null);
    setError("");
    if (verse) setPendingVerse(verse);
  }

  async function copyVerse(verse: Verse) {
    const sourceTranslation = chapter?.translation;
    if (!sourceTranslation?.copy_allowed) {
      setError("Copying is not enabled for this translation.");
      return;
    }
    try {
      await navigator.clipboard.writeText(`${verse.text} — ${chapter?.book.name} ${verse.chapter}:${verse.number} (${sourceTranslation.abbreviation})`);
      setError("Verse copied.");
    } catch {
      setError("Copy is unavailable in this browser. Select the verse text to copy it.");
    }
  }

  async function savePrivateStudyItem(kind: "bookmarks" | "highlights" | "notes", body: Record<string, unknown>, success: string) {
    if (!token) {
      setError("Sign in to save private study data.");
      return;
    }
    setSavingStudyItem(true);
    try {
      const response = await fetch(`/api/v1/me/bible/${kind}`, {
        method: "POST",
        headers: { "content-type": "application/json", accept: "application/json", authorization: `Bearer ${token}` },
        body: JSON.stringify(body),
      });
      if (!response.ok) {
        setError(response.status === 401 ? "Your session expired. Sign in again to save." : "That study item could not be saved. Please try again.");
        return;
      }
      setError(success);
      if (kind === "notes") {
        setNoteDraft("");
        setNoteEditorOpen(false);
      }
    } catch {
      setError("We couldn't reach the service. Your private note was not saved.");
    } finally {
      setSavingStudyItem(false);
    }
  }

  const direction = selectedTranslation?.direction ?? "ltr";

  return (
    <main className={`bible-platform ${readerMode === "focus" ? "bible-platform--focus" : ""}`} dir={direction}>
      <header className="bible-topline">
        <a className="bible-brand" href="/">iCONFESS <span>/ Bible</span></a>
        <a href="/app" className="bible-return">Back to iCONFESS <span aria-hidden="true">↗</span></a>
      </header>

      <div className="bible-layout">
        <aside className="bible-sidebar" aria-label="Bible navigation">
          <p className="bible-kicker">A quiet place to read</p>
          <h1>Scripture, at your pace.</h1>
          <p className="bible-intro">Read, search, and return to the words that matter. Your reading translation and app language can be different.</p>

          <div className="bible-controls">
            <label htmlFor="bible-translation">Translation</label>
            <select id="bible-translation" value={translationID} onChange={(event) => { setTranslationID(event.target.value); setSearchResults(null); }} disabled={translations.length === 0}>
              {translations.length === 0 && <option value="">No approved translations yet</option>}
              {translations.map((item) => <option value={item.id} key={item.id}>{item.abbreviation} · {item.name}</option>)}
            </select>
            {selectedTranslation && <p className="bible-language">{selectedTranslation.language_name}{selectedTranslation.license ? ` · ${selectedTranslation.license}` : ""}</p>}

            <label htmlFor="bible-book">Book</label>
            <select id="bible-book" value={bookID} onChange={(event) => { setBookID(event.target.value); setChapterNumber(1); setSelectedVerse(null); }} disabled={!books.length}>
              {books.map((item) => <option value={item.id} key={item.id}>{item.name}</option>)}
            </select>

            <label htmlFor="bible-chapter">Chapter</label>
            <select id="bible-chapter" value={chapterNumber} onChange={(event) => { setChapterNumber(Number(event.target.value)); setSelectedVerse(null); }} disabled={!books.length}>
              {Array.from({ length: books.find((item) => item.id === bookID)?.chapter_count ?? 0 }, (_, index) => index + 1).map((number) => <option key={number} value={number}>{number}</option>)}
            </select>
          </div>

          <form className="bible-search" onSubmit={runSearch} role="search">
            <label htmlFor="bible-search-input">Find a passage or phrase</label>
            <div className="bible-search__control">
              <input id="bible-search-input" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="John 3:16 or hope" />
              <button type="submit" aria-label="Search the Bible">⌕</button>
            </div>
            <span>Try a reference, a word, or a phrase.</span>
          </form>

          <div className="bible-sidebar__footer">
            <div className="bible-language-row"><span>Available languages</span><strong>{languages.length || "—"}</strong></div>
            <p>Only translations confirmed and enabled for iCONFESS are shown.</p>
          </div>
        </aside>

        <section className="bible-reading" aria-label="Bible reader" aria-busy={busy}>
          {searchResults ? (
            <div className="bible-results">
              <button type="button" className="bible-back" onClick={() => setSearchResults(null)}>← Back to reading</button>
              <p className="bible-kicker">Search results</p>
              <h2>{searchText}</h2>
              {searchResults.results.length === 0 ? <p className="bible-empty">No matching text was found in this translation.</p> : searchResults.results.map((item, index) => {
                if (searchResults.kind === "reference") {
                  const passage = item as Passage;
                  const first = passage.verses[0];
                  if (!first) return null;
                  return <button className="bible-search-result" key={`${first.id}-${index}`} onClick={() => openReference(first.book_id, first.chapter, first.number)}><strong>{passage.reference}</strong><span>{passage.verses.map((v) => v.text).join(" ")}</span></button>;
                }
                const result = item as { translation: Translation; book: Book; chapter: number; verse: number; text: string };
                return <button className="bible-search-result" key={`${result.book.id}.${result.chapter}.${result.verse}`} onClick={() => openReference(result.book.id, result.chapter, result.verse)}><strong>{result.book.name} {result.chapter}:{result.verse}</strong><span>{result.text}</span><small>{result.translation.abbreviation}</small></button>;
              })}
            </div>
          ) : (
            <>
              <div className="bible-reader-toolbar">
                <div>
                  <p className="bible-kicker">{chapter?.translation.abbreviation ?? selectedTranslation?.abbreviation ?? "Bible"}</p>
                  <h2>{chapter?.book.name ?? books.find((item) => item.id === bookID)?.name ?? "Choose a translation"} <span>{chapter?.chapter ?? chapterNumber}</span></h2>
                </div>
                <div className="bible-reader-tools" aria-label="Reader settings">
                  <button type="button" onClick={() => setFontSize((size) => Math.max(16, size - 1))} aria-label="Decrease text size">A−</button>
                  <button type="button" onClick={() => setFontSize((size) => Math.min(30, size + 1))} aria-label="Increase text size">A+</button>
                  <button type="button" onClick={() => setReaderMode((mode) => mode === "focus" ? "standard" : "focus")} aria-pressed={readerMode === "focus"}>{readerMode === "focus" ? "Exit focus" : "Focus"}</button>
                </div>
              </div>

              {error && <div className="bible-notice" role="status">{error}{error.includes("unavailable") && <button type="button" onClick={() => setChapterNumber((value) => value)}>Retry</button>}</div>}
              {!busy && !error && translations.length === 0 && <div className="bible-empty-state"><span aria-hidden="true">✧</span><h3>Your Bible library is being prepared</h3><p>Translations appear here after their source and usage rights have been reviewed. We won’t invent or silently substitute a translation.</p></div>}
              {busy && <div className="bible-skeleton" aria-label="Loading chapter"><i /><i /><i /><i /><i /></div>}

              {chapter && !busy && <article className="bible-chapter" aria-label={`${chapter.book.name} chapter ${chapter.chapter}`}>
                <div className="bible-chapter__meta">{chapter.translation.attribution_required && chapter.translation.attribution_text && <span>{chapter.translation.attribution_text}</span>}</div>
                {chapter.verses.map((verse) => <p id={`verse-${verse.number}`} key={verse.id} className={`bible-verse ${selectedVerse?.id === verse.id ? "bible-verse--selected" : ""}`}>
                  <button type="button" className="bible-verse__number" aria-label={`Select verse ${verse.number}`} onClick={() => setSelectedVerse((selected) => selected?.id === verse.id ? null : verse)}>{verse.number}</button>
                  <button type="button" className="bible-verse__text" onClick={() => setSelectedVerse((selected) => selected?.id === verse.id ? null : verse)}>{verse.text}</button>
                </p>)}
                {selectedVerse && <div className="bible-verse-actions" role="group" aria-label={`Actions for verse ${selectedVerse.number}`}>
                  <strong>{chapter.book.name} {chapter.chapter}:{selectedVerse.number}</strong>
                  <button type="button" onClick={() => void copyVerse(selectedVerse)} disabled={!chapter.translation.copy_allowed}>Copy</button>
                  <button type="button" onClick={async () => {
                    const shareText = `${selectedVerse.text} — ${chapter.book.name} ${chapter.chapter}:${selectedVerse.number} (${chapter.translation.abbreviation})`;
                    if (navigator.share) await navigator.share({ title: `${chapter.book.name} ${chapter.chapter}:${selectedVerse.number}`, text: shareText }).catch(() => {});
                    else await copyVerse(selectedVerse);
                  }} disabled={!chapter.translation.share_allowed}>Share</button>
                  {token ? <>
                    <button type="button" disabled={savingStudyItem} onClick={() => void savePrivateStudyItem("bookmarks", { translation_id: chapter.translation.id, book_id: selectedVerse.book_id, chapter: selectedVerse.chapter, verse: selectedVerse.number }, "Private bookmark saved.")}>Bookmark</button>
                    <button type="button" disabled={savingStudyItem} onClick={() => void savePrivateStudyItem("highlights", { translation_id: chapter.translation.id, book_id: selectedVerse.book_id, chapter: selectedVerse.chapter, verse: selectedVerse.number, color: "yellow" }, "Private highlight saved.")}>Highlight</button>
                    <button type="button" aria-expanded={noteEditorOpen} onClick={() => setNoteEditorOpen((open) => !open)}>Add note</button>
                  </> : <Link href="/login">Sign in to save private study</Link>}
                  {token && noteEditorOpen && <div className="bible-note-editor">
                    <label htmlFor="bible-private-note">Private note for {chapter.book.name} {chapter.chapter}:{selectedVerse.number}</label>
                    <textarea id="bible-private-note" value={noteDraft} onChange={(event) => setNoteDraft(event.target.value)} maxLength={20000} rows={3} />
                    <button type="button" disabled={savingStudyItem || !noteDraft.trim()} onClick={() => void savePrivateStudyItem("notes", { translation_id: chapter.translation.id, book_id: selectedVerse.book_id, chapter: selectedVerse.chapter, verse_start: selectedVerse.number, verse_end: selectedVerse.number, body: noteDraft }, "Private note saved.")}>Save note</button>
                  </div>}
                </div>}
                <nav className="bible-chapter-nav" aria-label="Chapter navigation">
                  <button type="button" onClick={() => changeChapter(-1)} disabled={currentBookIndex === 0 && chapterNumber === 1}>← Previous</button>
                  <span>{chapter.book.name} {chapter.chapter}</span>
                  <button type="button" onClick={() => changeChapter(1)} disabled={currentBookIndex === books.length - 1 && chapterNumber === (books[currentBookIndex]?.chapter_count ?? chapterNumber)}>Next →</button>
                </nav>
              </article>}
            </>
          )}
        </section>
      </div>
    </main>
  );
}
