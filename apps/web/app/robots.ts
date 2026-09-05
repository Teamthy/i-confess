import type { MetadataRoute } from 'next';

export default function robots(): MetadataRoute.Robots {
  const base = process.env.NEXT_PUBLIC_SITE_URL || 'https://iconfess.app';
  return {
    rules: [
      {
        userAgent: '*',
        allow: '/',
        disallow: ['/admin/', '/api/', '/t/'], // share tokens are private via token, but not crawlable
      },
    ],
    sitemap: `${base}/sitemap.xml`,
    host: base,
  };
}
