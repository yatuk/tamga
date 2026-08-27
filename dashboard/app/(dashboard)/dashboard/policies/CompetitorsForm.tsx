"use client";

import { useQuery } from "@tanstack/react-query";
import { Eye, EyeOff, ExternalLink } from "lucide-react";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { getSeverityBadge, getActionBadge } from "@/lib/badges";

interface Competitor {
  name: string;
  patterns: string[];
  severity: string;
  action: string;
  description: string;
  enabled: boolean;
}

interface PolicyResponse {
  name: string;
  version: string;
  yaml?: string;
  competitors?: Competitor[];
}

function parseCompetitors(yaml: string): Competitor[] {
  try {
    // Simple YAML competitor block parser — extracts competitors: list.
    const compMatch = yaml.match(/^competitors:\s*\n([\s\S]*?)(?=\n\S|$)/m);
    if (!compMatch) return [];

    const comps: Competitor[] = [];
    const blocks = compMatch[1].split(/\n  - /).filter(Boolean);
    for (const block of blocks) {
      const name = block.match(/^\s*name:\s*"?(.+?)"?\s*$/m)?.[1] ?? "";
      const enabled = !block.includes("enabled: false");
      const severity = block.match(/^\s*severity:\s*(\w+)/m)?.[1] ?? "low";
      const action = block.match(/^\s*action:\s*(\w+)/m)?.[1] ?? "log";
      const description = block.match(/^\s*description:\s*"?(.+?)"?\s*$/m)?.[1] ?? "";
      const patternMatches = block.match(/^\s*patterns:\s*\n([\s\S]*?)(?=\n  \w|\n\s*$|$)/m);
      const patterns: string[] = [];
      if (patternMatches) {
        const plines = patternMatches[1].split("\n");
        for (const pline of plines) {
          const p = pline.replace(/^\s*-\s*"?(.+?)"?\s*$/, "$1").trim();
          if (p) patterns.push(p);
        }
      }
      if (name) {
        comps.push({ name, patterns, severity, action, description, enabled });
      }
    }
    return comps;
  } catch {
    return [];
  }
}

type Props = { adminKey: string };

export function CompetitorsForm({ adminKey }: Props) {
  const { data, isLoading, error } = useQuery({
    queryKey: ["policy-competitors", adminKey],
    queryFn: async () => {
      const base = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8443";
      const r = await fetch(`${base}/api/v1/policies`, {
        headers: { "X-Tamga-Admin-Key": adminKey },
      });
      if (!r.ok) throw new Error(`policy fetch failed: ${r.status}`);
      const json = (await r.json()) as PolicyResponse;
      const raw = json.yaml ?? "";
      const yamlCompetitors = parseCompetitors(raw);
      return {
        name: json.name ?? "unknown",
        version: json.version ?? "0",
        competitors:
          json.competitors && json.competitors.length > 0
            ? json.competitors
            : yamlCompetitors,
      };
    },
    staleTime: 30_000,
  });

  const competitors = data?.competitors ?? [];

  return (
    <TerminalFrame title="Rakip Modeller">
      <div className="space-y-2 p-3">
        {/* Header */}
        <div className="flex items-center justify-between">
          <div>
            <p className="text-[11px] uppercase tracking-[0.14em] text-status-critical">
              Competitor Intelligence
            </p>
            <p className="mt-1 text-xs text-fg-subtle">
              Detects competitor brand and product mentions in LLM prompts.
              Configure via{" "}
              <code className="rounded-sm bg-surface-subtle px-1 font-mono text-[11px] text-fg-muted">
                competitors
              </code>{" "}
              block in policy YAML.
            </p>
          </div>
          {data && (
            <span className="text-[10px] text-fg-subtle">
              {data.name} v{data.version}
            </span>
          )}
        </div>

        {/* Loading */}
        {isLoading && (
          <div className="flex items-center gap-2 py-8 text-center text-xs text-fg-subtle">
            <span className="inline-block h-2 w-2 animate-pulse rounded-full bg-surface-subtle0" />
            Loading competitor configuration…
          </div>
        )}

        {/* Error */}
        {error && (
          <div className="rounded-sm border border-status-critical/30 bg-status-critical/5 p-3 text-xs text-status-critical">
            Failed to load policy: {(error as Error).message}
          </div>
        )}

        {/* Empty */}
        {!isLoading && !error && competitors.length === 0 && (
          <div className="py-8 text-center">
            <p className="text-xs text-fg-subtle">
              No competitors configured.
            </p>
            <p className="mt-1 text-[11px] text-fg-muted dark:text-fg-subtle">
              Add a{" "}
              <code className="rounded-sm bg-surface-subtle px-1 font-mono text-[10px]">
                competitors:
              </code>{" "}
              block to your policy YAML to enable competitor detection.
            </p>
            <a
              href="https://github.com/tamga-dev/tamga/blob/dev/tamga/docs/benchmarks/README.md"
              target="_blank"
              rel="noreferrer"
              className="mt-3 inline-flex items-center gap-1.5 rounded-sm border border-border dark:border-border-strong bg-surface-subtle px-3 py-1.5 text-[11px] text-fg-muted hover:text-fg transition-colors"
            >
              See example policy
              <ExternalLink className="h-3 w-3" />
            </a>
          </div>
        )}

        {/* Competitor list */}
        {!isLoading && !error && competitors.length > 0 && (
          <div className="space-y-2">
            {competitors.map((c) => {
              const sev = getSeverityBadge(c.severity);
              const SevIcon = sev.icon;
              const act = getActionBadge(c.action);
              const ActIcon = act.icon;
              return (
                <div
                  key={c.name}
                  className="rounded-sm border border-border bg-surface-card/60 overflow-hidden"
                >
                  {/* Top row */}
                  <div className="flex items-center justify-between px-3 py-2">
                    <div className="flex items-center gap-2 min-w-0">
                      {c.enabled ? (
                        <Eye className="h-3.5 w-3.5 shrink-0 text-status-pass" />
                      ) : (
                        <EyeOff className="h-3.5 w-3.5 shrink-0 text-fg-subtle" />
                      )}
                      <span className="font-mono text-sm text-fg truncate">
                        {c.name}
                      </span>
                    </div>
                    <div className="flex items-center gap-1.5 shrink-0">
                      <span
                        className={`inline-flex items-center gap-1 rounded-sm border px-1.5 py-0.5 text-[10px] uppercase ${sev.cls}`}
                      >
                        <SevIcon className="h-2.5 w-2.5" />
                        {c.severity}
                      </span>
                      <span
                        className={`inline-flex items-center gap-1 rounded-sm border px-1.5 py-0.5 text-[10px] uppercase ${act.cls}`}
                      >
                        <ActIcon className="h-2.5 w-2.5" />
                        {c.action}
                      </span>
                    </div>
                  </div>

                  {/* Patterns */}
                  {c.patterns.length > 0 && (
                    <div className="border-t border-border px-3 py-1.5 bg-surface-subtle">
                      <div className="flex flex-wrap gap-1">
                        {c.patterns.map((p, i) => (
                          <code
                            key={i}
                            className="rounded-sm bg-surface-subtle px-1.5 py-0.5 font-mono text-[10px] text-fg-muted"
                          >
                            /{p}/
                          </code>
                        ))}
                      </div>
                    </div>
                  )}

                  {/* Description */}
                  {c.description && (
                    <div className="border-t border-border px-3 py-1.5">
                      <p className="text-[11px] text-fg-subtle truncate">
                        {c.description}
                      </p>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}

        {/* Footer stats */}
        {!isLoading && !error && competitors.length > 0 && (
          <div className="flex items-center gap-3 border-t border-border pt-3 text-[10px] text-fg-subtle">
            <span>
              {competitors.filter((c) => c.enabled).length} active
            </span>
            <span>·</span>
            <span>
              {competitors.filter((c) => c.enabled).length} of{" "}
              {competitors.length} competitors configured
            </span>
          </div>
        )}
      </div>
    </TerminalFrame>
  );
}
