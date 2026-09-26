import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";
import { ARTICLES } from "@/content/articles";
import { SITE_URL } from "@/lib/site";

export function generateStaticParams() {
  return ARTICLES.map((a) => ({ slug: a.slug }));
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{ slug: string }>;
}): Promise<Metadata> {
  const { slug } = await params;
  const article = ARTICLES.find((a) => a.slug === slug);
  if (!article) return { title: "Article not found", robots: { index: false } };
  return {
    title: article.title,
    description: article.excerpt,
    alternates: { canonical: `/journal/${article.slug}` },
    openGraph: {
      title: article.title,
      description: article.excerpt,
      url: `${SITE_URL}/journal/${article.slug}`,
      type: "article",
      publishedTime: article.date,
      authors: [article.author],
    },
  };
}

export default async function ArticlePage({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  const article = ARTICLES.find((a) => a.slug === slug);
  if (!article) notFound();

  const more = ARTICLES.filter((a) => a.slug !== slug).slice(0, 2);

  const jsonLd = {
    "@context": "https://schema.org",
    "@type": "BlogPosting",
    headline: article.title,
    description: article.excerpt,
    author: { "@type": "Organization", name: "iCONFESS" },
    datePublished: article.date,
    url: `${SITE_URL}/journal/${article.slug}`,
  };

  return (
    <div>
      <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd) }} />
      <SiteHeader />
      <main id="main">
        <section className="ic-page-hero on-ink">
          <div className="ic-container">
            <Link href="/journal" className="ic-backlink">← Journal</Link>
            <p className="ic-eyebrow">{article.category}</p>
            <h1 style={{ marginTop: "var(--ic-spacing-4)", maxWidth: "22ch" }}>{article.title}</h1>
            <p style={{ marginTop: "var(--ic-spacing-5)", fontSize: "var(--ic-font-size-caption)", color: "var(--ic-color-neutral-400)" }}>
              {article.author} · <time dateTime={article.date}>{article.date}</time> · {article.readingTime}
            </p>
          </div>
        </section>

        <section className="ic-section">
          <div className="ic-container ic-container--text ic-prose">
            {article.body.map((p, i) => (
              <p key={i}>{p}</p>
            ))}
          </div>
        </section>

        {more.length > 0 && (
          <section className="ic-section ic-section--mist">
            <div className="ic-container">
              <h2 className="ic-title" style={{ marginBottom: "var(--ic-spacing-7)" }}>Keep reading</h2>
              <div className="ic-grid ic-grid--3">
                {more.map((a) => (
                  <Link key={a.slug} href={`/journal/${a.slug}`} className="ic-card ic-card--hover ic-confession-card" style={{ textDecoration: "none" }}>
                    <span className="ic-confession-card__kicker">{a.category}</span>
                    <h3>{a.title}</h3>
                    <p style={{ fontFamily: "var(--ic-font-family-sans)" }}>{a.excerpt}</p>
                  </Link>
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
