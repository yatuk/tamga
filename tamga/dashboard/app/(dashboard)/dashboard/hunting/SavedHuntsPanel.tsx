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
    <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950/60 p-3">
      <div className="mb-2 flex items-center gap-2 text-[10px] uppercase tracking-wide text-zinc-600 dark:text-zinc-400">
        <Crosshair className="h-3 w-3" />
        Saved hunts
      </div>
      {savedHunts.length === 0 ? (
        <p className="text-xs text-zinc-600 dark:text-zinc-400">Henüz kayıtlı hunt yok.</p>
      ) : (
        <ul className="space-y-2">
          {savedHunts.map((h) => (
            <li key={h.id} className="flex items-start justify-between gap-2 rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-100 dark:bg-zinc-900/40 p-2">
              <div className="min-w-0 flex-1">
                <button
                  type="button"
                  onClick={() => onApply(h)}
                  className="block w-full text-left text-[11px] text-zinc-800 dark:text-zinc-200 hover:text-white"
                >
                  {h.name}
                </button>
                <div className="mt-0.5 text-[9px] text-zinc-500 dark:text-zinc-500">
                  {new Date(h.updated_at).toLocaleString("tr-TR")}
                </div>
              </div>
              <button
                type="button"
                aria-label="Delete hunt"
                className="text-zinc-600 dark:text-zinc-400 hover:text-red-400"
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
