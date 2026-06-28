"use client";

import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { PolicyDiff } from "@/components/dashboard/policies/PolicyDiff";

type Props = {
  adminKey: string;
};

export function PoliciesHistoryPanel({ adminKey }: Props) {
  return (
    <TerminalFrame title="Politika Geçmişi">
      <div className="space-y-3 p-3">
        <PolicyDiff adminKey={adminKey} />
      </div>
    </TerminalFrame>
  );
}
