"use client";

import { Suspense, useEffect, useState } from "react";
import dynamic from "next/dynamic";
import { ServerCrash } from "lucide-react";
import { SkeletonRows } from "@/components/app/states";
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar";
import { TooltipProvider } from "@/components/ui/tooltip";
import { AppHeader } from "./app-header";
import { AppSidebar } from "./app-sidebar";
import { useProxyHealth } from "./use-proxy-health";

// cmdk and the dialog it lives in are only needed once the palette is opened.
const CommandPalette = dynamic(() => import("./command-palette").then((m) => m.CommandPalette), { ssr: false });

/** Dashboard frame: sidebar, header, command palette and the proxy-down banner. */
export function AppShell({ children }: { children: React.ReactNode }) {
  const health = useProxyHealth();
  const [paletteOpen, setPaletteOpen] = useState(false);
  // Stays mounted after the first open so closing can animate.
  const [paletteLoaded, setPaletteLoaded] = useState(false);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPaletteLoaded(true);
        setPaletteOpen((open) => !open);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const openPalette = () => {
    setPaletteLoaded(true);
    setPaletteOpen(true);
  };

  return (
    <TooltipProvider delayDuration={200}>
      <SidebarProvider>
        <a
          href="#main"
          className="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus:border focus:bg-background focus:px-3 focus:py-2 focus:text-sm"
        >
          Skip to content
        </a>
        <AppSidebar health={health} />
        <SidebarInset className="min-w-0">
          {health.up === false ? <ProxyDownBanner reason={health.reason} /> : null}
          <AppHeader health={health} onOpenPalette={openPalette} />
          <main id="main" tabIndex={-1} className="mx-auto w-full max-w-[1600px] flex-1 px-4 py-5 outline-none sm:px-6 lg:px-8">
            {/* Pages read filters from the URL, which suspends during prerender. */}
            <Suspense fallback={<SkeletonRows rows={8} />}>{children}</Suspense>
          </main>
        </SidebarInset>
        {paletteLoaded ? <CommandPalette open={paletteOpen} onOpenChange={setPaletteOpen} /> : null}
      </SidebarProvider>
    </TooltipProvider>
  );
}

function ProxyDownBanner({ reason }: { reason: string }) {
  return (
    <div
      role="alert"
      className="flex items-center gap-2.5 border-b border-status-critical/40 bg-status-critical-bg px-4 py-2 text-status-critical"
    >
      <ServerCrash className="size-4 shrink-0" aria-hidden />
      <span className="font-mono text-[11px] font-bold tracking-[0.1em]">PROXY UNREACHABLE</span>
      <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground">
        {reason || "The Tamga proxy is not responding; LLM traffic may be unprotected."}
      </span>
    </div>
  );
}
