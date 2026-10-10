"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { DataTableSkeleton, InlineNotification } from "@carbon/react";
import { counted } from "@/components/console/names";
import { DataNotice, NoData, PageHead, Section } from "@/components/console/page";
import { useAdminKey } from "@/hooks/useAdminKey";
import { api, type TeamMember, type TeamRole } from "@/lib/api";
import {
  accessRemovedText,
  assignedRole,
  explainTeamRefusal,
  roleChangedText,
  ROLES,
  roleWord,
  signedOutText,
  type TeamAction,
} from "./access";
import { GiveRole, Members, type RowNote } from "./members";

const LEDE = "Which accounts hold a role on this proxy, and what each of them may do.";
/** Where the outcome of the form that names an account by id is said. No user id has a bracket in it. */
const FORM = "(form)";

export default function TeamPage() {
  const qc = useQueryClient();
  const [adminKey] = useAdminKey();
  // How the last change to each member ended, said at its row.
  const [notes, setNotes] = useState<Record<string, RowNote>>({});
  // Without a directory the proxy stops listing an account once its role is
  // removed. The row stays until the page is left, so what happened can be read.
  const [removed, setRemoved] = useState<TeamMember[]>([]);

  const team = useQuery({
    queryKey: ["tamga-team", adminKey],
    queryFn: () => api.listTeam(adminKey),
    enabled: !!adminKey,
    staleTime: 60 * 1000,
  });
  const defaultRole = team.data?.default_role;
  const listed: TeamMember[] = useMemo(() => team.data?.items ?? [], [team.data?.items]);
  const items = useMemo(
    () => [...listed, ...removed.filter((gone) => !listed.some((m) => m.user_id === gone.user_id))],
    [listed, removed],
  );

  const say = (at: string, note: RowNote) => setNotes((all) => ({ ...all, [at]: note }));
  const refresh = () => qc.invalidateQueries({ queryKey: ["tamga-team", adminKey] });
  const refused = (at: string, error: unknown, action: TeamAction) => say(at, { kind: "refused", text: explainTeamRefusal(error, action) });

  const setRole = useMutation({
    mutationFn: ({ id, role }: { id: string; role: TeamRole; at: string }) => api.setTeamRole(adminKey, id, role),
    onSuccess: (member, { id, role, at }) => {
      const before = items.find((m) => m.user_id === id);
      const was = before ? assignedRole(before, defaultRole) : "";
      // The proxy leaves the sessions alone when the role is the one the account had.
      const text =
        was === role
          ? `Already ${roleWord(role).toLowerCase()}. Nothing changed.`
          : was === "" && defaultRole !== "viewer"
            ? `Now ${roleWord(role).toLowerCase()}. The account can sign in.`
            : roleChangedText(role, member.sessions_valid_from);
      say(at, { kind: "done", text: at === FORM ? `${id}: ${text}` : text });
      setRemoved((all) => all.filter((m) => m.user_id !== id));
      void refresh();
    },
    onError: (e, { at }) => refused(at, e, "role"),
  });
  const removeRole = useMutation({
    mutationFn: (member: TeamMember) => api.removeTeamRole(adminKey, member.user_id),
    onSuccess: (_res, member) => {
      say(member.user_id, { kind: "done", text: accessRemovedText(defaultRole) });
      setRemoved((all) => [...all.filter((m) => m.user_id !== member.user_id), { ...member, role: "", updated_at: undefined, sessions_valid_from: undefined }]);
      void refresh();
    },
    onError: (e, member) => {
      refused(member.user_id, e, "remove");
      // "No role to remove" means the list here is behind the proxy's.
      void refresh();
    },
  });
  const signOut = useMutation({
    mutationFn: (member: TeamMember) => api.revokeTeamSessions(adminKey, member.user_id),
    onSuccess: (res, member) => {
      say(member.user_id, { kind: "done", text: signedOutText(res.sessions_valid_from) });
      void refresh();
    },
    onError: (e, member) => refused(member.user_id, e, "sign-out"),
  });
  const pending = setRole.isPending || removeRole.isPending || signOut.isPending;

  const roles = items.map((m) => assignedRole(m, defaultRole));
  const count = (role: TeamRole | "") => roles.filter((r) => r === role).length;
  const admins = count("admin");
  const without = count("");
  const holding = items.length - without;

  if (!adminKey || (team.isError && !team.data)) {
    return <NoData title="Team" lede={LEDE} adminKey={adminKey} error={team.error} what="the team" />;
  }

  return (
    <>
      <PageHead
        title="Team"
        lede={
          items.length === 0
            ? LEDE
            : `${counted(holding, "account")} ${holding === 1 ? "holds" : "hold"} a role on this proxy: ${counted(admins, "admin")}, ${counted(count("analyst"), "analyst")}, ${counted(count("viewer"), "viewer")}.${
                without > 0 ? ` ${counted(without, "account")} ${without === 1 ? "has" : "have"} none.` : ""
              }`
        }
      />

      <DataNotice adminKey={adminKey} failed={team.isError} error={team.error} what="the team" />

      {holding > 0 && admins === 0 ? (
        <InlineNotification
          kind="error"
          lowContrast
          hideCloseButton
          title="Nobody is an admin."
          subtitle="Only the admin key can manage the team and the settings until someone is."
          style={{ marginTop: "1.5rem", maxWidth: "none" }}
        />
      ) : null}
      {defaultRole === "viewer" ? (
        <InlineNotification
          kind="info"
          lowContrast
          hideCloseButton
          title="Accounts nobody gave a role are admitted as viewers."
          subtitle="TAMGA_SSO_DEFAULT_ROLE=viewer is set on the proxy, so anyone who can sign in reads the overview, incidents and reports. Removing a role leaves an account with that."
          style={{ marginTop: "1.5rem", maxWidth: "none" }}
        />
      ) : null}

      <div className="t-hunt">
        <Section
          id="members"
          title="Members"
          note={defaultRole === "" ? "An account must be given a role before it can sign in." : undefined}
        >
          {team.isLoading ? (
            <DataTableSkeleton columnCount={5} rowCount={4} showHeader={false} showToolbar={false} />
          ) : items.length === 0 ? (
            <p className="t-empty">No account holds a role yet. Give one a role by its user id.</p>
          ) : (
            <Members
              members={items}
              defaultRole={defaultRole}
              pending={pending}
              notes={notes}
              onRole={(m, role) => setRole.mutate({ id: m.user_id, role, at: m.user_id })}
              onRemove={(m) => removeRole.mutate(m)}
              onSignOut={(m) => signOut.mutate(m)}
            />
          )}
          {team.data?.clerk ? (
            <p className="t-footnote">
              Names and email addresses are read from the Clerk directory the proxy is connected to. Being listed there gives no access, and
              nobody signs in through Clerk.
            </p>
          ) : null}
        </Section>

        <div>
          <Section id="give-role" title="Give an account a role">
            <GiveRole pending={pending} note={notes[FORM]} onGive={(id, role) => setRole.mutate({ id, role, at: FORM })} />
          </Section>

          <Section id="roles" title="What each role can do" style={{ marginTop: "2.5rem" }}>
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
      </div>
    </>
  );
}
