"use client";

/**
 * /app — the personalised home (§29).
 *
 * Assembled from GET /home (continue listening, recent sessions, categories)
 * and GET /recommendations (ranked categories + confessions). Signed-out
 * visitors get the honest signed-out state — never a skeleton impersonating
 * a feed. Labels follow the evidence: "Recommended for you" only when the
 * server says the ranking is personalised, otherwise "Suggested starting
 * points".
 */

import { useEffect, useRef } from "react";
import Link from "next/link";
import { useAuth } from "@/lib/auth-context";
import { formatClock, useApiData } from "@/lib/app-api";
import { ANALYTICS_EVENTS, track } from "@/lib/analytics";
import type { Category, Confession } from "@/lib/api";
import type { Session } from "@/components/PlayerClient";
import { EmptyState, ErrorBlock, LoadingBlock, SignedOut } from "@/components/app-ui";

type HomeFeed = {
  continue: Session | null;
  recent_sessions?: Session[];
  categories?: Category[];
};

type Recommendations = {
  categories?: Category[];
  confessions?: Confession[];
  personalized?: boolean;
  count?: number;
};

function greeting(): string {
  const h = new Date().getHours();
  if (h < 12) return "Good morning";
  if (h < 18) return "Good afternoon";
  return "Good evening";
}

function sessionLabel(s: Session): string {
  if (s.title) return s.title;
  const mins = Math.round((s.actual_duration || s.duration_seconds || 0) / 60);
  return mins > 0 ? `${mins}-minute session` : "A session";
}

export function AppHomeClient() {
  const { user, token, loading: authLoading } = useAuth();
  const home = useApiData<HomeFeed>(token, "/home");
  const recs = useApiData<Recommendations>(token, "/recommendations");
  const opened = useRef(false);

  useEffect(() => {
    if (token && !opened.current) {
      opened.current = true;
      track(token, ANALYTICS_EVENTS.appOpened);
    }
  }, [token]);

  if (authLoading || home.loading) return <LoadingBlock rows={5} />;
  if (!token) return <SignedOut next="/app" />;
  if (home.error) return <ErrorBlock message={home.error.message} onRetry={() => { home.reload(); recs.reload(); }} />;

  const feed = home.data;
  const resumable = feed?.continue ?? null;
  const recent = (feed?.recent_sessions ?? []).filter((s) => !resumable || s.id !== resumable.id).slice(0, 5);
  const categories = feed?.categories ?? [];
  const rankedCats = recs.data?.categories ?? [];
  const rankedConfs = recs.data?.confessions ?? [];
  const personalised = recs.data?.personalized === true;
  const today = rankedConfs[0] ?? null;
  const firstName = user?.display_name?.split(" ")[0] || "friend";

  return (
    <div style={{ display: "grid", gap: "var(--ic-spacing-7)" }}>
      <div>
        <p className="ia-section-title">{greeting()}, {firstName}</p>
        <h1 style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-font-size-heading)", marginTop: "var(--ic-spacing-2)" }}>
          What do you want to return to today?
        </h1>
      </div>

      {resumable && (
        <section aria-label="Continue listening">
          <Link
            href={`/app/player?session=${encodeURIComponent(resumable.id)}`}
            className="ic-card ic-card--hover"
            style={{ display: "block", padding: "var(--ic-spacing-6)", textDecoration: "none", background: "var(--ic-color-brand-800)", borderColor: "var(--ic-color-brand-800)", color: "#fff" }}
          >
            <p style={{ fontSize: "var(--ic-font-size-caption)", letterSpacing: "0.08em", textTransform: "uppercase", color: "var(--ic-color-brand-200)" }}>
              Continue listening
            </p>
            <p style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-font-size-subheading)", marginTop: "var(--ic-spacing-2)" }}>
              {sessionLabel(resumable)}
            </p>
            <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-brand-100)", marginTop: "var(--ic-spacing-1)" }}>
              Pick up where you left off →
            </p>
          </Link>
        </section>
      )}

      {today && (
        <section aria-label="Today's experience">
          <h2 className="ia-section-title">Today&apos;s experience</h2>
          <Link
            href={`/app/confessions/${today.id}`}
            className="ic-card ic-card--hover"
            style={{ display: "block", padding: "var(--ic-spacing-6)", textDecoration: "none", marginTop: "var(--ic-spacing-3)" }}
          >
            <p style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-font-size-subheading)" }}>{today.title}</p>
            {(today.short_text || today.medium_text) && (
              <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", marginTop: "var(--ic-spacing-2)", display: "-webkit-box", WebkitLineClamp: 2, WebkitBoxOrient: "vertical", overflow: "hidden" }}>
                {today.short_text || today.medium_text}
              </p>
            )}
          </Link>
        </section>
      )}

      <div className="ia-quick" aria-label="Quick actions">
        <Link href="/app/bible" className="ia-quick__btn"><span className="ia-quick__icon">▤</span> Bible</Link>
        <Link href="/app/explore" className="ia-quick__btn"><span className="ia-quick__icon">◎</span> Explore</Link>
        <Link href="/app/session-builder" className="ia-quick__btn"><span className="ia-quick__icon">▷</span> New session</Link>
        <Link href="/app/community/create" className="ia-quick__btn"><span className="ia-quick__icon">+</span> Write</Link>
        <Link href="/app/schedule" className="ia-quick__btn"><span className="ia-quick__icon">◷</span> Schedule</Link>
        <Link href="/app/favorites" className="ia-quick__btn"><span className="ia-quick__icon">♡</span> Favorites</Link>
        <Link href="/app/search" className="ia-quick__btn"><span className="ia-quick__icon">⌕</span> Search</Link>
      </div>

      {categories.length > 0 && (
        <section>
          <h2 className="ia-section-title">Explore categories</h2>
          <div className="ic-grid ic-grid--3" style={{ marginTop: "var(--ic-spacing-3)" }}>
            {categories.slice(0, 6).map((c) => (
              <Link key={c.id} href={`/app/categories/${c.slug}`} className="ic-card ic-card--hover" style={{ padding: "var(--ic-spacing-5)", textDecoration: "none" }}>
                <h3 style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-font-size-subheading)", color: "var(--ic-color-neutral-950)" }}>{c.name}</h3>
                <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", marginTop: "var(--ic-spacing-2)" }}>{c.description}</p>
              </Link>
            ))}
          </div>
          <Link href="/app/categories" className="ic-btn ic-btn--secondary" style={{ marginTop: "var(--ic-spacing-4)" }}>
            See all {categories.length} categories
          </Link>
        </section>
      )}

      {(rankedCats.length > 0 || rankedConfs.length > 1) && (
        <section>
          <h2 className="ia-section-title">{personalised ? "Recommended for you" : "Suggested starting points"}</h2>
          <div className="ic-grid ic-grid--3" style={{ marginTop: "var(--ic-spacing-3)" }}>
            {rankedCats.slice(0, 3).map((c) => (
              <Link key={c.id} href={`/app/categories/${c.slug}`} className="ic-card ic-card--hover" style={{ padding: "var(--ic-spacing-5)", textDecoration: "none" }}>
                <h3 style={{ fontSize: "var(--ic-font-size-body)", fontWeight: "var(--ic-font-weight-semibold)" }}>{c.name}</h3>
                <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", marginTop: "var(--ic-spacing-2)" }}>{c.description}</p>
              </Link>
            ))}
            {rankedConfs.slice(1, 3).map((c) => (
              <Link key={c.id} href={`/app/confessions/${c.id}`} className="ic-card ic-card--hover" style={{ padding: "var(--ic-spacing-5)", textDecoration: "none" }}>
                <h3 style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-font-size-body)" }}>{c.title}</h3>
                <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", marginTop: "var(--ic-spacing-2)" }}>A confession to return to</p>
              </Link>
            ))}
          </div>
          <Link href="/app/recommendations" className="ic-btn ic-btn--text" style={{ marginTop: "var(--ic-spacing-3)" }}>
            More recommendations →
          </Link>
        </section>
      )}

      <section>
        <h2 className="ia-section-title">Your rhythm</h2>
        <div className="ic-grid ic-grid--3" style={{ marginTop: "var(--ic-spacing-3)" }}>
          <Link href="/app/routines" className="ic-card ic-card--hover" style={{ padding: "var(--ic-spacing-5)", textDecoration: "none" }}>
            <h3 style={{ fontSize: "var(--ic-font-size-body)", fontWeight: "var(--ic-font-weight-semibold)" }}>Routines</h3>
            <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", marginTop: "var(--ic-spacing-2)" }}>Morning, midday, evening.</p>
          </Link>
          <Link href="/app/schedule" className="ic-card ic-card--hover" style={{ padding: "var(--ic-spacing-5)", textDecoration: "none" }}>
            <h3 style={{ fontSize: "var(--ic-font-size-body)", fontWeight: "var(--ic-font-weight-semibold)" }}>Schedule</h3>
            <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", marginTop: "var(--ic-spacing-2)" }}>Sessions that build themselves.</p>
          </Link>
          <Link href="/app/streaks" className="ic-card ic-card--hover" style={{ padding: "var(--ic-spacing-5)", textDecoration: "none" }}>
            <h3 style={{ fontSize: "var(--ic-font-size-body)", fontWeight: "var(--ic-font-weight-semibold)" }}>Streaks</h3>
            <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", marginTop: "var(--ic-spacing-2)" }}>The return, counted gently.</p>
          </Link>
        </div>
      </section>

      {recent.length > 0 && (
        <section>
          <h2 className="ia-section-title">Recently played</h2>
          <div style={{ display: "grid", gap: "var(--ic-spacing-2)", marginTop: "var(--ic-spacing-3)" }}>
            {recent.map((s) => (
              <Link
                key={s.id}
                href={`/app/player?session=${encodeURIComponent(s.id)}`}
                className="ic-card ic-card--hover"
                style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline", gap: "var(--ic-spacing-4)", padding: "var(--ic-spacing-4) var(--ic-spacing-5)", textDecoration: "none" }}
              >
                <span style={{ fontSize: "var(--ic-font-size-bodySm)", fontWeight: "var(--ic-font-weight-medium)" }}>{sessionLabel(s)}</span>
                <span style={{ fontSize: "var(--ic-font-size-caption)", color: "var(--ic-color-neutral-500)", flexShrink: 0 }}>
                  {formatClock(s.actual_duration || s.duration_seconds || 0)} · {s.status.toLowerCase()}
                </span>
              </Link>
            ))}
          </div>
          <Link href="/app/history" className="ic-btn ic-btn--text" style={{ marginTop: "var(--ic-spacing-3)" }}>
            Full history →
          </Link>
        </section>
      )}

      {!resumable && recent.length === 0 && (
        <EmptyState
          title="Your practice starts here"
          body="Build your first session — five minutes is enough to begin."
          ctaHref="/app/session-builder"
          ctaLabel="Build a session"
        />
      )}
    </div>
  );
}


