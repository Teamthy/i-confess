"use client";

/* ============================================================================
   iCONFESS web — the Bible reader.

   Reading comfort first: one column, generous measure, verse numbers that a
   screen reader announces as verses, and controls that stay out of the way.
   Everything else hangs off a selected verse — highlight, bookmark, note,
   copy, share, listen, compare, cross references, and the two actions that
   make this a Bible inside iCONFESS rather than beside it: reflect on the
   verse, and confess from it.

   Rights are the server's decision, and the reader obeys them rather than
   re-deciding: copy, share and audio are only offered when the translation
   carries that grant, and attribution is shown whenever it is required.
   ========================================================================= */

import React, { useCallback, useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Icon } from "./ui";
import { useToast } from "@/lib/ui";
import { track } from "@/lib/store";
import { CatalogNotice, Unavailable, useCatalog, useFocusRetry, StructureBrowser } from "./bible";
import {
  BOOKS,
  HIGHLIGHT_COLORS,
  bible,
  bookByID,
  bookmarkFor,
  canonicalVerseID,
  chapterComplete,
  confessionsForVerse,
  confessionsForChapter,
  displayRef,
  highlightFor,
  nextChapter,
  noteFor,
  parseReference,
  previousChapter,
  readerHref,
  study,
  useBible,
  type APIChapter,
  type APIPassage,
  type HighlightColor,
} from "@/lib/bible";

type Props = {
  initialTranslation?: string;
  initialBook?: string;
  initialChapter?: number;
  initialVerse?: number;
  initialReference?: string;
};

export function BibleReader({
  initialTranslation,
  initialBook,
  initialChapter,
  initialVerse,
  initialReference,
}: Props) {
  const router = useRouter();
  const toast = useToast();
  const state = useBible();
  const { prefs } = state;

  const parsedInitial = initialReference ? parseReference(initialReference) : null;
  const catalog = useCatalog(initialTranslation && initialTranslation !== "-" ? initialTranslation : undefined);

  const [bookID, setBookID] = useState(parsedInitial?.bookID || initialBook || "John");
  const [chapter, setChapter] = useState(parsedInitial?.chapter || initialChapter || 1);
  const [chapterData, setChapterData] = useState<APIChapter | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [loading, setLoading] = useState(false);
  const chapterRequest = useRef<AbortController | null>(null);
  const [selected, setSelected] = useState<number | null>(parsedInitial?.startVerse ?? initialVerse ?? null);
  const [noteDraft, setNoteDraft] = useState("");
  const [showNav, setShowNav] = useState(false);
  const [crossRefs, setCrossRefs] = useState<string[] | null>(null);
  const [comparison, setComparison] = useState<APIPassage[] | null>(null);
  const [comparing, setComparing] = useState(false);
  const crossRefRequest = useRef<AbortController | null>(null);
  const comparisonRequest = useRef<AbortController | null>(null);

  const book = bookByID(bookID) || BOOKS[42];
  const mode = prefs.mode;
  const translation = catalog.current;
  const rtl = (translation?.direction || "ltr").toLowerCase() === "rtl";

  /* --------------------------------------------------------- chapter load */

  const load = useCallback(
    (targetBook: string, targetChapter: number) => {
      if (!catalog.currentID) return;
      chapterRequest.current?.abort();
      const controller = new AbortController();
      chapterRequest.current = controller;
      const translationID = catalog.currentID;
      setLoading(true);
      setError(null);
      setChapterData(null);
      bible
        .chapter(translationID, targetBook, targetChapter, controller.signal)
        .then((data) => {
          if (controller.signal.aborted || chapterRequest.current !== controller) return;
          setChapterData(data);
          study.recordRead(translationID, targetBook, targetChapter);
          track("chapter_opened", { translation: translationID, book: targetBook, chapter: targetChapter });
          // Prefetch the next chapter so continuous reading does not wait.
          const following = nextChapter(targetBook, targetChapter);
          if (following) void bible.chapter(translationID, following.bookID, following.chapter).catch(() => undefined);
        })
        .catch((err) => {
          if (controller.signal.aborted || chapterRequest.current !== controller || (err as Error)?.name === "AbortError") return;
          setChapterData(null);
          setError(err);
        })
        .finally(() => {
          if (chapterRequest.current === controller) {
            chapterRequest.current = null;
            setLoading(false);
          }
        });
      return () => {
        controller.abort();
        if (chapterRequest.current === controller) chapterRequest.current = null;
      };
    },
    [catalog.currentID]
  );

  useEffect(() => load(bookID, chapter), [load, bookID, chapter]);
  useEffect(() => () => {
    chapterRequest.current?.abort();
    crossRefRequest.current?.abort();
    comparisonRequest.current?.abort();
  }, []);
  useEffect(() => {
    crossRefRequest.current?.abort();
    comparisonRequest.current?.abort();
    crossRefRequest.current = null;
    comparisonRequest.current = null;
    setCrossRefs(null);
    setComparison(null);
    setComparing(false);
  }, [catalog.currentID]);

  const retryChapter = useCallback(() => {
    if (!catalog.currentID) {
      catalog.retry();
      return;
    }
    load(bookID, chapter);
  }, [catalog.currentID, catalog.retry, load, bookID, chapter]);

  useFocusRetry(!!error, () => load(bookID, chapter));

  /* Keep the URL honest: a deep link always names what is on screen. */
  useEffect(() => {
    if (!catalog.currentID) return;
    const href = readerHref(catalog.currentID, bookID, chapter);
    if (typeof window !== "undefined" && window.location.pathname !== href.split("#")[0]) {
      router.replace(href, { scroll: false });
    }
  }, [catalog.currentID, bookID, chapter, router]);

  const go = useCallback((targetBook: string, targetChapter: number, verse?: number) => {
    chapterRequest.current?.abort();
    crossRefRequest.current?.abort();
    comparisonRequest.current?.abort();
    crossRefRequest.current = null;
    comparisonRequest.current = null;
    setBookID(targetBook);
    setChapter(targetChapter);
    setSelected(verse ?? null);
    setCrossRefs(null);
    setComparison(null);
    setComparing(false);
    if (typeof window !== "undefined") window.scrollTo({ top: 0, behavior: "smooth" });
  }, []);

  const previous = previousChapter(bookID, chapter);
  const following = nextChapter(bookID, chapter);

  /* A deep link that names a verse should land on that verse, not at the top
     of the chapter it is in. */
  useEffect(() => {
    if (!chapterData || !selected) return;
    const target = document.getElementById(`v${selected}`);
    if (target) target.scrollIntoView({ block: "center", behavior: "smooth" });
    // only when the chapter arrives, not on every selection change
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [chapterData]);

  /* Keyboard: the reader is usable without a pointer. */
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement | null;
      if (target && /^(INPUT|TEXTAREA|SELECT)$/.test(target.tagName)) return;
      if (e.key === "ArrowRight" && following) go(following.bookID, following.chapter);
      if (e.key === "ArrowLeft" && previous) go(previous.bookID, previous.chapter);
      if (e.key === "Escape") setSelected(null);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [following, previous, go]);

  /* Touch: swipe between chapters, the way a reader expects on a phone. */
  const touch = useRef<{ x: number; y: number } | null>(null);
  const onTouchStart = (e: React.TouchEvent) => {
    touch.current = { x: e.changedTouches[0].clientX, y: e.changedTouches[0].clientY };
  };
  const onTouchEnd = (e: React.TouchEvent) => {
    if (!touch.current) return;
    const dx = e.changedTouches[0].clientX - touch.current.x;
    const dy = e.changedTouches[0].clientY - touch.current.y;
    touch.current = null;
    if (Math.abs(dx) < 70 || Math.abs(dy) > 60) return;
    const forward = rtl ? dx > 0 : dx < 0;
    if (forward && following) go(following.bookID, following.chapter);
    if (!forward && previous) go(previous.bookID, previous.chapter);
  };

  /* ----------------------------------------------------------- selection */

  const selectedVerse = useMemo(
    () => (selected ? chapterData?.verses.find((v) => v.number === selected) || null : null),
    [selected, chapterData]
  );
  const selectedID = selected ? canonicalVerseID(bookID, chapter, selected) : "";
  const selectedRef = selected ? displayRef(bookID, chapter, selected) : "";
  const existingNote = selectedID ? noteFor(state, selectedID) : undefined;

  useEffect(() => {
    setNoteDraft(existingNote?.text || "");
  }, [existingNote?.id, selectedID]);

  const copyAllowed = !!translation?.copy_allowed;
  const shareAllowed = !!translation?.share_allowed;
  const audioAllowed = !!translation?.audio_allowed;

  const copyVerse = async () => {
    if (!selectedVerse || !copyAllowed) return;
    const attribution = translation?.attribution_required
      ? ` — ${translation.attribution_text || translation.name}`
      : ` — ${translation?.abbreviation || ""}`;
    try {
      await navigator.clipboard.writeText(`“${selectedVerse.text}” ${selectedRef}${attribution}`);
      toast("Verse copied");
    } catch {
      toast("Your browser blocked the clipboard");
    }
  };

  const shareVerse = async () => {
    if (!selected || !shareAllowed) return;
    const url = `${window.location.origin}${readerHref(catalog.currentID, bookID, chapter, selected)}`;
    try {
      if (navigator.share) await navigator.share({ title: selectedRef, text: selectedVerse?.text, url });
      else {
        await navigator.clipboard.writeText(url);
        toast("Link copied");
      }
    } catch {
      /* the reader dismissed the share sheet */
    }
  };

  const speak = () => {
    if (!selectedVerse || !audioAllowed) return;
    try {
      const utterance = new SpeechSynthesisUtterance(`${selectedRef}. ${selectedVerse.text}`);
      utterance.rate = prefs.audioSpeed;
      window.speechSynthesis.cancel();
      window.speechSynthesis.speak(utterance);
      toast("Reading aloud on this device");
    } catch {
      toast("This browser cannot read aloud");
    }
  };

  const loadCrossRefs = () => {
    if (!selected) return;
    crossRefRequest.current?.abort();
    const controller = new AbortController();
    crossRefRequest.current = controller;
    setCrossRefs(null);
    bible
      .crossReferences(`${book.usfm}.${chapter}.${selected}`, controller.signal)
      .then((data) => {
        if (!controller.signal.aborted && crossRefRequest.current === controller) setCrossRefs(data.references || []);
      })
      .catch((failure) => {
        if (!controller.signal.aborted && crossRefRequest.current === controller && (failure as Error)?.name !== "AbortError") setCrossRefs([]);
      })
      .finally(() => {
        if (crossRefRequest.current === controller) crossRefRequest.current = null;
      });
  };

  /* Comparison needs two to four named translations — the handler refuses
     anything else. The reader's own edition leads, then whichever approved
     editions come next in the catalogue. */
  const comparisonSet = useMemo(() => {
    const ids = [catalog.currentID, ...(catalog.translations || []).map((t) => t.id)];
    return [...new Set(ids.filter(Boolean))].slice(0, 4);
  }, [catalog.currentID, catalog.translations]);

  const loadComparison = () => {
    comparisonRequest.current?.abort();
    comparisonRequest.current = null;
    if (!selected || comparisonSet.length < 2) {
      setComparing(false);
      setComparison([]);
      return;
    }
    const controller = new AbortController();
    comparisonRequest.current = controller;
    setComparing(true);
    setComparison(null);
    track("bible_compare_opened", { translations: comparisonSet.length });
    bible
      .compare(displayRef(bookID, chapter, selected), comparisonSet, controller.signal)
      .then((data) => {
        if (!controller.signal.aborted && comparisonRequest.current === controller) setComparison(data.passages || []);
      })
      .catch((failure) => {
        if (!controller.signal.aborted && comparisonRequest.current === controller && (failure as Error)?.name !== "AbortError") setComparison([]);
      })
      .finally(() => {
        if (comparisonRequest.current === controller) {
          comparisonRequest.current = null;
          setComparing(false);
        }
      });
  };

  /* ---------------------------------------------------------------- view */

  const verseStyle: React.CSSProperties = {
    fontSize: prefs.fontSize,
    lineHeight: prefs.lineHeight,
    fontFamily: prefs.fontFamily === "serif" ? 'Georgia, "Iowan Old Style", "Times New Roman", serif' : "var(--font)",
  };

  const chapterBody = (
    <ol
      className={"bible-verses" + (prefs.layout === "paragraph" ? " paragraph" : "") + (rtl ? " rtl" : "")}
      style={verseStyle}
      dir={rtl ? "rtl" : "ltr"}
      onTouchStart={onTouchStart}
      onTouchEnd={onTouchEnd}
    >
      {(chapterData?.verses || []).map((v) => {
        const verseID = canonicalVerseID(bookID, chapter, v.number);
        const highlight = highlightFor(state, verseID);
        const marked = bookmarkFor(state, verseID);
        const hasNote = noteFor(state, verseID);
        const confessions = confessionsForVerse(bookID, chapter, v.number);
        return (
          <li
            key={v.id || v.number}
            id={`v${v.number}`}
            className={
              "bverse" +
              (selected === v.number ? " selected" : "") +
              (highlight ? ` hl-${highlight.color}` : "") +
              (confessions.length ? " cited" : "")
            }
          >
            <button
              className="bverse-hit"
              aria-label={`Verse ${v.number}${highlight ? `, highlighted ${highlight.color}` : ""}${
                marked ? ", bookmarked" : ""
              }${hasNote ? ", has a note" : ""}`}
              aria-pressed={selected === v.number}
              onClick={() => {
                const next = selected === v.number ? null : v.number;
                crossRefRequest.current?.abort();
                comparisonRequest.current?.abort();
                crossRefRequest.current = null;
                comparisonRequest.current = null;
                setCrossRefs(null);
                setComparison(null);
                setComparing(false);
                setSelected(next);
                if (next) track("verse_opened", { book: bookID, chapter, verse: next });
              }}
            >
              {prefs.showVerseNumbers && <sup aria-hidden="true">{v.number}</sup>}
              <span>{v.text}</span>
              {marked && (
                <span className="bverse-mark" aria-hidden="true">
                  <Icon n="book" s={12} />
                </span>
              )}
              {hasNote && (
                <span className="bverse-mark" aria-hidden="true">
                  <Icon n="edit" s={12} />
                </span>
              )}
            </button>
          </li>
        );
      })}
    </ol>
  );

  const actions = selected && (
    <div className="bible-actions" role="dialog" aria-label={`Actions for ${selectedRef}`}>
      <div className="ba-head">
        <b>{selectedRef}</b>
        <button className="ba-close" aria-label="Close verse actions" onClick={() => setSelected(null)}>
          <Icon n="x" s={14} />
        </button>
      </div>

      <div className="ba-colors" role="group" aria-label="Highlight">
        {HIGHLIGHT_COLORS.map((c) => {
          const active = highlightFor(state, selectedID)?.color === c.id;
          return (
            <button
              key={c.id}
              className={"ba-swatch" + (active ? " active" : "")}
              style={{ background: c.swatch }}
              aria-label={`Highlight ${c.name}`}
              aria-pressed={active}
              onClick={() => {
                study.highlight(selectedID, selectedRef, c.id as HighlightColor);
                track("highlight_created", { color: c.id });
              }}
            />
          );
        })}
      </div>

      <div className="ba-row">
        <button
          onClick={() => {
            study.bookmark(selectedID, selectedRef);
            track("bookmark_created", {});
          }}
        >
          <Icon n="book" s={14} /> {bookmarkFor(state, selectedID) ? "Bookmarked" : "Bookmark"}
        </button>
        <button onClick={copyVerse} disabled={!copyAllowed} title={copyAllowed ? "" : "This translation does not grant copying"}>
          <Icon n="check" s={14} /> Copy
        </button>
        <button onClick={shareVerse} disabled={!shareAllowed} title={shareAllowed ? "" : "This translation does not grant sharing"}>
          <Icon n="share" s={14} /> Share
        </button>
        <button onClick={speak} disabled={!audioAllowed} title={audioAllowed ? "" : "This translation does not grant audio"}>
          <Icon n="play" s={14} /> Listen
        </button>
        <button onClick={loadCrossRefs}>
          <Icon n="compass" s={14} /> Cross references
        </button>
        <button onClick={loadComparison}>
          <Icon n="grid" s={14} /> Compare
        </button>
      </div>

      <div className="ba-note">
        <label htmlFor="bible-note">Note</label>
        <textarea
          id="bible-note"
          rows={3}
          value={noteDraft}
          placeholder="Private to you."
          onChange={(e) => setNoteDraft(e.target.value)}
        />
        <div className="ba-note-foot">
          <button
            className="btn btn-primary btn-sm"
            onClick={() => {
              if (!noteDraft.trim()) return;
              study.note(selectedID, selectedRef, noteDraft.trim());
              // The event records that a note happened, never its contents.
              track("note_created", {});
              toast("Note saved");
            }}
          >
            Save note
          </button>
          {existingNote && (
            <button
              className="btn btn-ghost btn-sm"
              onClick={() => {
                study.removeNote(existingNote.id);
                setNoteDraft("");
              }}
            >
              Delete
            </button>
          )}
        </div>
      </div>

      <div className="ba-collections">
        <label htmlFor="bible-collection">Add to collection</label>
        <select
          id="bible-collection"
          value=""
          onChange={(e) => {
            const value = e.target.value;
            if (!value) return;
            if (value === "__new") {
              const name = window.prompt("Name this collection", "Promises");
              if (!name?.trim()) return;
              const created = study.createCollection(name.trim());
              study.addToCollection(created, selectedID, selectedRef);
              toast(`Added to ${name.trim()}`);
              return;
            }
            study.addToCollection(value, selectedID, selectedRef);
            toast("Added to collection");
          }}
        >
          <option value="">Choose…</option>
          {state.collections.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
          <option value="__new">New collection…</option>
        </select>
      </div>

      <div className="ba-row secondary">
        <Link className="ba-link" href={`/app/confessions?ref=${encodeURIComponent(selectedRef)}`}>
          <Icon n="mic" s={14} /> Confess from this verse
        </Link>
        <Link className="ba-link" href={`/app/journal?ref=${encodeURIComponent(selectedRef)}`}>
          <Icon n="edit" s={14} /> Reflect
        </Link>
        <Link className="ba-link" href={`/bible/search?q=${encodeURIComponent(selectedRef)}`}>
          <Icon n="search" s={14} /> Search related
        </Link>
        <Link className="ba-link" href={`/bible/compare?reference=${encodeURIComponent(selectedRef)}`}>
          <Icon n="grid" s={14} /> Open in parallel
        </Link>
      </div>

      {crossRefs && (
        <div className="ba-list">
          <h5>Cross references</h5>
          {crossRefs.length === 0 ? (
            <p className="small">No reviewed cross references for this verse yet.</p>
          ) : (
            crossRefs.map((reference) => {
              const parsed = parseReference(reference);
              return parsed ? (
                <button key={reference} onClick={() => go(parsed.bookID, parsed.chapter, parsed.startVerse)}>
                  {displayRef(parsed.bookID, parsed.chapter, parsed.startVerse, parsed.endVerse)}
                </button>
              ) : (
                <span key={reference}>{reference}</span>
              );
            })
          )}
        </div>
      )}

      {(comparison || comparing) && (
        <div className="ba-list">
          <h5>Other translations</h5>
          {comparing && <p className="small">Comparing…</p>}
          {comparison?.length === 0 && !comparing && (
            <p className="small">No other approved translation carries this verse.</p>
          )}
          {(comparison || []).map((passage) => (
            <div className="ba-compare" key={passage.translation?.id || passage.reference}>
              <b>{passage.translation?.abbreviation || passage.translation?.name}</b>
              <p>{passage.verses?.map((v) => v.text).join(" ")}</p>
            </div>
          ))}
        </div>
      )}

      {confessionsForVerse(bookID, chapter, selected).length > 0 && (
        <div className="ba-list">
          <h5>Confessions from this verse</h5>
          {confessionsForVerse(bookID, chapter, selected).map(({ confession }) => (
            <Link key={confession.slug} href={`/confessions/${confession.slug}`}>
              {confession.title}
            </Link>
          ))}
        </div>
      )}
    </div>
  );

  const chapterConfessions = confessionsForChapter(bookID, chapter);

  return (
    <div className={`bible-shell mode-${mode}`}>
      <div className="bible-toolbar container-wide">
        <div className="btb-left">
          <button className="btb-book" onClick={() => setShowNav((v) => !v)} aria-expanded={showNav}>
            <b>{book.name}</b> <span>{chapter}</span> <Icon n="grid" s={13} />
          </button>
          <select
            aria-label="Translation"
            value={catalog.currentID}
            onChange={(e) => {
              catalog.setTranslation(e.target.value);
              track("translation_selected", { translation: e.target.value });
            }}
            disabled={!catalog.translations?.length}
          >
            {(catalog.translations || []).length === 0 && <option value="">No approved translation</option>}
            {(catalog.translations || []).map((t) => (
              <option key={t.id} value={t.id}>
                {t.abbreviation || t.id}
              </option>
            ))}
          </select>
        </div>
        <div className="btb-right">
          <div className="btb-modes" role="group" aria-label="Reader mode">
            {(["standard", "focus", "study", "audio"] as const).map((m) => (
              <button key={m} aria-pressed={mode === m} onClick={() => study.setPrefs({ mode: m })}>
                {m[0].toUpperCase() + m.slice(1)}
              </button>
            ))}
          </div>
          <button
            className="btb-icon"
            aria-label="Smaller text"
            onClick={() => study.setPrefs({ fontSize: Math.max(15, prefs.fontSize - 1) })}
          >
            <Icon n="minus" s={14} />
          </button>
          <button
            className="btb-icon"
            aria-label="Larger text"
            onClick={() => study.setPrefs({ fontSize: Math.min(30, prefs.fontSize + 1) })}
          >
            <Icon n="plus" s={14} />
          </button>
          <Link className="btb-icon" href="/bible/settings" aria-label="Reader settings">
            <Icon n="gear" s={15} />
          </Link>
        </div>
      </div>

      {showNav && (
        <div className="bible-nav-drop container-wide">
          <StructureBrowser
            translation={catalog.currentID}
            compact
            onPick={(pickedBook, pickedChapter) => {
              go(pickedBook.id, pickedChapter);
              setShowNav(false);
            }}
          />
        </div>
      )}

      <div className={"bible-reader container-wide" + (mode === "study" ? " with-panel" : "")}>
        <div className="bible-main">
          <div className="bible-chapter-head">
            <h1>
              {book.name} {chapter}
            </h1>
            <div className="bch-actions">
              <button
                className={"bch-read" + (chapterComplete(state, bookID, chapter) ? " done" : "")}
                onClick={() => study.completeChapter(bookID, chapter)}
              >
                <Icon n="check" s={13} /> {chapterComplete(state, bookID, chapter) ? "Read" : "Mark read"}
              </button>
            </div>
          </div>

          {error ? (
            <Unavailable error={error} onRetry={retryChapter} />
          ) : loading && !chapterData ? (
            <div className="bible-skeleton" aria-busy="true">
              {Array.from({ length: 8 }, (_, i) => (
                <span key={i} />
              ))}
            </div>
          ) : chapterData ? (
            chapterBody
          ) : catalog.status === "empty" || catalog.status === "unreachable" ? (
            <CatalogNotice catalog={catalog} />
          ) : catalog.loading ? (
            <div className="bible-note">Loading approved translations…</div>
          ) : (
            <div className="bible-note">Choose a translation to begin reading.</div>
          )}

          {(translation?.attribution_required || translation?.copyright) && chapterData && (
            <p className="bt-meta" style={{ marginTop: 26 }}>
              {translation.attribution_text || translation.copyright}
            </p>
          )}

          <nav className="bible-chapter-nav" aria-label="Chapter">
            {previous ? (
              <button onClick={() => go(previous.bookID, previous.chapter)}>
                <Icon n="arrowL" s={14} /> {bookByID(previous.bookID)?.name} {previous.chapter}
              </button>
            ) : (
              <span />
            )}
            {following && (
              <button className="primary" onClick={() => go(following.bookID, following.chapter)}>
                {bookByID(following.bookID)?.name} {following.chapter} <Icon n="arrow" s={14} />
              </button>
            )}
          </nav>
        </div>

        {mode === "study" && (
          <aside className="bible-panel" aria-label="Study">
            <h4>Study</h4>
            {selected ? (
              <p className="small">
                {selectedRef} is selected. Its cross references, comparisons and notes are in the verse panel.
              </p>
            ) : (
              <p className="small">Select a verse to see its cross references, comparisons and notes.</p>
            )}

            <h5>Confessions in this chapter</h5>
            {chapterConfessions.length === 0 ? (
              <p className="small">No confession in the reviewed corpus cites this chapter yet.</p>
            ) : (
              <ul className="bp-list">
                {chapterConfessions.slice(0, 8).map(({ confession, reference }) => (
                  <li key={confession.slug}>
                    <Link href={`/confessions/${confession.slug}`}>{confession.title}</Link>
                    <span>{reference}</span>
                  </li>
                ))}
              </ul>
            )}

            <h5>Your marks here</h5>
            {(() => {
              const marks = state.highlights
                .filter((h) => h.verseID.startsWith(`${book.usfm}.${chapter}.`))
                .slice(0, 8);
              return marks.length === 0 ? (
                <p className="small">Nothing highlighted in this chapter yet.</p>
              ) : (
                <ul className="bp-list">
                  {marks.map((h) => (
                    <li key={h.id}>
                      <span className={`hl-dot hl-${h.color}`} aria-hidden="true" />
                      {h.reference}
                    </li>
                  ))}
                </ul>
              );
            })()}

            <h5>This book</h5>
            <p className="small">
              {book.name} · {book.chapter_count} chapters · {book.verse_count.toLocaleString()} verses in the
              reference distribution · {book.testament === "old" ? "Old" : "New"} Testament
            </p>
          </aside>
        )}
      </div>

      {actions}
    </div>
  );
}
