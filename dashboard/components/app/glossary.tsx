"use client";

import { CircleHelp } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle, SheetTrigger } from "@/components/ui/sheet";

const GLOSSARY_TERMS: { term: string; definition: string }[] = [
  {
    term: "MTTR",
    definition:
      "Mean Time to Resolve: the average time from incident creation to closure. Lower MTTR indicates faster analyst response.",
  },
  {
    term: "P50 / P95 / P99",
    definition:
      "Request latency percentiles. P95 = 95% of requests are faster than this value. P99 is the tail latency. These are more informative than averages.",
  },
  {
    term: "RPS",
    definition:
      "Requests Per Second: the rate of API calls flowing through the proxy. Used to monitor traffic volume and detect anomalies.",
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
      "Personally Identifiable Information: data like names, emails, phone numbers, SSNs. Tamga's scanner detects and can redact PII before it reaches the LLM.",
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
      "Server-Sent Events: a one-way streaming connection from proxy to dashboard. Used for live event updates without polling.",
  },
  {
    term: "OWASP LLM",
    definition:
      "OWASP Top 10 for LLM Applications: industry-standard vulnerability categories: prompt injection, insecure output handling, training data poisoning, model DoS, etc.",
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

/** A header button that opens the list of terms used across the console. */
export function GlossaryButton() {
  return (
    <Sheet>
      <SheetTrigger asChild>
        <Button variant="outline" size="sm">
          <CircleHelp />
          Glossary
        </Button>
      </SheetTrigger>
      <SheetContent className="w-full gap-0 sm:max-w-md">
        <SheetHeader className="border-b">
          <SheetTitle>Glossary</SheetTitle>
          <SheetDescription>Terms used across the console.</SheetDescription>
        </SheetHeader>
        <dl className="flex-1 divide-y overflow-y-auto overscroll-contain">
          {GLOSSARY_TERMS.map(({ term, definition }) => (
            <div key={term} className="px-4 py-3">
              <dt className="font-mono text-xs font-medium text-foreground">{term}</dt>
              <dd className="mt-1 text-sm leading-relaxed text-muted-foreground">{definition}</dd>
            </div>
          ))}
        </dl>
      </SheetContent>
    </Sheet>
  );
}
