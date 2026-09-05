import type { MetadataRoute } from 'next';

// Sitemap — SEO §63 — indexable category/content pages, clean URLs.
// Categories are backend-controlled (39), never hard-coded; sitemap is
// generated from the canonical API (GET /v1/categories) at build time.
// For now, emit static routes; runtime fetches can augment with dynamic entries.
export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const base = process.env.NEXT_PUBLIC_SITE_URL || 'https://iconfess.app';
  const now = new Date();

  const staticRoutes: MetadataRoute.Sitemap = [
    { url: `${base}/`, lastModified: now, changeFrequency: 'daily', priority: 1 },
    { url: `${base}/pricing`, lastModified: now, changeFrequency: 'weekly', priority: 0.9 },
    { url: `${base}/plans`, lastModified: now, changeFrequency: 'weekly', priority: 0.9 },
    { url: `${base}/categories`, lastModified: now, changeFrequency: 'daily', priority: 0.8 },
    { url: `${base}/voices`, lastModified: now, changeFrequency: 'weekly', priority: 0.7 },
    { url: `${base}/faq`, lastModified: now, changeFrequency: 'monthly', priority: 0.5 },
    { url: `${base}/privacy`, lastModified: now, changeFrequency: 'yearly', priority: 0.3 },
    { url: `${base}/terms`, lastModified: now, changeFrequency: 'yearly', priority: 0.3 },
  ];

  // Try to enrich with live categories (best-effort, never fails build)
  try {
    const api = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';
    const res = await fetch(`${api}/v1/categories`, { next: { revalidate: 3600 } });
    if (res.ok) {
      const cats: Array<{ slug?: string; id: string; updated_at?: string }> = await res.json();
      for (const c of cats.slice(0, 39)) {
        const slug = c.slug || c.id;
        staticRoutes.push({
          url: `${base}/categories/${slug}`,
          lastModified: c.updated_at ? new Date(c.updated_at) : now,
          changeFrequency: 'weekly',
          priority: 0.6,
        });
      }
    }
  } catch {}

  return staticRoutes;
}
