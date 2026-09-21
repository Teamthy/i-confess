/** Personalised home: today's experience, continue listening, categories, recommendations. */
import Link from "next/link";
import { api } from "@/lib/api";
export default async function AppHomePage() {
  const [catsRes, voicesRes] = await Promise.all([
    api.categories(),
    api.voices(),
  ]);
  const categories = catsRes.ok ? catsRes.data : [];
  const voices = voicesRes.ok ? voicesRes.data : [];

  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <p className="ia-section-title">Welcome back</p>
        <h1>Your experience</h1>
        <p>What do you want to return to today?</p>
      </div>

      <div className="ia-quick">
        <Link href="/app/explore" className="ia-quick__btn">
          <span className="ia-quick__icon">◎</span> Explore
        </Link>
        <Link href="/app/categories" className="ia-quick__btn">
          <span className="ia-quick__icon">≡</span> Categories
        </Link>
        <Link href="/app/favorites" className="ia-quick__btn">
          <span className="ia-quick__icon">♡</span> Favorites
        </Link>
        <Link href="/app/history" className="ia-quick__btn">
          <span className="ia-quick__icon">↻</span> History
        </Link>
        <Link href="/app/community" className="ia-quick__btn">
          <span className="ia-quick__icon">⌂</span> Community
        </Link>
        <Link href="/app/search" className="ia-quick__btn">
          <span className="ia-quick__icon">⌕</span> Search
        </Link>
      </div>

      {categories.length > 0 && (
        <section>
          <h2 className="ia-section-title">Categories</h2>
          <div className="ic-grid ic-grid--3">
            {categories.slice(0, 6).map((c) => (
              <Link key={c.id} href={`/app/categories/${c.slug}`} className="ic-card ic-card--hover" style={{ padding: "var(--ic-spacing-5)", textDecoration: "none" }}>
                <h3 style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-font-size-subheading)", color: "var(--ic-color-neutral-950)" }}>{c.name}</h3>
                <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", marginTop: "var(--ic-spacing-2)" }}>{c.description}</p>
              </Link>
            ))}
          </div>
          <Link href="/app/categories" className="ic-btn ic-btn--secondary" style={{ marginTop: "var(--ic-spacing-5)" }}>See all categories</Link>
        </section>
      )}

      {voices.length > 0 && (
        <section>
          <h2 className="ia-section-title">Voices</h2>
          <div className="ic-grid ic-grid--3">
            {voices.slice(0, 3).map((v) => (
              <Link key={v.id} href={`/app/voices/${v.id}`} className="ia-voice">
                <div style={{ display: "flex", alignItems: "center", gap: "var(--ic-spacing-3)" }}>
                  <span className="ic-voice-avatar" aria-hidden="true">{v.name.charAt(0)}</span>
                  <div>
                    <h3>{v.name}</h3>
                    <p>{v.description}</p>
                  </div>
                </div>
              </Link>
            ))}
          </div>
        </section>
      )}
    </div>
  );
}
