"use client";

import type { ReactNode } from "react";
import { AnimatePresence, motion } from "motion/react";
import { Table, TableBody, TableCell, TableContainer, TableHead, TableHeader, TableRow } from "@carbon/react";
import type { PolicySimulateResult } from "@/lib/api";
import { ActionMark } from "./events";
import { PRODUCTIVE } from "./motion";
import { ACTION_WORDS, counted, kindName, typeName } from "./names";

type Finding = PolicySimulateResult["findings"][number];

const upper = (s?: string) => (s || "PASS").toUpperCase();

/** The prompt with the text behind each stop or redaction marked where it sits. */
function markedPrompt(text: string, findings: Finding[]): ReactNode[] {
  const spans: { from: number; to: number; action: string }[] = [];
  for (const f of findings) {
    if (!f.match) continue;
    for (let at = text.indexOf(f.match); at >= 0; at = text.indexOf(f.match, at + 1)) {
      spans.push({ from: at, to: at + f.match.length, action: upper(f.action) });
    }
  }
  // Nothing to mark: the caller shows no sample at all.
  if (spans.length === 0) return [];
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
  return parts;
}

/**
 * What a policy would do with a prompt: the verdict as one word, the text
 * that caused it, and each finding. Every run replaces the verdict, and the
 * swap shows that it is a new answer.
 */
export function Verdict({ result, prompt, running }: { result: PolicySimulateResult | null; prompt: string; running: boolean }) {
  const acted = result?.findings.filter((f) => ["BLOCK", "REDACT", "STRIP"].includes(upper(f.action))) ?? [];
  const marks = markedPrompt(prompt, acted);

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

          {/* The proxy masks what it matched, so the text cannot always be found again in the prompt. */}
          {marks.length > 0 ? (
            <div className="t-sample" style={{ marginTop: "1.5rem", maxHeight: "14rem", overflow: "auto" }}>
              {marks}
            </div>
          ) : null}

          {result.findings.length > 0 ? (
            <TableContainer className="t-table" style={{ marginTop: "1.5rem" }}>
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
            </TableContainer>
          ) : null}
        </motion.div>
      )}
    </AnimatePresence>
  );
}
