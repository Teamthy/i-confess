"use client";

import React, { useCallback, useEffect, useState } from "react";
import { Icon } from "./ui";
import { useApp } from "@/lib/store";

/*
 * Bible reader (original design, reference screenshot 2026-09-25).
 *
 * Scripture text is never bundled with the website: translations, books,
 * chapters and verses all arrive from the Go API over the same-origin proxy
 * (/api/v1/bible/*), which is the only place translation rights are evaluated
 * (approved translations only, per-translation rights flags). When the catalog
 * is unreachable the page says so and shows nothing, rather than falling back
 * to hardcoded verse text.
 */

type Translation = {
  id: string;
  name: string;
  abbreviation: string;
  language_name: string;
  public_domain: boolean;
  coverage?: string;
  license?: string;
  copyright?: string;
  verse_count?: number;
  book_count?: number;
  source_url?: string;
  attribution_required?: boolean;
  attribution_text?: string;
};

type Book = {
  id: string;
  name: string;
  testament: string;
  canonical_order: number;
  chapter_count: number;
};

type Verse = { id: string; number: number; text: string };

type Chapter = { translation: Translation; book: Book; chapter: number; verses: Verse[] };

const UNAVAILABLE =
  "The Scripture catalog is served rights-gated from the iCONFESS API and is not reachable from this deployment. No verse text is bundled with the website.";

async function getJSON<T>(url: string): Promise<T> {
  const res = await fetch(url, { cache: "no-store" });
  if (!res.ok) throw new Error(`${url} -> ${res.status}`);
  return (await res.json()) as T;
}

export function BiblePage() {
  const { user } = useApp();

  const [translations, setTranslations] = useState<Translation[] | null>(null);
  const [catalogError, setCatalogError] = useState(false);
  const [translationID, setTranslationID] = useState("");

  const [books, setBooks] = useState<Book[] | null>(null);
  const [bookID, setBookID] = useState("");

  const [chapterCount, setChapterCount] = useState(0);
  const [chapter, setChapter] = useState(1);

  const [chapterData, setChapterData] = useState<Chapter | null>(null);
  const [readerError, setReaderError] = useState(false);
  const [loadingChapter, setLoadingChapter] = useState(false);

  /* Approved translations, once. Prefer the abbreviation the reference shows
     if the server approves it; rights still come from the server response. */
  useEffect(() => {
    let live = true;
    getJSON<{ translations: Translation[] }>("/api/v1/bible/translations")
      .then((d) => {
        if (!live) return;
        const list = d.translations || [];
        setTranslations(list);
        if (list.length === 0) {
          setCatalogError(true);
          return;
        }
        const preferred = list.find((t) => /kjv/i.test(t.abbreviation || t.id)) || list[0];
        setTranslationID(preferred.id);
      })
      .catch(() => live && setCatalogError(true));
    return () => {
      live = false;
    };
  }, []);

  /* Books for the selected translation. */
  useEffect(() => {
    if (!translationID) return;
    let live = true;
    setBooks(null);
    setBookID("");
    getJSON<{ books: Book[] }>(`/api/v1/bible/books?translation=${encodeURIComponent(translationID)}`)
      .then((d) => {
        if (!live) return;
        const sorted = [...(d.books || [])].sort((a, b) => a.canonical_order - b.canonical_order);
        setBooks(sorted);
        if (sorted.length > 0) setBookID(sorted[0].id);
      })
      .catch(() => live && setCatalogError(true));
    return () => {
      live = false;
    };
  }, [translationID]);

  /* Chapter count for the selected book. */
  useEffect(() => {
    if (!translationID || !bookID) return;
    let live = true;
    getJSON<{ book: Book; chapters: number[] }>(
      `/api/v1/bible/books/${encodeURIComponent(bookID)}/chapters?translation=${encodeURIComponent(translationID)}`
    )
      .then((d) => {
        if (!live) return;
        setChapterCount(d.book?.chapter_count || (d.chapters || []).length);
        setChapter(1);
      })
      .catch(() => live && setCatalogError(true));
    return () => {
      live = false;
    };
  }, [translationID, bookID]);

  /* The chapter itself. */
  const loadChapter = useCallback(
    (n: number) => {
      if (!translationID || !bookID) return;
      setLoadingChapter(true);
      setReaderError(false);
      getJSON<Chapter>(`/api/v1/bible/${encodeURIComponent(translationID)}/${encodeURIComponent(bookID)}/${n}`)
        .then((d) => {
          setChapterData(d);
          setChapter(n);
        })
        .catch(() => setReaderError(true))
        .finally(() => setLoadingChapter(false));
    },
    [translationID, bookID]
  );

  useEffect(() => {
    if (translationID && bookID) loadChapter(1);
  }, [translationID, bookID, loadChapter]);

  const translation = (translations || []).find((t) => t.id === translationID) || null;
  const book = (books || []).find((b) => b.id === bookID) || null;

  return (
    <>
      <section className="bible-hero container-wide">
        <div>
          <span className="scripture-eyebrow">Scripture</span>
          <h1 className="h-display" style={{ marginTop: 22, maxWidth: "12ch" }}>
            The verses behind the words.
          </h1>
          <p className="lede" style={{ marginTop: 22, maxWidth: "46ch" }}>
            Every confession on iCONFESS stands on Scripture you can open, read and keep. Pick a
            translation, read the passage, mark the verse that stays with you, and follow it back
            to the confession it belongs to.
          </p>
        </div>

        <aside className="bible-translation-card" aria-label="Translation">
          <label className="bt-label" htmlFor="bible-translation">
            Translation
          </label>
          <select
            id="bible-translation"
            value={translationID}
            disabled={!translations || translations.length === 0}
            onChange={(e) => setTranslationID(e.target.value)}
          >
            {(translations || []).length === 0 && <option value="">No approved translation</option>}
            {(translations || []).map((t) => (
              <option key={t.id} value={t.id}>
                {t.abbreviation || t.id} · {t.language_name || "English"}
              </option>
            ))}
          </select>

          {catalogError ? (
            <p className="bt-meta" style={{ marginTop: 14 }}>
              {UNAVAILABLE}
            </p>
          ) : translation ? (
            <>
              <div className="bt-name">{translation.name}</div>
              <div className="bt-chips">
                {translation.coverage && <span className="rights-chip">{translation.coverage}</span>}
                <span className="rights-chip">
                  {translation.public_domain ? "Public domain" : translation.license || "Licensed"}
                </span>
              </div>
              <div className="bt-meta">
                {(translation.verse_count || 0).toLocaleString()} verses ·{" "}
                {translation.book_count || (books || []).length} books
                {translation.source_url && (
                  <>
                    {" · "}
                    <a href={translation.source_url} target="_blank" rel="noopener noreferrer">
                      source
                    </a>
                  </>
                )}
              </div>
            </>
          ) : (
            <p className="bt-meta" style={{ marginTop: 14 }}>
              Loading translation…
            </p>
          )}
        </aside>
      </section>

      <section className="bible-reader container-wide">
        <aside className="bible-rail" aria-label="Book and chapter">
          <label className="bt-label" htmlFor="bible-book">
            Book
          </label>
          <select
            id="bible-book"
            value={bookID}
            disabled={!books || books.length === 0}
            onChange={(e) => setBookID(e.target.value)}
          >
            {(books || []).length === 0 && <option value="">—</option>}
            {(books || []).map((b) => (
              <option key={b.id} value={b.id}>
                {b.name}
              </option>
            ))}
          </select>

          <div className="bible-chapters" role="group" aria-label="Chapters">
            {Array.from({ length: chapterCount }, (_, i) => i + 1).map((n) => (
              <button
                key={n}
                aria-current={n === chapter ? "page" : undefined}
                onClick={() => loadChapter(n)}
              >
                {n}
              </button>
            ))}
          </div>
        </aside>

        <div className="bible-main">
          {!user && <div className="bible-signin">Sign in to keep highlights and bookmarks on your account.</div>}

          {readerError ? (
            <div className="bible-note">
              {catalogError || !translation
                ? UNAVAILABLE
                : "This passage is not available from the Scripture service right now. The website never ships verse text of its own."}
            </div>
          ) : !chapterData ? (
            <div className="bible-note">{loadingChapter ? "Loading…" : "Choose a book to begin reading."}</div>
          ) : (
            <>
              <div className="bible-chapter-head">
                <h2>
                  {chapterData.book?.name || book?.name} {chapterData.chapter}
                </h2>
                {chapter < chapterCount && (
                  <button className="bible-next" onClick={() => loadChapter(chapter + 1)}>
                    {chapterData.book?.name || book?.name} {chapter + 1} <Icon n="arrow" s={14} />
                  </button>
                )}
              </div>
              <ol className="bible-verses">
                {chapterData.verses.map((v) => (
                  <li key={v.id || v.number}>
                    <sup>{v.number}</sup>
                    {v.text}
                  </li>
                ))}
              </ol>
              {(chapterData.translation?.attribution_required || chapterData.translation?.copyright) && (
                <p className="bt-meta" style={{ marginTop: 24 }}>
                  {chapterData.translation.attribution_text || chapterData.translation.copyright}
                </p>
              )}
            </>
          )}
        </div>
      </section>
    </>
  );
}
