// iCONFESS web — Next.js configuration.
//
// The design system lives at the repository root (design/tokens.json →
// design/generated). externalDir lets the app import those generated files
// directly so the website cannot drift from the tokens mobile and admin use.
//
// The Go API is reached two ways:
//   - server components call it directly via IC_API_URL (never exposed to the
//     browser), and
//   - browser code (the authenticated app) calls same-origin /api/* which the
//     Next server proxies to the same Go process. Relative URLs only, so the
//     app works behind any preview or production host.
//
// Signed audio is served by the API at /media/* and proxied the same way, so
// the browser never needs to know where the API runs.
/** @type {import('next').NextConfig} */
const apiOrigin = process.env.IC_API_URL || "http://127.0.0.1:8080";

const config = {
  reactStrictMode: true,
  poweredByHeader: false,
  compress: true,
  experimental: {
    externalDir: true,
  },
  images: {
    formats: ["image/avif", "image/webp"],
    deviceSizes: [640, 750, 828, 1080, 1200, 1920],
    imageSizes: [16, 32, 48, 64, 96],
  },
  async rewrites() {
    return [
      { source: "/api/:path*", destination: `${apiOrigin}/:path*` },
      { source: "/media/:path*", destination: `${apiOrigin}/media/:path*` },
    ];
  },
  async headers() {
    return [
      {
        source: "/(.*)",
        headers: [
          { key: "X-Content-Type-Options", value: "nosniff" },
          { key: "X-Frame-Options", value: "DENY" },
          { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
          {
            key: "Permissions-Policy",
            value: "camera=(), microphone=(), geolocation=()",
          },
        ],
      },
    ];
  },
};

export default config;
