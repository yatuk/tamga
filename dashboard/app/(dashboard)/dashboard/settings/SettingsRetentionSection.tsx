"use client";

import { Button } from "@/components/ui/button";
import { Panel } from "@/components/app/panel";

type Props = {
  retention: string;
  setRetention: (v: string) => void;
  saveRetention: () => void;
};

export function SettingsRetentionSection({ retention, setRetention, saveRetention }: Props) {
  return (
    <div>
      <Panel title="Retention">
        <div className="space-y-3 p-3">
          <div className="text-[11px] text-fg-muted">
            {"//"} Day limit applied in the dashboard only. Database retention is set in the proxy configuration; this
            preference affects UI filters only.
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <label className="text-[10px] uppercase tracking-wide text-fg-muted">DAYS</label>
            <input
              type="number"
              min={1}
              max={365}
              value={retention}
              onChange={(e) => setRetention(e.target.value)}
              className="h-9 w-28 rounded-sm border border-border bg-surface-card px-2 text-sm text-fg focus:outline-none"
            />
            <Button variant="outline" className="cursor-pointer rounded-sm bg-status-critical text-white hover:bg-status-critical" onClick={saveRetention}>
              Save
            </Button>
          </div>
        </div>
      </Panel>
    </div>
  );
}
