"use client";

import { Fragment, useMemo, useState } from "react";
import { type ColumnDef, flexRender, getCoreRowModel, useReactTable } from "@tanstack/react-table";
import { Badge } from "@/components/ui/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import type { SimAction, SimFinding } from "@/lib/tamga-simulate";
import { toUpperLocale } from "@/lib/utils/tr-string";
import {
  actionBadge,
  findingAction,
  findingConfidence,
  findingRiskScore,
  meterColor,
  severityBadge,
  SeverityIcon,
} from "./tryTamgaLiveHelpers";

type Props = {
  findings: SimFinding[];
};

export function TryTamgaLiveFindingsTable({ findings }: Props) {
  const [expandedRow, setExpandedRow] = useState<number | null>(null);

  const columns = useMemo<ColumnDef<SimFinding>[]>(
    () => [
      {
        id: "severity",
        header: "Severity",
        cell: ({ row }) => (
          <Badge className={`${severityBadge(row.original.severity)} inline-flex items-center gap-1`}>
            <SeverityIcon severity={row.original.severity} />
            {toUpperLocale(row.original.severity)}
          </Badge>
        ),
      },
      {
        id: "finding_type",
        header: "Finding Type",
        cell: ({ row }) => (
          <span className="font-mono text-[11px] text-slate-200">{`${row.original.type}:${row.original.category}`}</span>
        ),
      },
      {
        id: "data",
        header: "Data (Redacted/Key)",
        cell: ({ row }) => <span className="font-mono text-[11px] text-slate-400">{row.original.match}</span>,
      },
      {
        id: "risk",
        header: "Risk Score",
        cell: ({ row }) => {
          const score = findingRiskScore(row.original.severity);
          return (
            <div className="flex min-w-[130px] items-center gap-2">
              <div className="h-1.5 w-20 overflow-hidden rounded-none bg-slate-800">
                <div className={`h-full ${meterColor(score)}`} style={{ width: `${score}%` }} />
              </div>
              <span className="font-mono text-[11px] text-slate-300">{score}%</span>
            </div>
          );
        },
      },
      {
        id: "action_taken",
        header: "Action Taken",
        cell: ({ row }) => {
          const action = findingAction(row.original);
          return <Badge className={actionBadge(action as SimAction)}>{action}</Badge>;
        },
      },
      {
        id: "confidence",
        header: "Confidence",
        cell: ({ row }) => (
          <span className="font-mono text-[11px] text-zinc-700 dark:text-zinc-300">{findingConfidence(row.original.severity).toFixed(2)}</span>
        ),
      },
    ],
    [],
  );

  const table = useReactTable({
    data: findings,
    columns,
    getCoreRowModel: getCoreRowModel(),
  });

  if (findings.length === 0) {
    return <p className="font-mono text-xs text-zinc-500 dark:text-zinc-400">No findings - clean payload.</p>;
  }

  return (
    <div className="overflow-hidden rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950">
      <Table>
        <TableHeader>
          {table.getHeaderGroups().map((headerGroup) => (
            <TableRow key={headerGroup.id} className="border-zinc-200 dark:border-zinc-800">
              {headerGroup.headers.map((header) => (
                <TableHead key={header.id} className="h-7 px-2 font-mono text-[10px] uppercase text-zinc-500 dark:text-zinc-400">
                  {header.isPlaceholder ? null : flexRender(header.column.columnDef.header, header.getContext())}
                </TableHead>
              ))}
            </TableRow>
          ))}
        </TableHeader>
        <TableBody>
          {table.getRowModel().rows.map((row, idx) => (
            <Fragment key={row.id}>
              <TableRow
                className="cursor-pointer border-zinc-200 dark:border-zinc-800 hover:bg-zinc-100 dark:hover:bg-zinc-900"
                onClick={() => setExpandedRow(expandedRow === idx ? null : idx)}
              >
                {row.getVisibleCells().map((cell) => (
                  <TableCell key={cell.id} className="px-2 py-1 align-middle">
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </TableCell>
                ))}
              </TableRow>
              {expandedRow === idx && (
                <TableRow className="border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/60">
                  <TableCell colSpan={6} className="px-2 py-2 font-mono text-[11px] text-zinc-600 dark:text-zinc-400">
                    detail: category={row.original.category} match={row.original.match} severity={row.original.severity}{" "}
                    confidence={findingConfidence(row.original.severity).toFixed(2)}
                  </TableCell>
                </TableRow>
              )}
            </Fragment>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}
