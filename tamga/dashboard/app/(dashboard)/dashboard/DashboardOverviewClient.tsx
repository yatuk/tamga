"use client";

import { OverviewProvider } from "./OverviewContext";
import { OverviewViewPartA } from "./OverviewViewPartA";
import { OverviewViewPartB } from "./OverviewViewPartB";
import { OverviewViewPartC } from "./OverviewViewPartC";
import { useOverviewPage } from "./useOverviewPage";

export function DashboardOverviewClient() {
  const value = useOverviewPage();
  return (
    <OverviewProvider value={value}>
      <div className="dashboard-grid">
        <div className="col-span-12"><OverviewViewPartA /></div>
        <div className="col-span-12"><OverviewViewPartB /></div>
        <div className="col-span-12"><OverviewViewPartC /></div>
      </div>
    </OverviewProvider>
  );
}
