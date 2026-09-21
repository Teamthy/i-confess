/** Confession library — browse all published confessions. */

import type { Metadata } from "next";
import Link from "next/link";
import { api } from "@/lib/api";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";

export const metadata: Metadata = {
  title: "Confessions — iCONFESS",
  description: "Browse the confession library. 39 areas of life, each with words to speak and return to.",
};

export default async function ConfessionsPage() {
  const catsRes = await api.categories();
  const categories = catsRes.ok ? catsRes.data : [];

  // Fetch confessions from the first few categories
  const batches = await Promise.all(
    categories.slice(0, 6).map((c) => api.categoryConfessions(c.id)),
  );
  const confessions = batches.flatMap((r) => (r.ok ? r.data : []));

  return (
    <div>
      <SiteHeader />
      <main id="main">
        <section className="ic-page-hero">
          <div className="ic-container">
            <h1>Confessions</h1>
            <p className="ic-lede">
              {confessions.length} confessions across {categories.length} areas of life.
              Words to speak, hear, and return to.
            </p>
          </div>
        </section>

        <section className="ic-section">
          <div className="ic-container">
            {confessions.length > 0 ? (
              <div className="ic-grid ic-grid--3">
                {confessions.map((c) => (
                  <Link key={c.id} href={`/confessions/${c.id}`} className="ic-card ic-confession-card">
                    <span className="ic-confession-card__kicker">Confession</span>
                    <h3>{c.title}</h3>
                    <p>{c.short_text || c.medium_text || c.description}</p>
                    <footer>
                      <span>{c.language}</span>
                      <span>→</span>
                    </footer>
                  </Link>
                ))}
              </div>
            ) : (
              <div className="ic-state">
                <h3>The library is being prepared</h3>
                <p>Confessions will appear here when the API is available.</p>
              </div>
            )}
          </div>
        </section>
      </main>
      <SiteFooter />
    </div>
  );
}
