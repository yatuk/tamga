"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { DataTableSkeleton, InlineNotification } from "@carbon/react";
import { counted } from "@/components/console/names";
import { DataNotice, NoData, PageHead, Section } from "@/components/console/page";
import { useAdminKey } from "@/hooks/useAdminKey";
import { api, type TeamMember, type TeamRole } from "@/lib/api";
import { ApiError } from "@/lib/api/fetch-core";
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

/**
 * How the last change on the page ended. There is one, so a sentence about an
 * account is never left standing beside a later one that made it false.
 */
interface Said {
  /** The row it is said at, or FORM. */
  at: string;
  account: string;
  note: RowNote;
  /** The role the sentence leaves the account with, and the number of admins where it speaks of them. */
  role: TeamRole | "";
  admins?: number;
  /** The list the sentence was said over. */
  asOf: number;
}

export default function TeamPage() {
  const qc = useQueryClient();
  const [adminKey] = useAdminKey();
  const [said, setSaid] = useState<Said | null>(null);
  // An older proxy stops listing an account once its role is removed. The row
  // stays until the page is left, so what happened can be read. Where the
  // proxy lists the account itself, its row is the one shown.
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

  const roleOf = (id: string) => {
    const member = items.find((m) => m.user_id === id);
    return member ? assignedRole(member, defaultRole) : "";
  };
  const roles = items.map((m) => assignedRole(m, defaultRole));
  const count = (role: TeamRole | "") => roles.filter((r) => r === role).length;
  const admins = count("admin");
  const without = count("");
  const holding = items.length - without;

  const say = (at: string, account: string, note: RowNote, role: TeamRole | "", lastAdmin = false) =>
    setSaid({ at, account, note, role, admins: lastAdmin ? admins : undefined, asOf: team.dataUpdatedAt });
  const refresh = () => qc.invalidateQueries({ queryKey: ["tamga-team", adminKey] });
  const refused = (at: string, account: string, error: unknown, action: TeamAction) => {
    const code = error instanceof ApiError ? error.code : undefined;
    // "No role to remove" is about the account as the proxy has it, not as the list here had it.
    const noRole = action === "remove" && error instanceof ApiError && error.status === 404;
    say(at, account, { kind: "refused", text: explainTeamRefusal(error, action) }, noRole ? "" : roleOf(account), code === "last_admin");
  };
  // A sentence stands until the list is read again and no longer agrees with
  // it: someone else, or another tab, has changed the account since.
  const standing =
    said && (team.dataUpdatedAt === said.asOf || (roleOf(said.account) === said.role && (said.admins === undefined || said.admins === admins)))
      ? said
      : null;

  const setRole = useMutation({
    mutationFn: ({ id, role }: { id: string; role: TeamRole; at: string }) => api.setTeamRole(adminKey, id, role),
    onMutate: () => setSaid(null),
    onSuccess: (member, { id, role, at }) => {
      const was = roleOf(id);
      // The proxy leaves the sessions alone when the role is the one the account had.
      const text =
        was === role
          ? `Already ${roleWord(role).toLowerCase()}. Nothing changed.`
          : was === "" && defaultRole !== "viewer"
            ? `Now ${roleWord(role).toLowerCase()}. The account can sign in.`
            : roleChangedText(role, member.sessions_valid_from);
      say(at, id, { kind: "done", text: at === FORM ? `${id}: ${text}` : text }, role);
      setRemoved((all) => all.filter((m) => m.user_id !== id));
      void refresh();
    },
    onError: (e, { id, at }) => refused(at, id, e, "role"),
  });
  const removeRole = useMutation({
    mutationFn: (member: TeamMember) => api.removeTeamRole(adminKey, member.user_id),
    onMutate: () => setSaid(null),
    onSuccess: (res, member) => {
      say(member.user_id, member.user_id, { kind: "done", text: accessRemovedText(defaultRole) }, "");
      setRemoved((all) => [
        ...all.filter((m) => m.user_id !== member.user_id),
        { ...member, role: "", assigned: false, role_removed: true, updated_at: undefined, sessions_valid_from: res.sessions_valid_from },
      ]);
      void refresh();
    },
    onError: (e, member) => {
      refused(member.user_id, member.user_id, e, "remove");
      // "No role to remove" means the list here is behind the proxy's.
      void refresh();
    },
  });
  const signOut = useMutation({
    mutationFn: (member: TeamMember) => api.revokeTeamSessions(adminKey, member.user_id),
    onMutate: () => setSaid(null),
    onSuccess: (res, member) => {
      say(member.user_id, member.user_id, { kind: "done", text: signedOutText(res.sessions_valid_from) }, roleOf(member.user_id));
      void refresh();
    },
    onError: (e, member) => refused(member.user_id, member.user_id, e, "sign-out"),
  });
  const pending = setRole.isPending || removeRole.isPending || signOut.isPending;

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
              notes={standing && standing.at !== FORM ? { [standing.at]: standing.note } : {}}
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
            <GiveRole pending={pending} note={standing?.at === FORM ? standing.note : undefined} onGive={(id, role) => setRole.mutate({ id, role, at: FORM })} />
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
