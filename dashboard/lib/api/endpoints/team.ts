import { authHeaders, fetchAPI } from "@/lib/api/fetch-core";
import type { TeamList, TeamMember, TeamRole } from "@/lib/api/types-extended";

const member = (userId: string) => `/api/v1/team/${encodeURIComponent(userId)}`;

export const team = {
  listTeam: (adminKey: string) =>
    fetchAPI<TeamList>(
      "/api/v1/team",
      { headers: authHeaders(adminKey) },
    ),

  /** Ends the account's sessions and leaves its role as it is. */
  revokeTeamSessions: (adminKey: string, userId: string) =>
    fetchAPI<{ ok: boolean; user_id: string; sessions_valid_from?: number }>(
      `${member(userId)}/sessions/revoke`,
      { method: "POST", headers: authHeaders(adminKey) },
    ),

  /** Takes the role away: the account's sessions end and it has no access. */
  removeTeamRole: (adminKey: string, userId: string) =>
    fetchAPI<{ ok: boolean; user_id: string }>(
      `${member(userId)}/role`,
      { method: "DELETE", headers: authHeaders(adminKey) },
    ),

  /** Assigns a role. A role that differs from the one before also ends the account's sessions. */
  setTeamRole: (adminKey: string, userId: string, role: TeamRole) =>
    fetchAPI<TeamMember>(
      `${member(userId)}/role`,
      {
        method: "PUT",
        headers: { ...authHeaders(adminKey), "Content-Type": "application/json" },
        body: JSON.stringify({ role }),
      },
    ),
};
