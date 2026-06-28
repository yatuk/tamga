"use client";

import { useQuery } from "@tanstack/react-query";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { api } from "@/lib/api/client";
import { toUpperLocale } from "@/lib/utils/tr-string";
import type { TimeRange } from "@/lib/types";

export function ActiveModelsCard({
  adminKey,
  range = "7d",
}: {
  adminKey: string;
  range?: TimeRange;
}) {
  const { data, isLoading } = useQuery({
    queryKey: ["model-stats", adminKey, range],
    queryFn: () => api.getModelStats(adminKey, range),
    enabled: !!adminKey,
    staleTime: 60 * 1000,
  });

  const byFamily = data?.by_family ?? {};
  const byModel = data?.by_model ?? {};

  const families = Object.entries(byFamily).sort((a, b) => b[1] - a[1]);
  const models = Object.entries(byModel).sort((a, b) => b[1] - a[1]).slice(0, 8);
  const total = families.reduce((s, [, v]) => s + v, 0);

  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="text-[10px] uppercase tracking-[0.18em] text-zinc-600 dark:text-zinc-400">
          ACTIVE MODELS // {toUpperLocale(range)}
        </div>
        <CardTitle className="text-sm">
          {isLoading ? "—" : `${families.length} famil${families.length === 1 ? "y" : "ies"} · ${models.length} model`}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {isLoading ? (
          <div className="text-xs text-zinc-600 dark:text-zinc-400">Yükleniyor…</div>
        ) : families.length === 0 ? (
          <div className="text-xs text-zinc-600 dark:text-zinc-400">Henüz veri yok.</div>
        ) : (
          <>
            {/* Family bar chart */}
            <div className="space-y-1.5">
              {families.map(([fam, count]) => {
                const pct = total > 0 ? Math.round((count / total) * 100) : 0;
                return (
                  <div key={fam} className="space-y-0.5">
                    <div className="flex justify-between text-[10px] text-zinc-600 dark:text-zinc-400">
                      <span>{fam}</span>
                      <span>{count} ({pct}%)</span>
                    </div>
                    <div className="h-1 w-full rounded-full bg-zinc-200 dark:bg-zinc-800">
                      <div
                        className="h-1 rounded-full bg-blue-500"
                        style={{ width: `${pct}%` }}
                      />
                    </div>
                  </div>
                );
              })}
            </div>

            {/* Top models list */}
            {models.length > 0 && (
              <div className="border-t border-zinc-200 dark:border-zinc-800 pt-2">
                <div className="mb-1.5 text-[9px] uppercase tracking-widest text-zinc-600 dark:text-zinc-400">
                  Top models
                </div>
                <div className="space-y-1">
                  {models.map(([model, count]) => (
                    <div key={model} className="flex justify-between text-[10px]">
                      <span className="truncate text-zinc-700 dark:text-zinc-300">{model}</span>
                      <span className="ml-2 shrink-0 text-zinc-600 dark:text-zinc-400">{count}</span>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}
