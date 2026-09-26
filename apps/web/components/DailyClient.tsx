"use client";

/**
 * /app/daily — today's experience, from GET /home + GET /recommendations.
 *
 * /home is assembled server-side from what already exists in the account:
 * the resumable session, recent sessions, collections, and the category
 * catalogue. Recommendations is the one surface allowed to *suggest* — and
 * it says so honestly: the API returns `personalized: false` when there is
 * not enough stated or behavioural signal, and the heading below changes to
 * match, instead of dressing up a fallback as a recommendation.
 *
 * There is no fabricated "one confession a day" row, because no such backend
 * concept exists yet; what exists — an interrupted session worth resuming,
 * or a place to start — is what renders.
 */

import { useAuth } from "@/lib/auth-context";
import { useApiData, formatDate } from "@/lib/app-api";
import type { Category } from "@/lib/api";
import { ErrorBlock, LoadingBlock, SignedOut } from "@/components/app-ui";
import type { Session } from "@/components/PlayerClient";

type HomeResponse = {
  continue?: Session | null;
  recent_sessions?: Session[];
  collections?: { id: string; name: string; item_count?: number }[];
  categories?: Category[];
};

type Recommendation = {
  categories: (Category & { id: string })[];
  confessions: { id: string; title: string; category_id: string; description?: string; short_text?: string }[];
  count: number;
  personalized: boolean;
  listen_again?: { confession: { id: string; title: string }; times: number }[];
};

export function DailyClient() {
  const { token, loading: authLoading, user } = useAuth();
  const home = useApiData<HomeResponse>(token, "/home");
  const recs = useApiData<Recommendation>(token, "/recommendations");

  if (authLoading) return <LoadingBlock rows={3} />;
  if (!token) return <SignedOut next="/app/daily" />;
  if (home.loading && !home.data) return <LoadingBlock rows={3} />;
  if (home.error && !home.data) {
    if (home.error.status === 401) return <SignedOut next="/app/daily" />;
    return <ErrorBlock message={home.error.message} onRetry={home.reload} />;
  }

  const h = home.data ?? {};
  const cont = h.continue ?? null;
  const r = recs.data ?? null;

  const firstName = (user?.display_name ?? "").trim().split(/\s+/)[0] ?? "";

  return (
    <div style={{ display: "grid", gap: "var(--ic-spacing-7)" }}>
      <section className="ic-today" aria-labelledby="today-heading">
        <p className="ic-today__label">Today{firstName ? ` · ${firstName}` : ""}</p>
        <h2 id="today-heading" style={{ margin: 0, fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-font-size-heading)", fontWeight: "var(--ic-font-weight-regular)", lineHeight: "var(--ic-font-lineHeight-snug)" }}>
          {cont
            ? "You left something unfinished. That's allowed."
            : "Nothing is waiting — start something."}
        </h2>
        {cont ? (
          <ResumeCard session={cont} />
        ) : (
          <div className="ic-btn-row">
            <a href="/app/session-builder" className="ic-btn ic-btn--primary">Build today&rsquo;s session</a>
            <a href="/app/explore" className="ic-btn ic-btn--secondary">Browse first</a>
          </div>
        )}
      </section>

      {(recs.loading && !r) ? (
        <LoadingBlock rows={2} />
      ) : recs.error && !r ? (
        <ErrorBlock message={recs.error.message} onRetry={recs.reload} />
      ) : r && (r.confessions.length > 0 || r.categories.length > 0) ? (
        <section aria-labelledby="recs-heading">
          <h2 id="recs-heading" className="ia-section-title">
            {r.personalized ? "Recommended for you" : "Suggested starting points"}
          </h2>
          <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", margin: "0 0 var(--ic-spacing-4)" }}>
            {r.personalized
              ? "From what you've said you want to focus on and where your listening has actually gone."
              : "No personal signal yet — this is the broad catalogue speaking. Set interests, or listen a few times, and it sharpens."}
          </p>
          <div className="ic-grid ic-grid--3">
            {r.confessions.slice(0, 3).map((c) => (
              <a key={c.id} href={`/app/confessions/${c.id}`} className="ic-card ic-card--hover" style={{ padding: "var(--ic-spacing-5)", textDecoration: "none" }}>
                <h3 style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-font-size-subheading)", margin: 0, color: "var(--ic-color-neutral-950)" }}>
                  {c.title}
                </h3>
                <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", marginTop: "var(--ic-spacing-2)" }}>
                  {(c.short_text ?? c.description ?? "").slice(0, 140)}
                </p>
              </a>
            ))}
          </div>
          {r.categories.length > 0 && (
            <div className="ic-chipgrid" style={{ marginTop: "var(--ic-spacing-4)" }} aria-label="Suggested categories">
              {r.categories.slice(0, 6).map((c) => (
                <a key={c.id} href={`/app/session-builder?category=${encodeURIComponent(c.id)}`} className="ic-chipgrid__chip" style={{ textDecoration: "none" }}>
                  {c.name}
                </a>
              ))}
            </div>
          )}
        </section>
      ) : null}

      {(h.recent_sessions?.length ?? 0) > 0 && (
        <section aria-labelledby="recent-heading">
          <h2 id="recent-heading" className="ia-section-title">This week</h2>
          <ul className="ip-queue" style={{ listStyle: "none" }}>
            {h.recent_sessions!.slice(0, 5).map((s) => (
              <li key={s.id} className="ip-queue__item">
                <span className="ip-queue__num" aria-hidden="true">▷</span>
                <a
                  href={`/app/player?session=${encodeURIComponent(s.id)}`}
                  style={{ flex: 1, textDecoration: "none", color: "inherit" }}
                >
                  <strong style={{ fontWeight: "var(--ic-font-weight-medium)" }}>
                    {s.title || `${Math.round((s.actual_duration || s.duration_seconds || 0) / 60)}-minute session`}
                  </strong>
                  <span style={{ display: "block", color: "var(--ic-color-neutral-500)", fontSize: "var(--ic-font-size-caption)" }}>
                    {formatDate(s.created_at)} · {s.status.toLowerCase()}
                  </span>
                </a>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}

function ResumeCard({ session }: { session: Session }) {
  const total = session.actual_duration || session.duration_seconds || 0;
  return (
    <div className="ip-queue__item" style={{ alignItems: "center" }}>
      <span className="ip-queue__num" aria-hidden="true">
        {session.status === "PAUSED" ? "❚❚" : "▷"}
      </span>
      <span style={{ flex: 1 }}>
        <strong style={{ fontWeight: "var(--ic-font-weight-medium)" }}>
          {session.title || "Your last session"}
        </strong>
        <span style={{ display: "block", color: "var(--ic-color-neutral-500)", fontSize: "var(--ic-font-size-caption)" }}>
          {session.items?.length ? `${session.items.length} items · ` : ""}up to {Math.round(total / 60)} min · started {formatDate(session.started_at || session.created_at)}
        </span>
      </span>
      <a href={`/app/player?session=${encodeURIComponent(session.id)}`} className="ic-btn ic-btn--primary ic-btn--small">
        Resume
      </a>
    </div>
  );
}
