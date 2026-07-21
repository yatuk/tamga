/** @type {import('next').NextConfig} */

const withBundleAnalyzer = process.env.ANALYZE === "true"
  ? require("@next/bundle-analyzer")({ enabled: true })
  : (config) => config;

const nextConfig = {
  output: "standalone",
  // Next 15 client router cache — keep dynamic RSC payloads warm for
  // 30s and static ones for 3 minutes so repeat navs inside the same
  // session render instantly without refetching the tree.
  experimental: {
    staleTimes: {
      dynamic: 30,
      static: 180,
    },
  },

  async redirects() {
    // Marketing pages moved to tamgaproxy.com — forward legacy paths there.
    const site = process.env.NEXT_PUBLIC_MARKETING_URL || "https://tamgaproxy.com";
    return [
      { source: "/kvkk",              destination: `${site}/trust/kvkk`,     permanent: true },
      { source: "/security",          destination: `${site}/trust/security`, permanent: true },
      { source: "/docs/architecture", destination: `${site}/docs`,           permanent: true },
      { source: "/docs/quickstart",   destination: `${site}/docs`,           permanent: true },
      { source: "/docs/owasp-llm",    destination: `${site}/docs`,           permanent: true },
      { source: "/pricing",           destination: `${site}/pricing`,        permanent: true },
      { source: "/docs",              destination: `${site}/docs`,           permanent: true },
      { source: "/trust/:path*",      destination: `${site}/trust/:path*`,   permanent: true },
      { source: "/contact",           destination: `${site}/contact`,        permanent: true },
    ];
  },

  async headers() {
    return [
      {
        // Fonts: cache forever (content-hashed)
        source: "/(.*)\\.(woff2|woff|ttf|otf)",
        headers: [{ key: "Cache-Control", value: "public, max-age=31536000, immutable" }],
      },
      {
        // Images: cache 1 day
        source: "/(.*)\\.(jpg|jpeg|png|svg|webp|avif|ico)",
        headers: [{ key: "Cache-Control", value: "public, max-age=86400" }],
      },
      {
        // Pricing API: CDN cache 5 minutes
        source: "/api/v1/billing/pricing",
        headers: [{ key: "Cache-Control", value: "public, max-age=300" }],
      },
    ];
  },
};

module.exports = withBundleAnalyzer(nextConfig);
