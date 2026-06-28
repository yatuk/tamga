"use client";

import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { SettingsStatusChip } from "./SettingsStatusChip";

type Health = Awaited<ReturnType<typeof import("@/lib/api").api.getHealthDetailed>>;
type Runtime = Awaited<ReturnType<typeof import("@/lib/api").api.getHealthDetail>>;

type Props = {
  health: Health | undefined;
  runtime: Runtime | undefined;
};

export function SettingsRuntimeSection({ health, runtime }: Props) {
  return (
    <div>
      <div className="space-y-4">
        <div className="flex flex-wrap gap-2">
          <SettingsStatusChip label="proxy" value={runtime?.proxy ?? health?.proxy ?? "?"} good={(runtime?.proxy ?? health?.proxy) === "up"} />
          <SettingsStatusChip label="tls" value={runtime?.tls_enabled ? "enabled" : "plain http"} good={!!runtime?.tls_enabled} />
          <SettingsStatusChip
            label="mtls"
            value={runtime?.mtls_enabled ? "required" : "disabled"}
            good={!!runtime?.mtls_enabled}
            neutral={!runtime?.mtls_enabled}
          />
          <SettingsStatusChip
            label="redis"
            value={runtime?.redis_enabled ? "distributed" : "single-node"}
            good={!!runtime?.redis_enabled}
            neutral={!runtime?.redis_enabled}
          />
          <SettingsStatusChip
            label="database"
            value={runtime?.database ?? health?.database ?? "?"}
            good={(runtime?.database ?? health?.database) === "connected"}
            neutral={(runtime?.database ?? health?.database) === "not_configured"}
          />
        </div>

        <TerminalFrame title="Çalışma Durumu">
          <div className="space-y-1 p-3 text-xs text-zinc-700 dark:text-zinc-300">
            <div>
              version: <span className="text-zinc-900 dark:text-zinc-100">{runtime?.version || "—"}</span>
            </div>
            <div>
              policy_name: <span className="text-zinc-900 dark:text-zinc-100">{runtime?.policy_name || "—"}</span>
            </div>
            <div>
              scanner_count: <span className="text-zinc-900 dark:text-zinc-100">{runtime?.scanner_count ?? health?.scanner_count ?? 0}</span>
            </div>
            <div>
              uptime: <span className="text-zinc-900 dark:text-zinc-100">{runtime?.uptime_seconds ?? health?.uptime_seconds ?? 0}s</span>
            </div>
            <div className="text-[11px] text-zinc-600 dark:text-zinc-400">policy_path: {runtime?.policy_path ?? health?.policy_path ?? "—"}</div>
            <div className="text-[11px] text-zinc-600 dark:text-zinc-400">
              endpoint: <code>/api/v1/health/detail</code>
            </div>
          </div>
        </TerminalFrame>
      </div>
    </div>
  );
}
