"use client";

/**
 * /app/favorites — everything the account has saved, from GET /me/favorites.
 *
 * The favourites table is polymorphic (confessions, categories, voices and
 * sessions share it), and the server resolves each row's display title on
 * read — a bare (entity_type, entity_id) pair is an id, not something a
 * person can read. A row whose target no longer exists arrives with
 * `missing` set instead of being dropped: it renders, unlinked, so the list
 * never quietly loses something the account still holds.
 *
 * Read-only by scope: this page shows the library as the server reports it.
 */

import { useAuth } from "@/lib/auth-context";
import { useApiData, formatDate } from "@/lib/app-api";
import { EmptyState, ErrorBlock, LoadingBlock, SignedOut } from "@/components/app-ui";

type Favorite = {
  id: string;
  entity_type: string; // confession | category | session | voice
  entity_id: string;
  title?: string;
  subtitle?: string;
  missing?: boolean;
  created_at: string;
};

/** Where each kind of saved thing lives. All four target pages resolve ids. */
function hrefFor(f: Favorite): string | null {
  const id = encodeURIComponent(f.entity_id);
  switch (f.entity_type) {
    case "confession":
      return `/app/confessions/${id}`;
    case "category":
      // /app/categories/[slug] resolves by slug or id (lib/api.ts category()).
      return `/app/categories/${id}`;
    case "voice":
      return `/app/voices/${id}`;
    case "session":
      return `/app/sessions/${id}`;
    default:
      return null;
  }
}

const GROUPS: { type: string; heading: string; glyph: string }[] = [
  { type: "confession", heading: "Confessions", glyph: "✦" },
  { type: "category", heading: "Categories", glyph: "◇" },
  { type: "voice", heading: "Voices", glyph: "◉" },
  { type: "session", heading: "Sessions", glyph: "▷" },
];

export function FavoritesClient() {
  const { token, loading: authLoading } = useAuth();
  const favs = useApiData<Favorite[]>(token, "/me/favorites");

  if (authLoading) return <LoadingBlock rows={3} />;
  if (!token) return <SignedOut next="/app/favorites" />;
  if (favs.loading && !favs.data) return <LoadingBlock rows={3} />;
  if (favs.error && !favs.data) {
    if (favs.error.status === 401) return <SignedOut next="/app/favorites" />;
    return <ErrorBlock message={favs.error.message} onRetry={favs.reload} />;
  }

  const list = favs.data ?? [];

  if (list.length === 0) {
    return (
      <EmptyState
        title="No favorites yet"
        body="Save confessions as you explore, and they will appear here."
        ctaHref="/app/explore"
        ctaLabel="Explore confessions"
      />
    );
  }

  return (
    <div style={{ display: "grid", gap: "var(--ic-spacing-6)" }}>
      {GROUPS.map(({ type, heading, glyph }) => {
        const rows = list.filter((f) => f.entity_type === type);
        if (rows.length === 0) return null;
        return (
          <section key={type} aria-labelledby={`fav-${type}-heading`}>
            <h2 id={`fav-${type}-heading`} className="ia-section-title">
              {heading} <span style={{ color: "var(--ic-color-neutral-500)", fontWeight: "var(--ic-font-weight-regular)" }}>({rows.length})</span>
            </h2>
            <ul className="ip-queue" style={{ listStyle: "none" }}>
              {rows.map((f) => {
                const href = hrefFor(f);
                const label = f.title || f.subtitle || "Saved item";
                return (
                  <li key={f.id} className="ip-queue__item">
                    <span className="ip-queue__num" aria-hidden="true">{glyph}</span>
                    <span style={{ flex: 1 }}>
                      {href && !f.missing ? (
                        <a href={href} style={{ textDecoration: "none", color: "inherit" }}>
                          <strong style={{ fontWeight: "var(--ic-font-weight-medium)" }}>{label}</strong>
                        </a>
                      ) : (
                        <strong style={{ fontWeight: "var(--ic-font-weight-medium)", color: "var(--ic-color-neutral-500)" }}>{label}</strong>
                      )}
                      <span style={{ display: "block", color: "var(--ic-color-neutral-500)", fontSize: "var(--ic-font-size-caption)" }}>
                        {[f.subtitle, `Saved ${formatDate(f.created_at)}`].filter(Boolean).join(" · ")}
                      </span>
                      {f.missing && (
                        <span style={{ display: "block", fontSize: "var(--ic-font-size-caption)", color: "var(--ic-color-neutral-600)", marginTop: "var(--ic-spacing-1)" }}>
                          No longer available — the content behind this save has been removed.
                        </span>
                      )}
                    </span>
                  </li>
                );
              })}
            </ul>
          </section>
        );
      })}

      {/* Rows of an unknown kind still belong to the account; they render
          rather than vanish, and say honestly that there is nowhere to open
          them. */}
      {list.some((f) => !GROUPS.some((g) => g.type === f.entity_type)) && (
        <section aria-labelledby="fav-other-heading">
          <h2 id="fav-other-heading" className="ia-section-title">Other saves</h2>
          <ul className="ip-queue" style={{ listStyle: "none" }}>
            {list
              .filter((f) => !GROUPS.some((g) => g.type === f.entity_type))
              .map((f) => (
                <li key={f.id} className="ip-queue__item">
                  <span className="ip-queue__num" aria-hidden="true">·</span>
                  <span style={{ flex: 1 }}>
                    <strong style={{ fontWeight: "var(--ic-font-weight-medium)" }}>{f.title || "Saved item"}</strong>
                    <span style={{ display: "block", color: "var(--ic-color-neutral-500)", fontSize: "var(--ic-font-size-caption)" }}>
                      {f.entity_type} · Saved {formatDate(f.created_at)}
                    </span>
                  </span>
                </li>
              ))}
          </ul>
        </section>
      )}
    </div>
  );
}
