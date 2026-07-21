import type { MetadataRoute } from "next";

// This app is the dashboard only (the marketing site lives at
// tamgaproxy.com) — nothing here should be indexed.
export default function robots(): MetadataRoute.Robots {
  return {
    rules: [{ userAgent: "*", disallow: "/" }],
  };
}
