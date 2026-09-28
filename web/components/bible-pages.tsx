"use client";

/* ============================================================================
   iCONFESS web — the Bible routes around the reader: search, the translation
   and language catalogues, the private library (highlights, bookmarks, notes,
   collections, history), reading plans and reader settings.

   Everything private is local to this browser until an account claims it, and
   says so. Everything licensed comes from the API and shows the terms it
   arrived under.
   ========================================================================= */

import React, { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { Icon } from "./ui";
import { useToast } from "@/lib/ui";
import { track } from "@/lib/store";
import { usePlayer } from "@/lib/player";
import { CATALOG_UNAVAILABLE, RightsChips, TranslationSelect, useCatalog } from "./bible";
import {
  BOOKS,
  CANON,
  HIGHLIGHT_COLORS,
  bible,
  bookByID,
  displayRef,
  parseCanonicalVerseID,
  parseReference,
  readerHref,
  readingStats,
  study,
  useBible,
  type APIPassage,
  type BiblePlan,
  type BibleTopic,
  type Language,
  type SearchHit,
  type Translation,
} from "@/lib/bible";

/* --------------------------------------------------------------- search */

export function BibleSearchPage({ initialQuery, initialTranslation }: { initialQuery?: string; initialTranslation?: string }) {
  const catalog = useCatalog(initialTranslation);
  const state = useBible();
  const [query, setQuery] = useState(initialQuery || "");
  const [book, setBook] = useState("");
  const [hits, setHits] = useState<SearchHit[] | null>(null);
  const [passage, setPassage] = useState<APIPassage | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [loading, setLoading] = useState(false);

  const parsed = useMemo(() => parseReference(query), [query]);

  const run = useCallback(
    (text: string) => {
      const q = text.trim();
      if (!q || !catalog.currentID) return;
      setLoading(true);
      setError(null);
      setHits(null);
      setPassage(null);
      study.rememberSearch(q);
      track("search_performed", { translation: catalog.currentID, scoped_to_book: !!book });
      bible
        .search(q, catalog.currentID, book || undefined, 40)
        .then((data) => {
          if (data.kind === "reference") {
            setPassage((data.results[0] as APIPassage) || null);
            return;
          }
          setHits((data.results as SearchHit[]) || []);
        })
        .catch(setError)
        .finally(() => setLoading(false));
    },
    [catalog.currentID, book]
  );

  useEffect(() => {
    if (initialQuery && catalog.currentID) run(initialQuery);
    // run once the catalogue resolves; later searches come from the form
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [catalog.currentID]);

  const highlight = (text: string) => {
    const needle = query.trim();
    if (!needle || parsed) return text;
    const index = text.toLowerCase().indexOf(needle.toLowerCase());
    if (index < 0) return text;
    return (
      <>
        {text.slice(0, index)}
        <mark>{text.slice(index, index + needle.length)}</mark>
        {text.slice(index + needle.length)}
      </>
    );
  };

  return (
    <section className="container-wide bible-page">
      <header className="bible-page-head">
        <span className="scripture-eyebrow">Search</span>
        <h1 className="h1">Find the verse.</h1>
        <p className="lede">
          Type a reference — <b>John 3:16</b>, <b>Psalm 23</b>, <b>1 Cor 13:4-7</b> — to open it, or search the words
          themselves. Search runs server-side against the translations whose rights allow indexing.
        </p>
      </header>

      <form
        className="bible-search-bar big"
        role="search"
        onSubmit={(e) => {
          e.preventDefault();
          run(query);
        }}
      >
        <span className="bsb-icon">
          <Icon n="search" s={16} />
        </span>
        <input
          value={query}
          autoFocus
          aria-label="Search Scripture"
          placeholder="love, do not be afraid, John 3:16"
          onChange={(e) => setQuery(e.target.value)}
        />
        <select aria-label="Book filter" value={book} onChange={(e) => setBook(e.target.value)}>
          <option value="">All books</option>
          {BOOKS.map((b) => (
            <option key={b.id} value={b.id}>
              {b.name}
            </option>
          ))}
        </select>
        <button className="btn btn-primary btn-sm" type="submit">
          Search
        </button>
      </form>

      <div className="bible-search-meta">
        <div className="bsm-translation">
          <TranslationSelect catalog={catalog} id="bible-search-translation" label="In translation" />
        </div>
        {parsed && (
          <Link className="bsm-jump" href={readerHref(catalog.currentID, parsed.bookID, parsed.chapter, parsed.startVerse)}>
            Open {displayRef(parsed.bookID, parsed.chapter, parsed.startVerse, parsed.endVerse)} <Icon n="arrow" s={12} />
          </Link>
        )}
        {state.searches.length > 0 && (
          <div className="bsm-recent">
            <span className="small">Recent</span>
            {state.searches.slice(0, 6).map((s) => (
              <button
                key={s}
                onClick={() => {
                  setQuery(s);
                  run(s);
                }}
              >
                {s}
              </button>
            ))}
          </div>
        )}
      </div>

      {loading && <div className="bible-note">Searching…</div>}
      {!!error && <div className="bible-note">{CATALOG_UNAVAILABLE}</div>}

      {passage && (
        <div className="bible-result-passage">
          <h3>{passage.reference}</h3>
          <p>{passage.verses?.map((v) => `${v.number}. ${v.text}`).join(" ")}</p>
          {(() => {
            const p = parseReference(passage.reference);
            return p ? (
              <Link className="textlink" href={readerHref(catalog.currentID, p.bookID, p.chapter, p.startVerse)}>
                Read it in context <Icon n="arrow" s={12} />
              </Link>
            ) : null;
          })()}
        </div>
      )}

      {hits && (
        <>
          <p className="small" style={{ margin: "18px 0" }}>
            {hits.length === 0
              ? "No verse in the indexed translations matches that phrase."
              : `${hits.length} verse${hits.length === 1 ? "" : "s"}`}
          </p>
          <div className="bible-results">
            {hits.map((hit, i) => (
              <Link
                className="bresult"
                key={`${hit.book?.id}-${hit.chapter}-${hit.verse}-${i}`}
                href={readerHref(hit.translation?.id || catalog.currentID, hit.book?.id || "", hit.chapter, hit.verse)}
              >
                <b>
                  {hit.book?.name} {hit.chapter}:{hit.verse}
                </b>
                <p>{highlight(hit.text)}</p>
                <span>{hit.translation?.abbreviation || hit.translation?.id}</span>
              </Link>
            ))}
          </div>
        </>
      )}
    </section>
  );
}

/* --------------------------------------------------------- translations */

export function BibleTranslationsPage() {
  const catalog = useCatalog();
  const [filter, setFilter] = useState("");
  const [language, setLanguage] = useState("");

  const translations = catalog.translations || [];
  const languages = useMemo(() => {
    const set = new Map<string, string>();
    for (const t of translations) if (t.language_code) set.set(t.language_code, t.language_name || t.language_code);
    return [...set.entries()].sort((a, b) => a[1].localeCompare(b[1]));
  }, [translations]);

  const shown = translations.filter((t) => {
    if (language && t.language_code !== language) return false;
    if (!filter.trim()) return true;
    const needle = filter.trim().toLowerCase();
    return (
      t.name.toLowerCase().includes(needle) ||
      (t.abbreviation || "").toLowerCase().includes(needle) ||
      (t.language_name || "").toLowerCase().includes(needle)
    );
  });

  return (
    <section className="container-wide bible-page">
      <header className="bible-page-head">
        <span className="scripture-eyebrow">Translations</span>
        <h1 className="h1">Every approved edition.</h1>
        <p className="lede">
          A translation appears here only after its rights have been reviewed one use at a time — reading, copying,
          sharing, audio, offline and indexing are separate grants, and an unreviewed edition is absent rather than
          assumed.
        </p>
      </header>

      <div className="bible-filter-row">
        <input
          value={filter}
          placeholder="Filter by name or language"
          aria-label="Filter translations"
          onChange={(e) => setFilter(e.target.value)}
        />
        <select aria-label="Language" value={language} onChange={(e) => setLanguage(e.target.value)}>
          <option value="">All languages</option>
          {languages.map(([code, name]) => (
            <option key={code} value={code}>
              {name}
            </option>
          ))}
        </select>
        <Link className="textlink" href="/bible/languages">
          Browse languages <Icon n="arrow" s={12} />
        </Link>
      </div>

      {catalog.error && <div className="bible-note">{CATALOG_UNAVAILABLE}</div>}
      {catalog.loading && <div className="bible-note">Loading the catalogue…</div>}

      <div className="bible-translations">
        {shown.map((t) => (
          <article className="btrans" key={t.id}>
            <div className="btrans-top">
              <b>{t.abbreviation || t.id}</b>
              <span>{t.language_name || t.language_code}</span>
            </div>
            <h3>{t.name}</h3>
            {t.description && <p className="small">{t.description}</p>}
            <RightsChips t={t} />
            <div className="btrans-meta">
              {(t.verse_count || 0).toLocaleString()} verses · {t.book_count || "—"} books
              {t.direction === "rtl" && " · right-to-left"}
            </div>
            <div className="btrans-actions">
              <button className="btn btn-primary btn-sm" onClick={() => catalog.setTranslation(t.id)}>
                {catalog.currentID === t.id ? "Selected" : "Use this"}
              </button>
              <Link className="btn btn-ghost btn-sm" href={readerHref(t.id, "John", 1)}>
                Open reader
              </Link>
            </div>
            {t.attribution_required && <p className="btrans-attr">{t.attribution_text || t.copyright}</p>}
          </article>
        ))}
      </div>
    </section>
  );
}

/* ------------------------------------------------------------- languages */

export function BibleLanguagesPage() {
  const [languages, setLanguages] = useState<Language[] | null>(null);
  const [error, setError] = useState(false);
  const catalog = useCatalog();

  useEffect(() => {
    const controller = new AbortController();
    bible
      .languages(controller.signal)
      .then((d) => setLanguages(d.languages || []))
      .catch(() => setError(true));
    return () => controller.abort();
  }, []);

  const countFor = (code: string) => (catalog.translations || []).filter((t) => t.language_code === code).length;

  return (
    <section className="container-wide bible-page">
      <header className="bible-page-head">
        <span className="scripture-eyebrow">Languages</span>
        <h1 className="h1">Read in your language.</h1>
        <p className="lede">
          Languages are first-class here: a language is listed when an approved translation actually exists in it. A
          language with no licensed edition is absent rather than promised.
        </p>
      </header>

      {error && <div className="bible-note">{CATALOG_UNAVAILABLE}</div>}
      {!languages && !error && <div className="bible-note">Loading languages…</div>}

      <div className="bible-langs">
        {(languages || []).map((l) => (
          <Link className="blang" key={l.id} href={`/bible/translations?language=${encodeURIComponent(l.id)}`}>
            <b>{l.native_name || l.name}</b>
            <span>{l.name}</span>
            <em>
              {countFor(l.id)} translation{countFor(l.id) === 1 ? "" : "s"}
              {l.direction === "rtl" ? " · RTL" : ""}
            </em>
          </Link>
        ))}
      </div>
    </section>
  );
}

/* --------------------------------------------------------------- library */

type LibraryKind = "highlights" | "bookmarks" | "notes" | "history" | "collections";

export function BibleLibraryPage({ kind }: { kind: LibraryKind }) {
  const state = useBible();
  const catalog = useCatalog();
  const toast = useToast();
  const [newCollection, setNewCollection] = useState("");

  const titles: Record<LibraryKind, [string, string]> = {
    highlights: ["Highlights", "Every verse you have marked, by colour."],
    bookmarks: ["Bookmarks", "The verses you keep coming back to."],
    notes: ["Notes", "What you wrote beside the text. Private to you."],
    history: ["History", "Where you have been reading."],
    collections: ["Collections", "Your own groupings — Morning Scriptures, Healing, Promises."],
  };
  const [title, lede] = titles[kind];

  const hrefFor = (verseID: string) => {
    const parsed = parseCanonicalVerseID(verseID);
    return parsed ? readerHref(catalog.currentID, parsed.bookID, parsed.chapter, parsed.verse) : "/bible";
  };

  return (
    <section className="container-wide bible-page">
      <header className="bible-page-head">
        <span className="scripture-eyebrow">Library</span>
        <h1 className="h1">{title}</h1>
        <p className="lede">{lede}</p>
        <nav className="bible-lib-nav">
          {(["highlights", "bookmarks", "notes", "collections", "history"] as LibraryKind[]).map((k) => (
            <Link key={k} href={`/bible/${k}`} aria-current={k === kind ? "page" : undefined}>
              {titles[k][0]}
            </Link>
          ))}
        </nav>
      </header>

      {kind === "highlights" &&
        (state.highlights.length === 0 ? (
          <Empty what="highlight" />
        ) : (
          <div className="bible-marks">
            {state.highlights.map((h) => (
              <Link className={`bmark hl-${h.color}`} key={h.id} href={hrefFor(h.verseID)}>
                <b>{h.reference}</b>
                <span>{HIGHLIGHT_COLORS.find((c) => c.id === h.color)?.name}</span>
              </Link>
            ))}
          </div>
        ))}

      {kind === "bookmarks" &&
        (state.bookmarks.length === 0 ? (
          <Empty what="bookmark" />
        ) : (
          <div className="bible-marks">
            {state.bookmarks.map((b) => (
              <div className="bmark" key={b.id}>
                <Link href={hrefFor(b.verseID)}>
                  <b>{b.reference}</b>
                </Link>
                <button className="bmark-x" aria-label="Remove bookmark" onClick={() => study.removeBookmark(b.id)}>
                  <Icon n="x" s={12} />
                </button>
              </div>
            ))}
          </div>
        ))}

      {kind === "notes" &&
        (state.notes.length === 0 ? (
          <Empty what="note" />
        ) : (
          <div className="bible-notes">
            {state.notes.map((n) => (
              <article className="bnote" key={n.id}>
                <Link href={hrefFor(n.verseID)}>
                  <b>{n.reference}</b>
                </Link>
                <p>{n.text}</p>
                <div className="bnote-foot">
                  <span className="small">{new Date(n.updatedAt).toLocaleDateString()}</span>
                  <button onClick={() => study.removeNote(n.id)}>Delete</button>
                </div>
              </article>
            ))}
          </div>
        ))}

      {kind === "collections" && (
        <>
          <form
            className="bible-filter-row"
            onSubmit={(e) => {
              e.preventDefault();
              if (!newCollection.trim()) return;
              study.createCollection(newCollection.trim());
              setNewCollection("");
              toast("Collection created");
            }}
          >
            <input
              value={newCollection}
              placeholder="New collection — Promises, Healing, Morning Scriptures"
              aria-label="New collection name"
              onChange={(e) => setNewCollection(e.target.value)}
            />
            <button className="btn btn-primary btn-sm" type="submit">
              Create
            </button>
          </form>
          {state.collections.length === 0 ? (
            <Empty what="collection" />
          ) : (
            <div className="bible-collections">
              {state.collections.map((c) => (
                <article className="bcoll" key={c.id}>
                  <header>
                    <b>{c.name}</b>
                    <button aria-label={`Delete ${c.name}`} onClick={() => study.deleteCollection(c.id)}>
                      <Icon n="x" s={12} />
                    </button>
                  </header>
                  {c.items.length === 0 ? (
                    <p className="small">Empty. Add a verse from the reader.</p>
                  ) : (
                    <ul>
                      {c.items.map((i) => (
                        <li key={i.verseID}>
                          <Link href={hrefFor(i.verseID)}>{i.reference}</Link>
                        </li>
                      ))}
                    </ul>
                  )}
                </article>
              ))}
            </div>
          )}
        </>
      )}

      {kind === "history" &&
        (state.history.length === 0 ? (
          <Empty what="reading" />
        ) : (
          <div className="bible-marks">
            {state.history.map((h, i) => {
              const book = bookByID(h.bookID);
              return (
                <Link className="bmark" key={`${h.bookID}${h.chapter}${i}`} href={readerHref(h.translation, h.bookID, h.chapter)}>
                  <b>
                    {book?.name} {h.chapter}
                  </b>
                  <span>{new Date(h.at).toLocaleString(undefined, { month: "short", day: "numeric", hour: "numeric" })}</span>
                </Link>
              );
            })}
          </div>
        ))}
    </section>
  );
}

const Empty = ({ what }: { what: string }) => (
  <div className="bible-note">
    No {what} yet. Open the reader, tap a verse, and it appears here. Your marks stay in this browser until you sign
    in — nothing private is sent anywhere without an account.
  </div>
);

/* ----------------------------------------------------------------- plans */

export function BiblePlansPage() {
  const [plans, setPlans] = useState<BiblePlan[] | null>(null);
  const [error, setError] = useState(false);
  const state = useBible();

  useEffect(() => {
    const controller = new AbortController();
    bible
      .plans(controller.signal)
      .then((d) => setPlans(d.plans || []))
      .catch(() => setError(true));
    return () => controller.abort();
  }, []);

  return (
    <section className="container-wide bible-page">
      <header className="bible-page-head">
        <span className="scripture-eyebrow">Reading plans</span>
        <h1 className="h1">A way through.</h1>
        <p className="lede">
          Curated plans are editorially reviewed before they publish — a plan that has not been reviewed is not offered.
          Your progress through one is private.
        </p>
      </header>

      {error && <div className="bible-note">{CATALOG_UNAVAILABLE}</div>}
      {!plans && !error && <div className="bible-note">Loading plans…</div>}
      {plans?.length === 0 && (
        <div className="bible-note">
          No reading plan has been published yet. Plans are created and reviewed in the admin dashboard before they
          appear here.
        </div>
      )}

      <div className="bible-plans">
        {(plans || []).map((plan) => {
          const key = plan.slug || plan.id || "";
          const enrolled = !!state.plans[key];
          return (
            <article className="bplan" key={key}>
              <h3>{plan.title}</h3>
              {plan.description && <p className="small">{plan.description}</p>}
              <div className="bplan-meta">
                {plan.duration_days || 0} days
                {plan.reading_count ? ` · ${plan.reading_count} readings` : ""}
              </div>
              <div className="btrans-actions">
                <Link className="btn btn-ghost btn-sm" href={`/bible/plans/${encodeURIComponent(key)}`}>
                  Open plan
                </Link>
                <button
                  className="btn btn-primary btn-sm"
                  onClick={() => {
                    study.enrollPlan(key);
                    track("reading_plan_started", { plan: key });
                  }}
                >
                  {enrolled ? "Enrolled" : "Start"}
                </button>
              </div>
              {plan.source_note && <p className="btrans-attr">{plan.source_note}</p>}
            </article>
          );
        })}
      </div>
    </section>
  );
}

export function BiblePlanPage({ slug }: { slug: string }) {
  const [plan, setPlan] = useState<BiblePlan | null>(null);
  const [error, setError] = useState(false);
  const state = useBible();
  const catalog = useCatalog();

  useEffect(() => {
    const controller = new AbortController();
    bible
      .plan(slug, controller.signal)
      .then((data) => setPlan(data.plan))
      .catch(() => setError(true));
    return () => controller.abort();
  }, [slug]);

  const progress = state.plans[slug];

  return (
    <section className="container-wide bible-page">
      <header className="bible-page-head">
        <span className="scripture-eyebrow">Reading plan</span>
        <h1 className="h1">{plan?.title || slug}</h1>
        {plan?.description && <p className="lede">{plan.description}</p>}
        {plan?.duration_days ? (
          <p className="small" style={{ marginTop: 10 }}>
            {plan.duration_days} days · {Object.keys(progress?.days || {}).length} marked done
          </p>
        ) : null}
      </header>

      {error && <div className="bible-note">{CATALOG_UNAVAILABLE}</div>}

      <div className="bible-plan-days">
        {(plan?.days || []).map((day) => {
          const references = day.references || [];
          const done = !!progress?.days?.[day.day_number];
          return (
            <div className={"bplan-day" + (done ? " done" : "")} key={day.day_number}>
              <b>{day.title || `Day ${day.day_number}`}</b>
              <div className="bpd-refs">
                {references.map((reference) => {
                  const parsed = parseReference(reference);
                  return parsed ? (
                    <Link key={reference} href={readerHref(catalog.currentID, parsed.bookID, parsed.chapter, parsed.startVerse)}>
                      {displayRef(parsed.bookID, parsed.chapter, parsed.startVerse, parsed.endVerse)}
                    </Link>
                  ) : (
                    <span key={reference}>{reference}</span>
                  );
                })}
              </div>
              <button onClick={() => study.completePlanDay(slug, day.day_number)}>
                <Icon n="check" s={13} /> {done ? "Done" : "Mark done"}
              </button>
            </div>
          );
        })}
      </div>
    </section>
  );
}

/* -------------------------------------------------------------- settings */

export function BibleSettingsPage() {
  const state = useBible();
  const { prefs } = state;
  const catalog = useCatalog();
  const stats = readingStats(state);

  return (
    <section className="container-wide bible-page">
      <header className="bible-page-head">
        <span className="scripture-eyebrow">Reader settings</span>
        <h1 className="h1">Set it up to read.</h1>
        <p className="lede">
          Typography, layout and the reading palette. The Bible language and the interface language are separate
          settings — reading French does not change the interface.
        </p>
      </header>

      <div className="bible-settings">
        <div className="bset">
          <h4>Translation</h4>
          <TranslationSelect catalog={catalog} id="bible-settings-translation" label="Default translation" />
          <p className="small">
            {catalog.current
              ? `${catalog.current.name}${catalog.current.public_domain ? " · public domain" : ""}`
              : "No translation selected."}
          </p>
        </div>

        <div className="bset">
          <h4>Typography</h4>
          <label className="bt-label" htmlFor="set-size">
            Text size — {prefs.fontSize}px
          </label>
          <input
            id="set-size"
            type="range"
            min={15}
            max={30}
            value={prefs.fontSize}
            onChange={(e) => study.setPrefs({ fontSize: Number(e.target.value) })}
          />
          <label className="bt-label" htmlFor="set-leading">
            Line height — {prefs.lineHeight.toFixed(2)}
          </label>
          <input
            id="set-leading"
            type="range"
            min={1.4}
            max={2.4}
            step={0.05}
            value={prefs.lineHeight}
            onChange={(e) => study.setPrefs({ lineHeight: Number(e.target.value) })}
          />
          <label className="bt-label" htmlFor="set-family">
            Typeface
          </label>
          <select
            id="set-family"
            value={prefs.fontFamily}
            onChange={(e) => study.setPrefs({ fontFamily: e.target.value as "sans" | "serif" })}
          >
            <option value="serif">Serif — for long reading</option>
            <option value="sans">Sans — for screens</option>
          </select>
        </div>

        <div className="bset">
          <h4>Layout</h4>
          <label className="bt-label" htmlFor="set-layout">
            Verses
          </label>
          <select
            id="set-layout"
            value={prefs.layout}
            onChange={(e) => study.setPrefs({ layout: e.target.value as "verse" | "paragraph" })}
          >
            <option value="verse">One verse per line</option>
            <option value="paragraph">Flowing paragraphs</option>
          </select>
          <label className="bt-check">
            <input
              type="checkbox"
              checked={prefs.showVerseNumbers}
              onChange={(e) => study.setPrefs({ showVerseNumbers: e.target.checked })}
            />
            Show verse numbers
          </label>
          <label className="bt-label" htmlFor="set-theme">
            Reading palette
          </label>
          <select
            id="set-theme"
            value={prefs.theme}
            onChange={(e) => study.setPrefs({ theme: e.target.value as typeof prefs.theme })}
          >
            <option value="system">Follow the site</option>
            <option value="light">Light</option>
            <option value="sepia">Sepia</option>
            <option value="dark">Dark</option>
          </select>
        </div>

        <div className="bset">
          <h4>Your data</h4>
          <p className="small">
            {stats.highlights} highlights · {stats.bookmarks} bookmarks · {stats.notes} notes · {stats.chapters}{" "}
            chapters marked read, out of {CANON.chapter_count.toLocaleString()}.
          </p>
          <p className="small">
            These are stored in this browser. Signing in on a build connected to the API syncs them to your account
            through the same private endpoints the mobile app uses; nothing is shared with anyone else.
          </p>
          <div className="btrans-actions">
            <button
              className="btn btn-ghost btn-sm"
              onClick={() => {
                const blob = new Blob([JSON.stringify(state, null, 2)], { type: "application/json" });
                const url = URL.createObjectURL(blob);
                const a = document.createElement("a");
                a.href = url;
                a.download = "iconfess-bible-study.json";
                a.click();
                URL.revokeObjectURL(url);
              }}
            >
              Export my study data
            </button>
          </div>
        </div>
      </div>
    </section>
  );
}

/* ----------------------------------------------------------------- audio */

export function BibleAudioPage() {
  const catalog = useCatalog();
  const player = usePlayer();
  const [reference, setReference] = useState("John 3");
  const [url, setUrl] = useState("");
  const [error, setError] = useState<string>("");

  const translation = catalog.current;
  const allowed = !!translation?.audio_allowed;

  return (
    <section className="container-wide bible-page">
      <header className="bible-page-head">
        <span className="scripture-eyebrow">Audio Bible</span>
        <h1 className="h1">Hear it read.</h1>
        <p className="lede">
          Audio is a separate grant from text. A chapter plays only when the translation carries audio rights and an
          approved, checksum-verified recording has been registered for it — the API issues a short-lived signed URL,
          and nothing is synthesised on demand.
        </p>
      </header>

      <div className="bible-filter-row">
        <div style={{ minWidth: 220 }}>
          <TranslationSelect catalog={catalog} id="bible-audio-translation" label="Translation" />
        </div>
        <input
          value={reference}
          aria-label="Reference"
          placeholder="John 3"
          onChange={(e) => setReference(e.target.value)}
        />
        <button
          className="btn btn-primary btn-sm"
          disabled={!allowed}
          onClick={() => {
            setError("");
            setUrl("");
            bible
              .audio(catalog.currentID, reference)
              .then((d) => {
                setUrl(d.audio_url || "");
                // Real recordings play through the shared player, which owns
                // the Media Session controls — lock screen, headset and
                // Bluetooth transport all address this one element.
                if (d.audio_url) {
                  player.load(
                    {
                      itemId: d.id || reference,
                      sessionId: "bible",
                      title: d.reference || reference,
                      subtitle: d.translation?.abbreviation || catalog.currentID,
                      src: d.audio_url,
                      durationHint: d.duration_ms ? Math.round(d.duration_ms / 1000) : undefined,
                    },
                    true
                  );
                }
              })
              .catch(() =>
                setError(
                  "No approved recording is registered for that passage in this translation. Readable text never implies audio rights."
                )
              );
          }}
        >
          Get audio
        </button>
      </div>

      {!allowed && translation && (
        <div className="bible-note">
          {translation.name} does not carry an audio grant. That is a licensing fact about the edition, not a missing
          feature.
        </div>
      )}
      {error && <div className="bible-note">{error}</div>}
      {url && (
        <div className="bible-note" role="status">
          Playing {player.track?.title || reference} through the session player — lock-screen and headset controls
          are live. {player.failed && "This recording could not be played."}
        </div>
      )}
    </section>
  );
}

/* ---------------------------------------------------------------- topics */

/** Topics are not authored here and not hard-coded anywhere: the API derives
 *  them from the passages the reviewed confession corpus actually stands on,
 *  so this page renders whatever that corpus currently says. */
export function BibleTopicsPage() {
  const [topics, setTopics] = useState<BibleTopic[] | null>(null);
  const [error, setError] = useState(false);
  const [filter, setFilter] = useState("");

  useEffect(() => {
    const controller = new AbortController();
    bible
      .topics(controller.signal)
      .then((d) => setTopics(d.topics || []))
      .catch(() => setError(true));
    return () => controller.abort();
  }, []);

  const shown = (topics || []).filter((t) =>
    !filter.trim() ? true : t.name.toLowerCase().includes(filter.trim().toLowerCase())
  );

  return (
    <section className="container-wide bible-page">
      <header className="bible-page-head">
        <span className="scripture-eyebrow">Topics</span>
        <h1 className="h1">What Scripture says about it.</h1>
        <p className="lede">
          Every topic here is the set of passages the reviewed confession corpus stands on for that part of life —
          ranked by how many confessions cite them. Nothing is hand-placed, and a citation that does not resolve
          against the canon is dropped rather than guessed at.
        </p>
      </header>

      <div className="bible-filter-row">
        <input
          value={filter}
          placeholder="Filter topics"
          aria-label="Filter topics"
          onChange={(e) => setFilter(e.target.value)}
        />
      </div>

      {error && <div className="bible-note">{CATALOG_UNAVAILABLE}</div>}
      {!topics && !error && <div className="bible-note">Loading topics…</div>}
      {topics?.length === 0 && (
        <div className="bible-note">No reviewed confession cites Scripture yet, so there are no topics to show.</div>
      )}

      <div className="bible-topics">
        {shown.map((topic) => (
          <Link className="btopic" key={topic.slug} href={`/bible/topics/${encodeURIComponent(topic.slug)}`}>
            <b>{topic.name}</b>
            {topic.description && <span>{topic.description}</span>}
            <em>
              {topic.passage_count} passage{topic.passage_count === 1 ? "" : "s"}
            </em>
          </Link>
        ))}
      </div>
    </section>
  );
}

export function BibleTopicPage({ slug }: { slug: string }) {
  const catalog = useCatalog();
  const [topic, setTopic] = useState<BibleTopic | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    bible
      .topic(slug, controller.signal)
      .then((d) => {
        setTopic(d.topic);
        track("topic_opened", { topic: slug });
      })
      .catch(() => setError(true));
    return () => controller.abort();
  }, [slug]);

  return (
    <section className="container-wide bible-page">
      <header className="bible-page-head">
        <span className="scripture-eyebrow">Topic</span>
        <h1 className="h1">{topic?.name || slug}</h1>
        {topic?.description && <p className="lede">{topic.description}</p>}
        <nav className="bible-lib-nav">
          <Link href="/bible/topics">All topics</Link>
          <Link href={`/categories/${encodeURIComponent(slug)}`}>Confessions in this area</Link>
        </nav>
      </header>

      {error && (
        <div className="bible-note">
          That topic has no reviewed passages yet. Topics appear as the confession corpus cites Scripture.
        </div>
      )}

      <div className="bible-results">
        {(topic?.passages || []).map((passage) => {
          const parsed = parseReference(passage.reference);
          return (
            <Link
              className="bresult"
              key={passage.reference}
              href={
                parsed
                  ? readerHref(catalog.currentID, parsed.bookID, parsed.chapter, parsed.startVerse)
                  : "/bible"
              }
            >
              <b>{passage.reference}</b>
              <p>
                {passage.confession_count} confession{passage.confession_count === 1 ? "" : "s"} in this area stand
                on it.
              </p>
              <span>{passage.canonical_id}</span>
            </Link>
          );
        })}
      </div>
    </section>
  );
}

/* -------------------------------------------------------- parallel Bible */

/** Two to four translations of the same passage, side by side. The column
 *  count follows the width; the reference is the one thing every column
 *  shares, which is the point of reading this way. */
export function BibleComparePage({ initialReference }: { initialReference?: string }) {
  const catalog = useCatalog();
  const [reference, setReference] = useState(initialReference || "John 3:16");
  const [chosen, setChosen] = useState<string[]>([]);
  const [passages, setPassages] = useState<APIPassage[] | null>(null);
  const [error, setError] = useState<string>("");
  const [loading, setLoading] = useState(false);

  const translations = catalog.translations || [];
  const parsed = useMemo(() => parseReference(reference), [reference]);

  /* Default to the reader's edition plus the next approved one, because the
     endpoint requires at least two and a page that opens empty teaches
     nothing. */
  useEffect(() => {
    if (chosen.length > 0 || translations.length === 0) return;
    const ids = [catalog.currentID, ...translations.map((t) => t.id)].filter(Boolean);
    setChosen([...new Set(ids)].slice(0, 2));
  }, [translations, catalog.currentID, chosen.length]);

  const run = useCallback(() => {
    if (!parsed || chosen.length < 2) {
      setError(
        chosen.length < 2
          ? "Choose at least two translations — a parallel reading needs something to be parallel to."
          : "Enter a reference such as John 3:16, Psalm 23 or 1 Corinthians 13:4-7."
      );
      setPassages(null);
      return;
    }
    setError("");
    setLoading(true);
    bible
      .compare(reference, chosen)
      .then((data) => setPassages(data.passages || []))
      .catch(() =>
        setError("That passage is not available in every translation you chose. Try another combination.")
      )
      .finally(() => setLoading(false));
  }, [parsed, chosen, reference]);

  useEffect(() => {
    if (chosen.length >= 2 && parsed) run();
    // first render once a default pair exists
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [chosen.length]);

  const toggle = (id: string) => {
    setChosen((current) => {
      if (current.includes(id)) return current.filter((x) => x !== id);
      if (current.length >= 4) return current;
      return [...current, id];
    });
  };

  return (
    <section className="container-wide bible-page">
      <header className="bible-page-head">
        <span className="scripture-eyebrow">Parallel</span>
        <h1 className="h1">The same verse, side by side.</h1>
        <p className="lede">
          Two to four approved translations of one reference. The reference never changes when you change the
          column — that is the whole point of reading this way.
        </p>
      </header>

      <form
        className="bible-filter-row"
        onSubmit={(e) => {
          e.preventDefault();
          run();
        }}
      >
        <input
          value={reference}
          aria-label="Reference"
          placeholder="John 3:16"
          onChange={(e) => setReference(e.target.value)}
        />
        <button className="btn btn-primary btn-sm" type="submit" disabled={!parsed}>
          Compare
        </button>
      </form>

      <div className="bcompare-picker" role="group" aria-label="Translations to compare">
        {translations.map((t) => (
          <button
            key={t.id}
            aria-pressed={chosen.includes(t.id)}
            className={chosen.includes(t.id) ? "on" : undefined}
            onClick={() => toggle(t.id)}
          >
            {t.abbreviation || t.id}
          </button>
        ))}
        {translations.length < 2 && (
          <span className="small">Only one approved translation is available, so there is nothing to compare yet.</span>
        )}
      </div>

      {error && <div className="bible-note">{error}</div>}
      {loading && <div className="bible-note">Loading the passage…</div>}

      {passages && passages.length > 0 && (
        <div className="bcompare" style={{ ["--cols" as string]: String(Math.min(passages.length, 4)) }}>
          {passages.map((passage) => (
            <article className="bcompare-col" key={passage.translation?.id || passage.reference}>
              <header>
                <b>{passage.translation?.abbreviation || passage.translation?.name}</b>
                <span>{passage.translation?.language_name}</span>
              </header>
              <div
                className="bcompare-text"
                dir={(passage.translation?.direction || "ltr").toLowerCase() === "rtl" ? "rtl" : "ltr"}
              >
                {(passage.verses || []).map((v) => (
                  <p key={v.id || v.number}>
                    <sup>{v.number}</sup>
                    {v.text}
                  </p>
                ))}
              </div>
              {passage.translation?.attribution_required && (
                <p className="btrans-attr">{passage.translation.attribution_text || passage.translation.copyright}</p>
              )}
            </article>
          ))}
        </div>
      )}
    </section>
  );
}
