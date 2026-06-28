"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { motion } from "framer-motion";
import { ChevronDown } from "lucide-react";
import type { DashboardNavGroup } from "./dashboard-nav-config";

const STORAGE_KEY = "tamga-nav-collapsed";

function readCollapsed(): Record<string, boolean> {
  if (typeof window === "undefined") return {};
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? (JSON.parse(raw) as Record<string, boolean>) : {};
  } catch {
    return {};
  }
}

function writeCollapsed(state: Record<string, boolean>) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
  } catch {
    /* ignore quota */
  }
}

type Props = {
  navGroups: DashboardNavGroup[];
  router: { prefetch: (href: string) => void };
  isActive: (href: string) => boolean;
  navLinkBase: string;
  navLinkInactive: string;
  navLinkActive: string;
  desktopCollapsed: boolean;
  onNavigate?: () => void;
  useLayoutId?: boolean;
};

export function DashboardNavList({
  navGroups,
  router,
  isActive,
  navLinkBase,
  navLinkInactive,
  navLinkActive,
  desktopCollapsed,
  onNavigate,
  useLayoutId = true,
}: Props) {
  // Start with config defaults only — server and client agree on first render.
  // localStorage overrides are applied via useEffect after hydration.
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>(() => {
    const defaults: Record<string, boolean> = {};
    for (const g of navGroups) {
      if (g.label && g.defaultOpen === false) {
        defaults[g.label] = true;
      }
    }
    return defaults;
  });

  // After hydration, merge saved preferences on top of defaults.
  const [hydrated, setHydrated] = useState(false);
  useEffect(() => {
    const saved = readCollapsed();
    if (Object.keys(saved).length > 0) {
      setCollapsed((prev) => {
        const next = { ...prev };
        for (const g of navGroups) {
          if (g.label && g.label in saved) {
            next[g.label] = saved[g.label];
          }
        }
        return next;
      });
    }
    setHydrated(true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const toggle = useCallback((label: string) => {
    setCollapsed((prev) => {
      const next = { ...prev, [label]: !prev[label] };
      if (hydrated) writeCollapsed(next);
      return next;
    });
  }, [hydrated]);

  // Persist on change (only after hydration)
  useEffect(() => {
    if (hydrated) writeCollapsed(collapsed);
  }, [collapsed, hydrated]);

  return (
    <nav className="flex-1 space-y-0.5">
      {navGroups.map((group, gi) => {
        const isCollapsible = !!group.label;
        const isCollapsed = isCollapsible ? collapsed[group.label!] : false;
        return (
          <div key={gi} className={gi === 0 ? "" : "mt-3"}>
            {group.label ? (
              <button
                type="button"
                onClick={() => toggle(group.label!)}
                className={`w-full flex items-center gap-1 px-2 pb-1 pt-2 text-[9px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-200 transition-colors ${
                  desktopCollapsed ? "hidden" : "hidden lg:flex"
                }`}
              >
                <ChevronDown
                  className={`h-3 w-3 shrink-0 transition-transform ${isCollapsed ? "-rotate-90" : ""}`}
                />
                {group.label}
              </button>
            ) : null}
            {group.label && desktopCollapsed ? <div className="mx-2 my-1 h-px bg-zinc-100 dark:bg-zinc-900" aria-hidden /> : null}
            {!isCollapsed && group.items.map((item) => {
              const active = isActive(item.href);
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  prefetch
                  onClick={onNavigate}
                  onMouseEnter={() => router.prefetch(item.href)}
                  onFocus={() => router.prefetch(item.href)}
                  className={navLinkBase + (active ? navLinkActive : navLinkInactive)}
                  title={item.label}
                >
                  {active &&
                    (useLayoutId ? (
                      <motion.span
                        layoutId="sidebar-active-rail"
                        className="absolute left-0 top-1 h-[calc(100%-8px)] w-0.5 bg-emerald-500"
                        transition={{ duration: 0.2, type: "spring", bounce: 0.2 }}
                      />
                    ) : (
                      <span
                        className="absolute left-0 top-1.5 h-[calc(100%-12px)] w-0.5 bg-emerald-500"
                        aria-hidden
                      />
                    ))}
                  <item.icon className="h-4 w-4 shrink-0" />
                  <span className={`${desktopCollapsed ? "hidden" : "hidden lg:inline"}`}>{item.label}</span>
                </Link>
              );
            })}
          </div>
        );
      })}
    </nav>
  );
}
