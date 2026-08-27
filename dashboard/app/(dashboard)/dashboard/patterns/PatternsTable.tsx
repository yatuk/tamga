"use client";

import { Pencil, Trash2 } from "lucide-react";
import type { CustomPattern } from "@/lib/api";
import { toUpperLocale } from "@/lib/utils/tr-string";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { EmptyState } from "@/components/dashboard/EmptyState";
import { SkeletonTable } from "@/components/common/SkeletonRow";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { sevClass } from "./_constants";

type Props = {
  items: CustomPattern[];
  isLoading: boolean;
  onEdit: (p: CustomPattern) => void;
  onDelete: (id: string) => void;
  onToggleEnabled: (p: CustomPattern) => void;
};

const COLSPAN = 8;

export function PatternsTable({ items, isLoading, onEdit, onDelete, onToggleEnabled }: Props) {
  return (
    <div>
      <TerminalFrame
        title="Patterns"
        status={
          <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-fg-muted">
            {items.length} rows
          </span>
        }

      >
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-surface-subtle text-[10px] uppercase tracking-wide text-fg-muted">
              <tr>
                <th className="px-3 py-2">Name</th>
                <th className="px-3 py-2">Kind</th>
                <th className="px-3 py-2">Pattern</th>
                <th className="px-3 py-2">Severity</th>
                <th className="px-3 py-2">Hits</th>
                <th className="px-3 py-2">Last Matched</th>
                <th className="px-3 py-2">Enabled</th>
                <th className="px-3 py-2 text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {isLoading ? (
                <tr>
                  <td className="px-3 py-0" colSpan={COLSPAN}>
                    <SkeletonTable rows={6} cols={COLSPAN} />
                  </td>
                </tr>
              ) : items.length === 0 ? (
                <tr>
                  <td className="px-3 py-0" colSpan={COLSPAN}>
                    <EmptyState
                      icon="search"
                      title="No detection patterns defined yet"
                      description="Custom regex and keyword patterns detect sensitive data, prompt injections, and PII in LLM traffic."
                      suggestion="Create a pattern from the right panel — it takes effect immediately after scanner reload."
                    />
                  </td>
                </tr>
              ) : (
                items.map((p) => (
                  <tr key={p.id} className="border-t border-border hover:bg-surface-subtle/60">
                    <td className="px-3 py-2 text-fg">{p.name}</td>
                    <td className="px-3 py-2">
                      <Badge className="rounded-sm border border-border-strong bg-surface-subtle text-[10px] text-fg-muted">
                        {p.kind}
                      </Badge>
                    </td>
                    <td className="max-w-[260px] truncate px-3 py-2 text-[11px] text-fg-muted">
                      {p.pattern}
                    </td>
                    <td className="px-3 py-2">
                      <Badge className={`rounded-sm border text-[10px] ${sevClass(p.severity)}`}>
                        {toUpperLocale(p.severity)}
                      </Badge>
                    </td>
                    <td className="px-3 py-2 tabular-nums text-[11px] text-fg-muted">
                      —
                    </td>
                    <td className="px-3 py-2 text-[11px] text-fg-subtle">
                      —
                    </td>
                    <td className="px-3 py-2">
                      <Switch
                        checked={p.enabled}
                        onCheckedChange={() => onToggleEnabled(p)}
                        aria-label={`Toggle ${p.name}`}
                      />
                    </td>
                    <td className="px-3 py-2 text-right">
                      <div className="inline-flex gap-1">
                        <Button
                          className="cursor-pointer rounded-sm border border-border-strong bg-surface-subtle px-2 py-1 text-fg-muted hover:bg-surface-card"
                          onClick={() => onEdit(p)}
                        >
                          <Pencil className="h-3.5 w-3.5" />
                        </Button>
                        <Button
                          className="cursor-pointer rounded-sm border border-status-critical bg-status-critical/30 px-2 py-1 text-status-critical hover:bg-status-critical/40"
                          onClick={() => {
                            onDelete(p.id);
                          }}
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </TerminalFrame>
    </div>
  );
}
