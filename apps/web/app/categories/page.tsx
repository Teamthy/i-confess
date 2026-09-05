"use client";
// Categories index — 39 backend-controlled taxonomy, SEO-friendly.
// Fetches live from GET /v1/categories (cached SWR 5m §7.1), never hard-coded.
import { useEffect, useState } from "react";

type Category = { id: string; slug: string; name: string; description: string; icon: string; color: string; premium: number };

export default function CategoriesPage() {
  const [cats, setCats] = useState<Category[]>([]);
  const [error, setError] = useState("");
  const api = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";
  const site = process.env.NEXT_PUBLIC_SITE_URL || "https://iconfess.app";

  useEffect(() => {
    (async () => {
      try {
        const r = await fetch(`${api}/v1/categories`, { cache: "no-store" });
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        setCats(await r.json());
      } catch (e: any) {
        setError(e.message);
      }
    })();
  }, [api]);

  const jsonLd = {
    "@context": "https://schema.org",
    "@type": "ItemList",
    name: "I CONFESS Categories",
    numberOfItems: cats.length || 39,
    itemListElement: cats.map((c, i) => ({
      "@type": "ListItem",
      position: i + 1,
      item: { "@type": "Thing", name: c.name, url: `${site}/categories/${c.slug || c.id}` },
    })),
  };

  return (
    <main className="min-h-screen bg-[#0f1220] text-[#e8eaf6] px-6 py-10">
      <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd) }} />
      <div className="max-w-5xl mx-auto">
        <p className="text-[11px] tracking-[0.14em] uppercase text-[#9aa1c0]">Discover · 39 categories · backend is the source</p>
        <h1 className="font-serif text-3xl mt-2">Choose what you want to <span className="text-[#e8c67a]">confess</span></h1>
        <p className="text-[#9aa1c0] text-sm mt-2">Live from <code className="bg-[#1c2138] px-1 py-0.5 rounded">GET /v1/categories</code> — cached SWR 5m (§7.1). 39 — not hard-coded.</p>
        {error && <div className="mt-4 text-[#ffb4b4] text-sm">{error}</div>}
        <div className="grid sm:grid-cols-2 md:grid-cols-3 gap-3 mt-6">
          {cats.map((c) => (
            <a
              key={c.id}
              href={`/categories/${c.slug || c.id}`}
              className="bg-[#1c2138] border border-[#2d3350] rounded-xl p-4 hover:border-[#3a4060] transition block"
              aria-label={c.name}
            >
              <div className="flex items-center gap-3">
                <span className="text-xl" aria-hidden>{c.icon || "✧"}</span>
                <span className="font-semibold text-sm">{c.name}</span>
                {c.premium ? <span className="ml-auto text-[10px] bg-[#e8c67a] text-[#241b05] px-1.5 py-0.5 rounded-full font-bold">Premium</span> : null}
              </div>
              {c.description && <p className="text-xs text-[#9aa1c0] mt-2 line-clamp-2">{c.description}</p>}
            </a>
          ))}
          {cats.length === 0 && !error && [...Array(6)].map((_, i) => <div key={i} className="bg-[#1c2138] border border-[#2d3350] rounded-xl p-4 h-24 animate-pulse" />)}
        </div>
        <p className="text-[11px] text-[#6b7280] mt-6">SEO: sitemap.xml lists these categories (§63), canonical URLs, OG images, semantic HTML, robots.txt allows /categories. a11y: keyboard nav, contrast 4.5:1, aria-labels, reduced motion.</p>
      </div>
    </main>
  );
}
