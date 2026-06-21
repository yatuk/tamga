"use client";

import { PageHeader } from "@/components/dashboard/PageHeader";
import { API_BASE, SETTINGS_TABS } from "./_constants";
import { SettingsAccessSection } from "./SettingsAccessSection";
import { SettingsRetentionSection } from "./SettingsRetentionSection";
import { SettingsProvidersSection } from "./SettingsProvidersSection";
import { SettingsRuntimeSection } from "./SettingsRuntimeSection";
import { SettingsSSOSection } from "./SettingsSSOSection";
import { SettingsStatusChip } from "./SettingsStatusChip";
import { SettingsWebhooksSection } from "./SettingsWebhooksSection";
import { useSettingsPage } from "./useSettingsPage";

export default function SettingsPage() {
  const {
    tab,
    setTab,
    draft,
    setDraft,
    saved,
    retention,
    setRetention,
    health,
    runtime,
    keyList,
    hookList,
    ssoConfig,
    ssoLoading,
    ssoError,
    saveSSO,
    saveAdminKey,
    saveRetention,
    createKey,
    removeKey,
    createHook,
    removeHook,
    testHook,
  } = useSettingsPage();

  const dbStatus = health?.database || "unknown";

  return (
    <div className="space-y-2">
      <PageHeader
        eyebrow="ADMINISTRATION // SETTINGS"
        title="Settings"
        subtitle={`${API_BASE} · proxy ${health?.proxy || "unknown"} · db ${dbStatus}`}
        actions={
          <div className="flex flex-wrap items-center gap-1">
            <SettingsStatusChip label="PROXY" value={health?.proxy || "?"} good={health?.proxy === "up"} />
            <SettingsStatusChip
              label="DB"
              value={dbStatus}
              good={dbStatus === "connected"}
              neutral={dbStatus === "not_configured"}
            />
            <SettingsStatusChip label="SCAN" value={String(health?.scanner_count ?? 0)} good={(health?.scanner_count ?? 0) > 0} />
          </div>
        }
      />

      <div className="inline-flex overflow-hidden rounded-sm border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950">
        {SETTINGS_TABS.map((t) => (
          <button
            key={t.id}
            type="button"
            onClick={() => setTab(t.id)}
            className={`cursor-pointer px-3 py-1.5 text-xs uppercase tracking-wide ${
              tab === t.id ? "bg-emerald-600 text-white" : "text-zinc-600 dark:text-zinc-400 hover:bg-zinc-100 dark:hover:bg-zinc-900 hover:text-zinc-200"
            }`}
          >
            {t.label}
          </button>
        ))}
      </div>

      <div className="space-y-3">
        {tab === "access" ? (
          <SettingsAccessSection
            draft={draft}
            setDraft={setDraft}
            saved={saved}
            saveAdminKey={saveAdminKey}
            keyList={keyList}
            createKey={createKey}
            removeKey={removeKey}
          />
        ) : null}
        {tab === "raw-webhooks" ? (
          <SettingsWebhooksSection hookList={hookList} createHook={createHook} removeHook={removeHook} testHook={testHook} />
        ) : null}
        {tab === "retention" ? (
          <SettingsRetentionSection retention={retention} setRetention={setRetention} saveRetention={saveRetention} />
        ) : null}
        {tab === "providers" ? <SettingsProvidersSection health={health} adminKey={saved} /> : null}
        {tab === "runtime" ? <SettingsRuntimeSection health={health} runtime={runtime} /> : null}
        {tab === "sso" ? (
          <SettingsSSOSection
            config={ssoConfig}
            loading={ssoLoading}
            error={ssoError instanceof Error ? ssoError.message : null}
            onSave={saveSSO}
          />
        ) : null}
      </div>
    </div>
  );
}
