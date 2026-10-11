"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Button,
  DataTableSkeleton,
  Select,
  SelectItem,
  SkeletonText,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow,
  TextArea,
  TextInput,
} from "@carbon/react";
import { Play } from "@carbon/icons-react";
import { compare, Diff, diffWords, sameBut } from "@/components/console/diff";
import { ActionMark } from "@/components/console/events";
import { clockTime, dayLabel } from "@/components/console/names";
import { Remove, Section } from "@/components/console/page";
import { Verdict } from "@/components/console/verdict";
import { api, type PolicyRevision, type PolicySimulateResult } from "@/lib/api";
import { API_BASE, authHeaders } from "@/lib/api/fetch-core";
import type { CustomEntity } from "@/lib/api/types-core";
import { toast } from "@/lib/toast";
import { NOTIFY_MASK } from "./policyUtils";
import { explainRefusal, RefusalNotice, type Refusal } from "./refusal";

const cap = (s: string) => (s ? s[0].toUpperCase() + s.slice(1) : s);
const when = (iso?: string) => (iso ? `${dayLabel(iso)}, ${clockTime(iso)}` : "Not recorded");

/** What the draft changes against the policy the proxy is running. */
export function Changes({ original, draft }: { original: string; draft: string }) {
  const compared = useMemo(() => compare(original, draft), [original, draft]);
  const { lines } = compared;
  // A draft written with an admin key and read against the policy as a lower
  // key sees it (or the other way round) differs where the addresses are masked.
  const maskDiffers = lines.some((l) => l.type !== " " && l.text.includes(NOTIFY_MASK));
  return (
    <Section id="changes" title="The draft against the running policy" note={lines.length > 0 ? diffWords(lines) : undefined}>
      {maskDiffers ? (
        <p className="t-note" style={{ marginTop: "0.75rem" }}>
          A line that holds {NOTIFY_MASK} is an alert address the proxy hid from a key that is not an admin key. It differs because the draft
          and the running policy were read with different keys, not because of an edit.
        </p>
      ) : null}
      {lines.length === 0 ? (
        <p className="t-empty">The draft is the same as the policy the proxy is running{sameBut(compared)}.</p>
      ) : (
        <div style={{ marginTop: "1rem" }}>
          <Diff lines={lines} />
        </div>
      )}
    </Section>
  );
}

/** Runs a prompt against the draft without saving it or calling a provider. */
export function TryIt({
  sample,
  onSample,
  running,
  onRun,
  result,
}: {
  sample: string;
  onSample: (text: string) => void;
  running: boolean;
  onRun: () => void;
  result: PolicySimulateResult | null;
}) {
  return (
    <div className="t-columns" data-even="true" style={{ marginTop: 0 }}>
      <Section id="try" title="A prompt against the draft" note="Nothing is saved and nothing reaches a provider.">
        <form
          className="t-form"
          onSubmit={(e) => {
            e.preventDefault();
            onRun();
          }}
        >
          <TextArea
            id="policy-sample"
            labelText="Prompt"
            hideLabel
            rows={8}
            spellCheck={false}
            placeholder="Paste a prompt to test"
            value={sample}
            onChange={(e) => onSample(e.target.value)}
          />
          <div>
            <Button type="submit" kind="primary" size="md" renderIcon={Play} disabled={running || !sample.trim()}>
              {running ? "Running" : "Run it against the draft"}
            </Button>
          </div>
        </form>
      </Section>
      <Section id="verdict" title="What the draft would do" note={result ? `${result.policy_name}, version ${result.policy_version}` : undefined}>
        <Verdict result={result} prompt={sample} running={running} />
      </Section>
    </div>
  );
}

const savedAt = (rev: PolicyRevision) => rev.timestamp || rev.created_at;
const revisionName = (rev: PolicyRevision) => `${when(savedAt(rev))}  ${rev.id.slice(0, 8)}${rev.message ? `  ${rev.message}` : ""}`;

/** Compares two saved revisions of the policy. */
export function History({ adminKey }: { adminKey: string }) {
  const list = useQuery({
    queryKey: ["tamga-policy-history", adminKey],
    queryFn: () => api.listPolicyRevisions(adminKey),
    enabled: !!adminKey,
    staleTime: 30_000,
  });
  const sorted = useMemo(
    () =>
      [...(list.data || [])].sort(
        (a, b) => new Date(savedAt(b) || 0).getTime() - new Date(savedAt(a) || 0).getTime(),
      ),
    [list.data],
  );

  const [fromId, setFromId] = useState("");
  const [toId, setToId] = useState("");
  // Start with the newest revision against the one before it.
  const from = fromId || sorted[1]?.id || "";
  const to = toId || sorted[0]?.id || "";

  const left = useQuery({
    queryKey: ["tamga-policy-rev", adminKey, from],
    queryFn: () => api.getPolicyRevision(adminKey, from),
    enabled: !!adminKey && !!from,
    staleTime: 60_000,
  });
  const right = useQuery({
    queryKey: ["tamga-policy-rev", adminKey, to],
    queryFn: () => api.getPolicyRevision(adminKey, to),
    enabled: !!adminKey && !!to,
    staleTime: 60_000,
  });
  const compared = useMemo(
    () => (left.data && right.data ? compare(left.data.yaml || "", right.data.yaml || "") : null),
    [left.data, right.data],
  );
  const lines = compared?.lines ?? [];

  if (list.isLoading) return <SkeletonText paragraph lineCount={4} />;
  if (list.isError) return <p className="t-empty">The saved revisions could not be read.</p>;
  if (sorted.length < 2) {
    return (
      <p className="t-empty">
        {sorted.length === 0 ? "No revision has been saved yet." : "Only one revision has been saved."} Each save records one, and two are
        needed to compare.
      </p>
    );
  }

  const pick = (id: string, label: string, value: string, onChange: (id: string) => void, rev?: PolicyRevision) => (
    <div>
      <Select id={id} labelText={label} value={value} onChange={(e) => onChange(e.target.value)}>
        {sorted.map((r) => (
          <SelectItem key={r.id} value={r.id} text={revisionName(r)} />
        ))}
      </Select>
      <p className="t-note" style={{ marginTop: "0.5rem" }}>
        {rev ? `Saved by ${rev.author || "an unknown author"}${rev.message ? `: ${rev.message}` : "."}` : "Reading"}
      </p>
    </div>
  );

  return (
    <>
      <div className="t-columns" data-even="true" style={{ marginTop: 0 }}>
        {pick("policy-rev-from", "From", from, setFromId, left.data)}
        {pick("policy-rev-to", "To", to, setToId, right.data)}
      </div>
      <Section
        id="rev-changes"
        title="What changed between them"
        note={lines.length > 0 ? diffWords(lines) : `${sorted.length} revisions saved`}
        style={{ marginTop: "2.5rem" }}
      >
        {!compared ? (
          <SkeletonText paragraph lineCount={6} />
        ) : lines.length === 0 ? (
          <p className="t-empty">These two revisions are the same{sameBut(compared)}.</p>
        ) : (
          <div style={{ marginTop: "1rem" }}>
            <Diff lines={lines} />
          </div>
        )}
      </Section>
    </>
  );
}

const SEVERITIES = ["critical", "high", "medium", "low"] as const;
const ACTIONS = ["block", "strip", "redact", "warn", "log"] as const;
const ACTION_CHOICES: Record<(typeof ACTIONS)[number], string> = {
  block: "Stop the request",
  strip: "Remove the message part it is in",
  redact: "Redact the match",
  warn: "Warn",
  log: "Only log",
};

type EntityValues = { name: string; pattern: string; description: string; severity: (typeof SEVERITIES)[number]; action: (typeof ACTIONS)[number] };
const EMPTY: EntityValues = { name: "", pattern: "", description: "", severity: "medium", action: "log" };

function validRegex(pattern: string) {
  try {
    // Go takes flags in front of the expression, "(?i)". The browser does not.
    new RegExp(pattern.replace(/^\(\?[ims]+\)/, ""));
    return true;
  } catch {
    return false;
  }
}

/** Organisation-specific patterns with their own action: project names, internal IDs, customer codes. */
export function Entities({
  adminKey,
  readOnly = false,
  dualControl = false,
}: {
  adminKey: string;
  readOnly?: boolean;
  /** The proxy requires a second person: an entity is then changed in the document and proposed. */
  dualControl?: boolean;
}) {
  const qc = useQueryClient();
  // The form and the list each say their own refusal, where the action was taken.
  const [addRefusal, setAddRefusal] = useState<Refusal | null>(null);
  const [deleteRefusal, setDeleteRefusal] = useState<Refusal | null>(null);
  const locked = readOnly || dualControl;
  const [values, setValues] = useState<EntityValues>(EMPTY);
  const [submitted, setSubmitted] = useState(false);
  const set = <K extends keyof EntityValues>(key: K, value: EntityValues[K]) => setValues((v) => ({ ...v, [key]: value }));

  const nameError = !values.name.trim() ? "Give the entity a name." : "";
  const patternError = !values.pattern.trim()
    ? "Enter a regular expression."
    : validRegex(values.pattern)
      ? ""
      : "This is not a valid regular expression.";

  const list = useQuery({
    queryKey: ["custom-entities", adminKey],
    queryFn: () => api.listCustomEntities(adminKey),
    enabled: !!adminKey,
  });
  const create = useMutation({
    mutationFn: (entity: CustomEntity) => api.createCustomEntity(adminKey, entity),
    onSuccess: (_data, entity) => {
      void qc.invalidateQueries({ queryKey: ["custom-entities", adminKey] });
      setValues(EMPTY);
      setSubmitted(false);
      toast.success("Entity added", entity.name);
    },
    onError: (e) => setAddRefusal(explainRefusal(e, "entity-add")),
  });
  const remove = useMutation({
    mutationFn: (name: string) => api.deleteCustomEntity(adminKey, name),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["custom-entities", adminKey] }),
    onError: (e) => setDeleteRefusal(explainRefusal(e, "entity-delete")),
  });
  const items = list.data?.items ?? [];

  return (
    <div className="t-hunt" data-form="true" style={{ marginTop: 0 }}>
      <Section id="entities" title="Custom entities" note={items.length > 0 ? `${items.length} defined` : undefined}>
        {dualControl ? (
          <p className="t-note" style={{ marginTop: "1rem" }}>
            This proxy requires a second person for every policy change, so entities are not added or deleted here. Change{" "}
            <span className="t-data">custom_entities</span> in the document under Editor and propose it.
          </p>
        ) : null}
        {deleteRefusal ? <RefusalNotice refusal={deleteRefusal} onClose={() => setDeleteRefusal(null)} /> : null}
        {list.isLoading ? (
          <DataTableSkeleton columnCount={4} rowCount={3} showHeader={false} showToolbar={false} />
        ) : items.length === 0 ? (
          <p className="t-empty">None yet. Add one to catch a value that only your organisation has, and say what to do with it.</p>
        ) : (
          <TableContainer className="t-table">
            <Table size="md" aria-label="Custom entities">
              <TableHead>
                <TableRow>
                  <TableHeader>Name</TableHeader>
                  <TableHeader>Looks for</TableHeader>
                  <TableHeader>Severity</TableHeader>
                  <TableHeader>On a match</TableHeader>
                  <TableHeader>
                    <span className="cds--visually-hidden">Delete</span>
                  </TableHeader>
                </TableRow>
              </TableHead>
              <TableBody>
                {items.map((ce) => (
                  <TableRow key={ce.name}>
                    <TableCell>
                      {ce.name}
                      {ce.description ? <span className="t-note t-under">{ce.description}</span> : null}
                    </TableCell>
                    <TableCell className="t-clip" title={ce.pattern}>
                      <span className="t-data">{ce.pattern}</span>
                    </TableCell>
                    <TableCell>{cap(ce.severity)}</TableCell>
                    <TableCell>
                      <ActionMark action={ce.action.toUpperCase()} />
                    </TableCell>
                    <TableCell>
                      <span className="t-rowactions">
                        <Remove
                          label={`Delete ${ce.name}`}
                          verb="Delete the entity"
                          onConfirm={() => {
                            setDeleteRefusal(null);
                            remove.mutate(ce.name);
                          }}
                          disabled={remove.isPending || locked}
                        />
                      </span>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        )}
      </Section>

      <Section id="entity-form" title="New entity">
        <form
          noValidate
          className="t-form"
          onSubmit={(e) => {
            e.preventDefault();
            setSubmitted(true);
            setAddRefusal(null);
            if (!nameError && !patternError) create.mutate({ ...values, confidence: 0.85 });
          }}
        >
          <TextInput
            id="entity-name"
            labelText="Name"
            placeholder="project_mercury"
            autoComplete="off"
            spellCheck={false}
            value={values.name}
            onChange={(e) => set("name", e.target.value)}
            invalid={submitted && !!nameError}
            invalidText={nameError}
          />
          <TextInput
            id="entity-pattern"
            labelText="Regular expression"
            placeholder="Project[ -]?Mercury"
            autoComplete="off"
            spellCheck={false}
            className="t-data"
            value={values.pattern}
            onChange={(e) => set("pattern", e.target.value)}
            invalid={submitted && !!patternError}
            invalidText={patternError}
          />
          <div className="t-pair">
            <Select id="entity-severity" labelText="Severity" value={values.severity} onChange={(e) => set("severity", e.target.value as EntityValues["severity"])}>
              {SEVERITIES.map((s) => (
                <SelectItem key={s} value={s} text={cap(s)} />
              ))}
            </Select>
            <Select id="entity-action" labelText="On a match" value={values.action} onChange={(e) => set("action", e.target.value as EntityValues["action"])}>
              {ACTIONS.map((a) => (
                <SelectItem key={a} value={a} text={ACTION_CHOICES[a]} />
              ))}
            </Select>
          </div>
          <TextInput
            id="entity-description"
            labelText="Note (optional)"
            placeholder="Confidential project code name"
            autoComplete="off"
            value={values.description}
            onChange={(e) => set("description", e.target.value)}
          />
          <div className="t-buttons">
            <Button type="submit" kind="primary" size="md" disabled={create.isPending || locked}>
              {create.isPending ? "Adding" : "Add entity"}
            </Button>
          </div>
        </form>
        {addRefusal ? <RefusalNotice refusal={addRefusal} onClose={() => setAddRefusal(null)} /> : null}
      </Section>
    </div>
  );
}

interface Competitor {
  name: string;
  patterns: string[];
  severity: string;
  action: string;
  description: string;
  enabled: boolean;
}

/** The `competitors:` list of a policy document, when the API sends the document and not the list. */
function parseCompetitors(yaml: string): Competitor[] {
  try {
    const block = yaml.match(/^competitors:\s*\n([\s\S]*?)(?=\n\S|$)/m);
    if (!block) return [];
    const out: Competitor[] = [];
    for (const entry of block[1].split(/\n  - /).filter(Boolean)) {
      const name = entry.match(/^\s*name:\s*"?(.+?)"?\s*$/m)?.[1] ?? "";
      if (!name) continue;
      const patterns: string[] = [];
      const list = entry.match(/^\s*patterns:\s*\n([\s\S]*?)(?=\n  \w|\n\s*$|$)/m);
      if (list) {
        for (const line of list[1].split("\n")) {
          const p = line.replace(/^\s*-\s*"?(.+?)"?\s*$/, "$1").trim();
          if (p) patterns.push(p);
        }
      }
      out.push({
        name,
        patterns,
        enabled: !entry.includes("enabled: false"),
        severity: entry.match(/^\s*severity:\s*(\w+)/m)?.[1] ?? "low",
        // No action of its own: the competitor rule or the default action decides.
        action: entry.match(/^\s*action:\s*(\w+)/m)?.[1] ?? "",
        description: entry.match(/^\s*description:\s*"?(.+?)"?\s*$/m)?.[1] ?? "",
      });
    }
    return out;
  } catch {
    return [];
  }
}

/** Competitor names the running policy watches for. Read-only: they are edited in the policy document. */
export function Competitors({ adminKey }: { adminKey: string }) {
  const list = useQuery({
    queryKey: ["policy-competitors", adminKey],
    queryFn: async () => {
      const r = await fetch(`${API_BASE}/api/v1/policies`, { headers: authHeaders(adminKey) });
      if (!r.ok) throw new Error(`The policy request failed with status ${r.status}.`);
      // The proxy answers with a list that holds the one running policy.
      const body = (await r.json()) as unknown;
      const json = ((Array.isArray(body) ? body[0] : body) ?? {}) as { yaml?: string; competitors?: Competitor[] };
      return json.competitors && json.competitors.length > 0 ? json.competitors : parseCompetitors(json.yaml ?? "");
    },
    enabled: !!adminKey,
    staleTime: 30_000,
  });
  const items = list.data ?? [];
  const on = items.filter((c) => c.enabled).length;

  return (
    <Section id="competitors" title="Competitor mentions" note={items.length > 0 ? `${on} of ${items.length} switched on` : "Read from the running policy"}>
      {list.isLoading ? (
        <DataTableSkeleton columnCount={4} rowCount={3} showHeader={false} showToolbar={false} />
      ) : list.isError ? (
        <p className="t-empty">The running policy could not be read.</p>
      ) : items.length === 0 ? (
        <p className="t-empty">
          The policy names no competitors. Add a <code>competitors</code> list to the document under Editor to have their mentions caught.
        </p>
      ) : (
        <TableContainer className="t-table">
          <Table size="md" aria-label="Competitors">
            <TableHead>
              <TableRow>
                <TableHeader>Name</TableHeader>
                <TableHeader>Looks for</TableHeader>
                <TableHeader>Severity</TableHeader>
                <TableHeader>On a match</TableHeader>
                <TableHeader>In use</TableHeader>
              </TableRow>
            </TableHead>
            <TableBody>
              {items.map((c) => (
                <TableRow key={c.name}>
                  <TableCell>
                    {c.name}
                    {c.description ? <span className="t-note t-under">{c.description}</span> : null}
                  </TableCell>
                  <TableCell style={{ whiteSpace: "normal" }}>
                    <span className="t-data">{c.patterns.join(", ")}</span>
                  </TableCell>
                  <TableCell>{cap(c.severity)}</TableCell>
                  <TableCell>
                    {c.action ? <ActionMark action={c.action.toUpperCase()} /> : <span className="t-note">Set by the rule</span>}
                  </TableCell>
                  <TableCell>{c.enabled ? "Yes" : "Switched off"}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </Section>
  );
}
