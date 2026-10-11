"use client";

import {
  Button,
  CodeSnippet,
  ContentSwitcher,
  ProgressBar,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  TextArea,
} from "@carbon/react";
import { Copy, Play, Upload } from "@carbon/icons-react";
import { ACTION_WORDS, counted } from "@/components/console/names";
import { PageHead, ScrollTable, Section } from "@/components/console/page";
import { Verdict } from "@/components/console/verdict";
import type { PolicySource } from "./_constants";
import { PLAYGROUND_SNIPPETS, type RedTeamRow } from "./playgroundData";
import { usePlaygroundPage } from "./usePlaygroundPage";

const SOURCES: { id: PolicySource; label: string; hint: string }[] = [
  { id: "active", label: "Running", hint: "The policy the proxy is enforcing now." },
  { id: "draft", label: "Draft", hint: "Your unsaved draft from the Policies editor." },
  { id: "upload", label: "Pasted", hint: "A policy you paste below, used only for this test." },
];

const OUTCOMES: Record<RedTeamRow["outcome"], { word: string; action?: string }> = {
  match: { word: "Caught" },
  tn: { word: "Rightly passed" },
  fp: { word: "False positive", action: "REDACT" },
  miss: { word: "Missed", action: "BLOCK" },
  error: { word: "Errored", action: "BLOCK" },
};

const percent = (n: number) => `${(n * 100).toFixed(1)}%`;
const upper = (s?: string) => (s || "PASS").toUpperCase();

export default function PlaygroundPage() {
  const p = usePlaygroundPage();
  const source = SOURCES.find((s) => s.id === p.policySource) ?? SOURCES[0];
  const sum = p.batchSummary;
  // With no document the proxy would quietly fall back to the running policy.
  const noPolicy = p.policySource !== "active" && !p.effectiveYaml.trim();

  return (
    <>
      <PageHead title="Playground" lede="Try a prompt against a policy. Nothing here reaches a provider or touches live traffic.">
        <Button kind="tertiary" size="sm" renderIcon={Copy} onClick={p.copyCurl}>
          Copy as cURL
        </Button>
        <Button kind="tertiary" size="sm" renderIcon={Copy} onClick={p.copyJson}>
          Copy request body
        </Button>
      </PageHead>

      <div className="t-columns" data-even="true">
        <Section id="prompt" title="Prompt" note={`${p.prompt.length.toLocaleString("en")} characters`}>
          <form
            className="t-form"
            onSubmit={(e) => {
              e.preventDefault();
              void p.runSimulate();
            }}
          >
            <TextArea
              id="playground-prompt"
              labelText="Prompt"
              hideLabel
              rows={11}
              spellCheck={false}
              placeholder="Type or paste a prompt to test"
              value={p.prompt}
              onChange={(e) => p.setPrompt(e.target.value)}
            />
            <div className="t-examples">
              <span className="t-note">Or start from one of these:</span>
              {PLAYGROUND_SNIPPETS.map((s) => (
                <Button key={s.id} type="button" kind="ghost" size="sm" onClick={() => p.setPrompt(s.text)}>
                  {s.label}
                </Button>
              ))}
            </div>
            <div>
              <Button type="submit" kind="primary" size="md" renderIcon={Play} disabled={p.running || noPolicy || !p.prompt.trim()}>
                {p.running ? "Running" : "Run it against the policy"}
              </Button>
            </div>
          </form>
        </Section>

        <Section id="policy" title="Policy" note={source.hint}>
          <div className="t-form">
            <ContentSwitcher
              size="sm"
              selectedIndex={SOURCES.indexOf(source)}
              onChange={({ index }) => p.setPolicySource(SOURCES[index ?? 0].id)}
              style={{ width: "18rem" }}
            >
              {SOURCES.map((s) => (
                <Switch key={s.id} name={s.id} text={s.label} />
              ))}
            </ContentSwitcher>
            {p.policySource === "upload" ? (
              <TextArea
                id="playground-policy"
                data-code=""
                labelText="Policy document"
                hideLabel
                rows={13}
                spellCheck={false}
                placeholder="Paste a policy document"
                value={p.uploadYaml}
                onChange={(e) => p.setUploadYaml(e.target.value)}
              />
            ) : p.effectiveYaml ? (
              <CodeSnippet type="multi" feedback="Copied" maxCollapsedNumberOfRows={15} wrapText>
                {p.effectiveYaml}
              </CodeSnippet>
            ) : (
              <p className="t-empty">
                {p.policySource === "draft"
                  ? "There is no draft. Edit the policy on the Policies page first."
                  : "The running policy could not be read. Check the admin key under Settings."}
              </p>
            )}
          </div>
        </Section>
      </div>

      <Section
        id="decision"
        title="What the policy would do"
        note={p.result ? `${p.result.policy_name}, version ${p.result.policy_version}` : undefined}
        style={{ marginTop: "3rem" }}
      >
        <Verdict result={p.result} prompt={p.prompt} running={p.running} />
      </Section>

      <Section
        id="batch"
        title="Batch test"
        note={`A labelled set of prompts, run against the ${source.label.toLowerCase()} policy`}
        style={{ marginTop: "3rem" }}
      >
        <input ref={p.fileInputRef} type="file" accept=".csv,text/csv" className="cds--visually-hidden" tabIndex={-1} aria-hidden onChange={p.onUploadCsv} />
        <div className="t-formrow">
          <Button kind="tertiary" size="sm" onClick={p.loadBundledSamples}>
            Load the bundled samples
          </Button>
          <Button kind="tertiary" size="sm" renderIcon={Upload} onClick={() => p.fileInputRef.current?.click()}>
            Upload a CSV
          </Button>
          <Button kind="primary" size="sm" renderIcon={Play} onClick={() => void p.runBatch()} disabled={p.batchRunning || noPolicy || p.batchSamples.length === 0}>
            {p.batchSamples.length > 0 ? `Run ${counted(p.batchSamples.length, "prompt")}` : "Run"}
          </Button>
          <span className="t-note">
            A CSV needs the columns <code>id, category, expected_action, prompt</code>.
          </span>
        </div>

        {p.batchRunning ? (
          <div style={{ marginTop: "1.5rem", maxWidth: "32rem" }}>
            <ProgressBar
              label="Running the prompts"
              helperText={`${p.batchProgress.done} of ${p.batchProgress.total}`}
              value={p.batchProgress.done}
              max={Math.max(1, p.batchProgress.total)}
            />
          </div>
        ) : null}

        {sum ? (
          <p className="t-rest" style={{ maxWidth: "52rem" }}>
            Of the prompts that should have been acted on, it caught <b>{sum.tp}</b> and missed <b>{sum.fn}</b>: recall{" "}
            <b>{percent(sum.recall)}</b>. Of the prompts it acted on, <b>{sum.fp}</b> should have passed: precision{" "}
            <b>{percent(sum.precision)}</b>. F1 <b>{percent(sum.f1)}</b>.
            {sum.err > 0 ? <> {counted(sum.err, "prompt")} errored.</> : null}
          </p>
        ) : null}

        {p.batchRows.length > 0 ? (
          <ScrollTable className="t-table" style={{ marginTop: "1.5rem" }}>
            <Table size="sm" aria-label="Batch test results">
              <TableHead>
                <TableRow>
                  <TableHeader>Sample</TableHeader>
                  <TableHeader>Category</TableHeader>
                  <TableHeader>Expected</TableHeader>
                  <TableHeader>Got</TableHeader>
                  <TableHeader>Outcome</TableHeader>
                  <TableHeader className="t-right">Confidence</TableHeader>
                  <TableHeader>Prompt</TableHeader>
                </TableRow>
              </TableHead>
              <TableBody data-swap="">
                {p.batchRows.map((r, i) => {
                  const outcome = OUTCOMES[r.outcome];
                  return (
                    <TableRow key={`${r.id}-${i}`}>
                      <TableCell className="t-data">{r.id}</TableCell>
                      <TableCell className="t-data">{r.category}</TableCell>
                      <TableCell>{ACTION_WORDS[upper(r.expected)] ?? r.expected}</TableCell>
                      <TableCell>{ACTION_WORDS[upper(r.actual)] ?? r.actual}</TableCell>
                      <TableCell>
                        {outcome.action ? (
                          <span className="t-action" data-action={outcome.action}>
                            {outcome.word}
                          </span>
                        ) : (
                          outcome.word
                        )}
                      </TableCell>
                      <TableCell className="t-right">{Math.round(r.confidence * 100)}%</TableCell>
                      <TableCell className="t-clip" title={r.prompt}>
                        <span className="t-data">{r.prompt}</span>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </ScrollTable>
        ) : (
          <p className="t-empty">
            {p.batchSamples.length === 0
              ? "No samples loaded yet."
              : `${counted(p.batchSamples.length, "sample")} ready. Run them to compare what the policy does with what each one expects.`}
          </p>
        )}
      </Section>
    </>
  );
}
