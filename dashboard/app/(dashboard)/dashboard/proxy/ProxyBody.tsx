"use client";

import { Link, Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from "@carbon/react";
import { counted, uptimeText } from "@/components/console/names";
import { DataNotice, PageHead, Section, Statement } from "@/components/console/page";
import type { useProxyPage } from "./useProxyPage";

type Props = ReturnType<typeof useProxyPage>;
type PartStatus = Props["componentRows"][number]["status"];

/** Not configured is a deployment choice, not a fault: it gets no mark. */
const STATES: Record<PartStatus, { word: string; action?: "FAILING" | "STRUGGLING" }> = {
  ok: { word: "Working" },
  warning: { word: "Degraded", action: "STRUGGLING" },
  error: { word: "Failing", action: "FAILING" },
  disabled: { word: "Not set up" },
};

const NAMES: Record<string, string> = {
  "HTTP Server": "HTTP server",
  "Policy Engine": "Policy engine",
  "Scanner Pool": "Scanners",
  "Redis Cache": "Redis",
  "Event Bus": "Event bus",
  "Data Retention": "Data retention",
};

export function ProxyBody({ adminKey, isLoading, hasError, isOnline, health, detail, componentRows }: Props) {
  const failing = componentRows.filter((r) => r.status === "error");
  const degraded = componentRows.filter((r) => r.status === "warning");
  const inUse = componentRows.filter((r) => r.status !== "disabled").length;
  const database = health?.database;
  const dropped = health?.events_dropped ?? 0;

  return (
    <>
      <PageHead title="Proxy status" lede="Whether the proxy is up, and the state of each thing it depends on. Checked every 15 seconds." />

      <DataNotice adminKey={adminKey || "none"} failed={hasError} />

      <Statement
        loading={isLoading}
        quiet={isOnline}
        figure={isOnline ? "Up" : "Down"}
        sentence={
          !isOnline
            ? "The proxy is not answering its health check."
            : failing.length > 0
              ? `but ${counted(failing.length, "part")} of it ${failing.length === 1 ? "is" : "are"} failing: ${failing.map((r) => NAMES[r.component] ?? r.component).join(", ")}.`
              : degraded.length > 0
                ? `with ${counted(degraded.length, "part")} degraded: ${degraded.map((r) => NAMES[r.component] ?? r.component).join(", ")}.`
                : `and all ${inUse} parts in use are working.`
        }
        rest={
          health?.uptime_seconds ? (
            <>
              It has been running for <b>{uptimeText(health.uptime_seconds)}</b>.
            </>
          ) : undefined
        }
        facts={[
          ["Version", detail?.version || "Not known"],
          ["Scanners on every request", health?.scanner_count ?? "Not known"],
          [
            "Database",
            database === "connected" ? "Connected" : database === "not_configured" ? "None: events are kept in memory" : (database ?? "Not known"),
          ],
          ["TLS", detail?.tls_enabled ? "On" : "Off: terminate it in front of the proxy"],
          ["Events the bus dropped", dropped.toLocaleString("en")],
        ]}
      />

      <Section id="parts" title="Parts" note={`${inUse} in use, ${componentRows.length - inUse} not set up`}>
        <TableContainer className="t-table">
          <Table size="md" aria-label="Parts of the proxy">
            <TableHead>
              <TableRow>
                <TableHeader>Part</TableHeader>
                <TableHeader>State</TableHeader>
                <TableHeader>Detail</TableHeader>
                <TableHeader>Depends on</TableHeader>
              </TableRow>
            </TableHead>
            <TableBody>
              {componentRows.map((r) => {
                const state = STATES[r.status];
                return (
                  <TableRow key={r.component}>
                    <TableCell>{NAMES[r.component] ?? r.component}</TableCell>
                    <TableCell>
                      {state.action ? (
                        <span className="t-action" data-action={state.action}>
                          {state.word}
                        </span>
                      ) : r.status === "disabled" ? (
                        <span className="t-note">{state.word}</span>
                      ) : (
                        state.word
                      )}
                    </TableCell>
                    <TableCell className="t-data">{r.status === "disabled" ? "" : r.detail.replace(/ · /g, ", ")}</TableCell>
                    <TableCell className="t-note">{r.dependsOn}</TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </TableContainer>
        {detail?.trace_ui_url ? (
          <p className="t-footnote">
            Request traces are in{" "}
            <Link href={detail.trace_ui_url} target="_blank" rel="noreferrer">
              the trace viewer
            </Link>
            , which opens in a new tab.
          </p>
        ) : null}
      </Section>
    </>
  );
}
