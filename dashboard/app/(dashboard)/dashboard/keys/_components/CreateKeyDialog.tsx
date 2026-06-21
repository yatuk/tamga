"use client";

import { useState } from "react";
import { X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { KeyScope } from "../_constants";

type Props = {
  open: boolean;
  onClose: () => void;
  onCreate: (label: string, scope: string) => void;
  isPending: boolean;
};

const SCOPES: { value: KeyScope; label: string; desc: string }[] = [
  { value: "read", label: "Read-only", desc: "View stats, events — no modifications" },
  { value: "write", label: "Read & Write", desc: "Manage incidents, policies, patterns" },
  { value: "admin", label: "Full Admin", desc: "Full access including key management" },
];

export function CreateKeyDialog({ open, onClose, onCreate, isPending }: Props) {
  const [label, setLabel] = useState("");
  const [scope, setScope] = useState<KeyScope>("read");

  if (!open) return null;

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!label.trim()) return;
    onCreate(label.trim(), scope);
    setLabel("");
    setScope("read");
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="w-full max-w-sm rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-5 shadow-lg">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-sm font-semibold text-zinc-900 dark:text-zinc-100">
            Create API Key
          </h2>
          <button
            aria-label="Close dialog"
            onClick={onClose}
            className="cursor-pointer rounded-sm p-1 text-zinc-500 hover:bg-zinc-100 dark:hover:bg-zinc-900"
            type="button"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-[11px] font-medium text-zinc-600 dark:text-zinc-400 mb-1">
              Name
            </label>
            <Input
              value={label}
              onChange={(e) => setLabel(e.target.value)}
              placeholder="e.g. Production, CI Pipeline"
              className="rounded-sm"
              required
              autoFocus
            />
          </div>

          <div>
            <label className="block text-[11px] font-medium text-zinc-600 dark:text-zinc-400 mb-2">
              Scope
            </label>
            <div className="space-y-1.5">
              {SCOPES.map((s) => (
                <label
                  key={s.value}
                  className={`flex cursor-pointer items-start gap-2 rounded-sm border p-2 text-xs ${
                    scope === s.value
                      ? "border-emerald-500/40 bg-emerald-500/5"
                      : "border-zinc-200 dark:border-zinc-800 hover:bg-zinc-50 dark:hover:bg-zinc-900/60"
                  }`}
                >
                  <input
                    type="radio"
                    name="scope"
                    value={s.value}
                    checked={scope === s.value}
                    onChange={() => setScope(s.value)}
                    className="mt-0.5"
                  />
                  <div>
                    <div className="font-mono text-zinc-800 dark:text-zinc-200">{s.label}</div>
                    <div className="text-zinc-500">{s.desc}</div>
                  </div>
                </label>
              ))}
            </div>
          </div>

          <div className="flex justify-end gap-2 pt-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="cursor-pointer rounded-sm"
              onClick={onClose}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              size="sm"
              disabled={!label.trim() || isPending}
              className="cursor-pointer rounded-sm bg-emerald-600 text-white hover:bg-emerald-700"
            >
              {isPending ? "Creating..." : "Create"}
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
}
