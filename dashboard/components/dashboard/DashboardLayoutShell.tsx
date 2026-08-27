"use client";

import Link from "next/link";
import { Menu, PanelLeftClose, PanelLeftOpen } from "lucide-react";
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
      <div className="flex">
        <aside
        className={`${focusMode ? "md:hidden" : "hidden md:flex"} flex-col border-r border-border bg-surface-subtle px-2 py-3 ${
          desktopCollapsed ? "w-14" : "w-14 lg:w-56"
        }`}
      >
        <Link href="/dashboard" className="mb-5 flex items-center gap-2 px-2" title="Tamga">
          <TamgaLogo size={desktopCollapsed ? 26 : 32} priority />
          <div className={`${desktopCollapsed ? "hidden" : "hidden lg:block"} min-w-0`}>
            <div className="text-[13px] font-semibold leading-none text-fg">tamga</div>
            <div className="mt-1 text-[9px] uppercase tracking-[0.14em] text-fg-muted">v0.1.1</div>
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

        <div className="space-y-2 border-t border-border pt-3">
          <div className="flex items-center justify-end gap-2 px-1">
            <Button
              onClick={() => setDesktopCollapsed((v) => !v)}
              className="cursor-pointer rounded-sm border border-border-strong bg-surface-subtle text-fg-muted transition-colors duration-150 hover:bg-surface-card"
              aria-label="Toggle sidebar width"
            >
              {desktopCollapsed ? <PanelLeftOpen className="h-4 w-4" /> : <PanelLeftClose className="h-4 w-4" />}
            </Button>
          </div>
          <div className={`px-2 text-[9px] uppercase tracking-[0.18em] text-fg-muted ${desktopCollapsed ? "hidden" : "hidden lg:block"}`}>
            RUNTIME //
          </div>
          <div className={`px-2 text-[11px] ${desktopCollapsed ? "hidden" : "hidden lg:block"}`}>
            <span className="inline-flex items-center gap-1.5 text-fg-muted">
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
      </aside>

      <main className="min-w-0 flex-1 overflow-auto">
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
          className={`sticky top-0 z-20 hidden items-center justify-between border-b bg-surface-subtle px-4 py-2 md:flex ${
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
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setPaletteOpen(true)}
              className="hidden cursor-pointer rounded-sm border border-border bg-surface-subtle px-2 py-1 text-[11px] font-mono text-fg-muted transition-colors duration-150 hover:border-border-strong hover:text-fg lg:inline-flex"
              aria-label="Open command palette"
            >
              <span>Ctrl+K</span>
            </button>
          </div>
        </div>

        <div className={focusMode ? "p-2" : "p-3"}>
          <DashboardPageTransition>{children}</DashboardPageTransition>
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
