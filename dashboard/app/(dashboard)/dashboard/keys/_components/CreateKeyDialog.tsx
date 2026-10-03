"use client";

import { useState } from "react";
import { FormField } from "@/components/app/form-field";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";
import type { KeyScope } from "../_constants";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreate: (label: string, scope: string) => void;
  isPending: boolean;
};

const SCOPES: { value: KeyScope; label: string; desc: string }[] = [
  { value: "read", label: "Read", desc: "Send requests and read stats and events." },
  { value: "write", label: "Write", desc: "Also triage incidents and edit policies and patterns." },
  { value: "admin", label: "Admin", desc: "Everything, including managing other keys." },
];

export function CreateKeyDialog({ open, onOpenChange, onCreate, isPending }: Props) {
  const [label, setLabel] = useState("");
  const [scope, setScope] = useState<KeyScope>("read");

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <form
          className="space-y-5"
          onSubmit={(e) => {
            e.preventDefault();
            if (!label.trim()) return;
            onCreate(label.trim(), scope);
            setLabel("");
            setScope("read");
          }}
        >
          <DialogHeader>
            <DialogTitle>Create API key</DialogTitle>
            <DialogDescription>The key is shown once, right after you create it.</DialogDescription>
          </DialogHeader>

          <FormField label="Name" htmlFor="new-key-name" hint="Name it after the application that will use it.">
            <Input
              id="new-key-name"
              name="label"
              autoComplete="off"
              value={label}
              onChange={(e) => setLabel(e.target.value)}
              placeholder="billing-service…"
              required
            />
          </FormField>

          <fieldset className="space-y-2">
            <legend className="mb-1.5 text-xs text-muted-foreground">Scope</legend>
            {SCOPES.map((s) => (
              <div
                key={s.value}
                className={cn(
                  "flex items-start gap-3 border p-3 text-sm has-focus-visible:ring-[3px] has-focus-visible:ring-ring/50",
                  scope === s.value ? "border-foreground/40 bg-accent" : "hover:bg-accent/50",
                )}
              >
                <input
                  type="radio"
                  id={`new-key-scope-${s.value}`}
                  name="scope"
                  value={s.value}
                  checked={scope === s.value}
                  onChange={() => setScope(s.value)}
                  aria-describedby={`new-key-scope-${s.value}-desc`}
                  className="mt-1 accent-foreground outline-none"
                />
                <div>
                  <Label htmlFor={`new-key-scope-${s.value}`}>{s.label}</Label>
                  <p id={`new-key-scope-${s.value}-desc`} className="mt-0.5 text-xs text-muted-foreground">
                    {s.desc}
                  </p>
                </div>
              </div>
            ))}
          </fieldset>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={!label.trim() || isPending}>
              {isPending ? "Creating…" : "Create Key"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
