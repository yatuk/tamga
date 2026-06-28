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
                isDirty ? "border-amber-500/40 bg-amber-500/10 text-amber-400" : "border-emerald-500/30 bg-emerald-500/10 text-emerald-400"
              }`}
            >
              {isDirty ? "DRAFT" : "SYNCED"}
            </Badge>
            <Button className="cursor-pointer rounded-sm bg-red-600 text-white hover:bg-red-700" onClick={onSave} disabled={saving}>
              {saving ? "Kaydediliyor…" : "Save & Reload"}
            </Button>
            <Button className="cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800" onClick={onReload}>
              Reload disk
            </Button>
            <Button
              className="cursor-pointer rounded-sm border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-900 text-zinc-700 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-800"
              onClick={() => setDraft(originalYaml)}
            >
              Reset draft
            </Button>
          </>
        }
      />

      {isLoading ? (
        <div className="rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 p-4 space-y-2" role="status" aria-label="Loading policy editor">
          <div className="h-8 w-48 animate-pulse rounded bg-zinc-100 dark:bg-zinc-900/40" />
          <div className="h-[400px] animate-pulse rounded bg-zinc-100 dark:bg-zinc-900/40" />
          <span className="sr-only">Loading policy editor...</span>
        </div>
      ) : error ? (
        <div className="rounded-sm border border-red-500/30 bg-red-500/10 p-4 text-xs text-red-400" role="alert">
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
