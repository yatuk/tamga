"use client";

import { GlossaryButton } from "@/components/app/glossary";
import { PageHeader } from "@/components/app/page-header";
import { Panel } from "@/components/app/panel";
import { Stat, StatGrid } from "@/components/app/stat";
import { ErrorState, SkeletonRows } from "@/components/app/states";
import { StatusBadge, type Tone } from "@/components/app/status-badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { formatUptime } from "@/lib/utils/format";
import type { useProxyPage } from "./useProxyPage";

type Props = ReturnType<typeof useProxyPage>;
type ComponentStatus = Props["componentRows"][number]["status"];

const STATUS: Record<ComponentStatus, { label: string; tone: Tone }> = {
  ok: { label: "Healthy", tone: "pass" },
  warning: { label: "Degraded", tone: "medium" },
  error: { label: "Failing", tone: "critical" },
  // Not configured is a deployment choice, not a fault.
  disabled: { label: "Not configured", tone: "neutral" },
};

export function ProxyBody({ isLoading, hasError, isOnline, health, detail, componentRows }: Props) {
  const failing = componentRows.filter((r) => r.status === "error").length;
  const degraded = componentRows.filter((r) => r.status === "warning").length;
  const database = health?.database;

  const overall: { label: string; tone: Tone } = isLoading
    ? { label: "Checking…", tone: "neutral" }
    : !isOnline
      ? { label: "Offline", tone: "critical" }
      : failing > 0
        ? { label: `${failing} failing`, tone: "critical" }
        : degraded > 0
          ? { label: `${degraded} degraded`, tone: "medium" }
          : { label: "All healthy", tone: "pass" };

  return (
    <div className="space-y-6">
      <PageHeader
        title="Proxy status"
        description="Health of the proxy and the services it depends on. Refreshes every 15 seconds."
        actions={
          <>
            <StatusBadge tone={overall.tone}>{overall.label}</StatusBadge>
            <GlossaryButton />
          </>
        }
      />

      {hasError ? (
        <Panel>
          <ErrorState
            title="Could not reach the proxy"
            error="The health endpoint did not answer. Check that the proxy is running and that this origin is allowed by its CORS settings."
          />
        </Panel>
      ) : (
        <>
          <StatGrid>
            <Stat
              label="Proxy"
              value={isLoading ? "…" : isOnline ? "Online" : "Offline"}
              tone={isLoading ? "default" : isOnline ? "pass" : "critical"}
              hint={health?.uptime_seconds ? `Up ${formatUptime(health.uptime_seconds)}` : undefined}
            />
            <Stat label="Version" value={isLoading ? "…" : detail?.version || "—"} />
            <Stat
              label="Scanners"
              value={isLoading ? "…" : (health?.scanner_count ?? "—")}
              tooltip="Detectors that run on every request."
            />
            <Stat
              label="Database"
              value={isLoading ? "…" : database === "connected" ? "Connected" : database === "not_configured" ? "Not configured" : (database ?? "—")}
              tone={database === "connected" || database === "not_configured" || !database ? "default" : "critical"}
              hint={database === "not_configured" ? "Events are kept in memory" : undefined}
            />
            <Stat
              label="TLS"
              value={isLoading ? "…" : detail?.tls_enabled ? "Enabled" : "Plain HTTP"}
              tone={isLoading || detail?.tls_enabled ? "default" : "warn"}
              hint={!isLoading && !detail?.tls_enabled ? "Terminate TLS in front of the proxy" : undefined}
            />
            <Stat
              label="Events dropped"
              value={isLoading ? "…" : (health?.events_dropped ?? 0)}
              tone={(health?.events_dropped ?? 0) > 0 ? "warn" : "default"}
              tooltip="Events the bus could not deliver since the proxy started."
            />
          </StatGrid>

          <Panel title="Components" aside={`${componentRows.length} components`}>
            {isLoading ? (
              <SkeletonRows rows={5} />
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Component</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Details</TableHead>
                    <TableHead>Depends on</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {componentRows.map((r) => (
                    <TableRow key={r.component}>
                      <TableCell className="font-medium">{r.component}</TableCell>
                      <TableCell>
                        <StatusBadge tone={STATUS[r.status].tone}>{STATUS[r.status].label}</StatusBadge>
                      </TableCell>
                      <TableCell className="font-mono text-xs text-muted-foreground">{r.detail}</TableCell>
                      <TableCell className="text-xs text-muted-foreground">{r.dependsOn || "—"}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </Panel>

          {detail?.trace_ui_url ? (
            <p className="text-sm text-muted-foreground">
              Request traces are available in{" "}
              <a
                href={detail.trace_ui_url}
                target="_blank"
                rel="noreferrer"
                className="text-foreground underline decoration-border-strong underline-offset-4 hover:decoration-foreground"
              >
                the trace viewer
                <span className="sr-only"> (opens in a new tab)</span>
              </a>
              .
            </p>
          ) : null}
        </>
      )}
    </div>
  );
}
