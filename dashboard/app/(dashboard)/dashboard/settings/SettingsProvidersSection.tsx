"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { RotateCcw } from "lucide-react";
import { Panel } from "@/components/app/panel";
import { EmptyState } from "@/components/app/states";
import { CircuitBadge, StatusBadge } from "@/components/app/status-badge";
import { Button } from "@/components/ui/button";
import { api } from "@/lib/api";
import type { DashboardHealthDetailed } from "@/lib/api/types-core";
import { toast } from "@/lib/toast";
import { toLowerEn } from "@/lib/utils/case";

type Props = {
  health: DashboardHealthDetailed | undefined;
  adminKey: string;
};

export function SettingsProvidersSection({ health, adminKey }: Props) {
  const pools = health?.providers ?? [];
  const [pending, setPending] = useState<string | null>(null);
  const qc = useQueryClient();

  async function resetCircuit(pool: string, endpoint: string) {
    setPending(`${pool}:${endpoint}`);
    try {
      await api.resetUpstreamCircuit(adminKey, pool, endpoint);
      toast.success("Circuit reset", `${pool} / ${endpoint}`);
      await qc.invalidateQueries({ queryKey: ["tamga-settings-health"] });
    } catch (e) {
      toast.error("Could not reset the circuit", (e as Error).message);
    } finally {
      setPending(null);
    }
  }

  if (pools.length === 0) {
    return (
      <Panel title="Provider pools">
        <EmptyState
          icon="database"
          title="No provider pools"
          description={
            <>
              Define <span className="font-mono">providers.pools</span> in the policy and reload the proxy to see circuit
              breaker state here.
            </>
          }
        />
      </Panel>
    );
  }

  return (
    <div className="space-y-6">
      <p className="max-w-2xl text-sm text-muted-foreground">
        Circuit breaker state for each upstream in the policy. An open circuit receives no traffic; reset it after the
        upstream is healthy again.
      </p>
      {pools.map((pl) => (
        <Panel
          key={pl.pool}
          title={`${pl.pool} pool`}
          aside={
            <StatusBadge tone={pl.healthy_count === pl.total_count ? "pass" : pl.healthy_count === 0 ? "critical" : "medium"}>
              {pl.healthy_count}/{pl.total_count} healthy
            </StatusBadge>
          }
        >
          <ul className="divide-y">
            {pl.providers.map((p) => {
              const busy = pending === `${pl.pool}:${p.name}`;
              return (
                <li key={p.name} className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-mono text-sm">{p.name}</span>
                      <CircuitBadge state={p.state} />
                      {toLowerEn(p.state) === "open" ? (
                        <span className="text-xs text-status-critical">Out of rotation</span>
                      ) : null}
                    </div>
                    <p className="mt-1 font-mono text-xs text-muted-foreground">
                      {p.requests_in_window ?? "—"} requests in window ·{" "}
                      {typeof p.success_rate_observed === "number" ? `${(p.success_rate_observed * 100).toFixed(1)}%` : "—"}{" "}
                      success
                      {p.last_failure ? ` · last failure ${p.last_failure}` : ""}
                      {p.failure_reason ? ` (${p.failure_reason})` : ""}
                    </p>
                  </div>
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={busy || !adminKey}
                    onClick={() => void resetCircuit(pl.pool, p.name)}
                  >
                    <RotateCcw />
                    {busy ? "Resetting…" : "Reset Circuit"}
                  </Button>
                </li>
              );
            })}
          </ul>
        </Panel>
      ))}
    </div>
  );
}
