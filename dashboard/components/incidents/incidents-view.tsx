"use client";

import { useState } from "react";
import dynamic from "next/dynamic";
import { PageHeader } from "@/components/app/page-header";
import { useSecurityIncidentsConsole } from "@/hooks/security/useSecurityIncidentsConsole";
import { IncidentDetail } from "./incident-detail";
import { IncidentsTable } from "./incidents-table";
import { IncidentsToolbar } from "./incidents-toolbar";

// Only needed once an analyst marks something as a false positive.
const FalsePositiveDialog = dynamic(() => import("./false-positive-dialog").then((m) => m.FalsePositiveDialog), { ssr: false });

/** Triage console: filter bar, the virtualised queue and the selected incident. */
export function IncidentsView() {
  const m = useSecurityIncidentsConsole();
  const [fpRequestId, setFpRequestId] = useState<string | null>(null);

  return (
    <div className="space-y-4">
      <PageHeader
        title="Incidents"
        description="Blocked, redacted and warned requests waiting for a triage decision."
      />
      <IncidentsToolbar m={m} />

      <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,1fr)_26rem]">
        <IncidentsTable m={m} onFalsePositive={setFpRequestId} />
        <div className="xl:sticky xl:top-18">
          <IncidentDetail event={m.selected} m={m} onFalsePositive={setFpRequestId} />
        </div>
      </div>

      {fpRequestId !== null ? (
        <FalsePositiveDialog
          open
          onClose={() => setFpRequestId(null)}
          onConfirm={(reason) => {
            m.markFalsePositive(fpRequestId, reason);
            setFpRequestId(null);
          }}
        />
      ) : null}
    </div>
  );
}
