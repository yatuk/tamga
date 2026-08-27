"use client";

import { useState } from "react";
import { AlertTriangle, RotateCcw } from "lucide-react";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { Button } from "@/components/ui/button";
import { api } from "@/lib/api";
import type { DashboardHealthDetailed } from "@/lib/api/types-core";
import { toast } from "@/lib/toast";
import { toLowerEn, toUpperEn } from "@/lib/utils/tr-string";
import { useQueryClient } from "@tanstack/react-query";

function stateClass(state: string): string {
  const s = toLowerEn(state);
  if (s === "closed") return "text-status-pass";
  if (s === "open") return "text-status-critical";
  if (s === "half-open") return "text-status-medium";
  return "text-fg-muted";
}

type Props = {
  health: DashboardHealthDetailed | undefined;
  adminKey: string;
};

export function SettingsProvidersSection({ health, adminKey }: Props) {
  const pools = health?.providers ?? [];
  const [pending, setPending] = useState<string | null>(null);
  const qc = useQueryClient();

  async function resetCircuit(pool: string, endpoint: string) {
    const key = `${pool}:${endpoint}`;
    setPending(key);
    try {
      await api.resetUpstreamCircuit(adminKey, pool, endpoint);
      toast.success("Circuit sıfırlandı", `${pool} / ${endpoint}`);
      await qc.invalidateQueries({ queryKey: ["tamga-settings-health"] });
    } catch (e) {
      toast.error("Sıfırlanamadı", (e as Error).message);
    } finally {
      setPending(null);
    }
  }

  return (
    <div>
      <div className="space-y-2">
        <p className="text-sm text-[var(--text-secondary)]">
          Policy <code className="text-xs text-fg-muted">providers.pools</code> için circuit breaker
          durumu. Açık (open) devrelerde trafik bu endpoint&apos;e gitmez; bakım sonrası manuel sıfırlayın.
        </p>

        {pools.length === 0 ? (
          <div className="rounded-sm border border-border bg-surface-card px-4 py-6 text-sm text-fg-muted">
            Henüz provider havuzu yok — health/detailed içinde <code className="text-xs">providers</code>{" "}
            alanı boş. Politikada <code className="text-xs">providers.pools</code> tanımlayıp proxy&apos;yi
            yeniden yükleyin.
          </div>
        ) : (
          <div className="space-y-3">
            {pools.map((pl) => (
              <TerminalFrame
                key={pl.pool}
                title={`${toUpperEn(pl.pool.charAt(0)) + pl.pool.slice(1)} Havuzu`}

                status={
                  <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-fg-muted">
                    {pl.healthy_count}/{pl.total_count} healthy
                  </span>
                }
              >
                <div className="divide-y divide-border">
                  {pl.providers.map((p) => {
                    const isOpen = toLowerEn(p.state) === "open";
                    const busy = pending === `${pl.pool}:${p.name}`;
                    return (
                      <div
                        key={p.name}
                        className="flex flex-wrap items-center justify-between gap-3 px-3 py-2.5 text-xs"
                      >
                        <div className="min-w-0 flex-1 space-y-0.5">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="text-fg">{p.name}</span>
                            <span className={`rounded-sm border border-border-strong px-1.5 py-0.5 ${stateClass(p.state)}`}>
                              {p.state}
                            </span>
                            {isOpen ? (
                              <span className="inline-flex items-center gap-1 text-status-critical/90">
                                <AlertTriangle className="h-3 w-3" />
                                devre dışı
                              </span>
                            ) : null}
                          </div>
                          <div className="text-[11px] text-fg-muted">
                            req window: {p.requests_in_window ?? "—"} · success rate:{" "}
                            {typeof p.success_rate_observed === "number"
                              ? `${(p.success_rate_observed * 100).toFixed(1)}%`
                              : "—"}
                            {p.last_failure ? ` · last fail: ${p.last_failure}` : ""}
                            {p.failure_reason ? ` (${p.failure_reason})` : ""}
                          </div>
                        </div>
                        <Button
                          type="button"
                          disabled={busy || !adminKey}
                          className="h-8 shrink-0 cursor-pointer rounded-sm border border-border-strong bg-surface-subtle px-2 text-[11px] text-fg hover:bg-surface-card disabled:opacity-50"
                          onClick={() => void resetCircuit(pl.pool, p.name)}
                          title="Breaker sayacını sıfırla (yeni devre örneği)"
                        >
                          <RotateCcw className="mr-1 h-3 w-3" />
                          {busy ? "…" : "Reset circuit"}
                        </Button>
                      </div>
                    );
                  })}
                </div>
              </TerminalFrame>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
