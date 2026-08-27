"use client";

import { useState } from "react";
import { X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

type Props = {
  target: { id: string; label: string };
  onClose: () => void;
  onDelete: (id: string) => void;
  isPending: boolean;
};

export function DeleteKeyDialog({ target, onClose, onDelete, isPending }: Props) {
  const [typed, setTyped] = useState("");
  const confirmed = typed === target.label;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="w-full max-w-sm rounded-sm border border-status-critical/20 bg-surface-card p-5 shadow-lg">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-sm font-semibold text-status-critical">Revoke API Key</h2>
          <button
            aria-label="Close dialog"
            onClick={onClose}
            className="cursor-pointer rounded-sm p-1 text-fg-subtle hover:bg-surface-subtle"
            type="button"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <p className="mb-3 text-[11px] text-fg-subtle">
          This action is permanent. Type the key name <span className="font-mono text-fg-muted">{target.label}</span> to confirm.
        </p>

        <Input
          value={typed}
          onChange={(e) => setTyped(e.target.value)}
          placeholder={target.label}
          className="rounded-sm mb-4"
          autoFocus
        />

        <div className="flex justify-end gap-2">
          <Button
            variant="outline"
            size="sm"
            className="cursor-pointer rounded-sm"
            onClick={onClose}
          >
            Cancel
          </Button>
          <Button
            size="sm"
            disabled={!confirmed || isPending}
            className="cursor-pointer rounded-sm bg-status-critical text-white hover:bg-status-critical"
            onClick={() => onDelete(target.id)}
          >
            {isPending ? "Revoking..." : "Revoke Key"}
          </Button>
        </div>
      </div>
    </div>
  );
}
