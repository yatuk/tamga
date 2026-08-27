"use client";

import { motion, useReducedMotion } from "framer-motion";
import { Download, Pause, Play } from "lucide-react";
import { primaryOwasp } from "@/lib/owasp-llm";
import { toUpperEn, toUpperLocale } from "@/lib/utils/tr-string";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { buildIncidentsHref, relTime } from "./overviewHelpers";
import { OverviewActionBadge } from "./OverviewActionBadge";
import { useOverviewContext } from "./OverviewContext";

export function OverviewViewPartC() {
  const reduce = useReducedMotion();
  const {
    range,
    health,
    healthLoading,
    derived,
    searchedRecentEvents,
    quickSearch,
    setQuickSearch,
    quickSearchRef,
    exportRecentCsv,
    tickerEvents,
    tickerPaused,
    setTickerPaused,
    tickerIndex,
    showShortcuts,
    setShowShortcuts,
  } = useOverviewContext();

  return (
    <>
      <div>
        <Card>
          <CardHeader>
            <CardTitle>Incident Queue (Recent)</CardTitle>
            <CardDescription>Son 10 olay, analist onceligi icin siralanmis gorunum</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="mb-3">
              <input
                ref={quickSearchRef}
                value={quickSearch}
                onChange={(e) => setQuickSearch(e.target.value)}
                placeholder="Search recent incidents... (/)"
                className="h-8 w-full rounded-sm border border-border-strong bg-surface-subtle px-2 text-xs text-fg placeholder:text-fg-muted"
              />
            </div>
            <div className="mb-3 flex justify-end">
              <Button
                className="cursor-pointer border border-border-strong bg-white text-fg-muted hover:bg-surface-subtle dark:border-border dark:bg-surface-subtle dark:text-fg dark:hover:bg-surface-elevated"
                onClick={exportRecentCsv}
              >
                <Download className="mr-1 h-4 w-4" />
                Export CSV
              </Button>
            </div>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Request ID</TableHead>
                  <TableHead>Zaman</TableHead>
                  <TableHead>Provider</TableHead>
                  <TableHead>Aksiyon</TableHead>
                  <TableHead>Finding türü</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {searchedRecentEvents.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={5} className="text-fg-subtle dark:text-fg-subtle">
                      Veri yok.
                    </TableCell>
                  </TableRow>
                ) : (
                  searchedRecentEvents.map((e, _rowIdx) => {
                    const findingType = e.findings?.[0]?.type || "—";
                    const isCrit = toUpperEn(e.action || "") === "BLOCK" || findingType === "secret";
                    return (
                      <TableRow
                        key={e.request_id}
                        className="group transition-colors hover:bg-surface-subtle dark:hover:bg-surface-subtle/40"
                      >
                        <TableCell className="relative text-xs">
                          <span className="absolute left-0 top-1/2 hidden h-8 w-0.5 -translate-y-1/2 rounded-full bg-status-low opacity-0 transition-opacity group-hover:opacity-100 dark:bg-status-low md:block" />
                          {e.request_id.slice(0, 12)}
                        </TableCell>
                        <TableCell>{e.timestamp ? new Date(e.timestamp).toLocaleString("tr-TR") : "—"}</TableCell>
                        <TableCell>{e.provider || "unknown"}</TableCell>
                        <TableCell>
                          <OverviewActionBadge action={e.action} />
                        </TableCell>
                        <TableCell>
                          <span className="inline-flex flex-wrap items-center gap-1.5">
                            {isCrit && !reduce ? (
                              <motion.span className="inline-flex" animate={{ opacity: [1, 0.75, 1] }} transition={{ duration: 2.4, repeat: Infinity, ease: "easeInOut" }}>
                                <Badge className="border-status-critical bg-status-critical text-status-critical dark:border-status-critical dark:bg-status-critical/40 dark:text-status-critical">{findingType}</Badge>
                              </motion.span>
                            ) : (
                              <Badge className="border-border bg-surface-subtle text-fg-muted dark:border-border dark:bg-surface-elevated dark:text-fg">{findingType}</Badge>
                            )}
                            {(() => {
                              const owasp = primaryOwasp(e.findings || []);
                              return owasp ? (
                                <span
                                  className="inline-flex items-center rounded-sm border border-border-strong bg-surface-subtle px-1.5 py-0.5 text-[10px] uppercase tracking-wider text-fg-muted dark:border-border-strong dark:bg-surface-subtle dark:text-fg-subtle"
                                  title={`OWASP LLM Top 10 · ${owasp.label}`}
                                >
                                  {owasp.code}
                                </span>
                              ) : null;
                            })()}
                          </span>
                        </TableCell>
                      </TableRow>
                    );
                  })
                )}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </div>

      <div>
        <TerminalFrame
          title="Tamga Proxy"
          status={
            <span className="inline-flex items-center gap-1.5 px-2 text-[10px] uppercase tracking-[0.18em] text-status-pass">
              <span
                className={reduce ? "h-1.5 w-1.5 rounded-full bg-status-pass" : "h-1.5 w-1.5 animate-pulse rounded-full bg-status-pass"}
                aria-hidden
              />
              LIVE
            </span>
          }
        >
          <div className="flex items-center gap-3 px-3 py-2 text-xs">
            <Button
              type="button"
              className="h-7 rounded-sm border border-border-strong bg-surface-subtle px-2 text-fg-muted hover:bg-surface-card"
              onClick={() => setTickerPaused((v) => !v)}
            >
              {tickerPaused ? <Play className="h-3.5 w-3.5" /> : <Pause className="h-3.5 w-3.5" />}
              <span className="ml-1">{tickerPaused ? "Resume" : "Pause"}</span>
            </Button>
            <div className="min-w-0 flex-1 overflow-hidden">
              {tickerEvents.length === 0 ? (
                <span className="text-fg-muted">LIVE · no incident stream yet</span>
              ) : (
                <div className="space-y-0.5">
                  {[tickerEvents[tickerIndex], tickerEvents[(tickerIndex + 1) % tickerEvents.length]].map((e, i) => (
                    <a
                      key={`${i}-${e.request_id}-${e.timestamp}`}
                      href={buildIncidentsHref({ range, request_id: e.request_id })}
                      className="block truncate text-fg-muted hover:text-fg"
                    >
                      {new Date(e.timestamp).toLocaleTimeString("tr-TR", { hour12: false })} {e.request_id.slice(0, 10)}{" "}
                      {toUpperLocale(e.provider || "unknown")} {(e.model || "n/a").slice(0, 16)} {toUpperLocale(e.action || "PASS")}{" "}
                      {toUpperLocale(e.findings?.[0]?.type || "-")} {Math.round(e.scan_latency_ms || 0)}ms {relTime(e.timestamp)}
                    </a>
                  ))}
                </div>
              )}
            </div>
          </div>
        </TerminalFrame>
      </div>

      <div className="text-xs text-fg-subtle dark:text-fg-subtle">
        Uptime: {healthLoading ? "..." : `${health?.uptime_seconds ?? 0}s`} • Ortalama tarama:{" "}
        {typeof derived.totals.avgLatencyMs === "number" ? `${derived.totals.avgLatencyMs.toFixed(2)}ms` : "—"}
      </div>

      {showShortcuts && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4" onClick={() => setShowShortcuts(false)}>
          <div className="w-full max-w-md rounded-sm border border-border-strong bg-surface-card p-4" onClick={(e) => e.stopPropagation()}>
            <h3 className="mb-2 text-sm text-fg">Overview Shortcuts</h3>
            <div className="space-y-1 text-xs text-fg-muted">
              <div>
                <span className="text-fg">/</span> focus recent search
              </div>
              <div>
                <span className="text-fg">?</span> toggle shortcut help
              </div>
              <div>
                <span className="text-fg">i</span> go to incidents console
              </div>
              <div>
                <span className="text-fg">Esc</span> close this panel
              </div>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
