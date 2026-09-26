import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";
import { ConfessionCard } from "@/components/Sections";
import { api } from "@/lib/api";
import { SITE_URL } from "@/lib/site";

/**
 * Category detail. The slug resolves against the live category list; the
 * confession list comes straight from the backend. Only published categories
 * resolve here, because the public API only serves published categories.
 */
export async function generateStaticParams() {
  const res = await api.categories();
  return res.ok ? res.data.map((c) => ({ slug: c.slug })) : [];
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{ slug: string }>;
}): Promise<Metadata> {
  const { slug } = await params;
  const res = await api.category(slug);
  if (!res.ok) notFound(); // 404 status, not a 200 shell
  const c = res.data;
  return {
    title: `${c.name}`,
    description:
      c.description ||
      `Confessions and sessions in ${c.name} — one of 39 areas of life on iCONFESS.`,
    alternates: { canonical: `/categories/${c.slug}` },
    openGraph: {
      title: `${c.name} — iCONFESS`,
      description: c.description || `Spoken confessions in ${c.name}.`,
      url: `${SITE_URL}/categories/${c.slug}`,
    },
  };
}

export default async function CategoryDetailPage({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  const catRes = await api.category(slug);
  if (!catRes.ok) notFound();
  const category = catRes.data;

  const confRes = await api.categoryConfessions(category.id);
  const confessions = confRes.ok ? confRes.data : [];
  const featured = confessions[0];
  const rest = confessions.slice(1);

  // Related: the next categories in catalogue order, which is a deliberate
  // editorial order rather than a random pick.
  const allRes = await api.categories();
  const related = allRes.ok
    ? allRes.data.filter((c) => c.id !== category.id).slice(0, 3)
    : [];

  const jsonLd = {
    "@context": "https://schema.org",
    "@type": "CollectionPage",
    name: category.name,
    description: category.description,
    url: `${SITE_URL}/categories/${category.slug}`,
    hasPart: confessions.slice(0, 10).map((c) => ({
      "@type": "Article",
      headline: c.title,
      url: `${SITE_URL}/confessions/${c.id}`,
    })),
  };

  return (
    <div>
      <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd) }} />
      <SiteHeader />
      <main id="main">
        <section className="ic-page-hero on-ink">
          <div className="ic-container">
            <Link href="/categories" className="ic-backlink">
              ← All categories
            </Link>
            <p className="ic-eyebrow">
              {category.premium ? "Premium area of life" : "Area of life"}
            </p>
            <h1 style={{ marginTop: "var(--ic-spacing-4)" }}>{category.name}</h1>
            {category.description && (
              <p className="ic-lede" style={{ marginTop: "var(--ic-spacing-5)" }}>
                {category.description}
              </p>
            )}
            <div className="ic-btn-row" style={{ marginTop: "var(--ic-spacing-6)" }}>
              {featured && (
                <Link href={`/confessions/${featured.id}`} className="ic-btn ic-btn--on-dark">
                  Hear featured confession
                </Link>
              )}
              <Link href="/register" className="ic-btn ic-btn--secondary on-ink">
                Begin a session
              </Link>
            </div>
          </div>
        </section>

        <section className="ic-section" aria-labelledby="cat-confessions">
          <div className="ic-container">
            <h2 className="ic-title" id="cat-confessions" style={{ marginBottom: "var(--ic-spacing-7)" }}>
              In {category.name}
            </h2>
            {confRes.ok && confessions.length > 0 ? (
              <>
                <div className="ic-grid ic-grid--3">
                  {(featured ? [featured, ...rest.slice(0, 5)] : rest.slice(0, 6)).map((c) => (
                    <ConfessionCard key={c.id} confession={c} category={category} />
                  ))}
                </div>
                {rest.length > 5 && (
                  <p style={{ marginTop: "var(--ic-spacing-6)", fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-500)" }}>
                    And {rest.length - 5} more in the app.
                  </p>
                )}
              </>
            ) : (
              <div className="ic-state">
                <h3>Confessions are on their way</h3>
                <p>This area of life is being written and reviewed. Choose another to begin today.</p>
                <Link href="/categories" className="ic-btn ic-btn--secondary">Browse categories</Link>
              </div>
            )}
          </div>
        </section>

        {related.length > 0 && (
          <section className="ic-section ic-section--mist" aria-labelledby="related-title">
            <div className="ic-container">
              <h2 className="ic-title" id="related-title" style={{ marginBottom: "var(--ic-spacing-7)" }}>
                Related areas
              </h2>
              <div className="ic-grid ic-grid--3">
                {related.map((c) => (
                  <Link key={c.id} href={`/categories/${c.slug}`} className="ic-card ic-card--hover ic-confession-card" style={{ textDecoration: "none" }}>
                    <span className="ic-confession-card__kicker">Area of life</span>
                    <h3 style={{ fontSize: "var(--ic-font-size-heading)" }}>{c.name}</h3>
                    <p style={{ fontFamily: "var(--ic-font-family-sans)" }}>{c.description}</p>
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
