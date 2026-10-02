"use client";

import { usePathname } from "next/navigation";
import { Search } from "lucide-react";
import { ThemeToggle } from "@/components/theme-toggle";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import { Button } from "@/components/ui/button";
import { Kbd } from "@/components/ui/kbd";
import { Separator } from "@/components/ui/separator";
import { SidebarTrigger } from "@/components/ui/sidebar";
import { cn } from "@/lib/utils";
import { findNavItem } from "./nav";
import type { ProxyHealth } from "./use-proxy-health";
import { UserMenu } from "./user-menu";

export function AppHeader({ health, onOpenPalette }: { health: ProxyHealth; onOpenPalette: () => void }) {
  const pathname = usePathname();
  const current = findNavItem(pathname);

  return (
    <header className="sticky top-0 z-20 flex h-14 shrink-0 items-center gap-2 border-b bg-background px-3 sm:px-4">
      <SidebarTrigger aria-label="Toggle navigation" />
      <Separator orientation="vertical" className="mr-1 data-[orientation=vertical]:h-4" />

      <Breadcrumb className="min-w-0">
        <BreadcrumbList className="flex-nowrap font-mono text-[11px] tracking-[0.12em] uppercase">
          {current?.group ? (
            <>
              <BreadcrumbItem className="hidden sm:inline-flex">{current.group}</BreadcrumbItem>
              <BreadcrumbSeparator className="hidden sm:block">/</BreadcrumbSeparator>
            </>
          ) : null}
          <BreadcrumbItem className="min-w-0">
            <BreadcrumbPage className="truncate">{current?.item.label ?? "Dashboard"}</BreadcrumbPage>
          </BreadcrumbItem>
        </BreadcrumbList>
      </Breadcrumb>

      <div className="ml-auto flex items-center gap-2">
        <ProxyChip health={health} />
        <Button
          variant="outline"
          size="sm"
          onClick={onOpenPalette}
          aria-label="Open command palette"
          className="hidden w-56 justify-between text-xs font-normal text-muted-foreground md:inline-flex"
        >
          <span className="inline-flex items-center gap-2">
            <Search />
            Search or jump to…
          </span>
          <Kbd className="font-mono">Ctrl K</Kbd>
        </Button>
        <Button
          variant="outline"
          size="icon-sm"
          onClick={onOpenPalette}
          aria-label="Open command palette"
          className="md:hidden"
        >
          <Search />
        </Button>
        <ThemeToggle />
        <UserMenu />
      </div>
    </header>
  );
}

function ProxyChip({ health }: { health: ProxyHealth }) {
  if (health.up === undefined) return null;
  return (
    <div
      className="hidden items-center gap-1.5 border px-2 py-1 font-mono text-[11px] sm:inline-flex"
      title={health.up ? "Proxy healthy" : `Proxy ${health.reason || "down"}`}
    >
      <span className={cn("size-1.5", health.up ? "bg-status-pass" : "bg-status-critical")} aria-hidden />
      <span className="text-muted-foreground">proxy</span>
      <span className={health.up ? "text-status-pass" : "text-status-critical"}>{health.up ? "up" : "down"}</span>
      {health.up && health.scanP50 !== null ? <span className="text-fg-faint">· {health.scanP50} ms</span> : null}
    </div>
  );
}
