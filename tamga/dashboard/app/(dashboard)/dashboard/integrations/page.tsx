"use client";

import { PageHeader } from "@/components/dashboard/PageHeader";
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
        eyebrow="ADMINISTRATION // INTEGRATIONS"
        title="Integrations"
        subtitle={`${hooks.length} bağlı · ${INTEGRATION_PRESETS.length} hazır şablon · tile başına ayrıntılı rehber`}
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
