"use client";

import { useEffect, useMemo, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { useMotionValueEvent, useScroll } from "framer-motion";
import { api } from "@/lib/api";
import { toLowerEn } from "@/lib/utils/tr-string";

type CommandEntry = {
  id: string;
  label: string;
  hint: string;
  group: string;
  aliases?: string[];
  run: () => void;
};

const RECENCY_KEY = "tamga-cmd-recent";

function readRecents(): string[] {
  if (typeof window === "undefined") return [];
  try {
    const raw = localStorage.getItem(RECENCY_KEY);
    return raw ? (JSON.parse(raw) as string[]) : [];
  } catch { return []; }
}

export function useDashboardLayoutState() {
  const router = useRouter();
  const pathname = usePathname();
  const [mobileOpen, setMobileOpen] = useState(false);
  const [desktopCollapsed, setDesktopCollapsed] = useState(false);
  const [focusMode, setFocusMode] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [paletteQuery, setPaletteQuery] = useState("");
  const [scrolled, setScrolled] = useState(false);
  const { scrollY } = useScroll();
  useMotionValueEvent(scrollY, "change", (v) => setScrolled(v > 8));

  useEffect(() => {
    if (typeof window === "undefined") return;
    const stored = window.localStorage.getItem("tamga_focus_mode");
    if (stored === "1") setFocusMode(true);
  }, []);

  useEffect(() => {
    if (typeof window === "undefined") return;
    window.localStorage.setItem("tamga_focus_mode", focusMode ? "1" : "0");
  }, [focusMode]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "F" && e.shiftKey && !e.ctrlKey && !e.metaKey && !e.altKey) {
        const target = e.target as HTMLElement | null;
        if (target && (target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.isContentEditable)) {
          return;
        }
        e.preventDefault();
        setFocusMode((v) => !v);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const {
    data: health,
    isError: healthError,
    error: healthQueryError,
  } = useQuery({
    queryKey: ["tamga-health-sidebar"],
    queryFn: () => api.getHealthDetailed(),
    refetchInterval: 10_000,
    staleTime: 5_000,
    retry: 0,
  });
  const healthUp = !healthError && health?.proxy === "up";
  const healthReason = healthError
    ? (healthQueryError as Error | null)?.message || "service unreachable"
    : health?.proxy && health.proxy !== "up"
      ? health.proxy
      : "";
  const healthLatency =
    typeof health?.scan_latency_ms_p50 === "number" && Number.isFinite(health.scan_latency_ms_p50)
      ? Math.round(health.scan_latency_ms_p50 * 10) / 10
      : null;

  const isActive = (href: string) => pathname === href || (href !== "/dashboard" && pathname.startsWith(href));

  const commands = useMemo<CommandEntry[]>(() => {
    const base: CommandEntry[] = [
      { id: "go-overview", label: "Go Overview", hint: "g o", aliases: ["overview", "dashboard"], group: "NAVIGATE", run: () => router.push("/dashboard") },
      { id: "go-incidents", label: "Go Incidents", hint: "g i", aliases: ["inc", "security"], group: "NAVIGATE", run: () => router.push("/dashboard/security") },
      { id: "go-hunting", label: "Go Threat hunting", hint: "", aliases: ["hunt"], group: "NAVIGATE", run: () => router.push("/dashboard/hunting") },
      { id: "go-reports", label: "Go Reports", hint: "", aliases: ["report"], group: "NAVIGATE", run: () => router.push("/dashboard/reports") },
      { id: "go-policies", label: "Go Policies", hint: "", aliases: ["pol", "policy"], group: "NAVIGATE", run: () => router.push("/dashboard/policies") },
      { id: "go-playground", label: "Go Playground", hint: "", aliases: ["play", "sim"], group: "NAVIGATE", run: () => router.push("/dashboard/playground") },
      { id: "go-patterns", label: "Go Patterns", hint: "", aliases: ["pattern"], group: "NAVIGATE", run: () => router.push("/dashboard/patterns") },
      { id: "go-integrations", label: "Go Integrations", hint: "", aliases: ["int", "hooks"], group: "NAVIGATE", run: () => router.push("/dashboard/integrations") },
      { id: "go-team", label: "Go Team", hint: "", aliases: ["users"], group: "NAVIGATE", run: () => router.push("/dashboard/team") },
      { id: "go-audit", label: "Go Audit", hint: "", aliases: ["log"], group: "NAVIGATE", run: () => router.push("/dashboard/audit") },
      { id: "go-settings", label: "Go Settings", hint: "", aliases: ["set", "config"], group: "NAVIGATE", run: () => router.push("/dashboard/settings") },
      { id: "go-events", label: "Go Event explorer", hint: "", aliases: ["events", "raw"], group: "NAVIGATE", run: () => router.push("/dashboard/events") },
      { id: "go-traffic", label: "Go Traffic", hint: "", aliases: ["routing"], group: "NAVIGATE", run: () => router.push("/dashboard/traffic") },
      { id: "go-costs", label: "Go Token costs", hint: "", aliases: ["billing", "budget"], group: "NAVIGATE", run: () => router.push("/dashboard/costs") },
      { id: "go-latency", label: "Go Latency", hint: "", aliases: ["perf", "p95"], group: "NAVIGATE", run: () => router.push("/dashboard/latency") },
      { id: "go-proxy", label: "Go Proxy status", hint: "", aliases: ["health", "runtime"], group: "NAVIGATE", run: () => router.push("/dashboard/proxy") },
      { id: "go-keys", label: "Go API keys", hint: "", aliases: ["apikey", "tokens"], group: "NAVIGATE", run: () => router.push("/dashboard/keys") },
      { id: "filter-block", label: "Incidents: action BLOCK", hint: "", group: "FILTERS", run: () => router.push("/dashboard/security?action=BLOCK") },
      { id: "filter-injection", label: "Incidents: type injection", hint: "", group: "FILTERS", run: () => router.push("/dashboard/security?type=injection") },
      { id: "filter-open", label: "Incidents: triage Open", hint: "", group: "FILTERS", run: () => router.push("/dashboard/security?triage=Open") },
      { id: "filter-redact", label: "Incidents: action REDACT", hint: "", group: "FILTERS", run: () => router.push("/dashboard/security?action=REDACT") },
      { id: "action-new-pattern", label: "New custom pattern", hint: "", group: "ACTIONS", run: () => router.push("/dashboard/patterns?new=1") },
      { id: "action-new-integration", label: "New integration", hint: "", group: "ACTIONS", run: () => router.push("/dashboard/integrations") },
      { id: "action-simulate", label: "Simulate prompt", hint: "", group: "ACTIONS", run: () => router.push("/dashboard/playground") },
    ];
    const q = toLowerEn(paletteQuery.trim());

    // Dynamic: incident/provider lookup
    const incidentMatch = q.match(/^incident\s+([a-z0-9_-]+)/i);
    const providerMatch = q.match(/^provider\s+([a-z0-9_-]+)/i);
    const dynamic: CommandEntry[] = [];
    if (incidentMatch?.[1]) {
      const id = incidentMatch[1];
      dynamic.push({ id: `incident-${id}`, label: `Open incident ${id}`, hint: "", group: "ACTIONS", run: () => router.push(`/dashboard/security?request_id=${encodeURIComponent(id)}`) });
    }
    if (providerMatch?.[1]) {
      const p = toLowerEn(providerMatch[1]);
      dynamic.push({ id: `provider-${p}`, label: `Incidents: provider ${p}`, hint: "", group: "FILTERS", run: () => router.push(`/dashboard/security?provider=${encodeURIComponent(p)}`) });
    }

    // Filter by query — match against label, aliases, or id
    const filtered = q ? base.filter((c) => {
      const label = toLowerEn(c.label);
      if (label.includes(q)) return true;
      if (c.aliases?.some((a) => a.includes(q))) return true;
      return false;
    }) : base;

    // Recency boost: recently used commands float to top
    const recents = readRecents();
    const recentMap = new Map(recents.map((id, i) => [id, i]));
    const sorted = [...filtered].sort((a, b) => {
      const ra = recentMap.get(a.id);
      const rb = recentMap.get(b.id);
      if (ra !== undefined && rb !== undefined) return ra - rb;
      if (ra !== undefined) return -1;
      if (rb !== undefined) return 1;
      return 0;
    });

    return [...dynamic, ...sorted];
  }, [paletteQuery, router]);

  const grouped = useMemo(() => {
    const recents = readRecents();
    const recentCmds = commands.filter((c) => recents.includes(c.id));
    const otherCmds = commands.filter((c) => !recents.includes(c.id));

    const groups: Record<string, CommandEntry[]> = {};
    const order: string[] = [];

    // Recent group first (only when there's no query filtering)
    if (recentCmds.length > 0 && !paletteQuery.trim()) {
      groups["RECENT"] = recentCmds.slice(0, 5);
      order.push("RECENT");
    }

    for (const c of otherCmds) {
      if (!groups[c.group]) {
        groups[c.group] = [];
        order.push(c.group);
      }
      groups[c.group].push(c);
    }
    return order.map((k) => ({ label: k, items: groups[k] }));
  }, [commands, paletteQuery]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && toLowerEn(e.key) === "k") {
        e.preventDefault();
        setPaletteOpen((v) => !v);
      } else if (e.key === "Escape") {
        setPaletteOpen(false);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  useEffect(() => {
    if (!paletteOpen) {
      setPaletteQuery("");
    }
  }, [paletteOpen]);

  const navLinkBase =
    "relative flex cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 pl-2 text-xs font-mono transition-colors duration-150 ";
  const navLinkInactive = "text-zinc-600 dark:text-zinc-400 hover:bg-zinc-100 dark:hover:bg-zinc-900/70 hover:text-zinc-900 dark:hover:text-zinc-200";
  const navLinkActive = "bg-zinc-100 dark:bg-zinc-900 text-zinc-900 dark:text-zinc-100";

  return {
    router,
    pathname,
    mobileOpen,
    setMobileOpen,
    desktopCollapsed,
    setDesktopCollapsed,
    focusMode,
    setFocusMode,
    paletteOpen,
    setPaletteOpen,
    paletteQuery,
    setPaletteQuery,
    scrolled,
    health,
    healthUp,
    healthReason,
    healthLatency,
    isActive,
    commands,
    grouped,
    navLinkBase,
    navLinkInactive,
    navLinkActive,
  };
}
