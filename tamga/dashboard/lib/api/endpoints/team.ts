import { authHeaders, fetchAPI } from "@/lib/api/fetch-core";
import type { TeamMember, TeamRole } from "@/lib/api/types-extended";

export const team = {
  listTeam: (adminKey: string) =>
    fetchAPI<{ items: TeamMember[]; total: number; clerk: boolean }>(
      "/api/v1/team",
      { headers: authHeaders(adminKey) },
    ),

  setTeamRole: (adminKey: string, userId: string, role: TeamRole) =>
    fetchAPI<{ ok: boolean }>(
      `/api/v1/team/${encodeURIComponent(userId)}/role`,
      {
        method: "PUT",
        headers: { ...authHeaders(adminKey), "Content-Type": "application/json" },
        body: JSON.stringify({ role }),
      },
    ),
};
