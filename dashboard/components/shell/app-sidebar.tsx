"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
} from "@/components/ui/sidebar";
import { cn } from "@/lib/utils";
import { findNavItem, navGroups } from "./nav";
import { TamgaMark } from "./tamga-mark";
import type { ProxyHealth } from "./use-proxy-health";

export function AppSidebar({ health }: { health: ProxyHealth }) {
  const pathname = usePathname();
  const activeHref = findNavItem(pathname)?.item.href;

  return (
    <Sidebar collapsible="icon">
      <SidebarHeader className="h-14 justify-center border-b border-sidebar-border px-3">
        <Link href="/dashboard" className="flex items-center gap-2.5 text-foreground" aria-label="Tamga overview">
          <TamgaMark />
          <span className="min-w-0 leading-none group-data-[collapsible=icon]:hidden">
            <span className="block font-display text-xl font-extrabold tracking-wider">TAMGA</span>
            <span className="mt-0.5 block font-mono text-[9px] tracking-[0.16em] text-fg-faint">SECURITY CONSOLE</span>
          </span>
        </Link>
      </SidebarHeader>

      <SidebarContent className="gap-0 py-2">
        {navGroups.map((group, i) => (
          <SidebarGroup key={group.label ?? i} className="py-1.5">
            {group.label ? (
              <SidebarGroupLabel className="h-7 font-mono text-[10px] tracking-[0.16em] text-fg-faint uppercase">
                {group.label}
              </SidebarGroupLabel>
            ) : null}
            <SidebarGroupContent>
              <SidebarMenu className="gap-0.5">
                {group.items.map((item) => {
                  const active = item.href === activeHref;
                  return (
                    <SidebarMenuItem key={item.href}>
                      <SidebarMenuButton
                        asChild
                        isActive={active}
                        tooltip={item.label}
                        className={cn(
                          "relative text-[13px]",
                          active && "before:absolute before:inset-y-0 before:left-0 before:w-0.5 before:bg-brand",
                        )}
                      >
                        <Link href={item.href} aria-current={active ? "page" : undefined}>
                          <item.icon />
                          <span>{item.label}</span>
                        </Link>
                      </SidebarMenuButton>
                    </SidebarMenuItem>
                  );
                })}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        ))}
      </SidebarContent>

      <SidebarFooter className="border-t border-sidebar-border p-3 group-data-[collapsible=icon]:items-center group-data-[collapsible=icon]:p-2">
        <ProxyStatus health={health} />
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  );
}

function ProxyStatus({ health }: { health: ProxyHealth }) {
  const pending = health.up === undefined;
  const dot = pending ? "bg-fg-faint" : health.up ? "bg-status-pass" : "bg-status-critical";
  const label = pending ? "Checking proxy" : health.up ? "Proxy operational" : "Proxy unreachable";

  return (
    <div className="flex items-center gap-2.5" title={health.reason || label}>
      <span className={cn("size-2 shrink-0", dot)} aria-hidden />
      <div className="min-w-0 group-data-[collapsible=icon]:hidden">
        <div className={cn("truncate text-xs", health.up === false ? "text-status-critical" : "text-foreground")}>
          {label}
        </div>
        <div className="truncate font-mono text-[10px] text-fg-faint">
          {health.up && health.scanP50 !== null ? `scan p50 ${health.scanP50} ms` : health.reason || "—"}
        </div>
      </div>
    </div>
  );
}
