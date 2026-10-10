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
 * sessions, the only admin keeps the role (409 "last_admin"), and every member
 * says with "assigned" whether an admin gave it the role it is listed with.
 * An account whose role was removed stays listed with "role_removed", the
 * default role in place of its own, and the time of the removal. An account
 * that never had a role is listed only by the directory.
 *
 * teamMock.reset({ older: true }) answers as a proxy from before "assigned":
 * the field is absent, an account whose role was removed is no longer listed
 * (unless the directory lists it), and removing a role does not say from when
 * the sessions ended.
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
  /** An admin took the role away. The record then has no role. */
  role_removed?: boolean;
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
  older: false,
  /** The proxy's clock, in unix seconds. */
  now: 1791633720,
};

export const teamMock = {
  /** Back to three members with a role each, no directory and no default role. */
  reset(
    options: { members?: MockMember[]; directory?: typeof state.directory; defaultRole?: "" | "viewer"; now?: number; older?: boolean } = {},
  ) {
    state.members = clone(options.members ?? BASE_MEMBERS);
    state.directory = clone(options.directory ?? []);
    state.clerk = options.directory !== undefined;
    state.defaultRole = options.defaultRole ?? "";
    state.older = options.older ?? false;
    state.now = options.now ?? 1791633720;
  },
  /** The record the proxy holds for a user, or nothing. */
  member(id: string): MockMember | undefined {
    const found = state.members.find((m) => m.user_id === id);
    return found ? clone(found) : undefined;
  },
  /** Moves the proxy's clock, as time passing between two changes does. */
  set now(unix: number) {
    state.now = unix;
  },
  get now() {
    return state.now;
  },
  /** A change made by someone else: nothing in the page asked for it. */
  setElsewhere(id: string, role: string) {
    const found = state.members.find((m) => m.user_id === id);
    if (found) found.role = role;
    else state.members.push({ user_id: id, role, updated_at: new Date(state.now * 1000).toISOString() });
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

/** A member as the proxy answers with it: an older one does not say whether the role was assigned, or that it was removed. */
function answered(m: MockMember) {
  const { role_removed: removed, ...record } = m;
  if (state.older) return record;
  if (record.role !== "") return { ...record, assigned: true };
  // Without a role of its own the account has the default one.
  return { ...record, role: state.defaultRole, assigned: false, ...(removed ? { role_removed: true } : {}) };
}

function listed() {
  // The roles, and the accounts whose role was removed. An older proxy lists only the roles.
  const stored = state.members.filter((m) => m.role !== "" || (m.role_removed && !state.older));
  if (!state.clerk) return stored.map(answered);
  const fromDirectory = state.directory.map((d) => {
    const held = stored.find((m) => m.user_id === d.user_id);
    if (held) return { ...d, ...answered(held) };
    // Being in the directory assigns nothing: the role is the default, and no admin gave it.
    const unassigned = { ...d, role: state.defaultRole, updated_at: NEVER };
    return state.older ? unassigned : { ...unassigned, assigned: false };
  });
  return [...fromDirectory, ...stored.filter((m) => !state.directory.some((d) => d.user_id === m.user_id)).map(answered)];
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
    delete member.role_removed;
    member.updated_at = nowISO();
    return HttpResponse.json(answered(member));
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
    member.role_removed = true;
    member.updated_at = nowISO();
    if (state.older) return HttpResponse.json({ ok: true, user_id: id });
    return HttpResponse.json({ ok: true, user_id: id, assigned: false, sessions_valid_from: member.sessions_valid_from });
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
    const body = { ok: true, user_id: id, sessions_valid_from: member.sessions_valid_from };
    return HttpResponse.json(state.older ? body : { ...body, assigned: member.role !== "" });
  }),
];
