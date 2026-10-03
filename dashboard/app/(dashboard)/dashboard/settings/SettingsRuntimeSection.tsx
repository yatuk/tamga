"use client";

import { DetailList } from "@/components/app/detail-list";
import { Panel } from "@/components/app/panel";
import { StatusBadge, type Tone } from "@/components/app/status-badge";
import { formatUptime } from "@/lib/utils/format";

type Health = Awaited<ReturnType<typeof import("@/lib/api").api.getHealthDetailed>>;
type Runtime = Awaited<ReturnType<typeof import("@/lib/api").api.getHealthDetail>>;

type Props = {
  health: Health | undefined;
  runtime: Runtime | undefined;
};

export function SettingsRuntimeSection({ health, runtime }: Props) {
  const proxy = runtime?.proxy ?? health?.proxy;
  const database = runtime?.database ?? health?.database;
  // An optional feature that is off is a neutral fact, not a failure.
  const optional = (on: boolean | undefined): Tone => (on ? "pass" : "neutral");
  const uptime = runtime?.uptime_seconds ?? health?.uptime_seconds;

  return (
    <div className="grid gap-6 lg:grid-cols-2">
      <Panel title="Transport and storage">
        <DetailList
          items={[
            {
              label: "Proxy",
              value: (
                <StatusBadge tone={proxy === "up" ? "pass" : proxy ? "critical" : "neutral"}>{proxy ?? "unknown"}</StatusBadge>
              ),
            },
            {
              label: "TLS",
              value: (
                <StatusBadge tone={runtime?.tls_enabled ? "pass" : "medium"}>
                  {runtime?.tls_enabled ? "enabled" : "plain HTTP"}
                </StatusBadge>
              ),
            },
            {
              label: "Mutual TLS",
              value: (
                <StatusBadge tone={optional(runtime?.mtls_enabled)}>{runtime?.mtls_enabled ? "required" : "off"}</StatusBadge>
              ),
            },
            {
              label: "Rate limit state",
              value: (
                <StatusBadge tone={optional(runtime?.redis_enabled)}>
                  {runtime?.redis_enabled ? "Redis, shared" : "in memory, single node"}
                </StatusBadge>
              ),
            },
            {
              label: "Database",
              value: (
                <StatusBadge
                  tone={database === "connected" ? "pass" : !database || database === "not_configured" ? "neutral" : "critical"}
                >
                  {(database ?? "unknown").replace(/_/g, " ")}
                </StatusBadge>
              ),
            },
          ]}
        />
      </Panel>

      <Panel title="Build and policy" aside={<span className="font-mono">/api/v1/health/detail</span>}>
        <DetailList
          items={[
            { label: "Version", value: runtime?.version || "—", mono: true },
            { label: "Policy", value: runtime?.policy_name || "—", mono: true },
            { label: "Policy file", value: runtime?.policy_path ?? health?.policy_path ?? "—", mono: true },
            { label: "Scanners", value: runtime?.scanner_count ?? health?.scanner_count ?? 0, mono: true },
            { label: "Uptime", value: typeof uptime === "number" ? formatUptime(uptime) : "—", mono: true },
          ]}
        />
      </Panel>
    </div>
  );
}
