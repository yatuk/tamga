import { http, HttpResponse } from "msw";

const API = "/api/v1";

/*
 * The policy routes, answered as the proxy answers them (proxy/internal/api/
 * policy_edit.go, policy_history.go, policy_change.go).
 *
 * Who is calling is read from the admin key, by its first letters:
 *
 *   ""  or "wrong..."   no credential the proxy accepts: 401.
 *   "analyst..."        an API key of scope write: role analyst, actor
 *                       "apikey:<the key>". It may change rules (except a
 *                       rule's notify), output_rules, custom_entities,
 *                       competitors, name and version.
 *   "personal..."       an API key of scope admin that is one person's own:
 *                       role admin, actor "apikey:<the key>". Two of these
 *                       are two people, which dual control needs.
 *   anything else       the static admin key: role admin, actor "admin-key".
 *                       Every holder is the same actor.
 *
 * The mock reads a policy as JSON only (the console sends JSON; the proxy
 * also takes YAML). It refuses two things as the proxy's limits would, both
 * 422: a negative rate_limit.requests_per_minute and a rule action that is
 * not an action. Text that is not JSON is the 400 "policy parse" answer.
 *
 * A rule's alert address (notify[].webhook) is shown to an admin only
 * (policy_mask.go). An analyst reads "scheme://host/***masked***" in the
 * policy, the revisions and the proposals, and the answer carries the header
 * X-Tamga-Masked: notify.webhook. A "${NAME}" reference is never masked. A
 * policy that holds the mask is refused with 422 "masked_value", from an
 * admin too.
 *
 * State is kept between requests so that a write shows in the next read.
 * Tests start from a known state with policyMock.reset().
 */

type Doc = Record<string, unknown>;

export interface MockProposal {
  id: string;
  timestamp: string;
  author: string;
  message: string;
  yaml: string;
  status: "open" | "approved" | "rejected";
  approved_by?: string;
  approved_at?: string;
  rejected_by?: string;
  rejected_at?: string;
}

interface MockRevision {
  id: string;
  timestamp: string;
  author: string;
  message: string;
  yaml: string;
}

const BASE_POLICY: Doc = {
  version: "v1",
  name: "default",
  rules: {
    pii_detection: { action: "redact", severity: "high", notify: [{ channel: "slack", webhook: "${SLACK_SECURITY_WEBHOOK}" }] },
    prompt_injection: { action: "warn", severity: "medium" },
  },
  output_rules: null,
  rate_limit: { requests_per_minute: 600 },
  providers: { allowed: ["anthropic", "openai"] },
  custom_entities: [
    { name: "project_mercury", pattern: "Project[ -]?Mercury", description: "", severity: "medium", action: "log", confidence: 0.85 },
  ],
  competitors: [],
  governance: { require_dual_control: false },
};

const clone = <T>(v: T): T => JSON.parse(JSON.stringify(v)) as T;

const state = {
  policy: clone(BASE_POLICY),
  readOnly: false,
  /** Counts writes of the policy. A proposal written before the last one is stale. */
  writes: 0,
  revisions: [] as MockRevision[],
  proposals: [] as MockProposal[],
  /** The value of `writes` when each proposal was written. */
  writtenAt: new Map<string, number>(),
  nextId: 1,
};

const now = () => new Date().toISOString();
const newId = (prefix: string) => `${prefix}${String(state.nextId++).padStart(8, "0")}`;

export const policyMock = {
  /** Back to the starting policy, with no proposals and a writable file. */
  reset(options: { policy?: Doc; readOnly?: boolean } = {}) {
    state.policy = { ...clone(BASE_POLICY), ...clone(options.policy ?? {}) };
    state.readOnly = options.readOnly ?? false;
    state.writes = 0;
    state.revisions = [];
    state.proposals = [];
    state.writtenAt.clear();
    state.nextId = 1;
  },
  get policy(): Doc {
    return clone(state.policy);
  },
  get readOnly() {
    return state.readOnly;
  },
  /** The running policy with some sections replaced, as the text a console would send. */
  draft(changes: Doc): string {
    return JSON.stringify({ ...clone(state.policy), ...changes }, null, 2);
  },
  /** Puts a proposal in the list as if `author` had opened it. */
  addProposal(p: { author: string; yaml: string; message?: string; status?: MockProposal["status"] }): MockProposal {
    const proposal: MockProposal = {
      id: newId("prop"),
      timestamp: now(),
      author: p.author,
      message: p.message ?? "",
      yaml: p.yaml,
      status: p.status ?? "open",
    };
    state.proposals.push(proposal);
    state.writtenAt.set(proposal.id, state.writes);
    return proposal;
  },
  /** Settles a proposal behind the console's back, as someone else deciding it would. */
  settle(id: string, status: "approved" | "rejected") {
    const proposal = state.proposals.find((p) => p.id === id);
    if (proposal) proposal.status = status;
  },
  /** Makes the policy file read-only (or writable again) after the console has read the proxy's health. */
  setReadOnly(readOnly: boolean) {
    state.readOnly = readOnly;
  },
  /** Changes the running policy behind the console's back, as a write by someone else would. */
  writeBehind(changes: Doc) {
    install({ ...clone(state.policy), ...changes }, "apikey:someone-else", "direct write (PUT /policies)");
  },
};

// ── Who is calling ──────────────────────────────────────────────────────
interface Caller {
  actor: string;
  role: "admin" | "analyst";
}

function callerOf(request: Request): Caller | null {
  const key = request.headers.get("X-Tamga-Admin-Key") ?? "";
  if (key === "" || key.startsWith("wrong")) return null;
  if (key.startsWith("analyst")) return { actor: `apikey:${key}`, role: "analyst" };
  if (key.startsWith("personal")) return { actor: `apikey:${key}`, role: "admin" };
  return { actor: "admin-key", role: "admin" };
}

const unauthorized = () => HttpResponse.json({ error: "unauthorized" }, { status: 401 });
const sharedActor = (actor: string) => actor === "admin-key" || actor === "dev-mode";

// ── Alert addresses, as a caller below admin reads them ─────────────────
const NOTIFY_MASK = "***masked***";

function maskWebhook(value: string): string {
  const t = value.trim();
  if (t === "" || (t.startsWith("${") && t.endsWith("}")) || t.includes(NOTIFY_MASK)) return value;
  try {
    const u = new URL(t);
    if (u.protocol === "http:" || u.protocol === "https:") return `${u.protocol}//${u.host}/${NOTIFY_MASK}`;
  } catch {
    // Not a URL: nothing of it is shown.
  }
  return NOTIFY_MASK;
}

/** Calls `visit` with every notify entry of every rule that has a webhook. */
function eachWebhook(pol: Doc, visit: (rule: string, entry: Doc) => void) {
  for (const [name, rule] of Object.entries(isObject(pol.rules) ? pol.rules : {})) {
    const notify = isObject(rule) && Array.isArray(rule.notify) ? rule.notify : [];
    for (const entry of notify) if (isObject(entry) && typeof entry.webhook === "string") visit(name, entry);
  }
}

/** A copy of the policy as `who` may read it, and whether anything in it was masked. */
function readable(who: Caller, pol: Doc): { doc: Doc; masked: boolean } {
  if (who.role === "admin") return { doc: pol, masked: false };
  const doc = clone(pol);
  let masked = false;
  eachWebhook(doc, (_rule, entry) => {
    const shown = maskWebhook(entry.webhook as string);
    if (shown !== entry.webhook) {
      entry.webhook = shown;
      masked = true;
    }
  });
  return { doc, masked };
}

/** Policy text as `who` may read it. Text that is not JSON has no address the mock can find. */
function readableText(who: Caller, text: string): { text: string; masked: boolean } {
  try {
    const parsed: unknown = JSON.parse(text);
    if (!isObject(parsed)) return { text, masked: false };
    const { doc, masked } = readable(who, parsed);
    return masked ? { text: JSON.stringify(doc, null, 2), masked } : { text, masked };
  } catch {
    return { text, masked: false };
  }
}

/** Answers with records that carry policy text (revisions, proposals), each as `who` may read it. */
function withText<T extends { yaml: string }>(who: Caller, records: T | T[], init?: ResponseInit) {
  let masked = false;
  const show = (record: T): T => {
    const shown = readableText(who, record.yaml);
    masked = masked || shown.masked;
    return { ...record, yaml: shown.text };
  };
  const body = Array.isArray(records) ? records.map(show) : show(records);
  return HttpResponse.json(body, { ...init, headers: masked ? { "X-Tamga-Masked": "notify.webhook" } : undefined });
}

/** The rules whose alert address is the mask, as the proxy names them in its refusal. */
function maskedFields(pol: Doc): string[] {
  const fields = new Set<string>();
  eachWebhook(pol, (rule, entry) => {
    if ((entry.webhook as string).includes(NOTIFY_MASK)) fields.add(`rules.${rule}.notify`);
  });
  return [...fields].sort();
}

const maskedRefused = (fields: string[]) =>
  HttpResponse.json(
    {
      error:
        `the policy holds a masked webhook URL (${NOTIFY_MASK}) in ${fields.join(", ")}: ` +
        "this text was read without the admin role, which hides webhook URLs. " +
        "Write the URL itself or, better, a reference such as ${SLACK_WEBHOOK_URL}.",
      code: "masked_value",
      fields,
    },
    { status: 422 },
  );

// ── What differs between two policies ───────────────────────────────────
const ANALYST_SECTIONS = new Set(["name", "version", "rules", "output_rules", "custom_entities", "competitors"]);
const same = (a: unknown, b: unknown) => JSON.stringify(a ?? null) === JSON.stringify(b ?? null);
const isObject = (v: unknown): v is Doc => !!v && typeof v === "object" && !Array.isArray(v);
const keysOf = (...docs: unknown[]) => [...new Set(docs.flatMap((d) => (isObject(d) ? Object.keys(d) : [])))];

interface Change {
  section: string;
  path: string;
  privileged: boolean;
}

function diff(old: Doc, next: Doc): Change[] {
  const out: Change[] = [];
  for (const section of keysOf(old, next)) {
    const a = old[section];
    const b = next[section];
    if (same(a, b)) continue;
    const privileged = !ANALYST_SECTIONS.has(section);
    if (section === "rules") {
      for (const name of keysOf(a, b)) {
        const o = isObject(a) ? a[name] : undefined;
        const n = isObject(b) ? b[name] : undefined;
        if (same(o, n)) continue;
        if (isObject(o) && isObject(n)) {
          for (const k of keysOf(o, n)) {
            if (!same(o[k], n[k])) out.push({ section, path: `rules.${name}.${k}`, privileged: k === "notify" });
          }
          continue;
        }
        out.push({ section, path: `rules.${name}`, privileged: false });
        const notify = (isObject(o) && o.notify) || (isObject(n) && n.notify);
        if (Array.isArray(notify) && notify.length > 0) out.push({ section, path: `rules.${name}.notify`, privileged: true });
      }
    } else if (isObject(a) || isObject(b)) {
      const keys = keysOf(a, b).filter((k) => !same(isObject(a) ? a[k] : undefined, isObject(b) ? b[k] : undefined));
      if (keys.length === 0) out.push({ section, path: section, privileged });
      for (const k of keys) out.push({ section, path: `${section}.${k}`, privileged });
    } else if (Array.isArray(a) || Array.isArray(b)) {
      const named = (list: unknown) => new Map((Array.isArray(list) ? list : []).map((e: Doc) => [String(e?.name ?? ""), e]));
      const am = named(a);
      const bm = named(b);
      const names = [...new Set([...am.keys(), ...bm.keys()])].filter((n) => !same(am.get(n), bm.get(n)));
      if (names.length === 0) out.push({ section, path: section, privileged });
      for (const n of names) out.push({ section, path: `${section}.${n}`, privileged });
    } else {
      out.push({ section, path: section, privileged });
    }
  }
  return out.sort((x, y) => x.path.localeCompare(y.path));
}

const changedPaths = (old: Doc, next: Doc) => diff(old, next).map((c) => c.path);

function privilegedSections(old: Doc, next: Doc): string[] {
  const names = diff(old, next)
    .filter((c) => c.privileged)
    .map((c) => (c.section === "rules" ? c.path : c.section));
  return [...new Set(names)].sort();
}

// ── Refusals, worded as the proxy words them ────────────────────────────
const ACTIONS = new Set(["block", "strip", "redact", "warn", "log", "pass"]);

interface Issue {
  field: string;
  rule: string;
  message: string;
  severity: "error" | "warning";
}

function issuesOf(pol: Doc): Issue[] {
  const issues: Issue[] = [];
  const perMinute = isObject(pol.rate_limit) ? pol.rate_limit.requests_per_minute : undefined;
  if (typeof perMinute === "number" && perMinute < 0) {
    issues.push({ field: "rate_limit.requests_per_minute", rule: "min", message: "must not be negative", severity: "error" });
  }
  const rules = isObject(pol.rules) ? pol.rules : {};
  for (const [name, rule] of Object.entries(rules)) {
    const action = isObject(rule) ? String(rule.action ?? "") : "";
    if (action && !ACTIONS.has(action.toLowerCase())) {
      issues.push({ field: `rules.${name}.action`, rule: "enum", message: `unknown action "${action}"`, severity: "error" });
    }
  }
  if (!("prompt_injection" in rules)) {
    issues.push({ field: "rules.prompt_injection", rule: "coverage", message: "no rule covers prompt injection", severity: "warning" });
  }
  return issues;
}

const validationFailed = (issues: Issue[]) =>
  HttpResponse.json(
    { error: { code: "validation_failed", message: "Policy validation failed", details: issues.filter((i) => i.severity === "error") } },
    { status: 422 },
  );

const dualControlRefused = (who: Caller) =>
  HttpResponse.json(
    {
      error:
        "governance.require_dual_control is on: the policy cannot be written directly. " +
        "Open a proposal (POST /api/v1/policies/proposals) and have a second person approve it. " +
        "Turning dual control off is itself a proposal a second person approves." +
        (sharedActor(who.actor) ? ' Every caller that uses the static admin key is the same actor ("admin-key"), so dual control cannot be satisfied this way.' : ""),
      code: "dual_control_required",
    },
    { status: 409 },
  );

const readOnlyRefused = () =>
  HttpResponse.json(
    {
      error: "The policy file is read-only where the proxy runs, so it cannot be changed from the console.",
      code: "policy_read_only",
    },
    { status: 409 },
  );

function privilegeRefused(who: Caller, next: Doc) {
  if (who.role === "admin") return null;
  const sections = privilegedSections(state.policy, next);
  if (sections.length === 0) return null;
  return HttpResponse.json(
    {
      error:
        `this change needs the admin role: it touches ${sections.join(", ")}. ` +
        "Without it you may change rules (except notify), output_rules, custom_entities and competitors.",
      code: "admin_role_required",
      privileged_sections: sections,
    },
    { status: 403 },
  );
}

const dualControlOn = () => isObject(state.policy.governance) && state.policy.governance.require_dual_control === true;

/** Reads `{ "yaml": "..." }` and decodes the text. Answers the request itself when the text is refused. */
async function readPolicy(request: Request, { dryRun = false } = {}): Promise<{ text: string; policy: Doc } | { refused: Response }> {
  const body = (await request.json().catch(() => ({}))) as { yaml?: string };
  const text = body.yaml ?? "";
  if (!text.trim()) return { refused: HttpResponse.json({ error: "empty policy" }, { status: 400 }) };
  let policy: unknown;
  try {
    policy = JSON.parse(text);
  } catch (e) {
    return { refused: HttpResponse.json({ error: `policy parse: ${(e as Error).message}` }, { status: 400 }) };
  }
  if (!isObject(policy)) return { refused: HttpResponse.json({ error: "policy parse: not a mapping" }, { status: 400 }) };
  const issues = issuesOf(policy);
  if (issues.some((i) => i.severity === "error")) return { refused: validationFailed(issues) };
  const masked = dryRun ? [] : maskedFields(policy);
  if (masked.length > 0) return { refused: maskedRefused(masked) };
  return { text, policy };
}

/** Puts a policy in force and records the revision. Returns the write's answer. */
function install(next: Doc, actor: string, message: string, text?: string) {
  const changed = changedPaths(state.policy, next);
  state.policy = clone(next);
  state.writes++;
  const revision: MockRevision = { id: newId("rev"), timestamp: now(), author: actor, message, yaml: text ?? JSON.stringify(next, null, 2) };
  state.revisions.push(revision);
  return { changed, revision_id: revision.id };
}

function settled(p: MockProposal) {
  if (p.status === "open") return null;
  return HttpResponse.json(
    { error: `this proposal is already ${p.status} and cannot be approved or rejected again`, code: "proposal_closed", status: p.status },
    { status: 409 },
  );
}

export const policyHandlers = [
  http.get(`${API}/policies`, ({ request }) => {
    const who = callerOf(request);
    if (!who) return unauthorized();
    const { doc, masked } = readable(who, state.policy);
    return HttpResponse.json([doc], { headers: masked ? { "X-Tamga-Masked": "notify.webhook" } : undefined });
  }),

  http.post(`${API}/policies/reload`, ({ request }) =>
    callerOf(request) ? HttpResponse.json({ ok: true, name: state.policy.name, changed: [] }) : unauthorized(),
  ),

  // A dry run checks the limits only: it does not refuse the mask, the write does.
  http.post(`${API}/policies/validate`, async ({ request }) => {
    if (!callerOf(request)) return unauthorized();
    const read = await readPolicy(request, { dryRun: true });
    if ("refused" in read) return read.refused;
    return HttpResponse.json({ valid: true, warnings: issuesOf(read.policy).filter((i) => i.severity === "warning") });
  }),

  http.put(`${API}/policies`, async ({ request }) => {
    const who = callerOf(request);
    if (!who) return unauthorized();
    if (dualControlOn()) return dualControlRefused(who);
    const read = await readPolicy(request);
    if ("refused" in read) return read.refused;
    const refused = privilegeRefused(who, read.policy);
    if (refused) return refused;
    if (state.readOnly) return readOnlyRefused();
    const res = install(read.policy, who.actor, "direct write (PUT /policies)", read.text);
    return HttpResponse.json({ ok: true, name: state.policy.name, version: state.policy.version, ...res });
  }),

  http.get(`${API}/policies/history`, ({ request }) => {
    const who = callerOf(request);
    return who ? withText(who, state.revisions) : unauthorized();
  }),

  http.get(`${API}/policies/revisions/:id`, ({ request, params }) => {
    const who = callerOf(request);
    if (!who) return unauthorized();
    const rev = state.revisions.find((r) => r.id === params.id);
    return rev ? withText(who, rev) : HttpResponse.json({ error: "revision not found" }, { status: 404 });
  }),

  // The one policy route the analyst role is kept out of altogether.
  http.post(`${API}/policies/rollback/:id`, ({ request, params }) => {
    const who = callerOf(request);
    if (!who) return unauthorized();
    if (who.role !== "admin") return HttpResponse.json({ error: `role ${who.role} lacks policies:admin` }, { status: 403 });
    const rev = state.revisions.find((r) => r.id === params.id);
    if (!rev) return HttpResponse.json({ error: "revision not found" }, { status: 404 });
    if (dualControlOn()) return dualControlRefused(who);
    install(JSON.parse(rev.yaml) as Doc, who.actor, `rollback to ${rev.id}`, rev.yaml);
    return HttpResponse.json({ ok: true, revision: rev.id });
  }),

  // ── Proposals ─────────────────────────────────────────────────────────
  http.get(`${API}/policies/proposals`, ({ request }) => {
    const who = callerOf(request);
    return who ? withText(who, state.proposals) : unauthorized();
  }),

  // Open under dual control too: it is the way in. The author is the caller; a name in the body is not read.
  http.post(`${API}/policies/proposals`, async ({ request }) => {
    const who = callerOf(request);
    if (!who) return unauthorized();
    const body = (await request.clone().json().catch(() => ({}))) as { message?: string };
    const read = await readPolicy(request);
    if ("refused" in read) return read.refused;
    const refused = privilegeRefused(who, read.policy);
    if (refused) return refused;
    const proposal = policyMock.addProposal({ author: who.actor, yaml: read.text, message: body.message ?? "" });
    return withText(who, proposal, { status: 201 });
  }),

  http.post(`${API}/policies/proposals/:id/approve`, ({ request, params }) => {
    const who = callerOf(request);
    if (!who) return unauthorized();
    const prop = state.proposals.find((p) => p.id === params.id);
    if (!prop) return HttpResponse.json({ error: "proposal not found" }, { status: 404 });
    const closed = settled(prop);
    if (closed) return closed;
    if (dualControlOn() && (who.actor === prop.author || sharedActor(who.actor))) {
      return HttpResponse.json(
        {
          error:
            "dual control required: the author of a proposal cannot approve it" +
            (sharedActor(who.actor) ? '. Every caller that uses the static admin key is the same actor ("admin-key"), so dual control cannot be satisfied this way.' : ""),
          code: "self_approval",
        },
        { status: 403 },
      );
    }
    if (state.writes > (state.writtenAt.get(prop.id) ?? 0)) {
      return HttpResponse.json(
        {
          error:
            "the policy changed after this proposal was written, so approving it would undo that change. " +
            "Reject it and open a new proposal against the current policy.",
          code: "proposal_stale",
        },
        { status: 409 },
      );
    }
    const next = JSON.parse(prop.yaml) as Doc;
    const issues = issuesOf(next);
    if (issues.some((i) => i.severity === "error")) return validationFailed(issues);
    const masked = maskedFields(next);
    if (masked.length > 0) return maskedRefused(masked);
    const refused = privilegeRefused(who, next);
    if (refused) return refused;
    if (state.readOnly) return readOnlyRefused();
    const res = install(next, who.actor, `approve proposal ${prop.id}`, prop.yaml);
    prop.status = "approved";
    prop.approved_by = who.actor;
    prop.approved_at = now();
    return HttpResponse.json({ ok: true, ...res });
  }),

  http.post(`${API}/policies/proposals/:id/reject`, ({ request, params }) => {
    const who = callerOf(request);
    if (!who) return unauthorized();
    const prop = state.proposals.find((p) => p.id === params.id);
    if (!prop) return HttpResponse.json({ error: "proposal not found" }, { status: 404 });
    const closed = settled(prop);
    if (closed) return closed;
    // Whoever could not approve it for lack of the admin role cannot turn it down either; its author can withdraw it.
    if (who.actor !== prop.author) {
      const refused = privilegeRefused(who, JSON.parse(prop.yaml) as Doc);
      if (refused) return refused;
    }
    prop.status = "rejected";
    prop.rejected_by = who.actor;
    prop.rejected_at = now();
    return withText(who, prop);
  }),

  // ── Custom entities: each change rewrites the policy, so it is a direct write ──
  http.get(`${API}/policies/custom-entities`, ({ request }) => {
    if (!callerOf(request)) return unauthorized();
    const items = (state.policy.custom_entities as Doc[] | null) ?? [];
    return HttpResponse.json({ items, total: items.length });
  }),

  http.post(`${API}/policies/custom-entities`, async ({ request }) => {
    const who = callerOf(request);
    if (!who) return unauthorized();
    const entity = (await request.json().catch(() => ({}))) as Doc;
    if (!entity.name || !entity.pattern) return HttpResponse.json({ error: "name and pattern are required" }, { status: 400 });
    if (dualControlOn()) return dualControlRefused(who);
    const items = (state.policy.custom_entities as Doc[] | null) ?? [];
    if (items.some((e) => e.name === entity.name)) {
      return HttpResponse.json({ error: "entity with this name already exists" }, { status: 409 });
    }
    if (state.readOnly) return readOnlyRefused();
    install({ ...state.policy, custom_entities: [...items, entity] }, who.actor, `add custom entity ${String(entity.name)}`);
    return HttpResponse.json({ ok: true, entity }, { status: 201 });
  }),

  http.delete(`${API}/policies/custom-entities/:name`, ({ request, params }) => {
    const who = callerOf(request);
    if (!who) return unauthorized();
    if (dualControlOn()) return dualControlRefused(who);
    const items = (state.policy.custom_entities as Doc[] | null) ?? [];
    if (!items.some((e) => e.name === params.name)) return HttpResponse.json({ error: "entity not found" }, { status: 404 });
    if (state.readOnly) return readOnlyRefused();
    install({ ...state.policy, custom_entities: items.filter((e) => e.name !== params.name) }, who.actor, `delete custom entity ${String(params.name)}`);
    return HttpResponse.json({ ok: true });
  }),
];
