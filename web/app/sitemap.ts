import type { MetadataRoute } from "next";
import { BOOKS } from "@/lib/canon";

/* Sitemap.
 *
 * The Bible section is the first part of the site with a canonical URL per
 * resource, so it is the first part worth submitting. The rule the brief sets
 * — do not generate millions of near-duplicate pages — is why this lists the
 * 66 book entry points and not all 1,189 chapters: a crawler that reaches
 * /bible/kjv/John finds every chapter of John from there, and the reader's
 * own canonical tags name the chapter it is actually showing.
 *
 * Verse text is rights-gated and served per translation, so only the default
 * public-domain edition is advertised here. */

const BASE = "https://iconfess.app";
const DEFAULT_TRANSLATION = "kjv";

export default function sitemap(): MetadataRoute.Sitemap {
  const now = new Date();
  const fixed = [
    "",
    "/how-it-works",
    "/explore",
    "/categories",
    "/journal",
    "/premium",
    "/faq",
    "/about",
    "/bible",
    "/bible/search",
    "/bible/books",
    "/bible/verse-of-the-day",
    "/bible/translations",
    "/bible/languages",
    "/bible/topics",
    "/bible/compare",
    "/bible/plans",
    "/bible/audio",
  ].map((path) => ({
    url: `${BASE}${path}`,
    lastModified: now,
    changeFrequency: path.startsWith("/bible") ? ("weekly" as const) : ("monthly" as const),
    priority: path === "" ? 1 : path === "/bible" ? 0.9 : 0.6,
  }));

  const books = BOOKS.map((book) => ({
    url: `${BASE}/bible/${DEFAULT_TRANSLATION}/${book.id}`,
    lastModified: now,
    changeFrequency: "yearly" as const,
    priority: 0.5,
  }));

  return [...fixed, ...books];
}
