/** @type {import('next').NextConfig} */
// The Go API is proxied same-origin so browser code never needs to know where
// the API runs: /api/* and signed /media/* forward to IC_API_URL. Relative URLs
// only, so the app works behind any preview or production host (same pattern as
// apps/web).
const apiOrigin = process.env.IC_API_URL || "http://127.0.0.1:8080";

const nextConfig = {
  reactStrictMode: true,
  async rewrites() {
    return [
      { source: "/500", destination: "/sys500" },
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
          { key: "Permissions-Policy", value: "camera=(), geolocation=()" },
        ],
      },
    ];
  },
};
export default nextConfig;
