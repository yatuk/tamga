"use client";

import type { ReactNode } from "react";
import { InlineNotification } from "@carbon/react";
import { NEEDS_KEY, type HealthReading, type HealthStanding } from "@/hooks/useHealth";

/**
 * Says that the proxy is up but kept the detail of its health back, and why.
 * Renders nothing while the detail is there or still on its way, and nothing
 * while the proxy is not answering: "it is up" would be false then.
 */
export function HealthDetailNotice({ state }: { state: HealthStanding }) {
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

export { NEEDS_KEY };

const asOfClock = new Intl.DateTimeFormat("en-GB", { hour: "2-digit", minute: "2-digit" });

/** "as of 14:02, the last answer": said of anything shown from an answer the proxy gave before it stopped answering. */
export const asOfWords = (answeredAt: number, capital = false) =>
  `${capital ? "As" : "as"} of ${asOfClock.format(new Date(answeredAt))}, the last answer`;

/**
 * A value read from the health check, as it may be shown now.
 *
 * A value that is not there is written as the reading says: "Needs an admin
 * key" only while the proxy answers and keeps it back, "Not known" otherwise.
 * A value held from before the check began to fail is set in the secondary
 * ink and says when it was true; the caption of its list or table (AsOf)
 * says the same once, in full, for the eye.
 */
export function held(reading: HealthReading, value: ReactNode | null | undefined): ReactNode {
  if (value === undefined || value === null || value === "") return reading.missing;
  if (!reading.stale) return value;
  return (
    <span className="t-asof">
      {value}
      <span className="cds--visually-hidden">, {asOfWords(reading.answeredAt)}</span>
    </span>
  );
}

/** The caption over a list or a table whose values are the proxy's last answer. Renders nothing otherwise. */
export function AsOf({ reading }: { reading: HealthReading }) {
  if (!reading.stale) return null;
  return <p className="t-asof t-asofline">{asOfWords(reading.answeredAt, true)}</p>;
}

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
