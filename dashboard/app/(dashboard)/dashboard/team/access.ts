import { clockTime, dayLabel } from "@/components/console/names";
import { ApiError, permissionDenied, sessionRefused } from "@/lib/api/fetch-core";
import type { TeamList, TeamMember, TeamRole } from "@/lib/api";

export const ROLES: { role: TeamRole; name: string; can: string }[] = [
  { role: "admin", name: "Admin", can: "Everything: policies, settings, integrations, the team and the audit log." },
  { role: "analyst", name: "Analyst", can: "Triage incidents and edit policies and patterns." },
  { role: "viewer", name: "Viewer", can: "Read the overview, incidents and reports." },
];

export const roleWord = (role: TeamRole) => ROLES.find((r) => r.role === role)?.name ?? role;

export type DefaultRole = TeamList["default_role"];

/** Go's zero time: the proxy holds no record of the account. */
const neverSet = (iso?: string) => !iso || iso.startsWith("0001-");

/**
 * The role an admin gave the account, or "" when nobody gave it one.
 *
 * The proxy says which it is with "assigned". An older proxy does not, and
 * there it is read from the time the role was set: where accounts without a
 * role are admitted as viewers, such a proxy lists the account with "viewer"
 * in place of nothing and holds no record of it, so no time either.
 */
export function assignedRole(member: TeamMember, defaultRole: DefaultRole): TeamRole | "" {
  if (member.assigned === false) return "";
  if (member.assigned === undefined && member.role && member.role === defaultRole && neverSet(member.updated_at)) return "";
  return member.role;
}

/** What an account without a role has. */
export const unassignedWord = (defaultRole: DefaultRole) => (defaultRole === "viewer" ? "Viewer by default" : "No access");

/**
 * When the role was given, or that it was taken away and when. Nothing for an
 * account that never had one.
 */
export function roleSetLabel(member: TeamMember, defaultRole: DefaultRole): string {
  const when = neverSet(member.updated_at) ? "" : dayLabel(member.updated_at as string);
  if (assignedRole(member, defaultRole) !== "") return when;
  if (!member.role_removed) return "";
  return when ? `Role removed ${when}` : "Role removed";
}

const iso = (unixSeconds: number) => new Date(unixSeconds * 1000).toISOString();

/** When an account's sessions were last ended, or "" when they never were. */
export function sessionsEndedLabel(member: TeamMember): string {
  const from = member.sessions_valid_from;
  return from ? `Sessions ended ${dayLabel(iso(from))}, ${clockTime(iso(from))}` : "";
}

/** What ending the sessions did, from the time the proxy answered with. */
function tokensEnded(from?: number): string {
  const which = from ? `Tokens issued before ${clockTime(iso(from))}` : "Tokens issued before now";
  return `${which} no longer work; other replicas follow within 30 seconds.`;
}

export const signedOutText = (from?: number) => `Signed out. ${tokensEnded(from)}`;

/** A role that changed also ended the account's sessions; the proxy's answer says when. */
export const roleChangedText = (role: TeamRole, from?: number) =>
  `Now ${roleWord(role).toLowerCase()}, and signed out to sign in again with that role. ${tokensEnded(from)}`;

export const accessRemovedText = (defaultRole: DefaultRole) =>
  defaultRole === "viewer"
    ? "Role removed and signed out. This proxy admits accounts without a role as viewers, so the account can still sign in as one."
    : "Access removed and signed out. The account cannot sign in until it is given a role again.";

export type TeamAction = "role" | "remove" | "sign-out";

const NOT_DONE: Record<TeamAction, string> = {
  role: "The role was not changed",
  remove: "Access was not removed",
  "sign-out": "Nobody was signed out",
};

/** What the proxy answered when it refused a change to a member, as one sentence for the row. */
export function explainTeamRefusal(error: unknown, action: TeamAction): string {
  const notDone = NOT_DONE[action];
  const known = permissionDenied(error) ?? sessionRefused(error);
  if (known) return known.refused;
  if (!(error instanceof ApiError)) return `${notDone}: the proxy did not answer.`;
  if (error.code === "own_role") {
    return action === "remove"
      ? "You cannot remove your own access. Another admin can, or whoever holds the admin key."
      : "You cannot change your own role. Another admin can, or whoever holds the admin key.";
  }
  if (error.code === "last_admin") return `This is the only admin. ${notDone}: make someone else an admin first.`;
  if (error.status === 400) {
    return error.message === "invalid user id"
      ? `${notDone}. The proxy does not take this as a user id: it is letters, digits and _ - . @ | : + only, up to 128 characters.`
      : `${notDone}. The proxy did not accept the request: ${error.message}.`;
  }
  if (error.status === 404 && action === "remove") return "This account has no role to remove.";
  if (error.status === 401) return `${notDone}. The proxy did not accept the admin key: check it under Settings.`;
  if (error.status === 503) return `${notDone}. The proxy could not store the change. Try again.`;
  return `${notDone}. The proxy said: ${error.message}`;
}
