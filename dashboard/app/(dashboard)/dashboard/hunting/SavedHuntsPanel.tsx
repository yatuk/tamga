"use client";

import { Crosshair, Trash2 } from "lucide-react";
import type { SavedHunt } from "./_types";

type Props = {
  savedHunts: SavedHunt[];
  onApply: (h: SavedHunt) => void;
  onDelete: (id: string) => void;
};

export function SavedHuntsPanel({ savedHunts, onApply, onDelete }: Props) {
  return (
    <div className="rounded-sm border border-border bg-surface-card/60 p-3">
      <div className="mb-2 flex items-center gap-2 text-xs uppercase tracking-wide text-fg-muted">
        <Crosshair className="h-3 w-3" />
        Saved hunts
      </div>
      {savedHunts.length === 0 ? (
        <p className="text-xs text-fg-muted">No saved hunts yet.</p>
      ) : (
        <ul className="space-y-2">
          {savedHunts.map((h) => (
            <li key={h.id} className="flex items-start justify-between gap-2 rounded-sm border border-border bg-surface-subtle p-2">
              <div className="min-w-0 flex-1">
                <button
                  type="button"
                  onClick={() => onApply(h)}
                  className="block w-full text-left text-xs text-fg hover:text-white"
                >
                  {h.name}
                </button>
                <div className="mt-0.5 text-xs text-fg-subtle">
                  {new Date(h.updated_at).toLocaleString("en-GB")}
                </div>
              </div>
              <button
                type="button"
                aria-label="Delete hunt"
                className="text-fg-muted hover:text-status-critical"
                onClick={() => onDelete(h.id)}
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
