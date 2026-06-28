"use client";

import { useState } from "react";
import {
  AlertTriangle,
  ChevronDown,
  ChevronRight,
  Clock3,
  Eye,
  Shield,
  ShieldAlert,
  Siren,
  FlaskConical,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { primaryOwasp } from "@/lib/owasp-llm";
import type { SecurityEvent } from "@/lib/api";
import { toUpperLocale } from "@/lib/utils/tr-string";
import { getActionBadge, primarySeverity, relativeTime } from "@/lib/security/security-events-model";

// ── Types ──────────────────────────────────────────────────────────────────────

type SevLevel = "critical" | "high" | "medium" | "low";

interface TriageGroup {
  level: SevLevel;
  label: string;
  icon: typeof Siren;
  color: string;
  bg: string;
  border: string;
  text: string;
  events: SecurityEvent[];
  expanded: boolean;
}

interface SeverityTriagePanelProps {
  events: SecurityEvent[];
  onSelectEvent: (event: SecurityEvent) => void;
  onAck?: (requestId: string) => void;
  onOpenPlayground?: (requestId: string) => void;
  className?: string;
}

// ── Group config ───────────────────────────────────────────────────────────────

const GROUP_CONFIG: Record<SevLevel, Omit<TriageGroup, "events" | "expanded">> = {
  critical: { level: "critical", label: "CRITICAL", icon: Siren, color: "#ef4444", bg: "bg-red-500/5", border: "border-red-500/20", text: "text-red-400" },
  high: { level: "high", label: "HIGH", icon: ShieldAlert, color: "#f59e0b", bg: "bg-amber-500/5", border: "border-amber-500/20", text: "text-amber-400" },
  medium: { level: "medium", label: "MEDIUM", icon: AlertTriangle, color: "#eab308", bg: "bg-yellow-500/5", border: "border-yellow-500/20", text: "text-yellow-400" },
  low: { level: "low", label: "LOW", icon: Shield, color: "#3b82f6", bg: "bg-blue-500/5", border: "border-blue-500/20", text: "text-blue-400" },
};

// ── Main Export ────────────────────────────────────────────────────────────────

export function SeverityTriagePanel({
  events,
  onSelectEvent,
  onAck,
  onOpenPlayground,
  className = "",
}: SeverityTriagePanelProps) {
  const [expandedGroups, setExpandedGroups] = useState<Set<SevLevel>>(new Set(["critical", "high"]));

  // Group events by severity
  const groups: TriageGroup[] = (["critical", "high", "medium", "low"] as SevLevel[]).map((level) => {
    const cfg = GROUP_CONFIG[level];
    const filtered = events.filter((e) => {
      const sev = primarySeverity(e.findings || []);
      return sev === level;
    });
    return { ...cfg, events: filtered, expanded: expandedGroups.has(level) };
  });

  const totalEvents = groups.reduce((s, g) => s + g.events.length, 0);

  const toggleGroup = (level: SevLevel) => {
    setExpandedGroups((prev) => {
      const next = new Set(prev);
      if (next.has(level)) next.delete(level);
      else next.add(level);
      return next;
    });
  };

  if (totalEvents === 0) {
    return (
      <div className={`rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-5 text-center ${className}`}>
        <Shield className="mx-auto h-8 w-8 text-zinc-700" />
        <p className="mt-2 text-xs text-zinc-600 dark:text-zinc-400">No incidents to triage</p>
        <p className="mt-1 text-[10px] text-zinc-600 dark:text-zinc-400">All clear — new events will appear here</p>
      </div>
    );
  }

  return (
    <div className={`rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 ${className}`}>
      {/* Header */}
      <div className="flex items-center justify-between border-b border-zinc-200 dark:border-zinc-800 px-4 py-2.5">
        <div className="flex items-center gap-2">
          <Siren className="h-3.5 w-3.5 text-red-400" />
          <span className="text-[11px] font-semibold uppercase tracking-[0.1em] text-zinc-700 dark:text-zinc-300">
            Severity Triage
          </span>
        </div>
        <span className="text-[10px] tabular-nums text-zinc-600 dark:text-zinc-400">
          {totalEvents} events · {groups.filter(g => g.events.length > 0).length} tiers
        </span>
      </div>

      {/* Groups */}
      <div className="divide-y divide-zinc-800/50">
        {groups.map((group, _gi) => {
          if (group.events.length === 0) return null;
          const Icon = group.icon;

          return (
            <div key={group.level}>
              {/* Group header */}
              <button
                type="button"
                onClick={() => toggleGroup(group.level)}
                className={`flex w-full cursor-pointer items-center gap-3 px-4 py-2.5 transition-colors hover:bg-zinc-100 dark:hover:bg-zinc-900/40 ${group.bg}`}
              >
                <span className={`shrink-0 ${group.text}`}>
                  {group.expanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
                </span>
                <Icon className={`h-4 w-4 ${group.text}`} />
                <span className={`text-[11px] font-bold uppercase tracking-[0.1em] ${group.text}`}>
                  {group.label}
                </span>
                <Badge className={`rounded-sm border ${group.border} ${group.bg} text-[10px] tabular-nums ${group.text}`}>
                  {group.events.length}
                </Badge>
                {/* Compact severity bar */}
                <div className="ml-auto flex h-1.5 w-24 overflow-hidden rounded-full bg-zinc-100 dark:bg-zinc-900">
                  <div
                    className="h-full"
                    style={{
                      width: `${Math.round((group.events.length / totalEvents) * 100)}%`,
                      backgroundColor: group.color,
                      opacity: 0.6,
                    }}
                  />
                </div>
              </button>

              {/* Group events */}
              {group.expanded && (
                  <div className="overflow-hidden">
                    {group.events.slice(0, 8).map((event, _ei) => {
                      const findings = event.findings || [];
                      const findingSummary = findings.slice(0, 2).map((f) => `${f.category || f.type}`).join(" · ") || "—";
                      const owasp = primaryOwasp(findings);
                      const entity = `${event.provider || "?"} / ${(event.model || "?").slice(0, 18)}`;

                      return (
                        <div
                          key={event.request_id}
                          className={`flex items-center gap-3 border-t px-4 py-2 transition-colors hover:bg-zinc-100 dark:hover:bg-zinc-900/60 ${
                            group.border.includes("red") ? "border-red-500/10" :
                            group.border.includes("amber") ? "border-amber-500/10" :
                            group.border.includes("yellow") ? "border-yellow-500/10" :
                            "border-blue-500/10"
                          }`}
                        >
                          {/* Severity dot */}
                          <span className="h-2 w-2 shrink-0 rounded-full" style={{ backgroundColor: group.color }} />

                          {/* Entity + Request ID */}
                          <div className="min-w-0 flex-1">
                            <div className="flex items-center gap-2">
                              <span className="text-[11px] text-zinc-800 dark:text-zinc-200 truncate">
                                {findingSummary}
                              </span>
                              {owasp && (
                                <span
                                  className="shrink-0 rounded-sm bg-red-500/10 px-1 py-0.5 text-[8px] font-bold text-red-400"
                                  title={`OWASP LLM Top 10 · ${owasp.label}`}
                                >
                                  {owasp.code}
                                </span>
                              )}
                            </div>
                            <div className="mt-0.5 flex items-center gap-2 text-[9px] text-zinc-600 dark:text-zinc-400">
                              <span>{entity}</span>
                              <span>·</span>
                              <span className="text-zinc-600 dark:text-zinc-400">{event.request_id.slice(0, 12)}</span>
                              <span>·</span>
                              <span className="inline-flex items-center gap-0.5">
                                <Clock3 className="h-2.5 w-2.5" />
                                {relativeTime(event.timestamp)}
                              </span>
                            </div>
                          </div>

                          {/* Action badge */}
                          <Badge className={`shrink-0 rounded-sm text-[9px] ${getActionBadge(event.action)}`}>
                            {toUpperLocale(event.action || "PASS")}
                          </Badge>

                          {/* Quick actions */}
                          <div className="flex shrink-0 items-center gap-1">
                            {onAck && (
                              <button
                                type="button"
                                onClick={(e) => { e.stopPropagation(); onAck(event.request_id); }}
                                className="rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-1.5 py-0.5 text-[9px] text-zinc-600 dark:text-zinc-400 hover:bg-zinc-200 dark:hover:bg-zinc-800 hover:text-zinc-200"
                              >
                                Ack
                              </button>
                            )}
                            {onOpenPlayground && (
                              <button
                                type="button"
                                onClick={(e) => { e.stopPropagation(); onOpenPlayground(event.request_id); }}
                                className="rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-1 py-0.5 text-zinc-600 dark:text-zinc-400 hover:bg-zinc-200 dark:hover:bg-zinc-800 hover:text-zinc-200"
                                title="Test in Playground" aria-label="Test in Playground"
                              >
                                <FlaskConical className="h-3 w-3" />
                              </button>
                            )}
                            <button
                              type="button"
                              onClick={() => onSelectEvent(event)}
                              className="rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 px-1.5 py-0.5 text-zinc-600 dark:text-zinc-400 hover:bg-zinc-200 dark:hover:bg-zinc-800 hover:text-zinc-200"
                              title="Inspect" aria-label="Inspect incident"
                            >
                              <Eye className="h-3 w-3" />
                            </button>
                          </div>
                        </div>
                      );
                    })}
                    {group.events.length > 8 && (
                      <div className="border-t border-zinc-200 dark:border-zinc-800 px-4 py-1.5 text-center text-[9px] text-zinc-600 dark:text-zinc-400">
                        +{group.events.length - 8} more in {group.label} · use filters to narrow
                      </div>
                    )}
                  </div>
                )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
