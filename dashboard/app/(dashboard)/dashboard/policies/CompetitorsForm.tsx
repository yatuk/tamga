"use client";

import { useQuery } from "@tanstack/react-query";
import { Panel } from "@/components/app/panel";
import { EmptyState, ErrorState, SkeletonRows } from "@/components/app/states";
import { ActionBadge, SeverityBadge, StatusBadge } from "@/components/app/status-badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { API_BASE, authHeaders } from "@/lib/api/fetch-core";

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

/** Competitor names the active policy watches for. Read-only; edit them in the policy. */
export function CompetitorsForm({ adminKey }: Props) {
  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ["policy-competitors", adminKey],
    queryFn: async () => {
      const r = await fetch(`${API_BASE}/api/v1/policies`, { headers: authHeaders(adminKey) });
      if (!r.ok) throw new Error(`The policy request failed with status ${r.status}.`);
      const json = (await r.json()) as PolicyResponse;
      return json.competitors && json.competitors.length > 0 ? json.competitors : parseCompetitors(json.yaml ?? "");
    },
    enabled: !!adminKey,
    staleTime: 30_000,
  });

  const competitors = data ?? [];
  const active = competitors.filter((c) => c.enabled).length;

  return (
    <Panel
      title="Competitor mentions"
      description="Brand and product names the policy looks for in prompts"
      aside={competitors.length > 0 ? `${active} of ${competitors.length} enabled` : undefined}
    >
      {isLoading ? (
        <SkeletonRows rows={3} />
      ) : error ? (
        <ErrorState title="Could not load the policy" error={error} onRetry={() => void refetch()} />
      ) : competitors.length === 0 ? (
        <EmptyState
          title="No competitors configured"
          description={
            <>
              Add a <span className="font-mono">competitors</span> list to the policy in the Editor tab to detect competitor
              mentions.
            </>
          }
        />
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>State</TableHead>
              <TableHead>Severity</TableHead>
              <TableHead>Action</TableHead>
              <TableHead>Patterns</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {competitors.map((c) => (
              <TableRow key={c.name}>
                <TableCell className="font-medium">
                  {c.name}
                  {c.description ? <p className="text-xs font-normal text-muted-foreground">{c.description}</p> : null}
                </TableCell>
                <TableCell>
                  <StatusBadge tone={c.enabled ? "pass" : "neutral"}>{c.enabled ? "enabled" : "disabled"}</StatusBadge>
                </TableCell>
                <TableCell>
                  <SeverityBadge severity={c.severity} />
                </TableCell>
                <TableCell>
                  <ActionBadge action={c.action} />
                </TableCell>
                <TableCell className="whitespace-normal">
                  <div className="flex flex-wrap gap-1">
                    {c.patterns.map((pattern) => (
                      <code key={pattern} className="bg-muted px-1.5 py-0.5 font-mono text-xs text-fg-muted">
                        {pattern}
                      </code>
                    ))}
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </Panel>
  );
}
