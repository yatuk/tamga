"use client";

import { useMemo } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ExternalLink, UserCog, User as UserIcon } from "lucide-react";
import { api, type TeamMember, type TeamRole } from "@/lib/api";
import { Badge } from "@/components/ui/badge";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { EmptyState } from "@/components/dashboard/EmptyState";
import { SkeletonTable } from "@/components/common/SkeletonRow";
import { TerminalFrame } from "@/components/dashboard/TerminalFrame";
import { toast } from "@/lib/toast";
import { useAdminKey } from "@/hooks/useAdminKey";

function roleBadge(r: TeamRole) {
  switch (r) {
    case "admin":
      return "border-status-critical/40 bg-status-critical/10 text-status-critical";
    case "analyst":
      return "border-status-medium/40 bg-status-medium/10 text-status-medium";
    default:
      return "border-border-strong bg-surface-subtle text-fg-muted";
  }
}

const ROLE_BAR_COLORS: Record<string, string> = {
  admin: "bg-status-critical",
  analyst: "bg-status-medium",
  viewer: "bg-zinc-400 dark:bg-surface-subtle0",
};

export default function TeamPage() {
  const qc = useQueryClient();
  const [adminKey] = useAdminKey();

  const { data, isLoading } = useQuery({
    queryKey: ["tamga-team", adminKey],
    queryFn: () => api.listTeam(adminKey),
    enabled: !!adminKey,
    staleTime: 60 * 1000,
  });

  const roleMut = useMutation({
    mutationFn: ({ id, role }: { id: string; role: TeamRole }) =>
      api.setTeamRole(adminKey, id, role),
    onSuccess: () => {
      toast.success("Role updated");
      qc.invalidateQueries({ queryKey: ["tamga-team", adminKey] });
    },
    onError: (e: Error) => toast.error("Update failed", e.message),
  });

  const items: TeamMember[] = useMemo(() => data?.items ?? [], [data?.items]);
  const clerkOK = data?.clerk ?? false;

  const counts = useMemo(() => ({
    total: items.length,
    admin: items.filter((m) => m.role === "admin").length,
    analyst: items.filter((m) => m.role === "analyst").length,
    viewer: items.filter((m) => m.role === "viewer").length,
  }), [items]);

  const rolePercents = useMemo(() => {
    const t = counts.total || 1;
    return {
      admin: (counts.admin / t) * 100,
      analyst: (counts.analyst / t) * 100,
      viewer: (counts.viewer / t) * 100,
    };
  }, [counts]);

  return (
    <div className="space-y-2">
      <PageHeader
        eyebrow="ADMINISTRATION // TEAM"
        title="Team"
        subtitle={
          <span>
            {counts.total} üye · admin {counts.admin} · analyst {counts.analyst} · viewer {counts.viewer}{" "}
            {clerkOK ? "· Clerk bağlı" : "· Clerk yapılandırılmadı"}
          </span>
        }
        actions={
          clerkOK ? (
            <a
              href="https://dashboard.clerk.com"
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-1 rounded-sm border border-border-strong bg-surface-subtle px-3 py-1.5 text-[11px] text-fg-muted hover:bg-surface-card"
            >
              Invite on Clerk <ExternalLink className="h-3 w-3" />
            </a>
          ) : null
        }
      />

      {!clerkOK ? (
        <div>
          <div className="rounded-sm border border-status-medium/40 bg-status-medium/5 p-3 text-[11px] text-status-medium">
            {"//"} CLERK_SECRET_KEY ortam değişkeni tanımlı değil — yalnızca yerel rol atamaları görünür.
            Kullanıcı kimliklerini Clerk üzerinden çekmek için{" "}
            <span className="text-status-medium">CLERK_SECRET_KEY</span>{" "}
            ayarlayın ve proxy&apos;yi yeniden başlatın.
          </div>
        </div>
      ) : null}

      {/* Member count + role distribution bar */}
      {!isLoading && items.length > 0 && (
        <div className="rounded-sm border border-border bg-surface-card p-3">
          <div className="flex items-center justify-between gap-4">
            <div>
              <div className="font-mono text-2xl font-semibold tabular-nums text-fg">
                {counts.total}
              </div>
              <div className="text-[10px] uppercase tracking-[0.12em] text-fg-subtle">
                team members
              </div>
            </div>
            <div className="flex-1 max-w-md">
              <div className="flex items-center gap-2 mb-1.5">
                {(["admin", "analyst", "viewer"] as const).map((role) => (
                  <div key={role} className="flex items-center gap-1">
                    <span className={`inline-block h-2 w-2 rounded-full ${ROLE_BAR_COLORS[role]}`} />
                    <span className="text-[10px] uppercase tracking-[0.12em] text-fg-subtle">
                      {role} {counts[role]}
                    </span>
                  </div>
                ))}
              </div>
              {/* Horizontal stacked bar */}
              <div className="flex h-2.5 rounded-sm overflow-hidden">
                {counts.admin > 0 && (
                  <div
                    className="bg-status-critical h-full"
                    style={{ width: `${rolePercents.admin}%` }}
                    title={`admin: ${counts.admin}`}
                  />
                )}
                {counts.analyst > 0 && (
                  <div
                    className="bg-status-medium h-full"
                    style={{ width: `${rolePercents.analyst}%` }}
                    title={`analyst: ${counts.analyst}`}
                  />
                )}
                {counts.viewer > 0 && (
                  <div
                    className="bg-zinc-400 dark:bg-surface-subtle0 h-full"
                    style={{ width: `${rolePercents.viewer}%` }}
                    title={`viewer: ${counts.viewer}`}
                  />
                )}
              </div>
            </div>
          </div>
        </div>
      )}

      <div>
        <TerminalFrame
          title="Team Members"
          status={
            <span className="px-2 text-[10px] uppercase tracking-[0.18em] text-fg-muted">
              {items.length} members
            </span>
          }

        >
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="bg-surface-subtle text-[10px] uppercase tracking-wide text-fg-muted">
                <tr>
                  <th className="px-3 py-2">User</th>
                  <th className="px-3 py-2">Email</th>
                  <th className="px-3 py-2">Role</th>
                  <th className="px-3 py-2">Updated</th>
                </tr>
              </thead>
              <tbody>
                {isLoading ? (
                  <tr>
                    <td className="px-3 py-0" colSpan={4}>
                      <SkeletonTable rows={5} cols={4} />
                    </td>
                  </tr>
                ) : items.length === 0 ? (
                  <tr>
                    <td className="px-3 py-0" colSpan={4}>
                      <EmptyState
                        icon="shield"
                        title="No team members found"
                        description="Team members are managed through Clerk authentication and role assignments in the proxy."
                        suggestion="Connect Clerk to start adding team members, or configure CLERK_SECRET_KEY in the proxy environment."
                      />
                    </td>
                  </tr>
                ) : (
                  items.map((m) => (
                    <tr key={m.user_id} className="border-t border-border hover:bg-surface-subtle/60">
                      <td className="px-3 py-2">
                        <div className="flex items-center gap-2">
                          {m.image_url ? (
                            // eslint-disable-next-line @next/next/no-img-element
                            <img
                              src={m.image_url}
                              alt=""
                              className="h-6 w-6 rounded-full border border-border"
                            />
                          ) : (
                            <div className="flex h-6 w-6 items-center justify-center rounded-full border border-border bg-surface-subtle">
                              <UserIcon className="h-3.5 w-3.5 text-fg-muted" />
                            </div>
                          )}
                          <div className="min-w-0">
                            <div className="truncate text-fg">
                              {m.name || m.user_id}
                            </div>
                            <div className="truncate text-[10px] text-fg-muted">
                              {m.user_id}
                            </div>
                          </div>
                        </div>
                      </td>
                      <td className="px-3 py-2 text-[11px] text-fg-muted">
                        {m.email || "—"}
                      </td>
                      <td className="px-3 py-2">
                        <div className="inline-flex items-center gap-2">
                          <Badge
                            className={`rounded-sm border text-[10px] uppercase ${roleBadge(
                              m.role,
                            )}`}
                          >
                            <UserCog className="mr-1 h-3 w-3" />
                            {m.role}
                          </Badge>
                          <select
                            value={m.role}
                            onChange={(e) =>
                              roleMut.mutate({
                                id: m.user_id,
                                role: e.target.value as TeamRole,
                              })
                            }
                            disabled={roleMut.isPending}
                            className="cursor-pointer rounded-sm border border-border bg-surface-card px-1.5 py-1 text-[11px] text-fg focus:outline-none"
                          >
                            <option value="admin">admin</option>
                            <option value="analyst">analyst</option>
                            <option value="viewer">viewer</option>
                          </select>
                        </div>
                      </td>
                      <td className="px-3 py-2 text-[10px] text-fg-muted">
                        {m.updated_at ? new Date(m.updated_at).toLocaleString() : "—"}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </TerminalFrame>
      </div>

      <div>
        <div className="rounded-sm border border-border bg-surface-card p-3">
          <div className="text-[10px] uppercase tracking-[0.18em] text-fg-muted">
            ROLES //
          </div>
          <div className="mt-2 grid gap-2 sm:grid-cols-3">
            <div className="rounded-sm border border-status-critical/30 bg-status-critical/5 p-2">
              <Badge className="rounded-sm border border-status-critical/40 bg-status-critical/10 text-[10px] uppercase text-status-critical">
                admin
              </Badge>
              <div className="mt-1 text-[11px] text-fg-muted">
                Full access — policies, settings, integrations, team, audit
              </div>
            </div>
            <div className="rounded-sm border border-status-medium/30 bg-status-medium/5 p-2">
              <Badge className="rounded-sm border border-status-medium/40 bg-status-medium/10 text-[10px] uppercase text-status-medium">
                analyst
              </Badge>
              <div className="mt-1 text-[11px] text-fg-muted">
                Can triage incidents, edit policies and patterns
              </div>
            </div>
            <div className="rounded-sm border border-border-strong bg-surface-subtle p-2">
              <Badge className="rounded-sm border border-border-strong bg-surface-subtle text-[10px] uppercase text-fg-muted">
                viewer
              </Badge>
              <div className="mt-1 text-[11px] text-fg-muted">
                Read-only — overview, incidents, reports
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
