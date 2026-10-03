"use client";

import { useEffect, useState } from "react";
import { Check, Copy } from "lucide-react";
import { Button } from "@/components/ui/button";

interface CopyButtonProps extends Omit<React.ComponentProps<typeof Button>, "onClick" | "value"> {
  /** The text put on the clipboard. */
  value: string;
  /** What is being copied, for the accessible name: "request ID", "key prefix". */
  label: string;
}

/** Copies a value and confirms with a check mark for a moment. */
export function CopyButton({ value, label, variant = "ghost", size = "icon-xs", children, ...props }: CopyButtonProps) {
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!copied) return;
    const timer = window.setTimeout(() => setCopied(false), 1500);
    return () => window.clearTimeout(timer);
  }, [copied]);

  return (
    <Button
      type="button"
      variant={variant}
      size={size}
      aria-label={children ? undefined : `Copy ${label}`}
      title={`Copy ${label}`}
      onClick={(e) => {
        e.stopPropagation();
        navigator.clipboard
          ?.writeText(value)
          .then(() => setCopied(true))
          .catch(() => {});
      }}
      {...props}
    >
      {copied ? <Check className="text-status-pass" aria-hidden /> : <Copy aria-hidden />}
      {children}
      <span className="sr-only" aria-live="polite">
        {copied ? "Copied" : ""}
      </span>
    </Button>
  );
}
