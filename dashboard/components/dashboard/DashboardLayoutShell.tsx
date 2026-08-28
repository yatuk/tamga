"use client";

import Link from "next/link";
import { Command, Menu, PanelLeftClose, PanelLeftOpen } from "lucide-react";
import { DashboardPageTransition } from "@/components/dashboard/DashboardPageTransition";
import { TamgaLogo } from "@/components/TamgaLogo";
import { ThemeToggle } from "@/components/theme-toggle";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { dashboardNavGroups } from "./dashboard-nav-config";
import { DashboardCommandPalette } from "./DashboardCommandPalette";
import { DashboardNavList } from "./DashboardNavList";
import { DashboardRuntimeChip } from "./DashboardRuntimeChip";
import { GlobalAlertBanner, useGlobalAlerts } from "./GlobalAlertBanner";
import { useDashboardLayoutState } from "./useDashboardLayoutState";

export function DashboardLayoutShell({ children }: { children: React.ReactNode }) {
  const {
    router,
    mobileOpen,
    setMobileOpen,
    desktopCollapsed,
    setDesktopCollapsed,
    focusMode,
    paletteOpen,
    setPaletteOpen,
    paletteQuery,
    setPaletteQuery,
    scrolled,
    healthUp,
    healthReason,
    healthLatency,
    isActive,
    grouped,
    commands,
    navLinkBase,
    navLinkInactive,
    navLinkActive,
  } = useDashboardLayoutState();

  const runtimeChip = (
    <DashboardRuntimeChip healthUp={healthUp} healthReason={healthReason} healthLatency={healthLatency} />
  );

  const globalAlerts = useGlobalAlerts({
    proxyUp: healthUp,
    healthReason,
  });

  return (
    <div className="min-h-screen bg-surface-base text-fg">
      <GlobalAlertBanner alerts={globalAlerts} />
      <div className="flex min-h-screen">
        <aside
        className={`${focusMode ? "md:hidden" : "hidden md:flex"} sticky top-0 h-screen shrink-0 flex-col border-r border-border bg-surface-subtle px-2.5 py-4 ${
          desktopCollapsed ? "w-16" : "w-16 lg:w-64"
        }`}
      >
        <Link href="/dashboard" className="mb-6 flex items-center gap-3 px-2" title="Tamga">
          <TamgaLogo size={desktopCollapsed ? 28 : 34} priority />
          <div className={`${desktopCollapsed ? "hidden" : "hidden lg:block"} min-w-0`}>
            <div className="text-[15px] font-semibold leading-none tracking-[-0.02em] text-fg">tamga</div>
            <div className="mt-1.5 text-[10px] text-fg-muted">Security control plane</div>
          </div>
        </Link>

        <DashboardNavList
          navGroups={dashboardNavGroups}
          router={router}
          isActive={isActive}
          navLinkBase={navLinkBase}
          navLinkInactive={navLinkInactive}
          navLinkActive={navLinkActive}
          desktopCollapsed={desktopCollapsed}
          useLayoutId
        />

        <div className="mt-auto space-y-3 border-t border-border pt-3">
          <div className={`rounded-sm border border-border bg-surface-card p-2.5 ${desktopCollapsed ? "hidden" : "hidden lg:block"}`}>
            <div className="mb-1.5 flex items-center justify-between gap-2 text-[10px] text-fg-muted">
              <span>Runtime</span>
              <span className="font-mono tabular-nums">{healthLatency != null ? `${healthLatency}ms` : "—"}</span>
            </div>
            <span className={`inline-flex items-center gap-2 text-xs ${healthUp ? "text-status-pass" : "text-status-critical"}`}>
              <span className={`h-2 w-2 rounded-full ${healthUp ? "bg-status-pass" : "bg-status-critical"}`} aria-hidden />
              {healthUp ? "Proxy operational" : "Proxy unavailable"}
            </span>
          </div>
          <div className="flex items-center justify-end gap-2 px-1">
            <Button
              onClick={() => setDesktopCollapsed((v) => !v)}
              className="cursor-pointer rounded-sm border border-border-strong bg-surface-subtle text-fg-muted transition-colors duration-150 hover:bg-surface-card"
              aria-label="Toggle sidebar width"
            >
              {desktopCollapsed ? <PanelLeftOpen className="h-4 w-4" /> : <PanelLeftClose className="h-4 w-4" />}
            </Button>
          </div>
        </div>
      </aside>

      <main className="min-w-0 flex-1">
        <div
          className={`sticky top-0 z-20 flex items-center justify-between border-b bg-surface-subtle px-3 py-2 md:hidden ${
            scrolled
              ? "border-border-strong shadow-sm"
              : "border-border"
          }`}
        >
          <Button
            className="cursor-pointer rounded-sm border border-border-strong bg-surface-subtle text-fg-muted transition-colors duration-150 hover:bg-surface-card"
            onClick={() => setMobileOpen(true)}
            aria-label="Open menu"
          >
            <Menu className="h-4 w-4" />
          </Button>
          <Link href="/dashboard" className="flex items-center gap-2" aria-label="Tamga">
            <TamgaLogo size={30} priority />
            <span className="sr-only">Tamga</span>
          </Link>
          <div className="flex items-center gap-2">
            {runtimeChip}
            <ThemeToggle />
          </div>
        </div>

        <div
          className={`sticky top-0 z-20 hidden h-14 items-center justify-between border-b bg-surface-base/95 px-5 backdrop-blur-md md:flex ${
            scrolled
              ? "border-border-strong shadow-sm"
              : "border-border"
          }`}
        >
          <div className="flex items-center gap-3">
            {focusMode && (
              <Link href="/dashboard" className="flex items-center gap-2" aria-label="Tamga" title="Back to overview">
                <TamgaLogo size={22} priority />
                <span className="text-xs font-semibold text-fg">tamga</span>
              </Link>
            )}
            {runtimeChip}
            <span className="h-4 w-px bg-border" aria-hidden />
            <span className="text-xs text-fg-muted">Protected workspace</span>
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setPaletteOpen(true)}
              className="hidden min-w-52 cursor-pointer items-center justify-between gap-5 rounded-sm border border-border bg-surface-card px-3 py-2 text-[11px] text-fg-muted transition-colors duration-150 hover:border-border-strong hover:text-fg lg:inline-flex"
              aria-label="Open command palette"
            >
              <span className="inline-flex items-center gap-2"><Command className="h-3.5 w-3.5" /> Search or jump to…</span>
              <span className="font-mono text-[10px]">Ctrl K</span>
            </button>
            <ThemeToggle />
          </div>
        </div>

        <div className={focusMode ? "p-3" : "px-4 py-5 sm:px-6 lg:px-8"}>
          <div className="mx-auto w-full max-w-[1600px]">
            <DashboardPageTransition>{children}</DashboardPageTransition>
          </div>
        </div>
      </main>
      </div>{/* end flex container */}

      <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
        <SheetContent className="max-w-xs border-border bg-surface-subtle p-0">
          <SheetHeader className="border-b border-border px-4 py-3">
            <SheetTitle className="flex items-center gap-2">
              <TamgaLogo size={30} />
              <div>
                <div className="text-[13px] font-semibold leading-none text-fg">tamga</div>
                <div className="mt-1 text-[9px] uppercase tracking-[0.14em] text-fg-muted">v0.1.1</div>
              </div>
            </SheetTitle>
          </SheetHeader>
          <div className="flex h-full flex-col p-3">
            <DashboardNavList
              navGroups={dashboardNavGroups}
              router={router}
              isActive={isActive}
              navLinkBase={navLinkBase}
              navLinkInactive={navLinkInactive}
              navLinkActive={navLinkActive}
              desktopCollapsed={false}
              onNavigate={() => setMobileOpen(false)}
              useLayoutId={false}
            />
            <div className="mt-auto border-t border-border pt-3">
              <ThemeToggle />
              <div className="mt-2 px-1 text-[11px] text-fg-muted">
                <span className="inline-flex items-center gap-1.5">
                  <span className={`h-2 w-2 rounded-full ${healthUp ? "bg-status-pass" : "bg-status-critical"}`} aria-hidden />
                  {healthUp ? "Proxy up" : "Proxy down"}
                </span>
                {!healthUp && healthReason && (
                  <div className="mt-1 truncate text-[10px] text-status-critical" title={healthReason}>
                    reason: {healthReason}
                  </div>
                )}
              </div>
            </div>
          </div>
        </SheetContent>
      </Sheet>

      <DashboardCommandPalette
        open={paletteOpen}
        onClose={() => setPaletteOpen(false)}
        query={paletteQuery}
        onQueryChange={setPaletteQuery}
        grouped={grouped}
        commandsLength={commands.length}
      />
    </div>
  );
}
