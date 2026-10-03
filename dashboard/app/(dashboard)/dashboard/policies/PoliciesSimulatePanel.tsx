"use client";

import { Play } from "lucide-react";
import { FormField } from "@/components/app/form-field";
import { Panel } from "@/components/app/panel";
import { EmptyState } from "@/components/app/states";
import { ActionBadge, SeverityBadge } from "@/components/app/status-badge";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import type { PolicySimulateResult } from "@/lib/api";
import { humanizeFindingType } from "@/lib/humanize";

type Props = {
  sample: string;
  onSampleChange: (v: string) => void;
  simulating: boolean;
  onSimulate: () => void;
  simResult: PolicySimulateResult | null;
};

/** Runs a prompt against the draft without saving it or calling a provider. */
export function PoliciesSimulatePanel({ sample, onSampleChange, simulating, onSimulate, simResult }: Props) {
  return (
    <>
      <Panel title="Test a prompt against the draft">
        <form
          className="space-y-3 p-4"
          onSubmit={(e) => {
            e.preventDefault();
            onSimulate();
          }}
        >
          <FormField
            label="Prompt"
            htmlFor="policy-sample"
            hint="Nothing is sent to a provider and nothing is saved. The draft is evaluated as it is in the editor."
          >
            <Textarea
              id="policy-sample"
              name="sample"
              className="min-h-28 resize-y font-mono text-xs"
              spellCheck={false}
              value={sample}
              onChange={(e) => onSampleChange(e.target.value)}
              placeholder="Paste a prompt to test…"
            />
          </FormField>
          <Button type="submit" disabled={simulating || !sample.trim()}>
            {simulating ? <Spinner /> : <Play />}
            {simulating ? "Running…" : "Run Simulation"}
          </Button>
        </form>
      </Panel>

      {simResult ? (
        <Panel
          title="Decision"
          description={`${simResult.policy_name} v${simResult.policy_version}`}
          aside={<ActionBadge action={simResult.action || "PASS"} />}
        >
          {simResult.findings.length === 0 ? (
            <EmptyState icon="shield" title="No findings" description="The draft would pass this prompt unchanged." />
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Type</TableHead>
                  <TableHead>Category</TableHead>
                  <TableHead>Severity</TableHead>
                  <TableHead>Action</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {simResult.findings.map((f, i) => (
                  <TableRow key={i}>
                    <TableCell>{humanizeFindingType(f.type)}</TableCell>
                    <TableCell className="font-mono text-xs">{f.category}</TableCell>
                    <TableCell>
                      <SeverityBadge severity={f.severity} />
                    </TableCell>
                    <TableCell>
                      <ActionBadge action={f.action} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </Panel>
      ) : null}
    </>
  );
}
