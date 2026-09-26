import type { MetadataRoute } from "next";
import { api } from "@/lib/api";

/**
 * Robots — public marketing pages are crawlable; everything private
 * (authenticated app, share tokens, the API surface) is not. Unpublished or
 * user content is never exposed to search engines (§69).
 */
export default function robots(): MetadataRoute.Robots {
  const base = process.env.NEXT_PUBLIC_SITE_URL || "https://iconfess.app";
  return {
    rules: [
      {
        userAgent: "*",
        allow: "/",
        disallow: ["/app/", "/admin/", "/api/", "/t/", "/verify-email", "/reset-password"],
      },
    ],
    sitemap: `${base}/sitemap.xml`,
  };
}
