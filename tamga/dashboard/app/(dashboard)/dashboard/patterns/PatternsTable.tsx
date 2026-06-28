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
          <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">
            {items.length} rows
          </span>
        }

      >
        <div className="overflow-x-auto">
          <table className="w-full text-left text-xs">
            <thead className="bg-zinc-100 dark:bg-zinc-900 text-[10px] uppercase tracking-wide text-zinc-600 dark:text-zinc-400">
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
                  <tr key={p.id} className="border-t border-zinc-200 dark:border-zinc-800 hover:bg-zinc-100 dark:hover:bg-zinc-900/60">
                    <td className="px-3 py-2 text-zinc-900 dark:text-zinc-100">{p.name}</td>
                    <td className="px-3 py-2">
                      <Badge className="rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-[10px] text-zinc-700 dark:text-zinc-300">
                        {p.kind}
                      </Badge>
                    </td>
                    <td className="max-w-[260px] truncate px-3 py-2 text-[11px] text-zinc-600 dark:text-zinc-400">
                      {p.pattern}
                    </td>
                    <td className="px-3 py-2">
                      <Badge className={`rounded-sm border text-[10px] ${sevClass(p.severity)}`}>
                        {toUpperLocale(p.severity)}
                      </Badge>
                    </td>
                    <td className="px-3 py-2 tabular-nums text-[11px] text-zinc-600 dark:text-zinc-400">
                      —
                    </td>
                    <td className="px-3 py-2 text-[11px] text-zinc-500 dark:text-zinc-400">
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
                          className="cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-2 py-1 text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800"
                          onClick={() => onEdit(p)}
                        >
                          <Pencil className="h-3.5 w-3.5" />
                        </Button>
                        <Button
                          className="cursor-pointer rounded-sm border border-red-900 bg-red-950/30 px-2 py-1 text-red-400 hover:bg-red-900/40"
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
