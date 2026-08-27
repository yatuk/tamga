"use client";

import dynamic from "next/dynamic";

export const OverviewTrafficChart = dynamic(
  () => import("@/components/dashboard/charts/OverviewTrafficChart").then((m) => m.OverviewTrafficChart),
  {
    ssr: false,
    loading: () => <div className="h-[320px] w-full animate-pulse rounded-sm bg-surface-subtle" />,
  },
);

export const OverviewProviderPie = dynamic(
  () => import("@/components/dashboard/charts/OverviewProviderPie").then((m) => m.OverviewProviderPie),
  {
    ssr: false,
    loading: () => <div className="mx-auto h-[220px] w-full animate-pulse rounded-sm bg-surface-subtle" />,
  },
);
