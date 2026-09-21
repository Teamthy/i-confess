import type { Metadata } from "next";
import Link from "next/link";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";
import { SectionHead } from "@/components/Sections";
import { ARTICLES } from "@/content/articles";

export const metadata: Metadata = {
  title: "Journal",
  description:
    "Reflections on words, ritual, and the practice of speaking what you believe. The iCONFESS journal.",
  alternates: { canonical: "/journal" },
};

/**
 * The journal. Article content is a typed local content model
 * (apps/web/content/articles.ts) — presentation is separated from content and
 * the shape mirrors the future articles API. When the backend grows an
 * articles endpoint (audit §5), this page swaps its data source in one file.
 */
export default function JournalPage() {
  const [lead, ...rest] = ARTICLES;

  return (
    <div>
      <SiteHeader />
      <main id="main">
        <section className="ic-page-hero on-ink">
          <div className="ic-container">
            <p className="ic-eyebrow">Journal</p>
            <h1 style={{ marginTop: "var(--ic-spacing-4)" }}>Notes on words and ritual.</h1>
            <p className="ic-lede" style={{ marginTop: "var(--ic-spacing-5)" }}>
              Short essays from the team making iCONFESS — on language,
              repetition, and building a practice that lasts.
            </p>
          </div>
        </section>

        <section className="ic-section">
          <div className="ic-container">
            <Link
              href={`/journal/${lead.slug}`}
              className="ic-card ic-card--hover"
              style={{ display: "grid", gap: "var(--ic-spacing-4)", padding: "clamp(var(--ic-spacing-6), 5vw, var(--ic-spacing-8))", textDecoration: "none", background: "var(--ic-color-neutral-950)", borderColor: "var(--ic-color-neutral-950)", color: "var(--ic-color-neutral-100)" }}
            >
              <span className="ic-frame__kicker">{lead.category}</span>
              <h2 style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-web-title)", lineHeight: 1.2, color: "var(--ic-color-neutral-0)", maxWidth: "24ch" }}>
                {lead.title}
              </h2>
              <p style={{ color: "var(--ic-color-neutral-400)", maxWidth: "60ch", lineHeight: "var(--ic-font-lineHeight-relaxed)" }}>
                {lead.excerpt}
              </p>
              <footer style={{ fontSize: "var(--ic-font-size-caption)", color: "var(--ic-color-neutral-500)" }}>
                {lead.author} · {lead.date} · {lead.readingTime}
              </footer>
            </Link>
          </div>
        </section>

        <section className="ic-section ic-section--mist">
          <div className="ic-container">
            <SectionHead eyebrow="More reading" title="From the journal." />
            <div className="ic-grid ic-grid--3">
              {rest.map((a) => (
                <Link key={a.slug} href={`/journal/${a.slug}`} className="ic-card ic-card--hover ic-confession-card" style={{ textDecoration: "none" }}>
                  <span className="ic-confession-card__kicker">{a.category}</span>
                  <h3>{a.title}</h3>
                  <p style={{ fontFamily: "var(--ic-font-family-sans)" }}>{a.excerpt}</p>
                  <footer>
                    <span>{a.author}</span>
                    <span>{a.readingTime}</span>
                  </footer>
                </Link>
              ))}
            </div>
          </div>
        </section>
      </main>
      <SiteFooter />
    </div>
  );
}
