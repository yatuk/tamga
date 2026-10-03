"use client";

import Link from "next/link";
import { Download, ExternalLink } from "lucide-react";
import { Panel } from "@/components/app/panel";
import { EmptyState } from "@/components/app/states";
import { StatusBadge } from "@/components/app/status-badge";
import { Button } from "@/components/ui/button";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { humanizeFindingType } from "@/lib/humanize";
import type { TimeRange } from "@/lib/types";
import { formatInt } from "@/lib/utils/format";

type Row = { type: string; count: number; pct: number; code: string; note: string };

type Props = {
  owaspCoverageRows: Row[];
  range: TimeRange;
  exportEventsCsv: () => void;
  isExporting: boolean;
};

const linkClass = "text-foreground underline decoration-border-strong underline-offset-4 hover:decoration-foreground";

export function ReportsOwaspAndCompliance({ owaspCoverageRows, range, exportEventsCsv, isExporting }: Props) {
  return (
    <div className="grid gap-6 lg:grid-cols-2">
      <Panel title="OWASP LLM Top 10 mapping" aside={<StatusBadge>Approximate</StatusBadge>}>
        {owaspCoverageRows.length === 0 ? (
          <EmptyState
            icon="shield"
            title="No findings to map"
            description="Rows appear once the proxy detects PII, secrets or injection attempts."
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Finding type</TableHead>
                <TableHead className="text-right">Count</TableHead>
                <TableHead>OWASP</TableHead>
                <TableHead>Category</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {owaspCoverageRows.map((row) => (
                <TableRow key={row.type}>
                  <TableCell>{humanizeFindingType(row.type)}</TableCell>
                  <TableCell className="text-right font-mono text-xs tabular-nums">
                    {formatInt(row.count)} <span className="text-muted-foreground">· {row.pct}%</span>
                  </TableCell>
                  <TableCell>{row.code === "—" ? "—" : <StatusBadge tone="medium">{row.code}</StatusBadge>}</TableCell>
                  <TableCell className="text-muted-foreground">{row.note}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
        <p className="border-t px-4 py-3 text-xs text-muted-foreground">
          This maps finding families to OWASP categories by name. For audit evidence, use the technique on each incident
          together with the audit log export.{" "}
          <a href="https://genai.owasp.org/llm-top-10/" target="_blank" rel="noreferrer" className={linkClass}>
            OWASP LLM Top 10
            <ExternalLink className="ml-1 inline size-3" aria-hidden />
            <span className="sr-only"> (opens in a new tab)</span>
          </a>
        </p>
      </Panel>

      <Panel title="Audit evidence" description="Where to get what an auditor asks for">
        <ul className="divide-y text-sm">
          <li className="flex items-center justify-between gap-4 px-4 py-3">
            <div className="min-w-0">
              <p>Event export</p>
              <p className="text-xs text-muted-foreground">Every scanned request in the last {range}, as CSV.</p>
            </div>
            <Button variant="outline" size="sm" onClick={exportEventsCsv} disabled={isExporting}>
              <Download />
              Download CSV
            </Button>
          </li>
          <li className="flex items-center justify-between gap-4 px-4 py-3">
            <div className="min-w-0">
              <p>Audit log</p>
              <p className="text-xs text-muted-foreground">Hash-chained record of configuration and governance changes.</p>
            </div>
            <Button asChild variant="outline" size="sm">
              <Link href="/dashboard/audit">Open Audit Log</Link>
            </Button>
          </li>
          <li className="flex items-center justify-between gap-4 px-4 py-3">
            <div className="min-w-0">
              <p>Trust center</p>
              <p className="text-xs text-muted-foreground">Data residency and sub-processors.</p>
            </div>
            <Button asChild variant="outline" size="sm">
              <a href="https://tamgaproxy.com/trust" target="_blank" rel="noreferrer">
                tamgaproxy.com
                <ExternalLink />
                <span className="sr-only"> (opens in a new tab)</span>
              </a>
            </Button>
          </li>
          <li className="flex items-center justify-between gap-4 px-4 py-3">
            <div className="min-w-0">
              <p>Compliance mappings</p>
              <p className="text-xs text-muted-foreground">KVKK, BDDK, GDPR and OWASP LLM notes in the repository.</p>
            </div>
            <Button asChild variant="outline" size="sm">
              <a href="https://github.com/yatuk/tamga/tree/main/docs/compliance" target="_blank" rel="noreferrer">
                GitHub
                <ExternalLink />
                <span className="sr-only"> (opens in a new tab)</span>
              </a>
            </Button>
          </li>
        </ul>
      </Panel>
    </div>
  );
}
