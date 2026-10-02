"use client";

import { Plus, Play, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Panel } from "@/components/app/panel";
import { usePolicyEntities } from "./usePolicyEntities";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from "@/components/ui/table";

const inputCls =
  "mt-1 w-full rounded-sm border border-border bg-surface-card px-2 py-1.5 text-xs text-fg focus:border-status-critical/40 ";
const labelCls =
  "text-xs uppercase tracking-[0.16em] text-fg-muted";

function actionClass(action: string) {
  switch (action.toUpperCase()) {
    case "BLOCK":
      return "text-status-critical";
    case "REDACT":
      return "text-status-medium";
    case "WARN":
      return "text-status-medium";
    default:
      return "text-fg-subtle";
  }
}

// PolicyEntitiesPanel manages policy-level custom entities — user-defined PII
// patterns that carry an action (BLOCK/REDACT/WARN) and confidence, and are
// persisted in the policy. Distinct from the runtime regex/literal patterns
// above (which have no action). Includes a simulate-against-active-policy tester.
export function PolicyEntitiesPanel() {
  const {
    draft,
    setDraft,
    items,
    isLoading,
    createMut,
    deleteMut,
    onSubmit,
    sampleText,
    setSampleText,
    simResult,
    onSimulate,
    simulating,
  } = usePolicyEntities();

  return (
    <div className="grid gap-3 lg:grid-cols-[1fr_360px]">
      {/* Existing policy entities */}
      <Panel title="Policy Entities">
        <div className="p-3">
          <p className="mb-2 text-xs text-fg-muted">
            User-defined PII entities with an enforcement action. Applied by the
            custom scanner and persisted in the active policy.
          </p>
          {isLoading ? (
            <p className="py-6 text-center text-xs text-fg-subtle">Loading…</p>
          ) : items.length === 0 ? (
            <p className="py-6 text-center text-xs text-fg-subtle">
              No policy entities yet. Define one on the right.
            </p>
          ) : (
            <div className="overflow-x-auto">
              <Table className="w-full text-left">
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>Pattern</TableHead>
                    <TableHead>Action</TableHead>
                    <TableHead>Severity</TableHead>
                    <TableHead>Conf.</TableHead>
                    <TableHead></TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((e) => (
                    <TableRow
                      key={e.name}
                    >
                      <TableCell className="font-medium">{e.name}</TableCell>
                      <TableCell className="font-mono">{e.pattern}</TableCell>
                      <TableCell className={`py-1 pr-2 font-medium ${actionClass(e.action)}`}>
                        {e.action.toUpperCase()}
                      </TableCell>
                      <TableCell>{e.severity}</TableCell>
                      <TableCell>{e.confidence ?? "—"}</TableCell>
                      <TableCell>
                        <button
                          aria-label={`delete ${e.name}`}
                          className="text-fg-subtle hover:text-status-critical"
                          onClick={() => deleteMut.mutate(e.name)}
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}

          {/* Test against active policy */}
          <div className="mt-4 border-t border-border pt-3">
            <label className={labelCls}>Test against active policy</label>
            <Textarea
              className={`${inputCls} h-16 font-mono`}
              value={sampleText}
              onChange={(e) => setSampleText(e.target.value)}
              placeholder="Paste sample text, e.g. customer ACME-12345678 record" aria-label="Paste sample text, e.g. customer ACME-12345678 record" />
            <Button
              variant="outline"
              size="sm"
              className="mt-2"
              onClick={onSimulate}
              disabled={simulating}
            >
              <Play className="mr-1 h-3.5 w-3.5" />
              {simulating ? "Running…" : "Simulate"}
            </Button>
            {simResult && (
              <div className="mt-2 rounded-sm border border-border p-2 text-xs">
                <div className="text-fg-muted">
                  action: <span className={actionClass(simResult.action)}>{simResult.action}</span> ·{" "}
                  {simResult.findings.length} finding(s)
                </div>
                {simResult.findings.map((f, i) => (
                  <div key={i} className="mt-1 text-fg-muted">
                    <span className="font-mono text-status-medium">{f.category}</span> — {f.match}{" "}
                    <span className="text-fg-subtle">({f.action})</span>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      </Panel>

      {/* Create form */}
      <Panel title="New Policy Entity">
        <div className="space-y-3 p-3">
          <div>
            <label className={labelCls}>Name</label>
            <Input
              className={inputCls}
              value={draft.name}
              onChange={(e) => setDraft({ ...draft, name: e.target.value })}
              placeholder="fib_musteri_no" aria-label="fib_musteri_no" />
          </div>
          <div>
            <label className={labelCls}>Pattern (regex)</label>
            <Input
              className={`${inputCls} font-mono`}
              value={draft.pattern}
              onChange={(e) => setDraft({ ...draft, pattern: e.target.value })}
              placeholder="ACME-\d{8}" aria-label="ACME-\d{8}" />
          </div>
          <div>
            <label className={labelCls}>Description</label>
            <Input
              className={inputCls}
              value={draft.description}
              onChange={(e) => setDraft({ ...draft, description: e.target.value })}
              placeholder="Customer number" aria-label="Customer number" />
          </div>
          <div className="grid grid-cols-2 gap-2">
            <div>
              <label className={labelCls}>Action</label>
              <NativeSelect
                className={inputCls}
                value={draft.action}
                onChange={(e) => setDraft({ ...draft, action: e.target.value })}
              >
                <NativeSelectOption value="BLOCK">BLOCK</NativeSelectOption>
                <NativeSelectOption value="REDACT">REDACT</NativeSelectOption>
                <NativeSelectOption value="WARN">WARN</NativeSelectOption>
              </NativeSelect>
            </div>
            <div>
              <label className={labelCls}>Severity</label>
              <NativeSelect
                className={inputCls}
                value={draft.severity}
                onChange={(e) =>
                  setDraft({ ...draft, severity: e.target.value as typeof draft.severity })
                }
              >
                <NativeSelectOption value="critical">critical</NativeSelectOption>
                <NativeSelectOption value="high">high</NativeSelectOption>
                <NativeSelectOption value="medium">medium</NativeSelectOption>
                <NativeSelectOption value="low">low</NativeSelectOption>
              </NativeSelect>
            </div>
          </div>
          <div>
            <label className={labelCls}>Confidence ({draft.confidence.toFixed(2)})</label>
            <input
              type="range"
              min={0}
              max={1}
              step={0.05}
              className="mt-1 w-full"
              value={draft.confidence}
              onChange={(e) => setDraft({ ...draft, confidence: Number(e.target.value) })}
            />
          </div>
          <Button variant="outline" className="w-full" onClick={onSubmit} disabled={createMut.isPending}>
            <Plus className="mr-1 h-3.5 w-3.5" />
            {createMut.isPending ? "Creating…" : "Create Entity"}
          </Button>
        </div>
      </Panel>
    </div>
  );
}
