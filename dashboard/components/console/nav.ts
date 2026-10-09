export type NavItem = {
  href: string;
  label: string;
  /** Extra words the command palette matches on. */
  keywords?: string[];
};

export type NavGroup = {
  label?: string;
  items: NavItem[];
};

export const navGroups: NavGroup[] = [
  {
    items: [{ href: "/dashboard", label: "Overview" }],
  },

  {
    label: "Triage",
    items: [
      { href: "/dashboard/security", label: "Incidents", keywords: ["security", "queue"] },
      { href: "/dashboard/hunting", label: "Threat hunting", keywords: ["hunt", "search"] },
      { href: "/dashboard/events", label: "Event explorer", keywords: ["raw", "log"] },
    ],
  },

  {
    label: "Analytics",
    items: [
      { href: "/dashboard/traffic", label: "Traffic", keywords: ["routing", "requests"] },
      { href: "/dashboard/trends", label: "Trends" },
      { href: "/dashboard/costs", label: "Token costs", keywords: ["billing", "budget", "spend"] },
      { href: "/dashboard/latency", label: "Latency", keywords: ["performance", "p95"] },
      { href: "/dashboard/reports", label: "Reports" },
    ],
  },

  {
    label: "Policy",
    items: [
      { href: "/dashboard/policies", label: "Policies" },
      { href: "/dashboard/playground", label: "Playground", keywords: ["simulate", "red team"] },
      { href: "/dashboard/patterns", label: "Patterns", keywords: ["custom entities", "regex"] },
    ],
  },

  {
    label: "System",
    items: [
      { href: "/dashboard/proxy", label: "Proxy status", keywords: ["health", "runtime"] },
      { href: "/dashboard/scanner-pool", label: "Scanner pool", keywords: ["workers"] },
      { href: "/dashboard/keys", label: "API keys", keywords: ["tokens"] },
      { href: "/dashboard/integrations", label: "Integrations", keywords: ["webhooks", "slack", "siem"] },
      { href: "/dashboard/audit", label: "Audit", keywords: ["hash chain", "trail"] },
      { href: "/dashboard/team", label: "Team", keywords: ["users", "roles"] },
      { href: "/dashboard/settings", label: "Settings", keywords: ["config"] },
    ],
  },
];

export const navItems: NavItem[] = navGroups.flatMap((g) => g.items);

/** The nav entry a pathname belongs to; the longest matching href wins. */
export function findNavItem(pathname: string): { group?: string; item: NavItem } | undefined {
  let best: { group?: string; item: NavItem } | undefined;
  for (const g of navGroups) {
    for (const item of g.items) {
      const match = pathname === item.href || (item.href !== "/dashboard" && pathname.startsWith(item.href + "/"));
      if (match && (!best || item.href.length > best.item.href.length)) best = { group: g.label, item };
    }
  }
  return best;
}
