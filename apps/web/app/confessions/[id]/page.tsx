import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";
import { ConfessionCard } from "@/components/Sections";
import { api } from "@/lib/api";
import { SITE_URL } from "@/lib/site";

/**
 * Confession detail.
 *
 * The public endpoint only serves published canonical content; user
 * (UGC) confessions are never reachable here. Indexing follows content
 * status automatically for that reason — a confession page that exists is
 * a published confession.
 */
export async function generateMetadata({
  params,
}: {
  params: Promise<{ id: string }>;
}): Promise<Metadata> {
  const { id } = await params;
  const res = await api.confession(id);
  if (!res.ok) notFound(); // during metadata generation this yields a real 404 status
  const c = res.data;
  return {
    title: c.title,
    description:
      c.short_text ||
      c.description ||
      `A spoken confession${c.scriptures?.length ? ` standing on ${c.scriptures[0].book}` : ""} from the iCONFESS library.`,
    alternates: { canonical: `/confessions/${c.id}` },
    openGraph: {
      title: `${c.title} — iCONFESS`,
      description: c.short_text || c.description || c.title,
      url: `${SITE_URL}/confessions/${c.id}`,
      type: "article",
    },
  };
}

export default async function ConfessionPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const res = await api.confession(id);
  if (!res.ok) notFound();
  const c = res.data;

  const [catsRes, voicesRes] = await Promise.all([api.categories(), api.voices()]);
  const category = catsRes.ok ? catsRes.data.find((x) => x.id === c.category_id) : undefined;
  const voices = voicesRes.ok ? voicesRes.data : [];

  const relatedRes = category ? await api.categoryConfessions(category.id) : null;
  const related = (relatedRes?.ok ? relatedRes.data : [])
    .filter((x) => x.id !== c.id)
    .slice(0, 3);

  const body =
    c.long_text || c.medium_text || c.short_text || c.description || "";

  const jsonLd = {
    "@context": "https://schema.org",
    "@type": "Article",
    headline: c.title,
    text: body,
    articleSection: category?.name,
    url: `${SITE_URL}/confessions/${c.id}`,
  };

  return (
    <div>
      <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd) }} />
      <SiteHeader />
      <main id="main">
        <section className="ic-page-hero on-ink">
          <div className="ic-container">
            <Link href={category ? `/categories/${category.slug}` : "/explore"} className="ic-backlink">
              ← {category ? category.name : "Explore"}
            </Link>
            {category && <p className="ic-eyebrow">{category.name}</p>}
            <h1 style={{ marginTop: "var(--ic-spacing-4)" }}>{c.title}</h1>
            {c.description && (
              <p className="ic-lede" style={{ marginTop: "var(--ic-spacing-5)" }}>{c.description}</p>
            )}
          </div>
        </section>

        <section className="ic-section">
          <div className="ic-container ic-container--text">
            {/* The words */}
            <div className="ic-card" style={{ padding: "clamp(var(--ic-spacing-6), 5vw, var(--ic-spacing-8))" }}>
              <div className="ic-confession-card__kicker" style={{ marginBottom: "var(--ic-spacing-5)" }}>
                <span>The words</span>
                {c.variants && c.variants.length > 0 && (
                  <span>
                    {Math.round(
                      Math.min(...c.variants.map((v) => v.duration_seconds)) / 60,
                    )}–{Math.round(Math.max(...c.variants.map((v) => v.duration_seconds)) / 60)} min
                  </span>
                )}
              </div>
              <p className="ic-scripture">{body}</p>
              {c.scriptures && c.scriptures.length > 0 && (
                <div style={{ marginTop: "var(--ic-spacing-6)", borderTop: "1px solid var(--ic-color-neutral-200)", paddingTop: "var(--ic-spacing-5)" }}>
                  <p className="ic-frame__kicker" style={{ marginBottom: "var(--ic-spacing-3)" }}>Standing on</p>
                  <ul role="list" style={{ display: "grid", gap: "var(--ic-spacing-2)" }}>
                    {c.scriptures.map((s) => (
                      <li key={s.id} style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-700)" }}>
                        <strong>
                          {s.book}
                          {s.chapter ? ` ${s.chapter}` : ""}
                          {s.verse ? `:${s.verse}` : ""}
                        </strong>{" "}
                        ({s.translation}
                        {s.is_direct_quote ? ", direct quotation" : ", allusion"})
                      </li>
                    ))}
                  </ul>
                </div>
              )}
            </div>

            {/* The experience */}
            <div className="ic-frame" style={{ marginTop: "var(--ic-spacing-6)" }}>
              <div className="ic-frame__bar" aria-hidden="true">
                <span className="ic-frame__dot" /><span className="ic-frame__dot" /><span className="ic-frame__dot" />
                <span style={{ marginLeft: "auto" }}>hear it spoken</span>
              </div>
              <div className="ic-frame__body">
                <span className="ic-frame__kicker">A spoken experience</span>
                <p style={{ color: "var(--ic-color-neutral-300)", fontSize: "var(--ic-font-size-bodySm)", lineHeight: "var(--ic-font-lineHeight-relaxed)" }}>
                  This confession is part of guided sessions — spoken in a curated
                  voice, paced, and paired with the Scripture it stands on. Sign
                  in to hear it.
                </p>
                <div style={{ display: "flex", gap: "var(--ic-spacing-3)", flexWrap: "wrap", alignItems: "center" }}>
                  {voices[0] && (
                    <span style={{ fontSize: "var(--ic-font-size-caption)", color: "var(--ic-color-neutral-400)" }}>
                      Voice — {voices[0].name}
                    </span>
                  )}
                  <Link href="/register" className="ic-btn ic-btn--on-dark ic-btn--small" style={{ marginLeft: "auto" }}>
                    Start your experience
                  </Link>
                </div>
              </div>
            </div>
          </div>
        </section>

        {related.length > 0 && (
          <section className="ic-section ic-section--mist" aria-labelledby="conf-related">
            <div className="ic-container">
              <h2 className="ic-title" id="conf-related" style={{ marginBottom: "var(--ic-spacing-7)" }}>
                Continue your experience
              </h2>
              <div className="ic-grid ic-grid--3">
                {related.map((r) => (
                  <ConfessionCard key={r.id} confession={r} category={category} />
                ))}
              </div>
            </div>
          </section>
        )}
      </main>
      <SiteFooter />
    </div>
  );
}
