"use client";

import { PageHeader } from "@/components/app/page-header";
import { INTEGRATION_PRESETS } from "./integrationPresets";
import { IntegrationConnectModal } from "./IntegrationConnectModal";
import { IntegrationsHooksTable } from "./IntegrationsHooksTable";
import { IntegrationsPresetGrid } from "./IntegrationsPresetGrid";
import { openIntegrationDraft } from "./integrationDraft";
import { useIntegrationsPage } from "./useIntegrationsPage";

export default function IntegrationsPage() {
  const { draft, setDraft, hooks, createMut, testMut, deleteMut } = useIntegrationsPage();

  return (
    <div className="space-y-2">
      <PageHeader
        title="Integrations"
        description={`${hooks.length} connected. Send incidents to chat, ticketing and SIEM tools.`}
      />

      <IntegrationsPresetGrid hooks={hooks} onConnect={(kind, name) => setDraft(openIntegrationDraft(kind, name))} />

      <IntegrationsHooksTable
        hooks={hooks}
        onTest={(id) => testMut.mutate(id)}
        onDelete={(id) => deleteMut.mutate(id)}
        onConnect={() => {
          // Open the first available preset's connect modal
          const first = INTEGRATION_PRESETS[0];
          if (first) setDraft(openIntegrationDraft(first.kind, first.name));
        }}
      />

      {draft ? <IntegrationConnectModal draft={draft} setDraft={setDraft} createMut={createMut} /> : null}
    </div>
  );
}
