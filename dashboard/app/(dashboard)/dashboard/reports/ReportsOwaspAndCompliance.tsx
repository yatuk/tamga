"use client";

import Link from "next/link";
import { ShieldCheck } from "lucide-react";
import { API_BASE } from "@/lib/api/fetch-core";
import { Badge } from "@/components/ui/badge";
import { Panel } from "@/components/app/panel";
import { EmptyState } from "@/components/app/states";
import type { ReportRange } from "./_constants";
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from "@/components/ui/table";

type Row = { type: string; count: number; pct: number; code: string; note: string };

type Props = {
  owaspCoverageRows: Row[];
  range: ReportRange;
  adminKey: string;
};

export function ReportsOwaspAndCompliance({ owaspCoverageRows, range, adminKey }: Props) {
  return (
    <div>
      <div className="grid gap-3 lg:grid-cols-2">
        <Panel
          title="OWASP LLM Coverage"
          aside={
            <Badge className="rounded-sm border border-border-strong bg-surface-subtle text-xs uppercase text-fg-muted">
              heuristic map
            </Badge>
          }

        >
          <div className="space-y-2 p-3">
            <p className="text-xs text-fg-muted">
              A <span className="text-fg-muted">coarse</span> mapping from finding families (findings/breakdown) to the
              OWASP LLM Top 10. For audit evidence, use the technique chip on the Incidents row together with the
              Audit export.
            </p>
            <div className="overflow-x-auto">
              <Table className="w-full text-left">
                <TableHeader>
                  <TableRow>
                    <TableHead>Finding type</TableHead>
                    <TableHead>Count</TableHead>
                    <TableHead>OWASP hint</TableHead>
                    <TableHead>Note</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {owaspCoverageRows.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={4}>
                        <EmptyState
                          icon="shield"
                          title="No findings detected"
                          description="Findings breakdown will appear once the proxy detects PII, secrets, or injection attempts. Ensure scanners are enabled in your policy."
                        />
                      </TableCell>
                    </TableRow>
                  ) : (
                    owaspCoverageRows.map((row) => (
                      <TableRow key={row.type}>
                        <TableCell>{row.type}</TableCell>
                        <TableCell>
                          {row.count} <span className="text-fg-muted">({row.pct}%)</span>
                        </TableCell>
                        <TableCell className="text-status-medium">{row.code}</TableCell>
                        <TableCell>{row.note}</TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </div>
            <Link href="/docs/owasp-llm" className="inline-block text-xs text-fg-subtle hover:text-status-low hover:underline">
              OWASP LLM Top 10 reference →
            </Link>
          </div>
        </Panel>

        <Panel
          title="Compliance Evidence"
          aside={<ShieldCheck className="h-3.5 w-3.5 text-status-pass" aria-hidden />}

        >
          <div className="space-y-3 p-3 text-xs text-fg-muted">
            <p className="text-xs uppercase tracking-wide text-fg-muted">KVKK / audit evidence</p>
            <ul className="list-inside list-disc space-y-1">
              <li>
                <Link className="text-fg-subtle hover:text-status-low hover:underline" href="https://tamgaproxy.com/trust">
                  Trust Center
                </Link>{" "}
                — data residency and sub-processors
              </li>
              <li>
                <Link className="text-fg-subtle hover:text-status-low hover:underline" href="/dashboard/audit">
                  Audit Logs
                </Link>{" "}
                — hash-chain verification and governance events
              </li>
              <li>
                <a
                  className="text-fg-subtle hover:text-status-low hover:underline"
                  href={(() => {
                    const r = range === "24h" ? "24h" : range === "30d" ? "30d" : "7d";
                    const base = `${API_BASE}/api/v1/events/export?range=${r}&format=csv`;
                    return adminKey ? `${base}&key=${encodeURIComponent(adminKey)}` : base;
                  })()}
                  target="_blank"
                  rel="noreferrer"
                >
                  CSV export (events)
                </a>
                {adminKey ? (
                  <span className="ml-1 text-xs text-fg-muted">(admin key in the query string)</span>
                ) : null}
              </li>
              <li>
                <a
                  className="text-fg-subtle hover:text-status-low hover:underline"
                  href="https://github.com/yatuk/tamga/blob/dev/tamga/docs/siem-json-export.md"
                  target="_blank"
                  rel="noreferrer"
                >
                  SIEM JSON schema notes (repo)
                </a>
              </li>
            </ul>
          </div>
        </Panel>
      </div>
    </div>
  );
}
