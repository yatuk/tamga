"use client";

import { useCallback, useMemo } from "react";
import { useRouter } from "next/navigation";
import { Clock3, FlaskConical, Server, Shield } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { JsonInspector } from "@/components/dashboard/JsonInspector";
import { primaryOwasp } from "@/lib/owasp-llm";
import { toUpperLocale } from "@/lib/utils/tr-string";
import {
  getActionBadge,
  getSeverityBadge,
  primarySeverity,
  relativeTime,
} from "@/lib/security/security-events-model";
import type { SecurityEvent } from "@/lib/api";
import type { IncidentsConsoleModel } from "@/hooks/security/useSecurityIncidentsConsole";

interface Props {
  event: SecurityEvent | null;
  m: IncidentsConsoleModel;
  onFpClick: (requestId: string) => void;
}

export function IncidentDetailPanel({ event, m, onFpClick }: Props) {
  const router = useRouter();

  const handleAck = useCallback(() => {
    if (!event) return;
    m.setIncidentState(event.request_id, { status: "In Progress" });
  }, [event, m]);

  const handleAssign = useCallback(() => {
    if (!event) return;
    m.setIncidentState(event.request_id, { assignee: "me", status: "In Progress" });
  }, [event, m]);

  const handleClose = useCallback(() => {
    if (!event) return;
    m.setIncidentState(event.request_id, { status: "Closed" });
  }, [event, m]);

  const handleFP = useCallback(() => {
    if (!event) return;
    onFpClick(event.request_id);
  }, [event, onFpClick]);

  const handleOpenPlayground = useCallback(() => {
    if (!event) return;
    router.push(`/dashboard/playground?request_id=${encodeURIComponent(event.request_id)}`);
  }, [event, router]);

  // All hooks must be called before any early return (Rules of Hooks).
  const findings = useMemo(() => event?.findings || [], [event?.findings]);
  const sev = primarySeverity(findings);
  const opState = event ? m.getIncidentState(event.request_id) : { status: "—", assignee: "" };
  const entity = event ? `${event.provider || "unknown"}${event.model ? ` / ${event.model}` : ""}` : "";
  const owasp = primaryOwasp(findings);

  const payload = useMemo(() => {
    if (!event) return {};
    const obj: Record<string, unknown> = {};
    if (event.request_id) obj.request_id = event.request_id;
    if (event.provider) obj.provider = event.provider;
    if (event.model) obj.model = event.model;
    obj.action = event.action || "PASS";
    if (event.timestamp) obj.timestamp = event.timestamp;
    if (event.endpoint) obj.endpoint = event.endpoint;
    if (findings.length > 0) obj.findings = findings;
    if (event.scan_latency_ms !== undefined) obj.scan_latency_ms = event.scan_latency_ms;
    return obj;
  }, [event, findings]);

  if (!event) {
    return (
      <div className="flex h-full items-center justify-center p-6 text-center">
        <div>
          <Shield className="mx-auto h-8 w-8 text-zinc-700" />
          <p className="mt-2 text-[11px] text-zinc-600 dark:text-zinc-400">
            Select an incident from the queue to inspect
          </p>
          <p className="mt-1 text-[10px] text-zinc-600 dark:text-zinc-500">
            j/k navigate · Enter select
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col">
      {/* Metadata bar */}
      <div className="border-b border-zinc-200 dark:border-zinc-800 px-3 py-2">
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-[11px]">
          <span className="inline-flex items-center gap-1.5">
            <span className="text-zinc-500">ID</span>
            <span className="font-mono text-zinc-900 dark:text-zinc-200">{event.request_id.slice(0, 16)}</span>
          </span>
          <span className="inline-flex items-center gap-1.5">
            <Clock3 className="h-3 w-3 text-zinc-500" />
            <span className="text-zinc-600 dark:text-zinc-400">{relativeTime(event.timestamp)}</span>
          </span>
          <span className="inline-flex items-center gap-1.5">
            <Server className="h-3 w-3 text-zinc-500" />
            <span className="text-zinc-600 dark:text-zinc-400">{entity}</span>
          </span>
        </div>
        {/* Severity + Action */}
        <div className="mt-2 flex flex-wrap items-center gap-2">
          <Badge className={getSeverityBadge(sev)}>{toUpperLocale(sev)}</Badge>
          <Badge className={getActionBadge(event.action)}>{toUpperLocale(event.action || "—")}</Badge>
          {owasp && (
            <span
              className="inline-flex items-center rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-1.5 py-0.5 font-mono text-[10px] uppercase tracking-wider text-zinc-700 dark:text-zinc-300"
              title={`OWASP LLM Top 10 · ${owasp.label}`}
            >
              {owasp.code}
            </span>
          )}
          <span className="text-[10px] text-zinc-500">
            Status: {opState.status} · {opState.assignee || "unassigned"}
          </span>
        </div>
      </div>

      {/* Findings */}
      {findings.length > 0 && (
        <div className="border-b border-zinc-200 dark:border-zinc-800 px-3 py-2">
          <div className="text-[10px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400 mb-1.5">
            Findings ({findings.length})
          </div>
          <div className="space-y-1">
            {findings.map((f, i) => (
              <div
                key={i}
                className="flex items-center justify-between rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/60 px-2 py-1.5 text-[11px]"
              >
                <span className="flex items-center gap-2">
                  <span className="font-mono text-zinc-900 dark:text-zinc-200">
                    {f.type}:{f.category}
                  </span>
                  {f.match && (
                    <span className="font-mono text-[10px] text-zinc-500 truncate max-w-[200px]">
                      &ldquo;{f.match}&rdquo;
                    </span>
                  )}
                </span>
                <span className="shrink-0 text-[10px] text-zinc-500">
                  {typeof f.confidence === "number"
                    ? `${Math.round(f.confidence > 1 ? f.confidence : f.confidence * 100)}%`
                    : "—"}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* JSON Inspector */}
      <div className="flex-1 min-h-0 overflow-hidden">
        <JsonInspector data={payload} className="h-full" autoExpandDepth={2} />
      </div>

      {/* Action bar */}
      <div className="border-t border-zinc-200 dark:border-zinc-800 px-3 py-2">
        <div className="flex flex-wrap items-center gap-1.5">
          <Button
            className="h-7 cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-2 text-[11px] text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800"
            onClick={handleAck}
          >
            Ack
          </Button>
          <Button
            className="h-7 cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-2 text-[11px] text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800"
            onClick={handleAssign}
          >
            Assign
          </Button>
          <Button
            className="h-7 cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-2 text-[11px] text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800"
            onClick={handleClose}
          >
            Close
          </Button>
          <Button
            className="h-7 cursor-pointer rounded-sm border border-amber-500/40 bg-amber-500/10 px-2 text-[11px] text-amber-400 hover:bg-amber-500/20"
            onClick={handleFP}
          >
            FP
          </Button>
          <Button
            className="h-7 cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-2 text-[11px] text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800"
            onClick={handleOpenPlayground}
            title="Test in Playground"
          >
            <FlaskConical className="h-3.5 w-3.5" />
            <span className="ml-1">Playground</span>
          </Button>
          <span className="ml-auto text-[10px] text-zinc-600 dark:text-zinc-400 flex gap-2">
            <span>j/k navigate</span>
            <span>Shift+A assign</span>
            <span>Shift+C close</span>
          </span>
        </div>
      </div>
    </div>
  );
}
