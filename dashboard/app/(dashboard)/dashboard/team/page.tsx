"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Button,
  DataTableSkeleton,
  InlineNotification,
  Select,
  SelectItem,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow,
} from "@carbon/react";
import { Launch } from "@carbon/icons-react";
import { counted, dayLabel } from "@/components/console/names";
import { DataNotice, NoData, PageHead, Section } from "@/components/console/page";
import { useAdminKey } from "@/hooks/useAdminKey";
import { api, type TeamMember, type TeamRole } from "@/lib/api";
import { toast } from "@/lib/toast";

const ROLES: { role: TeamRole; name: string; can: string }[] = [
  { role: "admin", name: "Admin", can: "Everything: policies, settings, integrations, the team and the audit log." },
  { role: "analyst", name: "Analyst", can: "Triage incidents and edit policies and patterns." },
  { role: "viewer", name: "Viewer", can: "Read the overview, incidents and reports." },
];

/**
 * The role is picked first and saved second. A select that saved on change
 * would save every role the arrow keys pass through on the way to the one meant.
 */
function RoleCell({
  member,
  lastAdmin,
  pending,
  onSave,
}: {
  member: TeamMember;
  lastAdmin: boolean;
  pending: boolean;
  onSave: (role: TeamRole) => void;
}) {
  const [draft, setDraft] = useState<TeamRole>(member.role);
  const changed = draft !== member.role;
  return (
    <span className="t-rolecell">
      <Select
        id={`role-${member.user_id}`}
        size="sm"
        inline
        hideLabel
        labelText={`Role of ${member.name || member.user_id}`}
        value={draft}
        disabled={pending}
        onChange={(e) => setDraft(e.target.value as TeamRole)}
      >
        {ROLES.map((r) => (
          <SelectItem key={r.role} value={r.role} text={r.name} />
        ))}
      </Select>
      {changed ? (
        <>
          <Button kind="primary" size="sm" disabled={pending} onClick={() => onSave(draft)}>
            {lastAdmin ? "Save, leaving no admin" : "Save role"}
          </Button>
          <Button kind="ghost" size="sm" disabled={pending} onClick={() => setDraft(member.role)}>
            Cancel
          </Button>
        </>
      ) : null}
    </span>
  );
}

export default function TeamPage() {
  const qc = useQueryClient();
  const [adminKey] = useAdminKey();

  const team = useQuery({
    queryKey: ["tamga-team", adminKey],
    queryFn: () => api.listTeam(adminKey),
    enabled: !!adminKey,
    staleTime: 60 * 1000,
  });
  const setRole = useMutation({
    mutationFn: ({ id, role }: { id: string; role: TeamRole }) => api.setTeamRole(adminKey, id, role),
    onSuccess: (_res, { role }) => {
      toast.success("Role updated", `Now ${role}`);
      void qc.invalidateQueries({ queryKey: ["tamga-team", adminKey] });
    },
    onError: (e: Error) => toast.error("Could not update the role", e.message),
  });

  const items: TeamMember[] = useMemo(() => team.data?.items ?? [], [team.data?.items]);
  const directory = team.data?.clerk ?? false;
  const count = (role: TeamRole) => items.filter((m) => m.role === role).length;
  const admins = count("admin");

  if (!adminKey || (team.isError && !team.data)) {
    return (
      <NoData
        title="Team"
        lede="Who can sign in to this console, and what each of them is allowed to do."
        adminKey={adminKey}
        error={team.error}
        what="the team"
      />
    );
  }

  return (
    <>
      <PageHead
        title="Team"
        lede={
          items.length === 0
            ? "Who can sign in to this console, and what each of them is allowed to do."
            : `${counted(items.length, "person", "people")} can sign in to this console: ${counted(admins, "admin")}, ${counted(count("analyst"), "analyst")}, ${counted(count("viewer"), "viewer")}.`
        }
      >
        {directory ? (
          <Button kind="tertiary" size="sm" renderIcon={Launch} href="https://dashboard.clerk.com" target="_blank" rel="noreferrer">
            Invite someone in Clerk
          </Button>
        ) : null}
      </PageHead>

      <DataNotice adminKey={adminKey} failed={team.isError} error={team.error} what="the team" />

      {items.length > 0 && admins === 0 ? (
        <InlineNotification
          kind="error"
          lowContrast
          hideCloseButton
          title="Nobody is an admin."
          subtitle="No one can manage the team or the settings until someone is."
          style={{ marginTop: "1.5rem", maxWidth: "none" }}
        />
      ) : null}
      {team.data && !directory ? (
        <InlineNotification
          kind="warning"
          lowContrast
          hideCloseButton
          title="The identity provider is not connected."
          subtitle="Only the roles stored in the proxy are shown. Set CLERK_SECRET_KEY on the proxy and restart it to see names and email addresses."
          style={{ marginTop: "1.5rem", maxWidth: "none" }}
        />
      ) : null}

      <div className="t-hunt">
        <Section id="members" title="Members">
          {team.isLoading ? (
            <DataTableSkeleton columnCount={4} rowCount={4} showHeader={false} showToolbar={false} />
          ) : items.length === 0 ? (
            <p className="t-empty">Nobody yet. People appear after they sign in, or once the identity provider is connected to the proxy.</p>
          ) : (
            <TableContainer className="t-table">
              <Table size="md" aria-label="Members">
                <TableHead>
                  <TableRow>
                    <TableHeader>Member</TableHeader>
                    <TableHeader>Email</TableHeader>
                    <TableHeader>Role</TableHeader>
                    <TableHeader>Role set</TableHeader>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {items.map((m) => (
                    <TableRow key={m.user_id}>
                      <TableCell>
                        {m.name || <span className="t-data">{m.user_id}</span>}
                        {m.name ? <span className="t-note t-data t-under">{m.user_id}</span> : null}
                      </TableCell>
                      <TableCell>{m.email || <span className="t-note">Not known</span>}</TableCell>
                      <TableCell>
                        <RoleCell
                          // A saved role is a new starting point for the draft.
                          key={`${m.user_id}|${m.role}`}
                          member={m}
                          lastAdmin={m.role === "admin" && admins === 1}
                          pending={setRole.isPending}
                          onSave={(role) => setRole.mutate({ id: m.user_id, role })}
                        />
                      </TableCell>
                      <TableCell>{m.updated_at ? dayLabel(m.updated_at) : ""}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </Section>

        <Section id="roles" title="What each role can do">
          <dl className="t-roles">
            {ROLES.map((r) => (
              <div key={r.role}>
                <dt>{r.name}</dt>
                <dd>{r.can}</dd>
              </div>
            ))}
          </dl>
        </Section>
      </div>
    </>
  );
}
