"use client";

/**
 * Floating cards overlapping the homepage hero (motif M3).
 *
 * Public visitors see curated content passed from the server (today's
 * confession, a featured category, a featured voice). Signed-in visitors
 * additionally see their real continue-listening state from GET /home —
 * resolved client-side because the session token never leaves the browser.
 * If /home fails, the cards fall back to the curated set; a failed fetch
 * never blanks the hero.
 */

import Link from "next/link";
import { useAuth } from "@/lib/auth-context";
import { appApi } from "@/lib/app-api";

import { useEffect, useState } from "react";

export type CuratedCards = {
  confession: { id: string; title: string; categoryName: string } | null;
  category: { slug: string; name: string; description: string } | null;
  voice: { id: string; name: string; description: string } | null;
  categoryCount: number;
};

type HomeContinue = {
  continue: { id: string; title?: string; status?: string } | null;
};

function PlayGlyph() {
  return (
    <svg width="14" height="14" viewBox="0 0 18 18" aria-hidden="true">
      <path d="M5 3l10 6-10 6V3z" fill="currentColor" />
    </svg>
  );
}

export function HomeExperienceCards({ curated }: { curated: CuratedCards }) {
  const { token, loading } = useAuth();
  const [resume, setResume] = useState<HomeContinue["continue"] | undefined>(undefined);

  useEffect(() => {
    if (loading || !token) {
      if (!loading) setResume(null);
      return;
    }
    let cancelled = false;
    appApi<HomeContinue>("/home", { token }).then((r) => {
      if (cancelled) return;
      setResume(r.ok ? r.data.continue : null);
    });
    return () => {
      cancelled = true;
    };
  }, [loading, token]);

  const first = resume ? (
    <Link key="resume" href={`/app/sessions/${resume.id}`} className="ic-float-card">
      <span className="ic-float-card__kicker">Continue listening</span>
      <span className="ic-float-card__title">{resume.title || "Your session is waiting"}</span>
      <span className="ic-float-card__meta">
        <span className="ic-float-card__play"><PlayGlyph /></span> Pick up where you left off
      </span>
    </Link>
  ) : curated.confession ? (
    <Link key="today" href={`/confessions/${curated.confession.id}`} className="ic-float-card">
      <span className="ic-float-card__kicker">Today&apos;s confession · {curated.confession.categoryName}</span>
      <span className="ic-float-card__title">{curated.confession.title}</span>
      <span className="ic-float-card__meta">
        <span className="ic-float-card__play"><PlayGlyph /></span> Hear it spoken
      </span>
    </Link>
  ) : null;

  return (
    <div className="ic-float__grid">
      {first}
      {curated.category && (
        <Link href={`/categories/${curated.category.slug}`} className="ic-float-card">
          <span className="ic-float-card__kicker">Featured area of life</span>
          <span className="ic-float-card__title">{curated.category.name}</span>
          <span className="ic-float-card__meta">
            {curated.categoryCount > 0 ? `One of ${curated.categoryCount} areas of life` : "Explore the library"} →
          </span>
        </Link>
      )}
      {curated.voice && (
        <Link href={`/voices/${curated.voice.id}`} className="ic-float-card">
          <span className="ic-float-card__kicker">Featured voice</span>
          <span className="ic-float-card__title">{curated.voice.name}</span>
          <span className="ic-float-card__meta">Hear a sample →</span>
        </Link>
      )}
    </div>
  );
}
