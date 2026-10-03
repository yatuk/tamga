"use client";

import { CopyButton } from "@/components/app/copy-button";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import type { RevealedKey } from "../useKeysPage";

type Props = {
  revealed: RevealedKey;
  onDismiss: () => void;
};

/** Shows a new key once. It closes only through its own button, not by clicking away. */
export function KeyRevealDialog({ revealed, onDismiss }: Props) {
  return (
    <Dialog open={!!revealed} onOpenChange={(open) => (open ? undefined : onDismiss())}>
      <DialogContent
        className="sm:max-w-lg"
        showCloseButton={false}
        onInteractOutside={(e) => e.preventDefault()}
        onEscapeKeyDown={(e) => e.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle>Copy the key now</DialogTitle>
          <DialogDescription>
            This is the only time the full key for <span className="font-mono text-foreground">{revealed?.label}</span> is
            shown. Store it in your secret manager.
          </DialogDescription>
        </DialogHeader>

        <div className="flex items-start gap-2 border bg-background p-3">
          <code className="min-w-0 flex-1 font-mono text-xs break-all select-all" translate="no">
            {revealed?.rawKey}
          </code>
          <CopyButton value={revealed?.rawKey ?? ""} label="API key" variant="outline" size="sm">
            Copy
          </CopyButton>
        </div>

        <DialogFooter>
          <Button onClick={onDismiss}>I Have Saved the Key</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
