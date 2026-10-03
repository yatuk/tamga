"use client";

import { PageHeader } from "@/components/app/page-header";
import { Panel } from "@/components/app/panel";
import { AdminKeyRequired } from "@/components/app/states";
import { IntegrationConnectModal } from "./IntegrationConnectModal";
import { openIntegrationDraft } from "./integrationDraft";
import { IntegrationsHooksTable } from "./IntegrationsHooksTable";
import { IntegrationsPresetGrid } from "./IntegrationsPresetGrid";
import { useIntegrationsPage } from "./useIntegrationsPage";

export default function IntegrationsPage() {
  const { adminKey, draft, setDraft, hooks, createMut, testMut, deleteMut } = useIntegrationsPage();

  const header = (
    <PageHeader
      title="Integrations"
      description="Send blocked requests and alerts to the chat, paging, ticketing and SIEM tools your team already uses."
    />
  );

  if (!adminKey) {
    return (
      <div className="space-y-6">
        {header}
        <Panel>
          <AdminKeyRequired />
        </Panel>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {header}

      <IntegrationsHooksTable
        hooks={hooks}
        testingId={testMut.isPending ? testMut.variables : undefined}
        onTest={(id) => testMut.mutate(id)}
        onDelete={(id) => deleteMut.mutate(id)}
      />

      <IntegrationsPresetGrid hooks={hooks} onConnect={(kind, name) => setDraft(openIntegrationDraft(kind, name))} />

      <IntegrationConnectModal draft={draft} setDraft={setDraft} createMut={createMut} />
    </div>
  );
}
