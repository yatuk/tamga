"use client";

import { Panel } from "@/components/app/panel";
import { PolicyDiff } from "@/components/dashboard/policies/PolicyDiff";

type Props = {
  adminKey: string;
};

export function PoliciesHistoryPanel({ adminKey }: Props) {
  return (
    <Panel title="Policy history">
      <div className="space-y-3 p-3">
        <PolicyDiff adminKey={adminKey} />
      </div>
    </Panel>
  );
}
