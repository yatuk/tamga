"use client";

import { X } from "lucide-react";

const GLOSSARY_TERMS: { term: string; definition: string }[] = [
  {
    term: "MTTR",
    definition:
      "Mean Time to Resolve — the average time from incident creation to closure. Lower MTTR indicates faster analyst response.",
  },
  {
    term: "P50 / P95 / P99",
    definition:
      "Request latency percentiles. P95 = 95% of requests are faster than this value. P99 is the tail latency. These are more informative than averages.",
  },
  {
    term: "RPS",
    definition:
      "Requests Per Second — the rate of API calls flowing through the proxy. Used to monitor traffic volume and detect anomalies.",
  },
  {
    term: "Shadow AI",
    definition:
      "Unsanctioned use of AI/LLM services outside the approved provider list. Tamga detects unrecognized provider endpoints in requests.",
  },
  {
    term: "Circuit Breaker",
    definition:
      "A resilience pattern that stops sending requests to a failing provider after consecutive errors. States: CLOSED (healthy), OPEN (blocked), HALF-OPEN (testing recovery).",
  },
  {
    term: "Prompt Injection",
    definition:
      "An attack where an adversary embeds malicious instructions in user input to override system behavior or extract confidential data from the LLM.",
  },
  {
    term: "PII",
    definition:
      "Personally Identifiable Information — data like names, emails, phone numbers, SSNs. Tamga's scanner detects and can redact PII before it reaches the LLM.",
  },
  {
    term: "Redaction",
    definition:
      "Tamga replaces detected sensitive substrings (PII, secrets) with placeholder text before forwarding the request to the LLM. The original data is never stored.",
  },
  {
    term: "Block vs. Warn",
    definition:
      "Block: the proxy rejects the request entirely (HTTP 403). Warn: the request proceeds but the event is logged and flagged for review.",
  },
  {
    term: "Scanner Pool",
    definition:
      "A bounded worker pool that runs Tamga's detection scanners concurrently. Controls max parallelism and queues overflow work.",
  },
  {
    term: "SSE",
    definition:
      "Server-Sent Events — a one-way streaming connection from proxy to dashboard. Used for live event updates without polling.",
  },
  {
    term: "OWASP LLM",
    definition:
      "OWASP Top 10 for LLM Applications — industry-standard vulnerability categories: prompt injection, insecure output handling, training data poisoning, model DoS, etc.",
  },
  {
    term: "Policy Rule",
    definition:
      "A YAML-defined rule in Tamga that maps a finding (type + category) to an action (block, redact, warn, log) and severity level.",
  },
  {
    term: "Audit Chain",
    definition:
      "Tamga cryptographically chains audit log entries via SHA-256 hashes. Each entry links to the previous one, making tampering detectable.",
  },
  {
    term: "Budget Burn",
    definition:
      "The rate at which your daily token or USD budget is consumed. When the limit is reached, the proxy returns HTTP 402 until the next UTC midnight reset.",
  },
];

interface GlossaryPanelProps {
  open: boolean;
  onClose: () => void;
}

export function GlossaryPanel({ open, onClose }: GlossaryPanelProps) {
  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4"
      onClick={onClose}
      role="presentation"
    >
      <div
        className="w-full max-w-xl max-h-[80vh] overflow-y-auto rounded-sm border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-950 p-4"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-labelledby="glossary-title"
      >
        <div className="flex items-center justify-between mb-3">
          <h3 id="glossary-title" className="text-sm font-semibold text-zinc-900 dark:text-zinc-100">
            Glossary
          </h3>
          <button
            type="button"
            onClick={onClose}
            className="cursor-pointer rounded-sm p-1 text-zinc-400 hover:text-zinc-600 dark:hover:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-900"
            aria-label="Close glossary"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="space-y-2.5">
          {GLOSSARY_TERMS.map(({ term, definition }) => (
            <div
              key={term}
              className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-900/60 p-2.5"
            >
              <div className="text-xs font-semibold text-zinc-800 dark:text-zinc-200 font-mono">
                {term}
              </div>
              <p className="mt-0.5 text-[11px] leading-relaxed text-zinc-600 dark:text-zinc-400">
                {definition}
              </p>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

/** A small "?" icon button to toggle the glossary panel.
 *  Include this in page header actions. */
export function GlossaryToggle({
  onClick,
}: {
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="inline-flex cursor-pointer items-center justify-center h-7 w-7 rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-500 hover:text-zinc-700 dark:hover:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800"
      title="Open glossary"
      aria-label="Open glossary"
    >
      <span className="text-xs font-semibold">?</span>
    </button>
  );
}
