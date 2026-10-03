"use client";

import { useMemo } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ExternalLink, User as UserIcon } from "lucide-react";
import { PageHeader } from "@/components/app/page-header";
import { Panel } from "@/components/app/panel";
import { Stat, StatGrid } from "@/components/app/stat";
import { AdminKeyRequired, EmptyState, ErrorState, SkeletonRows } from "@/components/app/states";
import { StatusBadge, type Tone } from "@/components/app/status-badge";
import { Button } from "@/components/ui/button";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useAdminKey } from "@/hooks/useAdminKey";
import { api, type TeamMember, type TeamRole } from "@/lib/api";
import { toast } from "@/lib/toast";

const ROLES: { role: TeamRole; tone: Tone; can: string }[] = [
  { role: "admin", tone: "high", can: "Everything: policies, settings, integrations, team and the audit log." },
  { role: "analyst", tone: "low", can: "Triage incidents and edit policies and patterns." },
  { role: "viewer", tone: "neutral", can: "Read the overview, incidents and reports." },
];

export default function TeamPage() {
  const qc = useQueryClient();
  const [adminKey] = useAdminKey();

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ["tamga-team", adminKey],
    queryFn: () => api.listTeam(adminKey),
    enabled: !!adminKey,
    staleTime: 60 * 1000,
  });

  const roleMut = useMutation({
    mutationFn: ({ id, role }: { id: string; role: TeamRole }) => api.setTeamRole(adminKey, id, role),
    onSuccess: (_res, { role }) => {
      toast.success("Role updated", `Now ${role}`);
      qc.invalidateQueries({ queryKey: ["tamga-team", adminKey] });
    },
    onError: (e: Error) => toast.error("Could not update the role", e.message),
  });

  const items: TeamMember[] = useMemo(() => data?.items ?? [], [data?.items]);
  const directoryConnected = data?.clerk ?? false;
  const count = (role: TeamRole) => items.filter((m) => m.role === role).length;

  const header = (
    <PageHeader
      title="Team"
      description="Who can sign in to this console and what each role is allowed to do."
      actions={
        directoryConnected ? (
          <Button asChild variant="outline" size="sm">
            <a href="https://dashboard.clerk.com" target="_blank" rel="noreferrer">
              Invite in Clerk
              <ExternalLink />
              <span className="sr-only"> (opens in a new tab)</span>
            </a>
          </Button>
        ) : null
      }
    />
  );

  if (!adminKey) {
    return (
      <div className="space-y-6">
        {header}
        <Panel>
          <AdminKeyRequired />
        </Panel>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {header}

      {data && !directoryConnected ? (
        <div role="status" className="border border-status-medium/40 bg-status-medium-bg px-4 py-3 text-sm text-status-medium">
          The identity provider is not connected, so only role assignments stored in the proxy are shown. Set{" "}
          <span className="font-mono text-xs">CLERK_SECRET_KEY</span> on the proxy and restart it to load names and email
          addresses.
        </div>
      ) : null}

      {items.length > 0 ? (
        <StatGrid>
          <Stat label="Members" value={items.length} />
          <Stat
            label="Admins"
            value={count("admin")}
            tone={count("admin") === 0 ? "critical" : "default"}
            hint={count("admin") === 0 ? "Nobody can manage the team" : undefined}
          />
          <Stat label="Analysts" value={count("analyst")} />
          <Stat label="Viewers" value={count("viewer")} />
        </StatGrid>
      ) : null}

      <Panel title="Members">
        {isLoading ? (
          <SkeletonRows rows={5} />
        ) : error ? (
          <ErrorState title="Could not load the team" error={error} onRetry={() => void refetch()} />
        ) : items.length === 0 ? (
          <EmptyState
            icon="shield"
            title="No members yet"
            description="Members appear after they sign in, or once the identity provider is connected to the proxy."
          />
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Member</TableHead>
                <TableHead>Email</TableHead>
                <TableHead>Role</TableHead>
                <TableHead>Role changed</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((m) => (
                <TableRow key={m.user_id}>
                  <TableCell>
                    <div className="flex items-center gap-3">
                      {m.image_url ? (
                        // eslint-disable-next-line @next/next/no-img-element
                        <img src={m.image_url} alt="" width={28} height={28} className="size-7 rounded-full border" />
                      ) : (
                        <span className="flex size-7 items-center justify-center rounded-full border bg-muted">
                          <UserIcon className="size-3.5 text-muted-foreground" aria-hidden />
                        </span>
                      )}
                      <div className="min-w-0">
                        <div className="truncate font-medium">{m.name || m.user_id}</div>
                        {m.name ? (
                          <div className="truncate font-mono text-xs text-muted-foreground" translate="no">
                            {m.user_id}
                          </div>
                        ) : null}
                      </div>
                    </div>
                  </TableCell>
                  <TableCell className="text-muted-foreground">{m.email || "—"}</TableCell>
                  <TableCell>
                    <NativeSelect
                      size="sm"
                      value={m.role}
                      aria-label={`Role of ${m.name || m.user_id}`}
                      onChange={(e) => roleMut.mutate({ id: m.user_id, role: e.target.value as TeamRole })}
                      disabled={roleMut.isPending}
                    >
                      {ROLES.map((r) => (
                        <NativeSelectOption key={r.role} value={r.role}>
                          {r.role}
                        </NativeSelectOption>
                      ))}
                    </NativeSelect>
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {m.updated_at ? new Date(m.updated_at).toLocaleString("en-GB") : "—"}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </Panel>

      <Panel title="What each role can do">
        <dl className="divide-y text-sm">
          {ROLES.map((r) => (
            <div key={r.role} className="flex flex-wrap items-baseline gap-x-4 gap-y-1 px-4 py-3">
              <dt className="w-20 shrink-0">
                <StatusBadge tone={r.tone}>{r.role}</StatusBadge>
              </dt>
              <dd className="min-w-0 flex-1 text-fg-muted">{r.can}</dd>
            </div>
          ))}
        </dl>
      </Panel>
    </div>
  );
}
