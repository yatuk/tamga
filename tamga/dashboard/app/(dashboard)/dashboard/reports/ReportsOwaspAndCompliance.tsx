"use client";

import Link from "next/link";
import { ShieldCheck } from "lucide-react";
import { API_BASE } from "@/lib/api/fetch-core";
import { Badge } from "@/components/ui/badge";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { EmptyState } from "@/components/dashboard/EmptyState";
import type { ReportRange } from "./_constants";

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
        <TerminalFrame
          title="OWASP LLM Coverage"
          status={
            <Badge className="rounded-sm border border-zinc-600 bg-zinc-100 dark:bg-zinc-900 text-[10px] uppercase text-zinc-600 dark:text-zinc-400">
              heuristic map
            </Badge>
          }

        >
          <div className="space-y-2 p-3">
            <p className="text-xs text-zinc-600 dark:text-zinc-400">
              Bulgu aileleri (findings/breakdown) üzerinden OWASP LLM Top 10 ile{" "}
              <span className="text-zinc-600 dark:text-zinc-400">kaba</span> eşleme — denetim kanıtı için Incidents satırındaki teknik chip ve
              Audit export birlikte kullanılmalıdır.
            </p>
            <div className="overflow-x-auto">
              <table className="w-full text-left text-[11px]">
                <thead>
                  <tr className="border-b border-zinc-200 dark:border-zinc-800 text-zinc-600 dark:text-zinc-400">
                    <th className="py-1 pr-2">Finding type</th>
                    <th className="py-1 pr-2">Count</th>
                    <th className="py-1 pr-2">OWASP hint</th>
                    <th className="py-1">Note</th>
                  </tr>
                </thead>
                <tbody>
                  {owaspCoverageRows.length === 0 ? (
                    <tr>
                      <td colSpan={4} className="py-6">
                        <EmptyState
                          icon="shield"
                          title="No findings detected"
                          description="Findings breakdown will appear once the proxy detects PII, secrets, or injection attempts. Ensure scanners are enabled in your policy."
                        />
                      </td>
                    </tr>
                  ) : (
                    owaspCoverageRows.map((row) => (
                      <tr key={row.type} className="border-b border-zinc-900 text-zinc-700 dark:text-zinc-300">
                        <td className="py-1 pr-2">{row.type}</td>
                        <td className="py-1 pr-2">
                          {row.count} <span className="text-zinc-600 dark:text-zinc-400">({row.pct}%)</span>
                        </td>
                        <td className="py-1 pr-2 text-amber-300">{row.code}</td>
                        <td className="py-1 text-zinc-600 dark:text-zinc-400">{row.note}</td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
            <Link href="/docs/owasp-llm" className="inline-block text-[10px] text-zinc-400 hover:text-blue-400 hover:underline">
              OWASP LLM Top 10 dokümanı →
            </Link>
          </div>
        </TerminalFrame>

        <TerminalFrame
          title="Compliance Evidence"
          status={<ShieldCheck className="h-3.5 w-3.5 text-emerald-500" aria-hidden />}

        >
          <div className="space-y-3 p-3 text-xs text-zinc-600 dark:text-zinc-400">
            <p className="text-[10px] uppercase tracking-wide text-zinc-600 dark:text-zinc-400">KVKK / denetim kanıtı</p>
            <ul className="list-inside list-disc space-y-1">
              <li>
                <Link className="text-zinc-400 hover:text-blue-400 hover:underline" href="/trust">
                  Trust Center
                </Link>{" "}
                — veri yerleşimi ve alt işleyenler
              </li>
              <li>
                <Link className="text-zinc-400 hover:text-blue-400 hover:underline" href="/dashboard/audit">
                  Audit Logs
                </Link>{" "}
                — hash-chain doğrulama ve yönetişim olayları
              </li>
              <li>
                <a
                  className="text-zinc-400 hover:text-blue-400 hover:underline"
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
                  <span className="ml-1 text-[10px] text-zinc-600 dark:text-zinc-400">(admin key query ile)</span>
                ) : null}
              </li>
              <li>
                <a
                  className="text-zinc-400 hover:text-blue-400 hover:underline"
                  href="https://github.com/yatuk/tamga/blob/dev/tamga/docs/siem-json-export.md"
                  target="_blank"
                  rel="noreferrer"
                >
                  SIEM JSON şema notu (repo)
                </a>
              </li>
            </ul>
          </div>
        </TerminalFrame>
      </div>
    </div>
  );
}
