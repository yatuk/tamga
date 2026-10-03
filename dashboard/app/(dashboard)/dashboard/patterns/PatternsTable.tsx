"use client";

import { Pencil, Trash2 } from "lucide-react";
import { ConfirmButton } from "@/components/app/confirm-button";
import { Panel } from "@/components/app/panel";
import { EmptyState, SkeletonRows } from "@/components/app/states";
import { SeverityBadge, StatusBadge } from "@/components/app/status-badge";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import type { CustomPattern } from "@/lib/api";

type Props = {
  items: CustomPattern[];
  isLoading: boolean;
  /** The pattern currently loaded in the form, highlighted in the list. */
  editingId?: string;
  onEdit: (p: CustomPattern) => void;
  onDelete: (id: string) => void;
  onToggleEnabled: (p: CustomPattern) => void;
};

export function PatternsTable({ items, isLoading, editingId, onEdit, onDelete, onToggleEnabled }: Props) {
  const enabled = items.filter((p) => p.enabled).length;
  return (
    <Panel title="Custom patterns" aside={items.length > 0 ? `${enabled} of ${items.length} enabled` : undefined}>
      {isLoading ? (
        <SkeletonRows rows={6} />
      ) : items.length === 0 ? (
        <EmptyState
          icon="search"
          title="No custom patterns"
          description="Add a regular expression or a literal to detect values the built-in scanners do not know about."
          suggestion="Use the form to create the first one."
        />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Kind</TableHead>
              <TableHead>Pattern</TableHead>
              <TableHead>Severity</TableHead>
              <TableHead>Enabled</TableHead>
              <TableHead>
                <span className="sr-only">Actions</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.map((p) => (
              <TableRow key={p.id} data-state={p.id === editingId ? "selected" : undefined}>
                <TableCell className="font-medium">{p.name}</TableCell>
                <TableCell>
                  <StatusBadge>{p.kind}</StatusBadge>
                </TableCell>
                <TableCell className="max-w-72 truncate font-mono text-xs" title={p.pattern} translate="no">
                  {p.pattern}
                </TableCell>
                <TableCell>
                  <SeverityBadge severity={p.severity} />
                </TableCell>
                <TableCell>
                  <Switch
                    checked={p.enabled}
                    onCheckedChange={() => onToggleEnabled(p)}
                    aria-label={`${p.enabled ? "Disable" : "Enable"} pattern ${p.name}`}
                  />
                </TableCell>
                <TableCell>
                  <div className="flex justify-end gap-1">
                    <Button variant="ghost" size="icon-sm" aria-label={`Edit pattern ${p.name}`} onClick={() => onEdit(p)}>
                      <Pencil />
                    </Button>
                    <ConfirmButton
                      variant="ghost"
                      size="icon-sm"
                      aria-label={`Delete pattern ${p.name}`}
                      title={`Delete pattern ${p.name}?`}
                      description="The scanners stop matching it."
                      onConfirm={() => onDelete(p.id)}
                    >
                      <Trash2 />
                    </ConfirmButton>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </Panel>
  );
}
