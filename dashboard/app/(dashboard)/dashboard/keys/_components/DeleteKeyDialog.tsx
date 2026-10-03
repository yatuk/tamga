"use client";

import { useState } from "react";
import { FormField } from "@/components/app/form-field";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";

type Props = {
  /** The key to revoke; the dialog is open while this is set. */
  target: { id: string; label: string } | null;
  onClose: () => void;
  onDelete: (id: string) => void;
  isPending: boolean;
};

/** Revoking cannot be undone, so the name has to be typed to confirm. */
export function DeleteKeyDialog({ target, onClose, onDelete, isPending }: Props) {
  return (
    <Dialog open={!!target} onOpenChange={(open) => (open ? undefined : onClose())}>
      <DialogContent className="sm:max-w-md">
        {/* Keyed by the target so the typed confirmation resets for every key. */}
        {target ? <RevokeForm key={target.id} target={target} onClose={onClose} onDelete={onDelete} isPending={isPending} /> : null}
      </DialogContent>
    </Dialog>
  );
}

function RevokeForm({ target, onClose, onDelete, isPending }: Omit<Props, "target"> & { target: { id: string; label: string } }) {
  const [typed, setTyped] = useState("");
  const confirmed = typed === target.label;

  return (
    <form
      className="space-y-5"
      onSubmit={(e) => {
        e.preventDefault();
        if (confirmed) onDelete(target.id);
      }}
    >
      <DialogHeader>
        <DialogTitle>Revoke API key {target.label}?</DialogTitle>
        <DialogDescription>
          Applications using this key are rejected immediately. This cannot be undone.
        </DialogDescription>
      </DialogHeader>

      <FormField
        label={`Type ${target.label} to confirm`}
        htmlFor="revoke-key-confirm"
      >
        <Input
          id="revoke-key-confirm"
          name="confirm"
          autoComplete="off"
          spellCheck={false}
          className="font-mono"
          value={typed}
          onChange={(e) => setTyped(e.target.value)}
        />
      </FormField>

      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose}>
          Cancel
        </Button>
        <Button type="submit" variant="destructive" disabled={!confirmed || isPending}>
          {isPending ? "Revoking…" : "Revoke Key"}
        </Button>
      </DialogFooter>
    </form>
  );
}
