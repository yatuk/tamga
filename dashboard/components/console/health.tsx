"use client";

import { InlineNotification } from "@carbon/react";
import type { HealthDetailState } from "@/hooks/useHealth";

/**
 * Says that the proxy is up but kept the detail of its health back, and why.
 * Renders nothing while the detail is there or still on its way.
 */
export function HealthDetailNotice({ state }: { state: HealthDetailState }) {
  if (state !== "no-key" && state !== "rejected") return null;
  const rejected = state === "rejected";
  return (
    <InlineNotification
      kind={rejected ? "error" : "info"}
      lowContrast
      hideCloseButton
      title={rejected ? "The proxy did not accept the admin key." : "No admin key in this browser."}
      subtitle={
        rejected
          ? "It is up, but it gives the detail of its state only to a valid admin key. Check the one under Settings."
          : "The proxy is up, but it gives the detail of its state only to an admin key. Add one under Settings."
      }
      style={{ marginTop: "1.5rem", maxWidth: "none" }}
    />
  );
}

/** What stands in for a value the proxy keeps back without an admin key. */
export const NEEDS_KEY = "Needs an admin key";

/** The proxy reports an endpoint's last failure as one of these classes, never as the error text. */
const FAILURE_WORDS: Record<string, string> = {
  timeout: "timed out",
  refused: "connection refused",
  dns: "name not found",
  tls: "TLS failed",
  upstream_5xx: "server error",
  not_allowed: "not an allowed destination",
  other: "failed",
};

export function failureWords(reason?: string) {
  return reason ? (FAILURE_WORDS[reason] ?? "failed") : "";
}
