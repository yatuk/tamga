"use client";

import { Pencil, Trash2 } from "lucide-react";
import type { CustomPattern } from "@/lib/api";
import { toUpperEn } from "@/lib/utils/case";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { EmptyState } from "@/components/app/states";
import { SkeletonRows } from "@/components/app/states";
import { Panel } from "@/components/app/panel";
import { sevClass } from "./_constants";
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from "@/components/ui/table";

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
      <Panel
        title="Patterns"
        aside={
          <span className="px-2 text-xs uppercase tracking-[0.18em] text-fg-muted">
            {items.length} rows
          </span>
        }

      >
        <div className="overflow-x-auto">
          <Table className="w-full text-left">
            <TableHeader className="uppercase">
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Kind</TableHead>
                <TableHead>Pattern</TableHead>
                <TableHead>Severity</TableHead>
                <TableHead>Hits</TableHead>
                <TableHead>Last Matched</TableHead>
                <TableHead>Enabled</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                <TableRow>
                  <TableCell colSpan={COLSPAN}>
                    <SkeletonRows rows={6} />
                  </TableCell>
                </TableRow>
              ) : items.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={COLSPAN}>
                    <EmptyState
                      icon="search"
                      title="No detection patterns defined yet"
                      description="Custom regex and keyword patterns detect sensitive data, prompt injections, and PII in LLM traffic."
                      suggestion="Create a pattern from the right panel — it takes effect immediately after scanner reload."
                    />
                  </TableCell>
                </TableRow>
              ) : (
                items.map((p) => (
                  <TableRow key={p.id}>
                    <TableCell>{p.name}</TableCell>
                    <TableCell>
                      <Badge className="rounded-sm border border-border-strong bg-surface-subtle text-xs text-fg-muted">
                        {p.kind}
                      </Badge>
                    </TableCell>
                    <TableCell className="max-w-[260px] truncate">
                      {p.pattern}
                    </TableCell>
                    <TableCell>
                      <Badge className={`rounded-sm border text-xs ${sevClass(p.severity)}`}>
                        {toUpperEn(p.severity)}
                      </Badge>
                    </TableCell>
                    <TableCell className="tabular-nums">
                      —
                    </TableCell>
                    <TableCell>
                      —
                    </TableCell>
                    <TableCell>
                      <Switch
                        checked={p.enabled}
                        onCheckedChange={() => onToggleEnabled(p)}
                        aria-label={`Toggle ${p.name}`}
                      />
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="inline-flex gap-1">
                        <Button variant="outline"
                          className="rounded-sm border border-border-strong bg-surface-subtle px-2 py-1 text-fg-muted hover:bg-surface-card"
                          onClick={() => onEdit(p)}
                        >
                          <Pencil className="h-3.5 w-3.5" />
                        </Button>
                        <Button variant="outline"
                          className="rounded-sm border border-status-critical bg-status-critical/30 px-2 py-1 text-status-critical hover:bg-status-critical/40"
                          onClick={() => {
                            onDelete(p.id);
                          }}
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
      </Panel>
    </div>
  );
}
