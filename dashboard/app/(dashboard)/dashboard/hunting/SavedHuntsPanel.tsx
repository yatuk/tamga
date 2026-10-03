"use client";

import { Trash2 } from "lucide-react";
import { ConfirmButton } from "@/components/app/confirm-button";
import { Panel } from "@/components/app/panel";
import { Button } from "@/components/ui/button";
import type { SavedHunt } from "./_types";

type Props = {
  savedHunts: SavedHunt[];
  onApply: (h: SavedHunt) => void;
  onDelete: (id: string) => void;
};

export function SavedHuntsPanel({ savedHunts, onApply, onDelete }: Props) {
  return (
    <Panel title="Saved hunts" aside={savedHunts.length > 0 ? savedHunts.length : undefined}>
      {savedHunts.length === 0 ? (
        <p className="px-4 py-6 text-sm text-muted-foreground">
          No saved hunts. Build a query and choose Save Hunt to keep it here.
        </p>
      ) : (
        <ul className="divide-y">
          {savedHunts.map((h) => (
            <li key={h.id} className="flex items-center gap-1 py-1 pr-2 pl-1">
              <Button
                variant="ghost"
                className="h-auto min-w-0 flex-1 flex-col items-start gap-0.5 px-3 py-2 text-left whitespace-normal"
                onClick={() => onApply(h)}
              >
                <span className="w-full truncate text-sm">{h.name}</span>
                <span className="font-mono text-xs font-normal text-muted-foreground">
                  {new Date(h.updated_at).toLocaleString("en-GB")}
                </span>
              </Button>
              <ConfirmButton
                variant="ghost"
                size="icon-sm"
                aria-label={`Delete saved hunt ${h.name}`}
                title={`Delete saved hunt ${h.name}?`}
                description="The saved query is removed. Events are not affected."
                onConfirm={() => onDelete(h.id)}
              >
                <Trash2 />
              </ConfirmButton>
            </li>
          ))}
        </ul>
      )}
    </Panel>
  );
}
