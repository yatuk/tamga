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
    <div className="min-h-screen bg-white dark:bg-zinc-950 text-zinc-900 dark:text-zinc-100">
      <GlobalAlertBanner alerts={globalAlerts} />
      <div className="flex">
        <aside
        className={`${focusMode ? "md:hidden" : "hidden md:flex"} flex-col border-r border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 px-2 py-3 ${
          desktopCollapsed ? "w-14" : "w-14 lg:w-56"
        }`}
      >
        <Link href="/dashboard" className="mb-5 flex items-center gap-2 px-2" title="Tamga">
          <TamgaLogo size={desktopCollapsed ? 26 : 32} priority />
          <div className={`${desktopCollapsed ? "hidden" : "hidden lg:block"} min-w-0`}>
            <div className="text-[13px] font-semibold leading-none text-zinc-900 dark:text-zinc-100">tamga</div>
            <div className="mt-1 text-[9px] uppercase tracking-[0.14em] text-zinc-600 dark:text-zinc-400">v0.1.1</div>
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

        <div className="space-y-2 border-t border-zinc-200 dark:border-zinc-800 pt-3">
          <div className="flex items-center justify-end gap-2 px-1">
            <Button
              onClick={() => setDesktopCollapsed((v) => !v)}
              className="cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-700 dark:text-zinc-300 transition-colors duration-150 hover:bg-zinc-200 dark:hover:bg-zinc-800"
              aria-label="Toggle sidebar width"
            >
              {desktopCollapsed ? <PanelLeftOpen className="h-4 w-4" /> : <PanelLeftClose className="h-4 w-4" />}
            </Button>
          </div>
          <div className={`px-2 text-[9px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400 ${desktopCollapsed ? "hidden" : "hidden lg:block"}`}>
            RUNTIME //
          </div>
          <div className={`px-2 text-[11px] ${desktopCollapsed ? "hidden" : "hidden lg:block"}`}>
            <span className="inline-flex items-center gap-1.5 text-zinc-600 dark:text-zinc-400">
              <span className={`h-2 w-2 rounded-full ${healthUp ? "bg-emerald-500" : "bg-red-500"}`} aria-hidden />
              {healthUp ? "Proxy up" : "Proxy down"}
            </span>
            {!healthUp && healthReason && (
              <div className="mt-1 truncate text-[10px] text-red-600 dark:text-red-400" title={healthReason}>
                reason: {healthReason}
              </div>
            )}
          </div>
        </div>
      </aside>

      <main className="min-w-0 flex-1 overflow-auto">
        <div
          className={`sticky top-0 z-20 flex items-center justify-between border-b bg-white dark:bg-zinc-950 px-3 py-2 md:hidden ${
            scrolled
              ? "border-zinc-300 dark:border-zinc-700 shadow-sm"
              : "border-zinc-200 dark:border-zinc-800"
          }`}
        >
          <Button
            className="cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-700 dark:text-zinc-300 transition-colors duration-150 hover:bg-zinc-200 dark:hover:bg-zinc-800"
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
          className={`sticky top-0 z-20 hidden items-center justify-between border-b bg-white dark:bg-zinc-950 px-4 py-2 md:flex ${
            scrolled
              ? "border-zinc-300 dark:border-zinc-700 shadow-sm"
              : "border-zinc-200 dark:border-zinc-800"
          }`}
        >
          <div className="flex items-center gap-3">
            {focusMode && (
              <Link href="/dashboard" className="flex items-center gap-2" aria-label="Tamga" title="Back to overview">
                <TamgaLogo size={22} priority />
                <span className="text-xs font-semibold text-zinc-800 dark:text-zinc-200">tamga</span>
              </Link>
            )}
            {runtimeChip}
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setPaletteOpen(true)}
              className="hidden cursor-pointer rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/60 px-2 py-1 text-[11px] font-mono text-zinc-600 dark:text-zinc-400 transition-colors duration-150 hover:border-zinc-300 dark:hover:border-zinc-700 hover:text-zinc-900 dark:hover:text-zinc-200 lg:inline-flex"
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
        <SheetContent className="max-w-xs border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-0">
          <SheetHeader className="border-b border-zinc-200 dark:border-zinc-800 px-4 py-3">
            <SheetTitle className="flex items-center gap-2">
              <TamgaLogo size={30} />
              <div>
                <div className="text-[13px] font-semibold leading-none text-zinc-900 dark:text-zinc-100">tamga</div>
                <div className="mt-1 text-[9px] uppercase tracking-[0.14em] text-zinc-600 dark:text-zinc-400">v0.1.1</div>
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
            <div className="mt-auto border-t border-zinc-200 dark:border-zinc-800 pt-3">
              <ThemeToggle />
              <div className="mt-2 px-1 text-[11px] text-zinc-600 dark:text-zinc-400">
                <span className="inline-flex items-center gap-1.5">
                  <span className={`h-2 w-2 rounded-full ${healthUp ? "bg-emerald-500" : "bg-red-500"}`} aria-hidden />
                  {healthUp ? "Proxy up" : "Proxy down"}
                </span>
                {!healthUp && healthReason && (
                  <div className="mt-1 truncate text-[10px] text-red-600 dark:text-red-400" title={healthReason}>
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
