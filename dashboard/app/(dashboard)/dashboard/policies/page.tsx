"use client";

import { RotateCcw, Save } from "lucide-react";
import { ConfirmButton } from "@/components/app/confirm-button";
import { PageHeader } from "@/components/app/page-header";
import { PageTabsList, PageTabsTrigger } from "@/components/app/page-tabs";
import { Panel } from "@/components/app/panel";
import { AdminKeyRequired, ErrorState, SkeletonRows } from "@/components/app/states";
import { StatusBadge } from "@/components/app/status-badge";
import { PolicyDiff } from "@/components/dashboard/policies/PolicyDiff";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { Tabs, TabsContent } from "@/components/ui/tabs";
import { POLICY_TABS, type PolicyTabKey } from "./_constants";
import { CompetitorsForm } from "./CompetitorsForm";
import { CustomEntityForm } from "./CustomEntityForm";
import { PoliciesDiffPanel } from "./PoliciesDiffPanel";
import { PoliciesSimulatePanel } from "./PoliciesSimulatePanel";
import { PolicyEditor } from "./policy-editor";
import { usePoliciesPage } from "./usePoliciesPage";

export default function PoliciesPage() {
  const m = usePoliciesPage();
  const { activePolicy } = m;
  const isDirty = Boolean(m.originalYaml && m.draft && m.originalYaml !== m.draft);
  const version = typeof activePolicy?.version === "string" ? activePolicy.version : null;
  const updatedAt = typeof activePolicy?.updated_at === "string" ? new Date(activePolicy.updated_at) : null;

  const header = (
    <PageHeader
      title="Policies"
      description={
        version || updatedAt ? (
          <>
            Active policy{version ? <span className="font-mono text-xs"> v{version}</span> : null}
            {updatedAt ? `, loaded ${updatedAt.toLocaleString("en-GB")}` : ""}.
          </>
        ) : (
          "The rules that decide what is blocked, redacted, warned or passed."
        )
      }
      actions={
        m.adminKey ? (
          <>
            <StatusBadge tone={isDirty ? "medium" : "neutral"}>{isDirty ? "Unsaved changes" : "No changes"}</StatusBadge>
            <ConfirmButton
              variant="ghost"
              size="sm"
              disabled={!isDirty}
              title="Discard your changes?"
              description="The draft goes back to the policy the proxy is running."
              confirmLabel="Discard"
              onConfirm={() => m.setDraft(m.originalYaml)}
            >
              Discard
            </ConfirmButton>
            <Button variant="outline" size="sm" onClick={m.onReload}>
              <RotateCcw />
              Reload From Disk
            </Button>
            <Button size="sm" onClick={m.onSave} disabled={m.saving || !isDirty}>
              {m.saving ? <Spinner /> : <Save />}
              {m.saving ? "Saving…" : "Save and Apply"}
            </Button>
          </>
        ) : null
      }
    />
  );

  if (!m.adminKey || m.isLoading || m.error) {
    return (
      <div className="space-y-6">
        {header}
        <Panel>
          {!m.adminKey ? (
            <AdminKeyRequired />
          ) : m.error ? (
            <ErrorState title="Could not load the policy" error={m.error} onRetry={() => void m.refetch()} />
          ) : (
            <SkeletonRows rows={10} />
          )}
        </Panel>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {header}

      <Tabs value={m.tab} onValueChange={(v) => m.setTab(v as PolicyTabKey)} className="gap-6">
        <PageTabsList>
          {POLICY_TABS.map((t) => (
            <PageTabsTrigger key={t.id} value={t.id}>
              {t.label}
            </PageTabsTrigger>
          ))}
        </PageTabsList>

        <TabsContent value="editor" className="space-y-6">
          <PolicyEditor draft={m.draft} onChange={m.setDraft} />
        </TabsContent>
        <TabsContent value="diff">
          <PoliciesDiffPanel originalYaml={m.originalYaml} draft={m.draft} />
        </TabsContent>
        <TabsContent value="simulate" className="space-y-6">
          <PoliciesSimulatePanel
            sample={m.sample}
            onSampleChange={m.setSample}
            simulating={m.simulating}
            onSimulate={m.onSimulate}
            simResult={m.simResult}
          />
        </TabsContent>
        <TabsContent value="history" className="space-y-6">
          <PolicyDiff adminKey={m.adminKey} />
        </TabsContent>
        <TabsContent value="entities">
          <CustomEntityForm adminKey={m.adminKey} />
        </TabsContent>
        <TabsContent value="competitors">
          <CompetitorsForm adminKey={m.adminKey} />
        </TabsContent>
      </Tabs>
    </div>
  );
}
