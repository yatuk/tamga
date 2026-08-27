"use client";

import { Button } from "@/components/ui/button";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";

type Props = {
  retention: string;
  setRetention: (v: string) => void;
  saveRetention: () => void;
};

export function SettingsRetentionSection({ retention, setRetention, saveRetention }: Props) {
  return (
    <div>
      <TerminalFrame title="Saklama Ayarları">
        <div className="space-y-3 p-3">
          <div className="text-[11px] text-fg-muted">
            {"//"} Dashboard üstünde uygulanan görsel gün sınırı. Veritabanı retention&apos;u proxy yapılandırmasında tanımlanır; bu
            tercih yalnızca UI filtreleri için kullanılır.
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
            <Button className="cursor-pointer rounded-sm bg-status-critical text-white hover:bg-status-critical" onClick={saveRetention}>
              Kaydet
            </Button>
          </div>
        </div>
      </TerminalFrame>
    </div>
  );
}
