// scripts/dev-bible.mjs — the Bible half of the local API FIXTURE.
//
// It answers the same /v1/bible/* and /v1/me/bible/* shapes the Go handlers
// answer, so the web reader exercises its real code paths in the sandbox,
// where Go cannot run. Two rules make it a fixture rather than a fake:
//
//   * The structure is the real one. web/lib/canon.json is generated from
//     server/internal/bible, so books, chapters and verse bounds here are the
//     same data the production API validates against.
//   * The text is real public-domain Scripture, fetched at build time by
//     scripts/dev-bible-corpus.py into /tmp and never committed. If it is
//     absent the catalogue is served empty and the reader shows its honest
//     "not reachable from this deployment" state — which is exactly what it
//     must do against a real outage.
//
// Rights are modelled, not waved through: the corpus marks audio_allowed
// false for both editions because no recording has been registered or
// reviewed, and GET /v1/bible/audio answers 404 accordingly.

import { existsSync, readFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import path from "node:path";

const CORPUS_PATH = process.env.IC_BIBLE_CORPUS || "/tmp/bible-corpus.json";

export function createBibleFixture({ root, confessions = [], categories = [] }) {
  const canon = JSON.parse(readFileSync(path.join(root, "web", "lib", "canon.json"), "utf8"));
  const booksByID = new Map(canon.books.map((b) => [b.id, b]));
  const booksByUSFM = new Map(canon.books.map((b) => [b.usfm, b]));

  let corpus = { translations: [], text: {} };
  if (!existsSync(CORPUS_PATH)) {
    const script = path.join(root, "scripts", "dev-bible-corpus.py");
    spawnSync("python3", [script], { stdio: "inherit" });
  }
  if (existsSync(CORPUS_PATH)) {
    try {
      corpus = JSON.parse(readFileSync(CORPUS_PATH, "utf8"));
    } catch {
      corpus = { translations: [], text: {} };
    }
  }

  const translations = corpus.translations || [];
  const translationByID = new Map(translations.map((t) => [t.id.toLowerCase(), t]));
  const text = corpus.text || {};

  /* ------------------------------------------------------------ helpers */

  const normalizeKey = (s) =>
    String(s)
      .trim()
      .toLowerCase()
      .replace(/[.\-']/g, "")
      .replace(/[^a-z0-9 ]/g, " ")
      .replace(/\s+/g, " ")
      .trim();

  function resolveBook(name) {
    if (!name) return null;
    let raw = String(name).trim();
    const dot = raw.indexOf(".");
    if (dot > 0 && /^\d/.test(raw.slice(dot + 1))) raw = raw.slice(0, dot);
    const id = canon.aliases[normalizeKey(raw)];
    return id ? booksByID.get(id) : null;
  }

  function parseReference(input) {
    let value = String(input || "").trim();
    if (!value) return null;
    const canonical = /^([1-3]?[A-Za-z]{2,6})\.(\d+)(?:\.(\d+)(?:-(\d+))?)?$/.exec(value);
    if (canonical) {
      value = canonical[3]
        ? `${canonical[1]} ${canonical[2]}:${canonical[3]}${canonical[4] ? `-${canonical[4]}` : ""}`
        : `${canonical[1]} ${canonical[2]}`;
    }
    const match = /^(.+?)\s+(\d+)(?::(\d+)(?:\s*[-–—]\s*(\d+))?)?$/.exec(value);
    if (!match) {
      const bare = resolveBook(value);
      return bare ? { book: bare, chapter: 1 } : null;
    }
    const book = resolveBook(match[1]);
    if (!book) return null;
    const chapter = Number(match[2]);
    if (!chapter || chapter > book.chapter_count) return null;
    const start = match[3] ? Number(match[3]) : 0;
    const end = match[4] ? Number(match[4]) : start;
    if (start && (start > book.chapters[chapter - 1] || end < start)) return null;
    return { book, chapter, start, end };
  }

  const display = (book, chapter, start, end) =>
    !start ? `${book.name} ${chapter}` : end && end !== start ? `${book.name} ${chapter}:${start}-${end}` : `${book.name} ${chapter}:${start}`;

  const bookInfo = (book) => ({
    id: book.id,
    name: book.name,
    testament: book.testament,
    canonical_order: book.canonical_order,
    chapter_count: book.chapter_count,
  });

  const versesFor = (translationID, bookID, chapter) => (text[translationID]?.[bookID] || {})[String(chapter)] || [];

  const verseDTO = (book, chapter, number, body) => ({
    id: `${book.usfm}.${chapter}.${number}`,
    number,
    text: body,
    book_id: book.id,
    chapter,
  });

  function chapterDTO(translation, book, chapter) {
    const verses = versesFor(translation.id, book.id, chapter);
    if (verses.length === 0) return null;
    return {
      translation,
      book: bookInfo(book),
      chapter,
      verses: verses.map((body, i) => verseDTO(book, chapter, i + 1, body)),
    };
  }

  /* Cross references come from the reviewed confession corpus: two passages
     are related here because a confession stands on both. That is a real
     editorial relationship rather than an invented concordance. */
  const crossRefs = new Map();
  for (const confession of confessions) {
    const refs = [];
    for (const scripture of confession.scriptures || []) {
      const book = resolveBook(scripture.book);
      if (!book) continue;
      const chapter = Number(scripture.chapter);
      if (!chapter || chapter > book.chapter_count) continue;
      const first = parseInt(String(scripture.verse), 10);
      refs.push({ book, chapter, verse: Number.isNaN(first) ? 0 : first });
    }
    for (const a of refs) {
      const key = `${a.book.usfm}.${a.chapter}.${a.verse || 1}`;
      const list = crossRefs.get(key) || [];
      for (const b of refs) {
        if (b === a) continue;
        const label = display(b.book, b.chapter, b.verse);
        if (!list.includes(label)) list.push(label);
      }
      crossRefs.set(key, list);
    }
  }

  /* The verse of the day rotates deterministically through the verses the
     reviewed corpus actually cites — never a random verse from anywhere. */
  const votdPool = [...crossRefs.keys()].sort();

  /* Topics, derived the way the Go handler derives them: the passages the
     published confession corpus cites, grouped by its category, ranked by how
     many confessions stand on each. Nothing is authored twice. */
  const categoryBySlugName = new Map(categories.map((c) => [c.name, c]));
  const topics = (() => {
    const grouped = new Map();
    for (const confession of confessions) {
      const category = categoryBySlugName.get(confession.category);
      if (!category) continue;
      let topic = grouped.get(category.slug);
      if (!topic) {
        topic = { id: category.id, slug: category.slug, name: category.name, description: category.description || "", passages: new Map() };
        grouped.set(category.slug, topic);
      }
      for (const scripture of confession.scriptures || []) {
        const book = resolveBook(scripture.book);
        if (!book) continue;
        const chapter = Number(scripture.chapter);
        if (!chapter || chapter > book.chapter_count) continue;
        const first = parseInt(String(scripture.verse), 10);
        const reference = display(book, chapter, Number.isNaN(first) ? 0 : first);
        const existing = topic.passages.get(reference);
        if (existing) {
          existing.confession_count += 1;
          continue;
        }
        topic.passages.set(reference, {
          reference,
          book_id: book.id,
          book_name: book.name,
          chapter,
          verses: Number.isNaN(first) ? "" : String(first),
          canonical_id: Number.isNaN(first) ? "" : `${book.usfm}.${chapter}.${first}`,
          confession_count: 1,
        });
      }
    }
    return [...grouped.values()]
      .map((topic) => ({
        ...topic,
        passages: [...topic.passages.values()].sort(
          (a, b) => b.confession_count - a.confession_count || a.reference.localeCompare(b.reference)
        ),
      }))
      .map((topic) => ({ ...topic, passage_count: topic.passages.length }))
      .filter((topic) => topic.passage_count > 0);
  })();

  const plans = [
    {
      id: "gospels-30",
      slug: "gospels-30",
      title: "The Gospels in 30 days",
      description: "Matthew, Mark, Luke and John, three or four chapters a day. Fixture plan for local development.",
      status: "published",
    },
    {
      id: "psalms-30",
      slug: "psalms-30",
      title: "Psalms in 30 days",
      description: "Five psalms a day, start to finish. Fixture plan for local development.",
      status: "published",
    },
  ];

  function planDays(slug) {
    const chapters = [];
    if (slug === "gospels-30") {
      for (const id of ["Matt", "Mark", "Luke", "John"]) {
        const book = booksByID.get(id);
        for (let c = 1; c <= book.chapter_count; c++) chapters.push(`${book.name} ${c}`);
      }
    } else {
      const book = booksByID.get("Ps");
      for (let c = 1; c <= book.chapter_count; c++) chapters.push(`${book.name} ${c}`);
    }
    const perDay = Math.ceil(chapters.length / 30);
    const days = [];
    for (let day = 1; day <= 30; day++) {
      const slice = chapters.slice((day - 1) * perDay, day * perDay);
      if (slice.length) days.push({ day, references: slice });
    }
    return days;
  }

  const notFound = (json, res) =>
    json(res, 404, { code: "BIBLE_NOT_FOUND", error: "That Bible passage or translation is not available." });

  const store = { highlights: [], bookmarks: [], notes: [], collections: [], history: [], progress: [], preferences: {} };

  /* ------------------------------------------------------------- routes */

  return function handleBible({ method, path: p, url, body, json, res, authed }) {
    if (!p.startsWith("/bible") && !p.startsWith("/me/bible")) return false;

    /* ---- private study data (the reader mirrors marks here when signed in) */
    if (p.startsWith("/me/bible")) {
      if (!authed) {
        json(res, 401, { error: "authentication required" });
        return true;
      }
      const tail = p.slice("/me/bible".length) || "/";
      const bucket = tail.split("/")[1] || "";
      if (method === "GET") {
        json(res, 200, { [bucket || "items"]: store[bucket] || [] });
        return true;
      }
      if (method === "PUT" && bucket === "preferences") {
        store.preferences = { ...store.preferences, ...(body || {}) };
        json(res, 200, store.preferences);
        return true;
      }
      if (method === "POST") {
        const record = { id: `${bucket}-${store[bucket]?.length ?? 0}`, ...(body || {}), created_at: new Date().toISOString() };
        if (Array.isArray(store[bucket])) store[bucket].unshift(record);
        json(res, 201, record);
        return true;
      }
      if (method === "DELETE") {
        json(res, 200, { ok: true });
        return true;
      }
      json(res, 405, { error: "method not allowed" });
      return true;
    }

    if (method !== "GET") {
      json(res, 405, { error: "method not allowed" });
      return true;
    }

    const rest = p.slice("/bible".length);

    if (rest === "/structure") {
      const wanted = (url.searchParams.get("translation") || "").toLowerCase();
      if (!wanted) {
        json(res, 200, canon);
        return true;
      }
      const translation = translationByID.get(wanted);
      if (!translation) return notFound(json, res) || true;
      const carried = new Set(Object.keys(text[translation.id] || {}));
      json(res, 200, {
        ...canon,
        translation: {
          id: translation.id,
          name: translation.name,
          abbreviation: translation.abbreviation,
          coverage: translation.coverage,
          direction: translation.direction,
          language_code: translation.language_code,
          available_book_ids: canon.books.filter((b) => carried.has(b.id)).map((b) => b.id),
          missing_book_ids: canon.books.filter((b) => !carried.has(b.id)).map((b) => b.id),
        },
      });
      return true;
    }

    if (rest === "/languages") {
      const seen = new Map();
      for (const t of translations) {
        if (!seen.has(t.language_code)) {
          seen.set(t.language_code, {
            id: t.language_code,
            iso639_1: t.language_code,
            iso639_3: t.language_code === "en" ? "eng" : t.language_code,
            bcp47: t.locale || t.language_code,
            name: t.language_name,
            native_name: t.language_name,
            direction: t.direction || "ltr",
            status: "active",
          });
        }
      }
      json(res, 200, { languages: [...seen.values()] });
      return true;
    }

    if (rest === "/translations") {
      const language = (url.searchParams.get("language") || "").toLowerCase();
      const query = (url.searchParams.get("q") || "").toLowerCase();
      json(res, 200, {
        translations: translations.filter(
          (t) =>
            (!language || t.language_code.toLowerCase() === language) &&
            (!query || `${t.name} ${t.abbreviation}`.toLowerCase().includes(query))
        ),
      });
      return true;
    }

    let m;
    if ((m = rest.match(/^\/translations\/([^/]+)$/))) {
      const translation = translationByID.get(decodeURIComponent(m[1]).toLowerCase());
      if (!translation) return notFound(json, res) || true;
      json(res, 200, translation);
      return true;
    }

    if (rest === "/books" || (m = rest.match(/^\/books\/([^/]+)(\/chapters)?$/))) {
      const translation = translationByID.get((url.searchParams.get("translation") || "").toLowerCase());
      if (!translation) {
        json(res, 400, { code: "BIBLE_TRANSLATION_REQUIRED", error: "Choose a Bible translation." });
        return true;
      }
      const carried = new Set(Object.keys(text[translation.id] || {}));
      if (rest === "/books") {
        json(res, 200, { books: canon.books.filter((b) => carried.has(b.id)).map(bookInfo) });
        return true;
      }
      const book = resolveBook(decodeURIComponent(m[1]));
      if (!book || !carried.has(book.id)) return notFound(json, res) || true;
      if (m[2]) {
        json(res, 200, {
          book: bookInfo(book),
          chapters: Array.from({ length: book.chapter_count }, (_, i) => i + 1),
        });
        return true;
      }
      json(res, 200, bookInfo(book));
      return true;
    }

    if (rest === "/passage") {
      const translation = translationByID.get((url.searchParams.get("translation") || "").toLowerCase());
      const parsed = parseReference(url.searchParams.get("reference") || "");
      if (!translation || !parsed) {
        json(res, 400, { code: "BIBLE_INVALID_REFERENCE", error: "Enter a passage and choose a translation." });
        return true;
      }
      const all = versesFor(translation.id, parsed.book.id, parsed.chapter);
      if (all.length === 0) return notFound(json, res) || true;
      const from = parsed.start ? parsed.start : 1;
      const to = parsed.start ? parsed.end || parsed.start : all.length;
      json(res, 200, {
        reference: display(parsed.book, parsed.chapter, parsed.start, parsed.end),
        translation,
        verses: all.slice(from - 1, to).map((bodyText, i) => verseDTO(parsed.book, parsed.chapter, from + i, bodyText)),
      });
      return true;
    }

    if (rest === "/search") {
      const query = (url.searchParams.get("q") || "").trim();
      if (!query) {
        json(res, 400, { code: "BIBLE_INVALID_QUERY", error: "Enter a search phrase up to 200 characters." });
        return true;
      }
      const translation =
        translationByID.get((url.searchParams.get("translation") || "").toLowerCase()) || translations[0];
      if (!translation) return notFound(json, res) || true;

      const reference = parseReference(query);
      if (reference) {
        const all = versesFor(translation.id, reference.book.id, reference.chapter);
        if (all.length === 0) return notFound(json, res) || true;
        const from = reference.start || 1;
        const to = reference.start ? reference.end || reference.start : all.length;
        json(res, 200, {
          kind: "reference",
          results: [
            {
              reference: display(reference.book, reference.chapter, reference.start, reference.end),
              translation,
              verses: all.slice(from - 1, to).map((bodyText, i) => verseDTO(reference.book, reference.chapter, from + i, bodyText)),
            },
          ],
        });
        return true;
      }

      const limit = Math.min(Math.max(Number(url.searchParams.get("limit")) || 20, 1), 50);
      const only = url.searchParams.get("book");
      const needle = query.toLowerCase();
      const results = [];
      for (const [bookID, chapters] of Object.entries(text[translation.id] || {})) {
        if (only && bookID !== only) continue;
        const book = booksByID.get(bookID);
        if (!book) continue;
        for (const [chapter, verses] of Object.entries(chapters)) {
          for (let i = 0; i < verses.length; i++) {
            if (!verses[i].toLowerCase().includes(needle)) continue;
            results.push({ translation, book: bookInfo(book), chapter: Number(chapter), verse: i + 1, text: verses[i] });
            if (results.length >= limit * 4) break;
          }
          if (results.length >= limit * 4) break;
        }
        if (results.length >= limit * 4) break;
      }
      results.sort((a, b) => a.book.canonical_order - b.book.canonical_order || a.chapter - b.chapter || a.verse - b.verse);
      json(res, 200, { kind: "text", results: results.slice(0, limit) });
      return true;
    }

    if (rest === "/cross-references") {
      const parsed = parseReference(url.searchParams.get("reference") || "");
      if (!parsed) {
        json(res, 400, { code: "BIBLE_INVALID_REFERENCE", error: "Enter a valid Bible reference." });
        return true;
      }
      const key = `${parsed.book.usfm}.${parsed.chapter}.${parsed.start || 1}`;
      json(res, 200, {
        reference: display(parsed.book, parsed.chapter, parsed.start, parsed.end),
        references: crossRefs.get(key) || [],
      });
      return true;
    }

    if (rest === "/compare") {
      const parsed = parseReference(url.searchParams.get("reference") || "");
      if (!parsed) {
        json(res, 400, { code: "BIBLE_INVALID_REFERENCE", error: "Enter a valid Bible reference." });
        return true;
      }
      // The Go handler requires between two and four explicitly named
      // translations; a client that omits them must fail here too, or it
      // would only break in production.
      const wanted = (url.searchParams.get("translations") || "")
        .split(",")
        .map((x) => x.trim().toLowerCase())
        .filter(Boolean);
      if (wanted.length < 2 || wanted.length > 4) {
        json(res, 400, { error: "Choose between two and four translations." });
        return true;
      }
      const passages = [];
      for (const translation of translations) {
        if (!wanted.includes(translation.id.toLowerCase())) continue;
        const all = versesFor(translation.id, parsed.book.id, parsed.chapter);
        if (all.length === 0) continue;
        const from = parsed.start || 1;
        const to = parsed.start ? parsed.end || parsed.start : Math.min(all.length, from + 4);
        passages.push({
          reference: display(parsed.book, parsed.chapter, parsed.start, parsed.end),
          translation,
          verses: all.slice(from - 1, to).map((bodyText, i) => verseDTO(parsed.book, parsed.chapter, from + i, bodyText)),
        });
      }
      if (passages.length < 2) {
        json(res, 400, { error: "Choose at least two distinct translations." });
        return true;
      }
      json(res, 200, { reference: display(parsed.book, parsed.chapter, parsed.start, parsed.end), passages });
      return true;
    }

    if (rest === "/verse-of-day") {
      const translation =
        translationByID.get((url.searchParams.get("translation") || "").toLowerCase()) || translations[0];
      if (!translation || votdPool.length === 0) return notFound(json, res) || true;
      const today = new Date();
      const dayOfYear = Math.floor((today - new Date(today.getFullYear(), 0, 0)) / 86400000);
      const key = votdPool[dayOfYear % votdPool.length];
      const [usfm, chapter, verse] = key.split(".");
      const book = booksByUSFM.get(usfm);
      const verses = book ? versesFor(translation.id, book.id, Number(chapter)) : [];
      const number = Math.min(Number(verse) || 1, verses.length);
      if (!book || verses.length === 0) return notFound(json, res) || true;
      // Shape parity with the Go handler: a single verse, and the canonical
      // verse ID as the reference.
      const dto = verseDTO(book, Number(chapter), number, verses[number - 1]);
      json(res, 200, {
        date: today.toISOString().slice(0, 10),
        editor_note: "Selected from the passages the reviewed confession corpus cites.",
        translation,
        verse: dto,
        reference: dto.id,
      });
      return true;
    }

    if (rest === "/topics") {
      json(res, 200, {
        topics: topics.map(({ passages, ...summary }) => summary),
      });
      return true;
    }
    if ((m = rest.match(/^\/topics\/([^/]+)$/))) {
      const topic = topics.find((t) => t.slug === decodeURIComponent(m[1]));
      if (!topic) {
        json(res, 404, { code: "BIBLE_NOT_FOUND", error: "That topic has no reviewed passages yet." });
        return true;
      }
      json(res, 200, { topic });
      return true;
    }

    if (rest === "/random") {
      const translation = translationByID.get((url.searchParams.get("translation") || "").toLowerCase());
      if (!translation) {
        json(res, 400, { code: "BIBLE_TRANSLATION_REQUIRED", error: "Choose a Bible translation." });
        return true;
      }
      const pool = topics.flatMap((topic) => topic.passages.map((p2) => ({ ...p2, topic })));
      if (pool.length === 0) return notFound(json, res) || true;
      const pick = pool[Math.floor(Math.random() * pool.length)];
      const book = booksByID.get(pick.book_id);
      const all = versesFor(translation.id, pick.book_id, pick.chapter);
      const number = Math.min(Number(pick.verses) || 1, all.length);
      if (!book || all.length === 0) return notFound(json, res) || true;
      json(res, 200, {
        reference: pick.reference,
        passage: {
          reference: pick.reference,
          translation,
          verses: [verseDTO(book, pick.chapter, number, all[number - 1])],
        },
        topic: { slug: pick.topic.slug, name: pick.topic.name },
      });
      return true;
    }

    if (rest === "/plans") {
      json(res, 200, {
        plans: plans.map((entry) => ({
          id: entry.id,
          slug: entry.slug,
          title: entry.title,
          description: entry.description,
          language: "en",
          duration_days: 30,
          reading_count: planDays(entry.slug).length,
          source_note: "Fixture plan generated from the canon for local development.",
        })),
      });
      return true;
    }
    if ((m = rest.match(/^\/plans\/([^/]+)$/))) {
      const plan = plans.find((x) => x.slug === decodeURIComponent(m[1]));
      if (!plan) return notFound(json, res) || true;
      json(res, 200, {
        plan: {
          id: plan.id,
          slug: plan.slug,
          title: plan.title,
          description: plan.description,
          language: "en",
          duration_days: 30,
          source_note: "Fixture plan generated from the canon for local development.",
          days: planDays(plan.slug).map((day) => ({
            day_number: day.day,
            title: `Day ${day.day}`,
            references: day.references,
          })),
        },
      });
      return true;
    }

    if (rest === "/audio") {
      // Text rights are not audio rights: no recording has been registered.
      json(res, 404, {
        code: "BIBLE_NOT_FOUND",
        error: "No approved recording is registered for that passage in this translation.",
      });
      return true;
    }

    // /bible/{translation}/{book}/{chapter}[/{verse}]
    if ((m = rest.match(/^\/([^/]+)\/([^/]+)\/(\d+)(?:\/(\d+))?$/))) {
      const translation = translationByID.get(decodeURIComponent(m[1]).toLowerCase());
      const book = resolveBook(decodeURIComponent(m[2]));
      const chapter = Number(m[3]);
      if (!translation || !book || !chapter || chapter > book.chapter_count) return notFound(json, res) || true;
      const data = chapterDTO(translation, book, chapter);
      if (!data) return notFound(json, res) || true;
      if (m[4]) {
        const number = Number(m[4]);
        const verse = data.verses[number - 1];
        if (!verse) return notFound(json, res) || true;
        json(res, 200, verse);
        return true;
      }
      json(res, 200, data);
      return true;
    }

    return notFound(json, res) || true;
  };
}
