"use client";

import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { PageHeader } from "@/components/dashboard/PageHeader";
import { PoliciesAnimatedTabs } from "./PoliciesAnimatedTabs";
import { usePoliciesPage } from "./usePoliciesPage";

export default function PoliciesPage() {
  const {
    adminKey,
    draft,
    setDraft,
    originalYaml,
    sample,
    setSample,
    saving,
    simulating,
    simResult,
    tab,
    setTab,
    activePolicy,
    isLoading,
    error,
    onReload,
    onSave,
    onSimulate,
  } = usePoliciesPage();

  const isDirty = Boolean(originalYaml && draft && originalYaml !== draft);

  return (
    <div className="space-y-2">
      <PageHeader
        eyebrow={`POLICY CONTROL // ${(activePolicy?.name as string | undefined) || "default"}`}
        title="Policy Editor"
        subtitle={
          <>
            v{(activePolicy?.version as string | undefined) ?? "—"} · last reload{" "}
            {typeof activePolicy?.updated_at === "string"
              ? new Date(activePolicy.updated_at as string).toLocaleString("tr-TR")
              : "—"}
          </>
        }
        actions={
          <>
            <Badge
              className={`rounded-sm border text-[10px] uppercase tracking-[0.14em] ${
                isDirty ? "border-status-medium/40 bg-status-medium/10 text-status-medium" : "border-status-pass/30 bg-status-pass/10 text-status-pass"
              }`}
            >
              {isDirty ? "DRAFT" : "SYNCED"}
            </Badge>
            <Button className="cursor-pointer rounded-sm bg-status-critical text-white hover:bg-status-critical" onClick={onSave} disabled={saving}>
              {saving ? "Kaydediliyor…" : "Save & Reload"}
            </Button>
            <Button className="cursor-pointer rounded-sm border border-border-strong bg-surface-subtle text-fg-muted hover:bg-surface-card" onClick={onReload}>
              Reload disk
            </Button>
            <Button
              className="cursor-pointer rounded-sm border border-border-strong bg-surface-subtle text-fg-muted hover:bg-surface-card"
              onClick={() => setDraft(originalYaml)}
            >
              Reset draft
            </Button>
          </>
        }
      />

      {isLoading ? (
        <div className="rounded-sm border border-border bg-surface-card p-4 space-y-2" role="status" aria-label="Loading policy editor">
          <div className="h-8 w-48 animate-pulse rounded bg-surface-subtle" />
          <div className="h-[400px] animate-pulse rounded bg-surface-subtle" />
          <span className="sr-only">Loading policy editor...</span>
        </div>
      ) : error ? (
        <div className="rounded-sm border border-status-critical/30 bg-status-critical/10 p-4 text-xs text-status-critical" role="alert">
          {(error as Error).message}
        </div>
      ) : (
        <div>
          <PoliciesAnimatedTabs
            tab={tab}
            onTabChange={setTab}
            draft={draft}
            setDraft={setDraft}
            originalYaml={originalYaml}
            adminKey={adminKey}
            sample={sample}
            setSample={setSample}
            simulating={simulating}
            onSimulate={onSimulate}
            simResult={simResult}
          />
        </div>
      )}
    </div>
  );
}
