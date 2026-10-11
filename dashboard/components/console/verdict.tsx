"use client";

import type { ReactNode } from "react";
import { AnimatePresence, motion } from "motion/react";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@carbon/react";
import { ScrollTable } from "./page";
import type { PolicySimulateResult } from "@/lib/api";
import { ActionMark } from "./events";
import { PRODUCTIVE } from "./motion";
import { ACTION_WORDS, counted, kindName, typeName } from "./names";

type Finding = PolicySimulateResult["findings"][number];

const upper = (s?: string) => (s || "PASS").toUpperCase();

/**
 * Where a finding sits in the text, as the proxy placed it: in UTF-16 code
 * units, which is how `text` is indexed. Undefined when the proxy gave no
 * place, or one that does not fit the text.
 */
function placed(text: string, f: Finding): { from: number; to: number } | undefined {
  const { start_utf16: from, end_utf16: to } = f;
  if (f.position !== "exact" || !Number.isInteger(from) || !Number.isInteger(to)) return undefined;
  if (from === undefined || to === undefined || from < 0 || from >= to || to > text.length) return undefined;
  return { from, to };
}

/**
 * The prompt with the text behind each stop or redaction marked where it
 * sits, and how many of the findings could not be placed in it.
 *
 * The proxy says where a finding is. One it says has no place (it was seen
 * only in a normalised form of the text, or the detector read the text as a
 * whole) is not marked, and is counted. A proxy older than the positions says
 * nothing: the match is then looked for in the text, which finds it only
 * when the proxy did not mask it.
 */
function markedPrompt(text: string, findings: Finding[]): { parts: ReactNode[]; unplaced: number } {
  const spans: { from: number; to: number; action: string }[] = [];
  let unplaced = 0;
  for (const f of findings) {
    if (f.position !== undefined) {
      const at = placed(text, f);
      if (at) spans.push({ ...at, action: upper(f.action) });
      else unplaced++;
      continue;
    }
    if (!f.match) continue;
    for (let at = text.indexOf(f.match); at >= 0; at = text.indexOf(f.match, at + 1)) {
      spans.push({ from: at, to: at + f.match.length, action: upper(f.action) });
    }
  }
  // Nothing to mark: the caller shows no sample at all.
  if (spans.length === 0) return { parts: [], unplaced };
  spans.sort((a, b) => a.from - b.from);
  const parts: ReactNode[] = [];
  let cursor = 0;
  spans.forEach((s, i) => {
    if (s.to <= cursor) return;
    const from = Math.max(s.from, cursor);
    if (from > cursor) parts.push(text.slice(cursor, from));
    parts.push(
      <mark key={i} data-action={s.action}>
        {text.slice(from, s.to)}
      </mark>,
    );
    cursor = s.to;
  });
  if (cursor < text.length) parts.push(text.slice(cursor));
  return { parts, unplaced };
}

/**
 * What a policy would do with a prompt: the verdict as one word, the text
 * that caused it, and each finding. Every run replaces the verdict, and the
 * swap shows that it is a new answer.
 */
export function Verdict({ result, prompt, running }: { result: PolicySimulateResult | null; prompt: string; running: boolean }) {
  const acted = result?.findings.filter((f) => ["BLOCK", "ROUTE", "REDACT", "STRIP"].includes(upper(f.action))) ?? [];
  // The text the verdict is about: what was sent, not what has been typed since.
  const { parts: marks, unplaced } = markedPrompt(result?.sample_text ?? prompt, acted);

  return (
    <AnimatePresence mode="wait" initial={false}>
      {!result ? (
        <motion.p key="none" className="t-empty" exit={{ opacity: 0 }} transition={{ duration: 0.12 }}>
          {running ? "Running." : "Run a prompt to see the verdict here."}
        </motion.p>
      ) : (
        <motion.div
          key={`${result.action}|${result.findings.length}|${running}`}
          initial={{ opacity: 0, y: 8 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0 }}
          transition={{ duration: 0.24, ease: PRODUCTIVE }}
        >
          <div className="t-figureline" style={{ paddingTop: "1.5rem" }}>
            <span className="t-figure t-verdict" data-action={upper(result.action)}>
              {ACTION_WORDS[upper(result.action)] ?? result.action}
            </span>
            <p>
              {result.findings.length === 0
                ? "Nothing was found. The prompt would go on unchanged."
                : `${counted(result.findings.length, "finding")}, ${acted.length} of them acted on.`}
            </p>
          </div>
          {/* A simulation sends nothing, so it names the pool and does not say whether the pool answers. */}
          {upper(result.action) === "ROUTE" ? (
            <p className="t-footnote">
              The request would be sent as written to {result.route_to ? <>the internal pool <span className="t-data">{result.route_to}</span></> : "an internal pool"}, and
              not to the provider. If that pool cannot take it, the request is stopped.
            </p>
          ) : null}

          {marks.length > 0 ? (
            <div className="t-sample" style={{ marginTop: "1.5rem", maxHeight: "14rem", overflow: "auto" }}>
              {marks}
            </div>
          ) : null}
          {/* A finding with no place in the text is in the table below, and is said not to be marked. */}
          {unplaced > 0 ? (
            <p className="t-footnote">
              {marks.length > 0
                ? `${counted(unplaced, "finding")} acted on ${unplaced === 1 ? "is" : "are"} not marked.`
                : `The ${unplaced === 1 ? "finding" : `${unplaced} findings`} acted on ${unplaced === 1 ? "is" : "are"} not marked in the text.`}{" "}
              {unplaced === 1 ? "It was" : "They were"} found in a normalised form of the text, or by a detector that reads it as a whole, so{" "}
              {unplaced === 1 ? "it has" : "they have"} no place in the text as it was sent.
            </p>
          ) : null}

          {result.findings.length > 0 ? (
            <ScrollTable className="t-table" style={{ marginTop: "1.5rem" }}>
              <Table size="sm" aria-label="Findings">
                <TableHead>
                  <TableRow>
                    <TableHeader>Finding</TableHeader>
                    <TableHeader>Kind</TableHeader>
                    <TableHeader>Action</TableHeader>
                    <TableHeader className="t-right">Confidence</TableHeader>
                    <TableHeader>Matched text</TableHeader>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {result.findings.map((f, i) => (
                    <TableRow key={i}>
                      <TableCell>{kindName(f.category)}</TableCell>
                      <TableCell>{typeName(f.type)}</TableCell>
                      <TableCell>
                        <ActionMark action={upper(f.action)} />
                      </TableCell>
                      <TableCell className="t-right">{Math.round((f.confidence || 0) * 100)}%</TableCell>
                      <TableCell className="t-clip" title={f.match}>
                        <span className="t-data">{f.match || "Not recorded"}</span>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </ScrollTable>
          ) : null}
        </motion.div>
      )}
    </AnimatePresence>
  );
}
