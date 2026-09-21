/** All 39 categories of life. */
import Link from "next/link";
import { api } from "@/lib/api";
import { railStyle } from "@/lib/categoryColor";
export default async function CategoriesPage() {
  const res = await api.categories();
  const categories = res.ok ? res.data : [];

  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Categories</h1>
        <p>{categories.length} areas of life — find the words that meet you where you are.</p>
      </div>
      <div className="ia-card-grid">
        {categories.map((c) => (
          <Link key={c.id} href={`/app/categories/${c.slug}`} className="ic-cat-card" style={railStyle(c.slug)}>
            <span className="ic-cat-card__initial" aria-hidden="true">{c.name.charAt(0)}</span>
            <h3>{c.name}</h3>
            <p>{c.description}</p>
            <span className="ic-cat-card__go">Explore →</span>
          </Link>
        ))}
      </div>
      {categories.length === 0 && (
        <div className="ic-state"><h3>No categories</h3><p>Something went wrong loading the library.</p></div>
      )}
    </div>
  );
}
