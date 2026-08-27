"use client";

import { Copy, Check } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { RevealedKey } from "../useKeysPage";

type Props = {
  revealed: RevealedKey;
  onDismiss: () => void;
  onCopy: (text: string) => void;
};

export function KeyRevealDialog({ revealed, onDismiss, onCopy }: Props) {
  if (!revealed) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="w-full max-w-md rounded-sm border border-status-medium/20 bg-surface-card p-5 shadow-lg">
        <div className="mb-4 flex items-start gap-3">
          <span className="mt-0.5 text-lg">⚠</span>
          <div>
            <h2 className="text-sm font-semibold text-status-medium">
              Copy this key now — it will not be shown again
            </h2>
            <p className="mt-1 text-[11px] text-fg-subtle">
              Key name: <span className="font-mono text-fg-muted">{revealed.label}</span>
            </p>
          </div>
        </div>

        <div className="rounded-sm border border-border bg-surface-subtle p-3">
          <code className="break-all text-xs font-mono text-fg select-all">
            {revealed.rawKey}
          </code>
        </div>

        <div className="mt-4 flex justify-between gap-2">
          <Button
            size="sm"
            variant="outline"
            className="cursor-pointer rounded-sm"
            onClick={() => onCopy(revealed.rawKey)}
          >
            <Copy className="mr-1 h-3.5 w-3.5" /> Copy
          </Button>
          <Button
            size="sm"
            className="cursor-pointer rounded-sm bg-status-pass text-white hover:bg-status-pass"
            onClick={onDismiss}
          >
            <Check className="mr-1 h-3.5 w-3.5" /> I have saved this key
          </Button>
        </div>
      </div>
    </div>
  );
}
