import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";
import { SectionHead, ConfessionCard } from "@/components/Sections";
import { api } from "@/lib/api";
import { SITE_URL } from "@/lib/site";

/**
 * Public session starters.
 *
 * Sessions are private by design — POST /sessions requires an account and
 * GET /sessions/{id} only ever serves the owner's rows. A public page that
 * took a session id would therefore be a promise the API cannot keep (and a
 * shape that invites ID enumeration). Instead this route serves curated
 * *starters*: the category, length and shape of a session anyone can build
 * for themselves, with a real sample confession from the library.
 *
 * Slugs are editorial presets over live catalogue data — the category copy
 * and the sample come from the API at request time, so a starter can never
 * describe content that does not exist.
 */

type Starter = {
  slug: string;
  categorySlug: string;
  minutes: number;
  title: string;
  lede: string;
};

const STARTERS: Starter[] = [
  {
    slug: "morning-peace",
    categorySlug: "peace",
    minutes: 5,
    title: "Morning peace",
    lede: "Five unhurried minutes before the noise begins. One area of life, one voice, words worth carrying into the day.",
  },
  {
    slug: "healing-at-midday",
    categorySlug: "healing",
    minutes: 10,
    title: "Healing at midday",
    lede: "A ten-minute reset in the middle of everything — spoken words for the body and the heart.",
  },
  {
    slug: "evening-rest",
    categorySlug: "peace",
    minutes: 15,
    title: "Evening rest",
    lede: "Fifteen minutes to release the day. The words slow down so you can too.",
  },
  {
    slug: "faith-for-the-week",
    categorySlug: "faith",
    minutes: 10,
    title: "Faith for the week",
    lede: "Begin the week by saying what you believe out loud — and meaning it a little more each time.",
  },
];

function starterFor(slug: string): Starter | undefined {
  return STARTERS.find((s) => s.slug === slug);
}

export async function generateStaticParams() {
  return STARTERS.map((s) => ({ slug: s.slug }));
}

export async function generateMetadata({ params }: { params: Promise<{ slug: string }> }): Promise<Metadata> {
  const { slug } = await params;
  const starter = starterFor(slug);
  if (!starter) notFound();
  return {
    title: `${starter.title} — a ${starter.minutes}-minute session starter`,
    description: starter.lede,
    alternates: { canonical: `/sessions/${starter.slug}` },
    openGraph: {
      title: `${starter.title} — iCONFESS`,
      description: starter.lede,
      url: `${SITE_URL}/sessions/${starter.slug}`,
      type: "article",
    },
  };
}

export default async function SessionStarterPage({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params;
  const starter = starterFor(slug);
  if (!starter) notFound();

  const catsRes = await api.categories();
  const categories = catsRes.ok ? catsRes.data : [];
  const category = categories.find((c) => c.slug === starter.categorySlug) ?? categories[0];

  const sampleRes = category ? await api.categoryConfessions(category.id) : null;
  const sample = sampleRes && sampleRes.ok ? sampleRes.data[0] : undefined;

  const builderHref = category
    ? `/app/session-builder?category=${encodeURIComponent(category.slug)}`
    : "/app/session-builder";

  return (
    <div>
      <SiteHeader light />
      <main id="main">
        <section className="ic-page-hero on-ink">
          <div className="ic-container">
            <p className="ic-eyebrow">Session starter · {starter.minutes} minutes</p>
            <h1 style={{ marginTop: "var(--ic-spacing-4)" }}>{starter.title}.</h1>
            <p className="ic-lede" style={{ marginTop: "var(--ic-spacing-5)" }}>{starter.lede}</p>
            <div className="ic-btn-row" style={{ marginTop: "var(--ic-spacing-6)" }}>
              <Link href={builderHref} className="ic-btn ic-btn--on-dark">
                Build this session
              </Link>
              <Link href="/sessions" className="ic-btn ic-btn--secondary on-ink">
                How sessions work
              </Link>
            </div>
          </div>
        </section>

        <section className="ic-section">
          <div className="ic-container">
            <SectionHead
              eyebrow="What you'll hear"
              title="A sample from this session's shelf."
              lede={
                category
                  ? `Built from ${category.name} — ${category.description ?? "words for this area of life"}. The full session assembles several confessions like this one around your ${starter.minutes} minutes.`
                  : "The full session assembles several confessions around your time."
              }
            />
            <div style={{ marginTop: "var(--ic-spacing-6)", maxWidth: "44rem" }}>
              {sample ? (
                <ConfessionCard confession={sample} category={category} />
              ) : (
                <div className="ic-state" role="status">
                  <h3>The library didn't answer</h3>
                  <p>This starter's sample couldn't load. The session builder will still assemble it from the live library.</p>
                  <Link href={builderHref} className="ic-btn ic-btn--secondary">
                    Build it anyway
                  </Link>
                </div>
              )}
            </div>
          </div>
        </section>

        <section className="ic-section ic-section--mist">
          <div className="ic-container">
            <SectionHead eyebrow="More starters" title="Begin somewhere else." />
            <div className="ic-grid ic-grid--3" style={{ marginTop: "var(--ic-spacing-6)" }}>
              {STARTERS.filter((s) => s.slug !== starter.slug).map((s) => (
                <Link key={s.slug} href={`/sessions/${s.slug}`} className="ic-card ic-card--hover" style={{ padding: "var(--ic-spacing-6)", textDecoration: "none", display: "grid", gap: "var(--ic-spacing-3)" }}>
                  <span className="ic-frame__kicker">{s.minutes} minutes</span>
                  <h3 style={{ fontSize: "var(--ic-font-size-subheading)", fontWeight: "var(--ic-font-weight-semibold)" }}>{s.title}</h3>
                  <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)" }}>{s.lede}</p>
                </Link>
              ))}
            </div>
          </div>
        </section>

        <section className="ic-section ic-section--ink on-ink" style={{ textAlign: "center" }}>
          <div className="ic-container ic-container--text">
            <h2 className="ic-display">Sessions are yours to keep.</h2>
            <p className="ic-lede" style={{ margin: "var(--ic-spacing-5) auto 0" }}>
              Build one in under a minute. Save it, schedule it, repeat it — the practice is the return.
            </p>
            <div className="ic-btn-row" style={{ justifyContent: "center", marginTop: "var(--ic-spacing-7)" }}>
              <Link href={builderHref} className="ic-btn ic-btn--on-dark">
                Build this session
              </Link>
            </div>
          </div>
        </section>
      </main>
      <SiteFooter />
    </div>
  );
}
