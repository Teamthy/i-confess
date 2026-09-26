/** Content discovery: browse categories, confessions, and voices. */
import Link from "next/link";
import { api } from "@/lib/api";
export default async function ExplorePage() {
  const [catsRes, voicesRes] = await Promise.all([
    api.categories(),
    api.voices(),
  ]);
  const categories = catsRes.ok ? catsRes.data : [];
  const voices = voicesRes.ok ? voicesRes.data : [];

  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Explore</h1>
        <p>Discover confessions, categories, and voices across 39 areas of life.</p>
      </div>

      <section>
        <h2 className="ia-section-title">Categories ({categories.length})</h2>
        <div className="ia-card-grid">
          {categories.map((c) => (
            <Link key={c.id} href={`/app/categories/${c.slug}`} className="ia-confession">
              <h3>{c.name}</h3>
              <p>{c.description}</p>
              {c.premium && <span className="ia-badge ia-badge--premium">Premium</span>}
            </Link>
          ))}
        </div>
        {categories.length === 0 && (
          <div className="ic-state"><h3>No categories available</h3><p>The library is being updated.</p></div>
        )}
      </section>

      <section>
        <h2 className="ia-section-title">Voices ({voices.length})</h2>
        <div className="ia-card-grid">
          {voices.map((v) => (
            <Link key={v.id} href={`/app/voices/${v.id}`} className="ia-voice">
              <div style={{ display: "flex", alignItems: "center", gap: "var(--ic-spacing-3)" }}>
                <span className="ic-voice-avatar" aria-hidden="true">{v.name.charAt(0)}</span>
                <div>
                  <h3>{v.name}</h3>
                  <p>{v.type}{v.gender ? ` · ${v.gender}` : ""}</p>
                </div>
              </div>
            </Link>
          ))}
        </div>
      </section>
    </div>
  );
}
