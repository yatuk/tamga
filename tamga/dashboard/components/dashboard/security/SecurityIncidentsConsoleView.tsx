"use client";

import { useCallback, useState } from "react";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { ResizableSplitPane } from "@/components/dashboard/ResizableSplitPane";
import { IncidentsFiltersCard } from "@/components/dashboard/security/incidents-console/IncidentsFiltersCard";
import { IncidentsQueueTableCard } from "@/components/dashboard/security/incidents-console/IncidentsQueueTableCard";
import { IncidentsSavedViewsColumn } from "@/components/dashboard/security/incidents-console/IncidentsSavedViewsColumn";
import { IncidentDetailPanel } from "@/components/dashboard/security/incidents-console/IncidentDetailPanel";
import { FpReasonModal } from "@/components/dashboard/security/incidents-console/FpReasonModal";
import type { IncidentsConsoleModel } from "@/hooks/security/useSecurityIncidentsConsole";

export function SecurityIncidentsConsoleView({ m }: { m: IncidentsConsoleModel }) {
  const [fpRequestId, setFpRequestId] = useState<string | null>(null);

  const handleFpClick = useCallback((requestId: string) => {
    setFpRequestId(requestId);
  }, []);

  const handleFpConfirm = useCallback(
    (reason: string) => {
      if (fpRequestId) {
        m.markFalsePositive(fpRequestId, reason);
      }
      setFpRequestId(null);
    },
    [fpRequestId, m],
  );

  const handleFpClose = useCallback(() => {
    setFpRequestId(null);
  }, []);

  return (
    <div className="space-y-2">
      <PageHeader
        eyebrow="TRIAGE"
        title="Incidents"
        subtitle={`${m.filtered.length} matching · triage ${m.triageFilter} · range ${m.timeRange}`}
      />

      {/* Filters + saved views bar */}
      <div className="flex flex-wrap items-start gap-2">
        <div className="flex-1 min-w-0">
          <IncidentsFiltersCard m={m} />
        </div>
        <div className="w-[220px] shrink-0">
          <IncidentsSavedViewsColumn m={m} />
        </div>
      </div>

      {/* War Room: split-pane */}
      <ResizableSplitPane
        left={<IncidentsQueueTableCard m={m} onFpClick={handleFpClick} />}
        right={<IncidentDetailPanel event={m.selected} m={m} onFpClick={handleFpClick} />}
        defaultLeftWidth="62%"
        storageKey="tamga-incidents-split"
        minLeftPx={400}
        minRightPx={300}
      />

      {/* FP Reason Modal */}
      <FpReasonModal
        open={fpRequestId !== null}
        onClose={handleFpClose}
        onConfirm={handleFpConfirm}
      />
    </div>
  );
}
