"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Modal, Search } from "@carbon/react";
import { navGroups } from "./nav";

type Entry = { label: string; note: string; href: string; words: string };

const SHORTCUTS: Entry[] = [
  { label: "Incidents that were stopped", note: "Filter", href: "/dashboard/security?action=BLOCK", words: "blocked" },
  { label: "Incidents that were redacted", note: "Filter", href: "/dashboard/security?action=REDACT", words: "" },
  { label: "Requests with a prompt injection", note: "Filter", href: "/dashboard/hunting?finding_type=injection", words: "hunt jailbreak" },
  { label: "Add a pattern", note: "Action", href: "/dashboard/patterns", words: "new custom regex" },
  { label: "Connect an integration", note: "Action", href: "/dashboard/integrations", words: "webhook slack siem" },
  { label: "Try a prompt against the policy", note: "Action", href: "/dashboard/playground", words: "simulate test" },
];

const PAGES: Entry[] = navGroups.flatMap((group) =>
  group.items.map((item) => ({
    label: item.label,
    note: group.label ?? "Page",
    href: item.href,
    words: (item.keywords ?? []).join(" "),
  })),
);

/**
 * Ctrl+K: every page, a few filtered views and common actions, found by
 * typing. "incident <id>" and "provider <name>" jump straight there. The
 * shortcut itself is handled by the shell, which loads this on first use and
 * puts the focus back where it was when this closes.
 */
export function CommandPalette({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const router = useRouter();
  const [query, setQuery] = useState("");
  const [at, setAt] = useState(0);
  const list = useRef<HTMLUListElement>(null);

  useEffect(() => {
    if (!open) setQuery("");
  }, [open]);

  const entries = useMemo(() => {
    const q = query.trim();
    const incident = q.match(/^incident\s+([\w-]+)/i)?.[1];
    const provider = q.match(/^provider\s+([\w-]+)/i)?.[1]?.toLowerCase();
    if (incident) {
      return [{ label: `Open request ${incident}`, note: "Incidents", href: `/dashboard/security?request_id=${encodeURIComponent(incident)}`, words: "" }];
    }
    if (provider) {
      return [{ label: `Requests sent to ${provider}`, note: "Threat hunting", href: `/dashboard/hunting?provider=${encodeURIComponent(provider)}`, words: "" }];
    }
    const words = q.toLowerCase().split(/\s+/).filter(Boolean);
    return [...PAGES, ...SHORTCUTS].filter((e) => {
      const hay = `${e.label} ${e.note} ${e.words}`.toLowerCase();
      return words.every((w) => hay.includes(w));
    });
  }, [query]);

  // A new set of results starts from its first entry.
  useEffect(() => setAt(0), [query]);
  useEffect(() => {
    list.current?.children[at]?.scrollIntoView({ block: "nearest" });
  }, [at]);

  const go = (href: string) => {
    onOpenChange(false);
    router.push(href);
  };

  return (
    // Carbon wraps Shift+Tab to the search field's hidden clear button, and the
    // focus then leaves the dialog. From the close button it goes to the field.
    <div
      onKeyDownCapture={(e) => {
        if (e.key === "Tab" && e.shiftKey && e.target instanceof Element && e.target.closest(".cds--modal-close-button, .cds--modal-close")) {
          e.preventDefault();
          e.stopPropagation();
          document.getElementById("palette-search")?.focus();
        }
      }}
    >
      <Modal open={open} passiveModal size="sm" className="t-palette" modalHeading="Go to" onRequestClose={() => onOpenChange(false)}>
        <Search
          id="palette-search"
          size="lg"
          labelText="Search pages and actions"
          placeholder="A page, or “incident” and a request ID"
          data-modal-primary-focus
          role="combobox"
          aria-expanded={entries.length > 0}
          aria-controls="palette-results"
          aria-activedescendant={entries[at] ? `palette-${at}` : undefined}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "ArrowDown") {
              e.preventDefault();
              setAt((i) => Math.min(entries.length - 1, i + 1));
            } else if (e.key === "ArrowUp") {
              e.preventDefault();
              setAt((i) => Math.max(0, i - 1));
            } else if (e.key === "Enter" && entries[at]) {
              e.preventDefault();
              go(entries[at].href);
            }
          }}
        />
        <p className="t-note" role="status" style={entries.length === 0 ? { padding: "1rem 0" } : undefined}>
          {entries.length === 0 ? "Nothing by that name." : ""}
        </p>
        {/* The list stays in the page when it is empty, because the field points at it. It is not a tab stop of its own. */}
        <ul id="palette-results" ref={list} role="listbox" aria-label="Results" tabIndex={-1}>
          {entries.map((e, i) => (
            // The search field keeps the focus; the pointer and the arrow keys only move the mark.
            // eslint-disable-next-line jsx-a11y/click-events-have-key-events
            <li key={e.href + e.label} id={`palette-${i}`} role="option" aria-selected={i === at} onMouseEnter={() => setAt(i)} onClick={() => go(e.href)}>
              <span>{e.label}</span>
              <span className="t-note">{e.note}</span>
            </li>
          ))}
        </ul>
      </Modal>
    </div>
  );
}
