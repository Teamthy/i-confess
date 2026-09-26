/** Session library — build and manage sessions. */
import Link from "next/link";
import { api } from "@/lib/api";
export default async function SessionsPage() {
  const res = await api.categories();
  const categories = res.ok ? res.data : [];

  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Sessions</h1>
        <p>Sessions are built from categories. Choose an area of life and a length, and iCONFESS assembles the experience.</p>
      </div>
      <Link href="/app/session-builder" className="ic-btn ic-btn--primary" style={{ width: "fit-content" }}>Build a session</Link>
      <section>
        <h2 className="ia-section-title">Start from a category</h2>
        <div className="ia-card-grid">
          {categories.slice(0, 12).map((c) => (
            <Link key={c.id} href={`/app/categories/${c.slug}`} className="ia-confession">
              <h3>{c.name}</h3>
              <p>{c.description}</p>
            </Link>
          ))}
        </div>
      </section>
    </div>
  );
}
