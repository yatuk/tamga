import {
  BarChart3,
  BookMarked,
  Cpu,
  Crosshair,
  DollarSign,
  FileText,
  FlaskConical,
  History,
  Key,
  LayoutDashboard,
  Plug,
  ScrollText,
  Search,
  Server,
  Settings,
  Shield,
  Timer,
  TrendingUp,
  Users,
  type LucideIcon,
} from "lucide-react";

export type NavItem = {
  href: string;
  label: string;
  icon: LucideIcon;
  /** Extra words the command palette matches on. */
  keywords?: string[];
};

export type NavGroup = {
  label?: string;
  items: NavItem[];
};

export const navGroups: NavGroup[] = [
  {
    items: [{ href: "/dashboard", label: "Overview", icon: LayoutDashboard }],
  },

  {
    label: "Triage",
    items: [
      { href: "/dashboard/security", label: "Incidents", icon: Shield, keywords: ["security", "queue"] },
      { href: "/dashboard/hunting", label: "Threat hunting", icon: Crosshair, keywords: ["hunt", "search"] },
      { href: "/dashboard/events", label: "Event explorer", icon: Search, keywords: ["raw", "log"] },
    ],
  },

  {
    label: "Analytics",
    items: [
      { href: "/dashboard/traffic", label: "Traffic", icon: BarChart3, keywords: ["routing", "requests"] },
      { href: "/dashboard/trends", label: "Trends", icon: TrendingUp },
      { href: "/dashboard/costs", label: "Token costs", icon: DollarSign, keywords: ["billing", "budget", "spend"] },
      { href: "/dashboard/latency", label: "Latency", icon: Timer, keywords: ["performance", "p95"] },
      { href: "/dashboard/reports", label: "Reports", icon: FileText },
    ],
  },

  {
    label: "Policy",
    items: [
      { href: "/dashboard/policies", label: "Policies", icon: ScrollText },
      { href: "/dashboard/playground", label: "Playground", icon: FlaskConical, keywords: ["simulate", "red team"] },
      { href: "/dashboard/patterns", label: "Patterns", icon: BookMarked, keywords: ["custom entities", "regex"] },
    ],
  },

  {
    label: "System",
    items: [
      { href: "/dashboard/proxy", label: "Proxy status", icon: Server, keywords: ["health", "runtime"] },
      { href: "/dashboard/scanner-pool", label: "Scanner Pool", icon: Cpu, keywords: ["workers"] },
      { href: "/dashboard/keys", label: "API keys", icon: Key, keywords: ["tokens"] },
      { href: "/dashboard/integrations", label: "Integrations", icon: Plug, keywords: ["webhooks", "slack", "siem"] },
      { href: "/dashboard/audit", label: "Audit", icon: History, keywords: ["hash chain", "trail"] },
      { href: "/dashboard/team", label: "Team", icon: Users, keywords: ["users", "roles"] },
      { href: "/dashboard/settings", label: "Settings", icon: Settings, keywords: ["config"] },
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
