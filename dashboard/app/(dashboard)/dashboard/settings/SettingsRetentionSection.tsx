"use client";

import { Button } from "@/components/ui/button";
import { Panel } from "@/components/app/panel";
import { Input } from "@/components/ui/input";

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
          <div className="text-xs text-fg-muted">
            {"//"} Day limit applied in the dashboard only. Database retention is set in the proxy configuration; this
            preference affects UI filters only.
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <label className="text-xs uppercase tracking-wide text-fg-muted">DAYS</label>
            <Input
              type="number"
              min={1}
              max={365}
              value={retention}
              onChange={(e) => setRetention(e.target.value)}
              className="w-28"
            />
            <Button variant="outline" className="rounded-sm bg-status-critical text-white hover:bg-status-critical" onClick={saveRetention}>
              Save
            </Button>
          </div>
        </div>
      </Panel>
    </div>
  );
}
