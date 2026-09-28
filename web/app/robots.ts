import type { MetadataRoute } from "next";

/* Private and per-account surfaces are never crawlable: the admin console,
   share-link resolution, the authenticated app shell and the reader's own
   library. Everything else — including the Bible section, whose text rights
   permit indexing — is open. */
export default function robots(): MetadataRoute.Robots {
  return {
    rules: [
      {
        userAgent: "*",
        allow: "/",
        disallow: ["/admin", "/admin/", "/app", "/app/", "/t/", "/bible/highlights", "/bible/bookmarks", "/bible/notes", "/bible/collections", "/bible/history", "/bible/settings"],
      },
    ],
    sitemap: "https://iconfess.app/sitemap.xml",
  };
}
