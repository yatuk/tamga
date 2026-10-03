"use client";

import { RefreshCw } from "lucide-react";
import { PageHeader } from "@/components/app/page-header";
import { Panel } from "@/components/app/panel";
import { AdminKeyRequired } from "@/components/app/states";
import { TimeRangeToggle } from "@/components/app/time-range";
import { Button } from "@/components/ui/button";
import { HuntingFilters } from "./HuntingFilters";
import { HuntingResults } from "./HuntingResults";
import { SavedHuntsPanel } from "./SavedHuntsPanel";
import { useHuntingPage } from "./useHuntingPage";

export default function HuntingPage() {
  const m = useHuntingPage();

  const header = (
    <PageHeader
      title="Threat hunting"
      description="Search every scanned request by action, finding, technique or text."
      actions={
        <>
          <TimeRangeToggle value={m.range} onChange={m.setRange} />
          <Button variant="outline" size="sm" onClick={() => void m.refetch()} disabled={m.isFetching || !m.adminKey}>
            <RefreshCw />
            Refresh
          </Button>
        </>
      }
    />
  );

  if (!m.adminKey) {
    return (
      <div className="space-y-6">
        {header}
        <Panel>
          <AdminKeyRequired />
        </Panel>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {header}

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_18rem]">
        <HuntingFilters
          filters={m.filters}
          setFilter={m.setFilter}
          shadow={m.shadow}
          setShadow={m.setShadow}
          activeFilterCount={m.activeFilterCount}
          clearFilters={m.clearFilters}
          saveHunt={m.saveHunt}
        />
        <SavedHuntsPanel savedHunts={m.savedHunts} onApply={m.applyHunt} onDelete={m.deleteHunt} />
      </div>

      <HuntingResults
        events={m.data?.events ?? []}
        total={m.data?.total ?? 0}
        page={m.page}
        setPage={m.setPage}
        isLoading={m.isLoading}
        error={m.error}
        onRetry={() => void m.refetch()}
        onClearFilters={m.activeFilterCount > 0 ? m.clearFilters : undefined}
      />
    </div>
  );
}
