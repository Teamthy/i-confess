import type { Metadata } from "next";
import Link from "next/link";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";
import { CategoryCard } from "@/components/Sections";
import { api } from "@/lib/api";

export const metadata: Metadata = {
  title: "Categories",
  description:
    "39 areas of life — whatever season you're in, there is a place to begin. Browse every iCONFESS category.",
  alternates: { canonical: "/categories" },
};

export default async function CategoriesPage() {
  const res = await api.categories();

  return (
    <div>
      <SiteHeader />
      <main id="main">
        <section className="ic-page-hero on-ink">
          <div className="ic-container">
            <p className="ic-eyebrow">Categories</p>
            <h1 style={{ marginTop: "var(--ic-spacing-4)" }}>
              {res.ok ? `Explore ${res.data.length} areas of life.` : "Explore every area of life."}
            </h1>
            <p className="ic-lede" style={{ marginTop: "var(--ic-spacing-5)" }}>
              Whatever season you're in, there is a place to begin.
            </p>
          </div>
        </section>

        <section className="ic-section">
          <div className="ic-container">
            <h2 className="ic-title" style={{ marginBottom: "var(--ic-spacing-7)" }}>
              {res.ok ? "The whole library, laid out." : "The library"}
            </h2>
            {res.ok ? (
              <div className="ic-grid ic-grid--3">
                {res.data.map((c, i) => (
                  // Editorial rhythm: every third card spans visually via order —
                  // the grid stays honest (no fake asymmetry via odd heights).
                  <CategoryCard key={c.id} category={c} />
                ))}
              </div>
            ) : (
              <div className="ic-state" role="alert">
                <h3>We couldn't reach the library</h3>
                <p>Something went wrong while loading the categories. Please try again soon.</p>
                <Link href="/" className="ic-btn ic-btn--secondary">Return home</Link>
              </div>
            )}
          </div>
        </section>
      </main>
      <SiteFooter />
    </div>
  );
}
