/** One category with its confessions. */
import Link from "next/link";
import { notFound } from "next/navigation";
import { api } from "@/lib/api";
import { railStyle } from "@/lib/categoryColor";
export default async function CategoryDetailPage({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params;
  const catRes = await api.category(slug);
  if (!catRes.ok) notFound();
  const cat = catRes.data;
  const confRes = await api.categoryConfessions(cat.id);
  const confessions = confRes.ok ? confRes.data : [];

  return (
    <div className="ia-page">
      <Link href="/app/categories" className="ic-backlink" style={{ color: "var(--ic-color-neutral-500)" }}>← Back to categories</Link>
      <div className="ia-page__head" style={{ padding: "var(--ic-spacing-6)", borderRadius: "var(--ic-radius-lg)", ...railStyle(cat.slug) as object, color: "var(--ic-color-neutral-0)" }}>
        <h1 style={{ color: "var(--ic-color-neutral-0)" }}>{cat.name}</h1>
        <p style={{ color: "var(--ic-color-neutral-200)" }}>{cat.description}</p>
        {cat.premium && <span className="ia-badge ia-badge--premium">Premium</span>}
      </div>
      <section>
        <h2 className="ia-section-title">Confessions ({confessions.length})</h2>
        <div className="ia-card-grid">
          {confessions.map((c) => (
            <Link key={c.id} href={`/app/confessions/${c.id}`} className="ia-confession">
              <h3>{c.title}</h3>
              <p>{c.short_text || c.medium_text || c.description}</p>
              <div className="ia-confession__meta">
                <span>{c.language}</span>
                {c.variants && c.variants.length > 0 && <span>{c.variants.length} variant{c.variants.length > 1 ? "s" : ""}</span>}
              </div>
            </Link>
          ))}
        </div>
        {confessions.length === 0 && (
          <div className="ic-state"><h3>No confessions yet</h3><p>This category is being prepared.</p></div>
        )}
      </section>
    </div>
  );
}
