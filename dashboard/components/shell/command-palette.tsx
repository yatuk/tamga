"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowRight, Filter, FlaskConical, Plug, Plus } from "lucide-react";
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { toLowerEn } from "@/lib/utils/case";
import { navGroups } from "./nav";

const FILTERS = [
  { label: "Incidents: blocked", href: "/dashboard/security?action=BLOCK" },
  { label: "Incidents: redacted", href: "/dashboard/security?action=REDACT" },
  { label: "Incidents: prompt injection", href: "/dashboard/security?type=injection" },
  { label: "Incidents: open triage", href: "/dashboard/security?triage=Open" },
];

const ACTIONS = [
  { label: "New custom pattern", href: "/dashboard/patterns?new=1", icon: Plus },
  { label: "Connect an integration", href: "/dashboard/integrations", icon: Plug },
  { label: "Simulate a prompt", href: "/dashboard/playground", icon: FlaskConical },
];

/** Ctrl/⌘+K palette: every page, common incident filters, quick actions. */
export function CommandPalette({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const router = useRouter();
  const [query, setQuery] = useState("");

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && toLowerEn(e.key) === "k") {
        e.preventDefault();
        onOpenChange(!open);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onOpenChange]);

  useEffect(() => {
    if (!open) setQuery("");
  }, [open]);

  // "incident req_123" and "provider openai" jump straight to a filtered queue.
  const lookups = useMemo(() => {
    const q = query.trim();
    const incident = q.match(/^incident\s+([\w-]+)/i)?.[1];
    const provider = q.match(/^provider\s+([\w-]+)/i)?.[1];
    return [
      incident && {
        label: `Open incident ${incident}`,
        href: `/dashboard/security?request_id=${encodeURIComponent(incident)}`,
      },
      provider && {
        label: `Incidents for provider ${toLowerEn(provider)}`,
        href: `/dashboard/security?provider=${encodeURIComponent(toLowerEn(provider))}`,
      },
    ].filter(Boolean) as { label: string; href: string }[];
  }, [query]);

  const go = (href: string) => {
    onOpenChange(false);
    router.push(href);
  };

  return (
    <CommandDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Command palette"
      description="Jump to a page, a filtered incident queue or an action."
      showCloseButton={false}
    >
      <CommandInput placeholder="Search pages, or type “incident <id>”…" value={query} onValueChange={setQuery} />
      <CommandList className="max-h-[420px]">
        <CommandEmpty>No matches.</CommandEmpty>

        {lookups.length > 0 ? (
          <CommandGroup heading="Lookup" forceMount>
            {lookups.map((l) => (
              <CommandItem key={l.href} value={l.label} forceMount onSelect={() => go(l.href)}>
                <ArrowRight />
                {l.label}
              </CommandItem>
            ))}
          </CommandGroup>
        ) : null}

        {navGroups.map((group, i) => (
          <CommandGroup key={group.label ?? i} heading={group.label ?? "Pages"}>
            {group.items.map((item) => (
              <CommandItem key={item.href} value={item.label} keywords={item.keywords} onSelect={() => go(item.href)}>
                <item.icon />
                {item.label}
              </CommandItem>
            ))}
          </CommandGroup>
        ))}

        <CommandGroup heading="Filters">
          {FILTERS.map((f) => (
            <CommandItem key={f.href} value={f.label} onSelect={() => go(f.href)}>
              <Filter />
              {f.label}
            </CommandItem>
          ))}
        </CommandGroup>

        <CommandGroup heading="Actions">
          {ACTIONS.map((a) => (
            <CommandItem key={a.href} value={a.label} onSelect={() => go(a.href)}>
              <a.icon />
              {a.label}
            </CommandItem>
          ))}
        </CommandGroup>
      </CommandList>
    </CommandDialog>
  );
}
