import { http, HttpResponse } from "msw";

const API = "/api/v1";

/*
 * The team routes, answered as the proxy answers them (proxy/internal/api/
 * team.go, proxy/internal/users/store.go).
 *
 * Who is calling is read from the admin key, by its first letters:
 *
 *   ""  or "wrong..."       no credential the proxy accepts: 401.
 *   "viewer...", "analyst..." a key whose role is below admin: 403
 *                           "permission_denied", the routes ask for team:admin.
 *   "session-<id>"          stands for user <id> signed in as an admin: actor
 *                           "user:<id>". The console sends no such thing; it
 *                           is here for the answer the proxy gives someone who
 *                           changes their own role (403 "own_role").
 *   anything else           the static admin key: actor "admin-key".
 *
 * As in the proxy: a role that differs from the one before ends the account's
 * sessions, an account whose role was removed is no longer listed (unless the
 * directory lists it), and the only admin keeps the role (409 "last_admin").
 *
 * State is kept between requests so that a write shows in the next read.
 * Tests start from a known state with teamMock.reset().
 */

export interface MockMember {
  user_id: string;
  email?: string;
  name?: string;
  role: string;
  updated_at: string;
  sessions_valid_from?: number;
}

/** What Go writes for a time that was never set. */
const NEVER = "0001-01-01T00:00:00Z";

const BASE_MEMBERS: MockMember[] = [
  { user_id: "1001", name: "Ayse Kaya", email: "ayse@example.com", role: "admin", updated_at: "2026-09-01T10:00:00Z", sessions_valid_from: 1788256800 },
  { user_id: "1002", name: "Mehmet Demir", email: "mehmet@example.com", role: "analyst", updated_at: "2026-09-02T10:00:00Z" },
  { user_id: "1003", role: "viewer", updated_at: "2026-09-03T10:00:00Z" },
];

const clone = <T>(v: T): T => JSON.parse(JSON.stringify(v)) as T;

const state = {
  /** Every record of the user store, the ones without a role included. */
  members: clone(BASE_MEMBERS),
  /** Accounts the Clerk directory lists. Being listed assigns nothing. */
  directory: [] as { user_id: string; name?: string; email?: string }[],
  clerk: false,
  defaultRole: "" as "" | "viewer",
  /** The proxy's clock, in unix seconds. */
  now: 1791633720,
};

export const teamMock = {
  /** Back to three members with a role each, no directory and no default role. */
  reset(options: { members?: MockMember[]; directory?: typeof state.directory; defaultRole?: "" | "viewer"; now?: number } = {}) {
    state.members = clone(options.members ?? BASE_MEMBERS);
    state.directory = clone(options.directory ?? []);
    state.clerk = options.directory !== undefined;
    state.defaultRole = options.defaultRole ?? "";
    state.now = options.now ?? 1791633720;
  },
  /** The record the proxy holds for a user, or nothing. */
  member(id: string): MockMember | undefined {
    const found = state.members.find((m) => m.user_id === id);
    return found ? clone(found) : undefined;
  },
  get now() {
    return state.now;
  },
};

function refusedCaller(request: Request) {
  const key = request.headers.get("X-Tamga-Admin-Key") ?? "";
  if (key === "" || key.startsWith("wrong")) return HttpResponse.json({ error: "unauthorized" }, { status: 401 });
  if (key.startsWith("viewer") || key.startsWith("analyst")) {
    const role = key.startsWith("viewer") ? "viewer" : "analyst";
    return HttpResponse.json(
      { error: `role ${role} lacks team:admin`, code: "permission_denied", required_permission: "team:admin" },
      { status: 403 },
    );
  }
  return null;
}

const actorOf = (request: Request) => {
  const key = request.headers.get("X-Tamga-Admin-Key") ?? "";
  return key.startsWith("session-") ? `user:${key.slice("session-".length)}` : "admin-key";
};

const validUserId = (id: string) => /^[A-Za-z0-9_\-.@|:+]{1,128}$/.test(id);
const badId = () => HttpResponse.json({ error: "invalid user id" }, { status: 400 });
const lastAdminRefused = () =>
  HttpResponse.json({ error: "this is the last admin; make someone else an admin first", code: "last_admin" }, { status: 409 });

const isLastAdmin = (id: string) =>
  state.members.find((m) => m.user_id === id)?.role === "admin" && !state.members.some((m) => m.user_id !== id && m.role === "admin");

/** The revocation time that covers every token issued up to now. */
const revokedNow = (m: MockMember) => Math.max(state.now, (m.sessions_valid_from ?? 0) + 1);
const nowISO = () => new Date(state.now * 1000).toISOString();

function listed() {
  const stored = state.members.filter((m) => m.role !== "");
  if (!state.clerk) return stored;
  const fromDirectory = state.directory.map((d) => {
    const held = stored.find((m) => m.user_id === d.user_id);
    return held ? { ...d, ...held } : { ...d, role: state.defaultRole, updated_at: NEVER };
  });
  return [...fromDirectory, ...stored.filter((m) => !state.directory.some((d) => d.user_id === m.user_id))];
}

export const teamHandlers = [
  http.get(`${API}/team`, ({ request }) => {
    const refused = refusedCaller(request);
    if (refused) return refused;
    const items = listed();
    return HttpResponse.json({ items, total: items.length, clerk: state.clerk, default_role: state.defaultRole });
  }),

  http.put(`${API}/team/:id/role`, async ({ request, params }) => {
    const refused = refusedCaller(request);
    if (refused) return refused;
    const id = String(params.id).trim();
    if (!validUserId(id)) return badId();
    if (actorOf(request) === `user:${id}`) {
      return HttpResponse.json({ error: "you cannot change your own role", code: "own_role" }, { status: 403 });
    }
    const body = (await request.json().catch(() => null)) as { role?: string } | null;
    if (!body) return HttpResponse.json({ error: "invalid body" }, { status: 400 });
    const role = String(body.role ?? "").trim().toLowerCase();
    if (!["admin", "analyst", "viewer"].includes(role)) return HttpResponse.json({ error: "invalid role" }, { status: 400 });
    if (role !== "admin" && isLastAdmin(id)) return lastAdminRefused();
    let member = state.members.find((m) => m.user_id === id);
    if (!member) {
      member = { user_id: id, role: "", updated_at: NEVER };
      state.members.push(member);
    }
    if (member.role !== role) member.sessions_valid_from = revokedNow(member);
    member.role = role;
    member.updated_at = nowISO();
    return HttpResponse.json(member);
  }),

  http.delete(`${API}/team/:id/role`, ({ request, params }) => {
    const refused = refusedCaller(request);
    if (refused) return refused;
    const id = String(params.id).trim();
    if (!validUserId(id)) return badId();
    if (actorOf(request) === `user:${id}`) {
      return HttpResponse.json({ error: "you cannot remove your own role", code: "own_role" }, { status: 403 });
    }
    if (isLastAdmin(id)) return lastAdminRefused();
    const member = state.members.find((m) => m.user_id === id);
    if (!member || member.role === "") return HttpResponse.json({ error: "this account has no role" }, { status: 404 });
    member.role = "";
    member.sessions_valid_from = revokedNow(member);
    member.updated_at = nowISO();
    return HttpResponse.json({ ok: true, user_id: id });
  }),

  // The user need not have a role, and an admin may end their own sessions.
  http.post(`${API}/team/:id/sessions/revoke`, ({ request, params }) => {
    const refused = refusedCaller(request);
    if (refused) return refused;
    const id = String(params.id).trim();
    if (!validUserId(id)) return badId();
    let member = state.members.find((m) => m.user_id === id);
    if (!member) {
      member = { user_id: id, role: "", updated_at: nowISO() };
      state.members.push(member);
    }
    member.sessions_valid_from = revokedNow(member);
    return HttpResponse.json({ ok: true, user_id: id, sessions_valid_from: member.sessions_valid_from });
  }),
];
