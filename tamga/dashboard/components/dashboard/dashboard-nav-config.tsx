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
  Users,
} from "lucide-react";

export type DashboardNavItem = {
  href: string;
  label: string;
  icon: typeof LayoutDashboard;
};

export type DashboardNavGroup = {
  label?: string;
  /** If false, the group starts collapsed. Defaults to true. */
  defaultOpen?: boolean;
  items: DashboardNavItem[];
};

export const dashboardNavGroups: DashboardNavGroup[] = [
  // ── OVERVIEW ──────────────────────────────────────────────────────────────────
  {
    items: [{ href: "/dashboard", label: "Overview", icon: LayoutDashboard }],
  },

  // ── TRIAGE & RESPONSE ─────────────────────────────────────────────────────────
  {
    label: "TRIAGE",
    items: [
      { href: "/dashboard/security", label: "Incidents", icon: Shield },
      { href: "/dashboard/hunting", label: "Threat hunting", icon: Crosshair },
      { href: "/dashboard/events", label: "Event explorer", icon: Search },
    ],
  },

  // ── ANALYTICS & OBSERVABILITY ─────────────────────────────────────────────────
  {
    label: "ANALYTICS",
    items: [
      { href: "/dashboard/traffic", label: "Traffic", icon: BarChart3 },
      { href: "/dashboard/costs", label: "Token costs", icon: DollarSign },
      { href: "/dashboard/latency", label: "Latency", icon: Timer },
      { href: "/dashboard/reports", label: "Reports", icon: FileText },
    ],
  },

  // ── POLICY MANAGEMENT ─────────────────────────────────────────────────────────
  {
    label: "POLICY",
    defaultOpen: false,
    items: [
      { href: "/dashboard/policies", label: "Policies", icon: ScrollText },
      { href: "/dashboard/playground", label: "Playground", icon: FlaskConical },
      { href: "/dashboard/patterns", label: "Patterns", icon: BookMarked },
    ],
  },

  // ── SYSTEM ────────────────────────────────────────────────────────────────────
  {
    label: "SYSTEM",
    defaultOpen: false,
    items: [
      { href: "/dashboard/proxy", label: "Proxy status", icon: Server },
      { href: "/dashboard/scanner-pool", label: "Scanner Pool", icon: Cpu },
      { href: "/dashboard/keys", label: "API keys", icon: Key },
      { href: "/dashboard/integrations", label: "Integrations", icon: Plug },
      { href: "/dashboard/audit", label: "Audit", icon: History },
      { href: "/dashboard/team", label: "Team", icon: Users },
      { href: "/dashboard/settings", label: "Settings", icon: Settings },
    ],
  },
];
