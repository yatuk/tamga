"use client";

import { Suspense, useState } from "react";
import { ServerCrash } from "lucide-react";
import { SkeletonRows } from "@/components/app/states";
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar";
import { TooltipProvider } from "@/components/ui/tooltip";
import { AppHeader } from "./app-header";
import { AppSidebar } from "./app-sidebar";
import { CommandPalette } from "./command-palette";
import { useProxyHealth } from "./use-proxy-health";

/** Dashboard frame: sidebar, header, command palette and the proxy-down banner. */
export function AppShell({ children }: { children: React.ReactNode }) {
  const health = useProxyHealth();
  const [paletteOpen, setPaletteOpen] = useState(false);

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
          <AppHeader health={health} onOpenPalette={() => setPaletteOpen(true)} />
          <main id="main" tabIndex={-1} className="mx-auto w-full max-w-[1600px] flex-1 px-4 py-5 outline-none sm:px-6 lg:px-8">
            {/* Pages read filters from the URL, which suspends during prerender. */}
            <Suspense fallback={<SkeletonRows rows={8} />}>{children}</Suspense>
          </main>
        </SidebarInset>
        <CommandPalette open={paletteOpen} onOpenChange={setPaletteOpen} />
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
