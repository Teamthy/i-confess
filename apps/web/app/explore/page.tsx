import type { Metadata } from "next";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";
import { CategoryRail } from "@/components/CategoryRail";
import { SectionHead } from "@/components/Sections";
import { api } from "@/lib/api";

export const metadata: Metadata = {
  title: "Explore",
  description:
    "Confessions, voices and sessions for this moment. Browse the iCONFESS library across 39 areas of life.",
  alternates: { canonical: "/explore" },
};

export default async function ExplorePage() {
  const [catsRes, voicesRes] = await Promise.all([api.categories(), api.voices()]);
  const categories = catsRes.ok ? catsRes.data : [];
  const voices = voicesRes.ok ? voicesRes.data : [];

  return (
    <div>
      <SiteHeader />
      <main id="main">
        <section className="ic-page-hero on-ink">
          <div className="ic-container">
            <p className="ic-eyebrow">Explore</p>
            <h1 style={{ marginTop: "var(--ic-spacing-4)" }}>Find something for this moment.</h1>
            <p className="ic-lede" style={{ marginTop: "var(--ic-spacing-5)" }}>
              The library, the voices, and the areas of life — everything public,
              in one quiet place.
            </p>
          </div>
        </section>

        {/* Featured */}
        <section className="ic-section" aria-labelledby="featured-title">
          <div className="ic-container">
            <SectionHead
              id="featured-title"
              eyebrow="Featured"
              title="Where most people begin."
              lede="A starting point, not a ranking — the areas of life listeners reach for first."
            />
            {catsRes.ok ? (
              <CategoryRail categories={categories.slice(0, 8)} />
            ) : (
              <div className="ic-state" role="alert">
                <h3>We couldn't reach the library</h3>
                <p>Something went wrong while loading this experience. Please try again soon.</p>
              </div>
            )}
          </div>
        </section>

        {/* Voices */}
        <section className="ic-section ic-section--mist" aria-labelledby="exp-voices-title">
          <div className="ic-container">
            <SectionHead
              id="exp-voices-title"
              eyebrow="Voices"
              title="Hear the words differently."
              lede="Curated voices, each with its own pace and character."
            />
            <div className="ic-grid ic-grid--3">
              {voices.map((v) => (
                <a key={v.id} href={`/voices/${v.id}`} className="ic-card ic-card--hover ic-voice-card">
                  <span className="ic-voice-avatar" aria-hidden="true">{v.name.charAt(0)}</span>
                  <h3>{v.name}{v.premium && <> <span className="ic-chip">Premium</span></>}</h3>
                  <p>{v.description}</p>
                </a>
              ))}
              {voices.length === 0 && (
                <div className="ic-state" style={{ gridColumn: "1 / -1" }}>
                  <h3>No voices are available right now</h3>
                  <p>The voice library is being refreshed. Please check back soon.</p>
                </div>
              )}
            </div>
          </div>
        </section>

        {/* All categories */}
        <section className="ic-section" aria-labelledby="exp-cats-title">
          <div className="ic-container">
            <SectionHead
              id="exp-cats-title"
              eyebrow="All areas of life"
              title="The whole library, laid out."
            />
            <div className="ic-grid ic-grid--3">
              {categories.map((c) => (
                <a key={c.id} href={`/categories/${c.slug}`} className="ic-card ic-card--hover ic-confession-card" style={{ textDecoration: "none" }}>
                  <span className="ic-confession-card__kicker">{c.premium ? "Premium area" : "Area of life"}</span>
                  <h3 style={{ fontSize: "var(--ic-font-size-heading)" }}>{c.name}</h3>
                  <p style={{ fontFamily: "var(--ic-font-family-sans)", fontSize: "var(--ic-font-size-bodySm)" }}>{c.description}</p>
                  <footer>
                    <span>Explore</span>
                    <span aria-hidden="true">→</span>
                  </footer>
                </a>
              ))}
            </div>
          </div>
        </section>
      </main>
      <SiteFooter />
    </div>
  );
}
