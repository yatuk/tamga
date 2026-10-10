"use client";

import { useMemo, type ReactNode } from "react";
import NextLink from "next/link";
import {
  Button,
  DataTableSkeleton,
  IconButton,
  Link,
  Select,
  SelectItem,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableHeader,
  TableRow,
  TextArea,
  TextInput,
  Toggle,
} from "@carbon/react";
import { Edit } from "@carbon/icons-react";
import { counted } from "@/components/console/names";
import { DataNotice, PageHead, Remove, Section } from "@/components/console/page";
import { useAdminKey } from "@/hooks/useAdminKey";
import type { PatternSeverity } from "@/lib/api";
import { EMPTY_DRAFT, type Draft } from "./_constants";
import { usePatternsPage } from "./usePatternsPage";

const KINDS: Record<string, string> = { regex: "Regular expression", literal: "Literal text" };
const SEVERITIES: PatternSeverity[] = ["low", "medium", "high", "critical"];
const cap = (s: string) => s[0].toUpperCase() + s.slice(1);

/** The sample text with every match marked, and how many there were. */
function marked(sample: string, draft: Draft, regex: RegExp | "invalid" | null): { count: number; parts: ReactNode[] } | null {
  if (!sample || !draft.pattern || regex === "invalid") return null;
  const found: [number, number][] = [];
  if (draft.kind === "regex" && regex) {
    for (const m of sample.matchAll(regex)) {
      if (m[0].length === 0) continue;
      found.push([m.index, m.index + m[0].length]);
    }
  } else if (draft.kind === "literal") {
    const hay = sample.toLowerCase();
    const needle = draft.pattern.toLowerCase();
    for (let i = hay.indexOf(needle); i >= 0; i = hay.indexOf(needle, i + needle.length)) found.push([i, i + needle.length]);
  }
  const parts: ReactNode[] = [];
  let at = 0;
  found.forEach(([from, to], i) => {
    if (from > at) parts.push(sample.slice(at, from));
    parts.push(<mark key={i}>{sample.slice(from, to)}</mark>);
    at = to;
  });
  if (at < sample.length) parts.push(sample.slice(at));
  return { count: found.length, parts };
}

export default function PatternsPage() {
  const [adminKey] = useAdminKey();
  const m = usePatternsPage();
  const { draft, setDraft } = m;
  const editing = Boolean(draft.id);
  const invalid = m.compiledRegex === "invalid";
  const ready = draft.name.trim() !== "" && draft.pattern.trim() !== "" && !invalid;
  const pending = m.createMut.isPending || m.updateMut.isPending;
  const enabled = m.items.filter((p) => p.enabled).length;
  const result = useMemo(() => marked(m.testInput, draft, m.compiledRegex), [m.testInput, draft, m.compiledRegex]);

  return (
    <>
      <PageHead
        title="Patterns"
        lede="Your own regular expressions and literal strings for the scanners to look for, on top of what they already detect."
      />

      <DataNotice adminKey={adminKey} error={m.error} what="the patterns" />

      <div className="t-hunt" style={{ gridTemplateColumns: "minmax(0, 1fr) 24rem" }}>
        <Section id="list" title="Your patterns" note={m.items.length > 0 ? `${enabled} of ${m.items.length} switched on` : undefined}>
          {m.isLoading ? (
            <DataTableSkeleton columnCount={5} rowCount={4} showHeader={false} showToolbar={false} />
          ) : !adminKey || m.error ? (
            <p className="t-empty">Your patterns are listed here once the proxy has been read.</p>
          ) : m.items.length === 0 ? (
            <p className="t-empty">None yet. Add one with the form to catch a value the built-in scanners do not know, such as a project codename.</p>
          ) : (
            <TableContainer className="t-table">
              <Table size="md" aria-label="Your patterns">
                <TableHead>
                  <TableRow>
                    <TableHeader>Name</TableHeader>
                    <TableHeader>Looks for</TableHeader>
                    <TableHeader>Severity</TableHeader>
                    <TableHeader>On</TableHeader>
                    <TableHeader>
                      <span className="cds--visually-hidden">Edit or delete</span>
                    </TableHeader>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {m.items.map((p) => (
                    <TableRow key={p.id} data-selected={p.id === draft.id}>
                      {/* Both are cut short, so that the switch and the row's buttons stay in view beside the form. */}
                      <TableCell className="t-clip" title={p.name}>
                        {p.name}
                      </TableCell>
                      <TableCell className="t-clip" title={p.pattern}>
                        <span className="t-data">{p.pattern}</span>
                        <span className="t-note t-under">{KINDS[p.kind] ?? p.kind}</span>
                      </TableCell>
                      <TableCell>{cap(p.severity)}</TableCell>
                      <TableCell>
                        <Toggle
                          id={`on-${p.id}`}
                          size="sm"
                          hideLabel
                          aria-label={`${p.name} is switched on`}
                          toggled={p.enabled}
                          onToggle={() =>
                            m.updateMut.mutate({
                              id: p.id,
                              d: { id: p.id, name: p.name, kind: p.kind, pattern: p.pattern, severity: p.severity, enabled: !p.enabled },
                            })
                          }
                        />
                      </TableCell>
                      <TableCell>
                        <span className="t-rowactions">
                          <IconButton kind="ghost" size="sm" label={`Edit ${p.name}`} align="left" onClick={() => m.editPattern(p)}>
                            <Edit />
                          </IconButton>
                          <Remove label={`Delete ${p.name}`} onConfirm={() => m.deleteMut.mutate(p.id)} />
                        </span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          )}
          <p className="t-footnote">
            A pattern here only detects. To give a match its own action, define it as a custom entity in the policy:{" "}
            <Link as={NextLink} href="/dashboard/policies?tab=entities">
              custom entities
            </Link>
            .
          </p>
        </Section>

        <Section id="form" title={editing ? `Editing ${draft.name || "a pattern"}` : "New pattern"}>
          <form
            className="t-form"
            onSubmit={(e) => {
              e.preventDefault();
              m.onSubmit();
            }}
          >
            <TextInput
              id="pattern-name"
              labelText="Name"
              placeholder="project-codename"
              autoComplete="off"
              spellCheck={false}
              value={draft.name}
              onChange={(e) => setDraft({ ...draft, name: e.target.value })}
            />
            <div className="t-pair" data-wide="first">
              <Select id="pattern-kind" labelText="Kind" value={draft.kind} onChange={(e) => m.setDraftKind(e.target.value as Draft["kind"])}>
                <SelectItem value="regex" text={KINDS.regex} />
                <SelectItem value="literal" text={KINDS.literal} />
              </Select>
              <Select
                id="pattern-severity"
                labelText="Severity"
                value={draft.severity}
                onChange={(e) => setDraft({ ...draft, severity: e.target.value as PatternSeverity })}
              >
                {SEVERITIES.map((s) => (
                  <SelectItem key={s} value={s} text={cap(s)} />
                ))}
              </Select>
            </div>
            <TextArea
              id="pattern-value"
              data-code=""
              labelText={KINDS[draft.kind]}
              rows={3}
              spellCheck={false}
              placeholder={draft.kind === "regex" ? "(?i)project-\\w+" : "ACME-SECRET"}
              value={draft.pattern}
              onChange={(e) => setDraft({ ...draft, pattern: e.target.value })}
              invalid={invalid}
              invalidText="This is not a valid regular expression."
            />
            <Toggle
              id="pattern-enabled"
              size="sm"
              labelText="Scanners use it"
              labelA="Off"
              labelB="On"
              toggled={draft.enabled}
              onToggle={(on) => setDraft({ ...draft, enabled: on })}
            />

            {/* No test button: the sample is checked as it is typed. */}
            <TextArea
              id="pattern-test"
              labelText="Try it on some text"
              rows={3}
              spellCheck={false}
              placeholder="Paste text the pattern should catch"
              value={m.testInput}
              onChange={(e) => m.setTestInput(e.target.value)}
            />
            {result ? (
              <div aria-live="polite">
                <p className="t-note" style={{ marginBottom: "0.4rem" }}>
                  {result.count === 0 ? "It catches nothing in this text." : `It catches ${counted(result.count, "piece")} of this text.`}
                </p>
                <div className="t-sample">{result.parts}</div>
              </div>
            ) : null}

            <div className="t-buttons">
              {editing ? (
                <Button type="button" kind="ghost" size="md" onClick={() => setDraft(EMPTY_DRAFT)}>
                  Cancel
                </Button>
              ) : null}
              <Button type="submit" kind="primary" size="md" disabled={pending || !ready}>
                {pending ? "Saving" : editing ? "Save changes" : "Add pattern"}
              </Button>
            </div>
          </form>
        </Section>
      </div>
    </>
  );
}
