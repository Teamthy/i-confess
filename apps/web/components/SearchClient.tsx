"use client";

/**
 * /app/search — GET /search, the only search the product has.
 *
 * The endpoint searches four things: confessions, categories, collections and
 * voices (its own default type set — the UI does not advertise a scripture
 * search the backend does not implement). Queries are debounced and committed
 * to the fetch path, so the hook's abort handling covers every superseded
 * keystroke; sub-two-character queries are a UI courtesy gate, not a claim
 * about the API.
 *
 * Results are the server's, ranked by the server's score, count reported by
 * the server. A collection result has no detail surface in the app yet, so it
 * renders unlinked rather than pointing somewhere that does not exist.
 */

import { useEffect, useRef, useState } from "react";
import { ANALYTICS_EVENTS, track } from "@/lib/analytics";
import { useAuth } from "@/lib/auth-context";
import { useApiData } from "@/lib/app-api";
import { ErrorBlock, LoadingBlock, SignedOut } from "@/components/app-ui";

type SearchResult = {
  id: string;
  type: string; // confession | category | collection | voice
  title: string;
  description?: string;
  score?: number;
};

type SearchResponse = { results: SearchResult[] | null; count: number };

const TYPES = [
  { value: "", label: "All" },
  { value: "confession", label: "Confessions" },
  { value: "category", label: "Categories" },
  { value: "collection", label: "Collections" },
  { value: "voice", label: "Voices" },
];

function hrefFor(r: SearchResult): string | null {
  const id = encodeURIComponent(r.id);
  switch (r.type) {
    case "confession":
      return `/app/confessions/${id}`;
    case "category":
      return `/app/categories/${id}`;
    case "voice":
      return `/app/voices/${id}`;
    default:
      return null;
  }
}

export function SearchClient() {
  const { token, loading: authLoading } = useAuth();
  const [raw, setRaw] = useState("");
  const [query, setQuery] = useState("");
  const [type, setType] = useState("");

  // Debounce: commit the typed value to the fetch path 300ms after the last
  // keystroke. The useApiData hook aborts the request a path supersedes.
  useEffect(() => {
    const t = setTimeout(() => setQuery(raw.trim()), 300);
    return () => clearTimeout(t);
  }, [raw]);

  const ready = query.length >= 2;
  // Below the courtesy gate the fetch path stays at the empty query, which the
  // API answers immediately with an empty set — no speculative single-letter
  // searches fire.
  const path = ready
    ? "/search?q=" +
      encodeURIComponent(query) +
      "&limit=30" +
      (type ? `&type=${encodeURIComponent(type)}` : "")
    : "/search?q=";
  const search = useApiData<SearchResponse>(token, path);

  if (authLoading) return <LoadingBlock rows={2} />;
  if (!token) return <SignedOut next="/app/search" />;

  const results = search.data?.results ?? [];

  return (
    <div style={{ display: "grid", gap: "var(--ic-spacing-5)" }}>
      <div style={{ maxWidth: "40rem" }}>
        <div className="ic-field">
          <label htmlFor="search-input">Search the library</label>
          <input
            id="search-input"
            type="search"
            placeholder="Try peace, healing, purpose…"
            autoComplete="off"
            value={raw}
            onChange={(e) => setRaw(e.target.value)}
            aria-describedby="search-status"
          />
        </div>
        <div className="ic-chipgrid" role="group" aria-label="Filter by type" style={{ marginTop: "var(--ic-spacing-3)" }}>
          {TYPES.map((t) => (
            <button
              key={t.value}
              type="button"
              className="ic-chipgrid__chip"
              aria-pressed={type === t.value}
              onClick={() => setType(t.value)}
            >
              {t.label}
            </button>
          ))}
        </div>
      </div>

      <div id="search-status" role="status" aria-live="polite">
        {!ready ? (
          <div className="ic-state">
            <h3>Start typing to search</h3>
            <p>Two characters or more — confessions, categories, collections, and voices.</p>
          </div>
        ) : search.loading && !search.data ? (
          <LoadingBlock rows={3} />
        ) : search.error && !search.data ? (
          <ErrorBlock message={search.error.message} onRetry={search.reload} />
        ) : results.length === 0 ? (
          <div className="ic-state">
            <h3>Nothing matched</h3>
            <p>No published {type || "content"} matches “{query}”. A different word may reach it.</p>
          </div>
        ) : (
          <>
            <p style={{ margin: 0, color: "var(--ic-color-neutral-600)", fontSize: "var(--ic-font-size-bodySm)" }}>
              {search.data?.count ?? results.length} result{(search.data?.count ?? results.length) === 1 ? "" : "s"} for “{query}”
            </p>
            <ul className="ip-queue" style={{ listStyle: "none" }}>
              {results.map((r) => {
                const href = hrefFor(r);
                return (
                  <li key={`${r.type}-${r.id}`} className="ip-queue__item">
                    <span className="ip-queue__num" aria-hidden="true">◦</span>
                    <span style={{ flex: 1 }}>
                      {href ? (
                        <a href={href} style={{ textDecoration: "none", color: "inherit" }}>
                          <strong style={{ fontWeight: "var(--ic-font-weight-medium)" }}>{r.title}</strong>
                        </a>
                      ) : (
                        <strong style={{ fontWeight: "var(--ic-font-weight-medium)" }}>{r.title}</strong>
                      )}
                      <span style={{ display: "block", color: "var(--ic-color-neutral-500)", fontSize: "var(--ic-font-size-caption)" }}>
                        {r.type}
                        {r.description ? ` · ${r.description.slice(0, 140)}` : ""}
                      </span>
                    </span>
                  </li>
                );
              })}
            </ul>
          </>
        )}
      </div>
    </div>
  );
}
