"use client";

import { useMemo } from "react";
import { useRouter } from "next/navigation";
import { FlaskConical } from "lucide-react";
import { Panel } from "@/components/app/panel";
import { EmptyState } from "@/components/app/states";
import { ActionBadge, SeverityBadge, StatusBadge } from "@/components/app/status-badge";
import { JsonInspector } from "@/components/dashboard/JsonInspector";
import { Button } from "@/components/ui/button";
import type { IncidentsConsoleModel } from "@/hooks/security/useSecurityIncidentsConsole";
import type { SecurityEvent } from "@/lib/api";
import { humanizeProvider } from "@/lib/humanize";
import { primaryOwasp } from "@/lib/owasp-llm";
import { primarySeverity } from "@/lib/security/security-events-model";

interface Props {
  event: SecurityEvent | null;
  m: IncidentsConsoleModel;
  onFalsePositive: (requestId: string) => void;
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="min-w-0">
      <dt className="font-mono text-[11px] tracking-[0.1em] text-muted-foreground uppercase">{label}</dt>
      <dd className="mt-0.5 truncate text-sm">{children}</dd>
    </div>
  );
}

/** Everything known about the selected incident, with the triage actions. */
export function IncidentDetail({ event, m, onFalsePositive }: Props) {
  const router = useRouter();
  const findings = useMemo(() => event?.findings || [], [event?.findings]);

  const payload = useMemo(() => {
    if (!event) return {};
    return {
      request_id: event.request_id,
      provider: event.provider,
      model: event.model,
      endpoint: event.endpoint,
      action: event.action || "PASS",
      timestamp: event.timestamp,
      scan_latency_ms: event.scan_latency_ms,
      findings,
    };
  }, [event, findings]);

  if (!event) {
    return (
      <Panel title="Incident detail" className="min-h-64">
        <EmptyState
          icon="shield"
          title="No incident selected"
          description="Select a row in the queue to see its findings and raw event."
        />
      </Panel>
    );
  }

  const state = m.getIncidentState(event.request_id);
  const owasp = primaryOwasp(findings);
  const id = event.request_id;

  return (
    <Panel
      title="Incident detail"
      aside={
        <>
          <SeverityBadge severity={primarySeverity(findings)} />
          <ActionBadge action={event.action} />
          {owasp ? <StatusBadge title={`OWASP LLM Top 10: ${owasp.label}`}>{owasp.code}</StatusBadge> : null}
        </>
      }
      bodyClassName="flex flex-col"
    >
      <dl className="grid grid-cols-2 gap-x-4 gap-y-3 border-b p-4">
        <div className="col-span-2 min-w-0">
          <dt className="font-mono text-[11px] tracking-[0.1em] text-muted-foreground uppercase">Request ID</dt>
          <dd className="mt-0.5 font-mono text-sm break-all" translate="no">
            {id}
          </dd>
        </div>
        <Field label="Provider">{humanizeProvider(event.provider || "unknown")}</Field>
        <Field label="Model">{event.model || "—"}</Field>
        <Field label="Time">
          {event.timestamp ? new Date(event.timestamp).toLocaleString("en-GB") : "—"}
        </Field>
        <Field label="Scan">{Math.round(event.scan_latency_ms || 0)} ms</Field>
        <Field label="Status">{state.status}</Field>
        <Field label="Assignee">{state.assignee || "Unassigned"}</Field>
      </dl>

      {findings.length > 0 ? (
        <div className="border-b">
          <h3 className="px-4 pt-3 font-mono text-[11px] tracking-[0.1em] text-muted-foreground uppercase">
            Findings ({findings.length})
          </h3>
          <ul className="divide-y px-4 py-1">
            {findings.map((f, i) => (
              <li key={`${f.type}-${f.category}-${i}`} className="flex items-center gap-3 py-2 text-sm">
                <div className="min-w-0 flex-1">
                  <div className="truncate">
                    <span className="text-muted-foreground">{f.type} / </span>
                    {f.category}
                  </div>
                  {f.match ? (
                    <div className="truncate font-mono text-xs text-muted-foreground" translate="no">
                      {f.match}
                    </div>
                  ) : null}
                </div>
                {typeof f.confidence === "number" ? (
                  <span className="shrink-0 font-mono text-xs text-muted-foreground tabular-nums">
                    {Math.round(f.confidence > 1 ? f.confidence : f.confidence * 100)}%
                  </span>
                ) : null}
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      <div className="max-h-72 min-h-40 flex-1 overflow-auto overscroll-contain">
        <JsonInspector data={payload} className="h-full" autoExpandDepth={2} />
      </div>

      <div className="flex flex-wrap items-center gap-2 border-t p-3">
        <Button size="sm" onClick={() => m.setIncidentState(id, { assignee: "me", status: "In Progress" })}>
          Assign to Me
        </Button>
        <Button variant="outline" size="sm" onClick={() => m.setIncidentState(id, { status: "In Progress" })}>
          Acknowledge
        </Button>
        <Button variant="outline" size="sm" onClick={() => m.setIncidentState(id, { status: "Closed" })}>
          Close
        </Button>
        <Button variant="outline" size="sm" onClick={() => onFalsePositive(id)}>
          False Positive…
        </Button>
        <Button
          variant="ghost"
          size="sm"
          className="ml-auto"
          onClick={() => router.push(`/dashboard/playground?request_id=${encodeURIComponent(id)}`)}
        >
          <FlaskConical />
          Playground
        </Button>
      </div>
    </Panel>
  );
}
