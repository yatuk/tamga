"use client";

import { PageHeader } from "@/components/app/page-header";
import { StatusBadge } from "@/components/app/status-badge";
import { PageTabsList, PageTabsTrigger } from "@/components/app/page-tabs";
import { Tabs, TabsContent } from "@/components/ui/tabs";
import { API_BASE, SETTINGS_TABS, type SettingsTabKey } from "./_constants";
import { SettingsAccessSection } from "./SettingsAccessSection";
import { SettingsProvidersSection } from "./SettingsProvidersSection";
import { SettingsRetentionSection } from "./SettingsRetentionSection";
import { SettingsRuntimeSection } from "./SettingsRuntimeSection";
import { SettingsSSOSection } from "./SettingsSSOSection";
import { useSettingsPage } from "./useSettingsPage";

export default function SettingsPage() {
  const m = useSettingsPage();
  const { health } = m;
  const dbStatus = health?.database || "unknown";
  const scanners = health?.scanner_count ?? 0;

  return (
    <div className="space-y-6">
      <PageHeader
        title="Settings"
        description={
          <>
            Access, alert delivery and runtime of the proxy at <span className="font-mono text-xs">{API_BASE}</span>.
          </>
        }
        actions={
          <>
            <StatusBadge tone={health?.proxy === "up" ? "pass" : health ? "critical" : "neutral"}>
              Proxy {health?.proxy || "unknown"}
            </StatusBadge>
            <StatusBadge
              tone={dbStatus === "connected" ? "pass" : dbStatus === "not_configured" || !health ? "neutral" : "critical"}
            >
              Database {dbStatus.replace(/_/g, " ")}
            </StatusBadge>
            <StatusBadge tone={scanners > 0 ? "pass" : health ? "critical" : "neutral"}>{scanners} scanners</StatusBadge>
          </>
        }
      />

      <Tabs value={m.tab} onValueChange={(v) => m.setTab(v as SettingsTabKey)} className="gap-6">
        <PageTabsList>
          {SETTINGS_TABS.map((t) => (
            <PageTabsTrigger key={t.id} value={t.id}>
              {t.label}
            </PageTabsTrigger>
          ))}
        </PageTabsList>

        <TabsContent value="access" className="space-y-6">
          <SettingsAccessSection
            draft={m.draft}
            setDraft={m.setDraft}
            saved={m.saved}
            saveAdminKey={m.saveAdminKey}
          />
        </TabsContent>
        <TabsContent value="retention">
          <SettingsRetentionSection retention={m.retention} setRetention={m.setRetention} saveRetention={m.saveRetention} />
        </TabsContent>
        <TabsContent value="providers">
          <SettingsProvidersSection health={health} adminKey={m.saved} />
        </TabsContent>
        <TabsContent value="runtime">
          <SettingsRuntimeSection health={health} runtime={m.runtime} />
        </TabsContent>
        <TabsContent value="sso">
          <SettingsSSOSection
            adminKey={m.saved}
            config={m.ssoConfig}
            loading={m.ssoLoading}
            error={m.ssoError}
            onSave={m.saveSSO}
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}
