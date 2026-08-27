"use client";

import { type FormEvent, useCallback, useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";

interface Props {
  open: boolean;
  onClose: () => void;
  onConfirm: (reason: string) => void;
  title?: string;
  description?: string;
  placeholder?: string;
  confirmLabel?: string;
}

export function FpReasonModal({
  open,
  onClose,
  onConfirm,
  title = "Mark as False Positive",
  description = "Please provide a reason for marking this incident as a false positive. This helps tune the scanning engine.",
  placeholder = "legitimate context — e.g. pentest traffic, internal tool, test key",
  confirmLabel = "Confirm",
}: Props) {
  const [reason, setReason] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  // Reset state when modal opens
  useEffect(() => {
    if (open) {
      setReason("");
      // Focus input after mount
      requestAnimationFrame(() => inputRef.current?.focus());
    }
  }, [open]);

  // Close on Escape
  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [open, onClose]);

  const handleSubmit = useCallback(
    (e: FormEvent) => {
      e.preventDefault();
      onConfirm(reason.trim() || placeholder);
    },
    [reason, onConfirm, placeholder],
  );

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/80"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
      aria-label={title}
    >
      <div
        className="relative w-full max-w-md rounded-sm border border-border bg-surface-base p-6 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Title */}
        <h2 className="text-lg font-semibold text-fg">{title}</h2>

        {/* Description */}
        <p className="mt-1 text-sm text-fg-subtle">{description}</p>

        {/* Input */}
        <form onSubmit={handleSubmit}>
          <input
            ref={inputRef}
            type="text"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder={placeholder}
            className="mt-4 w-full rounded-sm border border-border bg-surface-subtle px-3 py-2 text-sm text-fg placeholder:text-fg-muted focus:border-border-strong focus:outline-none"
          />

          {/* Footer */}
          <div className="mt-6 flex items-center justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={onClose}
              className="h-8 cursor-pointer rounded-sm border-border-strong bg-transparent px-3 text-xs text-fg-subtle hover:bg-surface-subtle hover:text-fg"
            >
              Cancel
            </Button>
            <Button
              type="submit"
              size="sm"
              className="h-8 cursor-pointer rounded-sm border border-status-medium/40 bg-status-medium/10 px-3 text-xs text-status-medium hover:bg-status-medium/20"
            >
              {confirmLabel}
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
}
