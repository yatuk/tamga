import type { MetadataRoute } from "next";

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL || "https://tamga.dev";

export default function sitemap(): MetadataRoute.Sitemap {
  const now = new Date();

  const routes: { path: string; priority: number; changeFrequency: MetadataRoute.Sitemap[number]["changeFrequency"] }[] = [
    { path: "", priority: 1.0, changeFrequency: "weekly" },
    { path: "/pricing", priority: 0.9, changeFrequency: "weekly" },
    { path: "/docs", priority: 0.9, changeFrequency: "weekly" },
    { path: "/trust", priority: 0.8, changeFrequency: "monthly" },
    { path: "/changelog", priority: 0.7, changeFrequency: "weekly" },
    { path: "/blog", priority: 0.7, changeFrequency: "weekly" },
    { path: "/compare", priority: 0.7, changeFrequency: "monthly" },
    { path: "/roi", priority: 0.7, changeFrequency: "monthly" },
    { path: "/evals", priority: 0.7, changeFrequency: "monthly" },
    { path: "/models", priority: 0.6, changeFrequency: "monthly" },
    { path: "/case-studies", priority: 0.6, changeFrequency: "monthly" },
    { path: "/whitepaper", priority: 0.6, changeFrequency: "monthly" },
    { path: "/contact", priority: 0.5, changeFrequency: "monthly" },
    { path: "/status", priority: 0.5, changeFrequency: "monthly" },
    { path: "/privacy", priority: 0.3, changeFrequency: "yearly" },
    { path: "/terms", priority: 0.3, changeFrequency: "yearly" },
    { path: "/dpa", priority: 0.3, changeFrequency: "yearly" },
    { path: "/responsible-disclosure", priority: 0.4, changeFrequency: "monthly" },
    { path: "/subprocessors", priority: 0.3, changeFrequency: "yearly" },
    { path: "/sign-in", priority: 0.4, changeFrequency: "monthly" },
    { path: "/sign-up", priority: 0.4, changeFrequency: "monthly" },
  ];

  return routes.map(({ path, priority, changeFrequency }) => ({
    url: `${SITE_URL}${path}`,
    lastModified: now,
    changeFrequency,
    priority,
  }));
}
