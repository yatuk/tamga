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
    return [
      { source: "/kvkk",              destination: "/trust/kvkk",        permanent: true },
      { source: "/security",          destination: "/trust/security",    permanent: true },
      { source: "/docs/architecture", destination: "/docs#architecture", permanent: true },
      { source: "/docs/quickstart",   destination: "/docs#quickstart",   permanent: true },
      { source: "/docs/owasp-llm",    destination: "/docs#owasp-llm",    permanent: true },
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
