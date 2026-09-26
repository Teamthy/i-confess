"use client";

/**
 * /app/history — the listening record, from GET /me/history.
 *
 * The API returns bare playback records: ids, durations, outcomes, times.
 * It deliberately carries no titles (the confession's row can change after
 * the fact; the record is the fact of listening). This page hydrates display
 * titles for the most recent confessions it names — real lookups against
 * GET /confessions/{id}, capped at twelve so a long history cannot fire a
 * request per row. Rows beyond the cap, or whose lookup fails, fall back to
 * an honest generic label; nothing is invented to fill the gap.
 */

import { useEffect, useMemo, useState } from "react";
import { useAuth } from "@/lib/auth-context";
import { appApi, useApiData, formatDate } from "@/lib/app-api";
import { EmptyState, ErrorBlock, LoadingBlock, SignedOut } from "@/components/app-ui";

type PlaybackRecord = {
  id: string;
  session_id?: string;
  confession_id?: string;
  duration_seconds?: number;
  completed: boolean;
  skipped: boolean;
  listened_at: string;
};

/** How many distinct confessions get a title lookup. Bounds the burst. */
const TITLE_LOOKUPS = 12;

export function HistoryClient() {
  const { token, loading: authLoading } = useAuth();
  const history = useApiData<PlaybackRecord[]>(token, "/me/history");
  const [titles, setTitles] = useState<Record<string, string>>({});

  const records = history.data ?? [];

  const idsKey = useMemo(
    () =>
      Array.from(
        new Set(records.map((r) => r.confession_id).filter((v): v is string => Boolean(v))),
      )
        .slice(0, TITLE_LOOKUPS)
        .join(","),
    [records],
  );

  useEffect(() => {
    if (!token || !idsKey) return;
    const ac = new AbortController();
    const ids = idsKey.split(",");
    Promise.all(
      ids.map((id) =>
        appApi<{ title?: string }>(`/confessions/${encodeURIComponent(id)}`, {
          token,
          signal: ac.signal,
        }).then((r) => (r.ok ? [id, r.data.title ?? ""] as const : [id, ""] as const)),
      ),
    ).then((pairs) => {
      if (ac.signal.aborted) return;
      setTitles(Object.fromEntries(pairs));
    });
    return () => ac.abort();
  }, [token, idsKey]);

  if (authLoading) return <LoadingBlock rows={4} />;
  if (!token) return <SignedOut next="/app/history" />;
  if (history.loading && !history.data) return <LoadingBlock rows={4} />;
  if (history.error && !history.data) {
    if (history.error.status === 401) return <SignedOut next="/app/history" />;
    return <ErrorBlock message={history.error.message} onRetry={history.reload} />;
  }

  if (records.length === 0) {
    return (
      <EmptyState
        title="Your history will appear here"
        body="Start listening to confessions and your history will build over time."
        ctaHref="/app/explore"
        ctaLabel="Explore confessions"
      />
    );
  }

  return (
    <ul className="ip-queue" style={{ listStyle: "none" }}>
      {records.map((r) => {
        const minutes = r.duration_seconds && r.duration_seconds > 0 ? Math.round(r.duration_seconds / 60) : 0;
        const outcome = r.completed ? "Completed" : r.skipped ? "Skipped" : "Listened";
        const glyph = r.completed ? "✓" : r.skipped ? "»" : "▷";
        const title = r.confession_id ? titles[r.confession_id] : "";
        return (
          <li key={r.id} className="ip-queue__item">
            <span className="ip-queue__num" aria-hidden="true">{glyph}</span>
            <span style={{ flex: 1 }}>
              {r.confession_id ? (
                <a
                  href={`/app/confessions/${encodeURIComponent(r.confession_id)}`}
                  style={{ textDecoration: "none", color: "inherit" }}
                >
                  <strong style={{ fontWeight: "var(--ic-font-weight-medium)" }}>
                    {title || "Confession"}
                  </strong>
                </a>
              ) : (
                <strong style={{ fontWeight: "var(--ic-font-weight-medium)" }}>Listening</strong>
              )}
              <span style={{ display: "block", color: "var(--ic-color-neutral-500)", fontSize: "var(--ic-font-size-caption)" }}>
                {formatDate(r.listened_at)}
                {minutes > 0 ? ` · ${minutes} min` : ""}
                {` · ${outcome}`}
              </span>
              {r.session_id && (
                <a
                  href={`/app/sessions/${encodeURIComponent(r.session_id)}`}
                  style={{ fontSize: "var(--ic-font-size-caption)", textDecoration: "none" }}
                >
                  From a session — see detail
                </a>
              )}
            </span>
          </li>
        );
      })}
    </ul>
  );
}
