"use client";

import { useQuery } from "@tanstack/react-query";
import { BarList } from "@/components/app/bar-list";
import { Panel } from "@/components/app/panel";
import { EmptyState, SkeletonRows } from "@/components/app/states";
import { api } from "@/lib/api/client";
import type { TimeRange } from "@/lib/types";

/** Which models the traffic in the window went to. */
export function ActiveModelsCard({ adminKey, range = "7d" }: { adminKey: string; range?: TimeRange }) {
  const { data, isLoading } = useQuery({
    queryKey: ["tamga-model-stats", adminKey, range],
    queryFn: () => api.getModelStats(adminKey, range),
    enabled: !!adminKey,
    staleTime: 60 * 1000,
  });

  const models = Object.entries(data?.by_model ?? {})
    .sort((a, b) => b[1] - a[1])
    .slice(0, 6);
  const families = Object.keys(data?.by_family ?? {}).length;

  return (
    <Panel
      title="Models"
      description="Share of requests"
      aside={models.length > 0 ? `${families} ${families === 1 ? "family" : "families"}` : undefined}
    >
      {isLoading ? (
        <SkeletonRows rows={3} />
      ) : models.length === 0 ? (
        <EmptyState icon="chart" title="No traffic" />
      ) : (
        <BarList items={models.map(([label, value]) => ({ label, value }))} mono />
      )}
    </Panel>
  );
}
