"use client";

import { useEffect, useState, useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type ApiKey, type Webhook } from "@/lib/api";
import { type SSOSettings } from "@/lib/api/client";
import { toast } from "@/lib/toast";
import { RETENTION_STORAGE, type SettingsTabKey } from "./_constants";
import { useAdminKey } from "@/hooks/useAdminKey";

export function useSettingsPage() {
  const [tab, setTab] = useState<SettingsTabKey>("access");
  const [adminKey, setAdminKey] = useAdminKey();
  const [draft, setDraft] = useState(adminKey);
  const [saved, setSaved] = useState(adminKey);
  const [retention, setRetention] = useState<string>("30");
  const qc = useQueryClient();

  useEffect(() => {
    if (typeof window === "undefined") return;
    setRetention(window.localStorage.getItem(RETENTION_STORAGE) || "30");
  }, []);

  useEffect(() => {
    setDraft(adminKey);
    setSaved(adminKey);
  }, [adminKey]);

  const { data: health } = useQuery({
    queryKey: ["tamga-settings-health"],
    queryFn: () => api.getHealthDetailed(),
    refetchInterval: 10_000,
    retry: 1,
  });

  const { data: runtime } = useQuery({
    queryKey: ["tamga-settings-runtime"],
    queryFn: () => api.getHealthDetail(),
    refetchInterval: 10_000,
    retry: 1,
    enabled: tab === "runtime",
  });

  const { data: keyList } = useQuery({
    queryKey: ["tamga-apikeys", saved],
    queryFn: () => api.listApiKeys(saved),
    enabled: !!saved,
  });

  const { data: hookList } = useQuery({
    queryKey: ["tamga-webhooks", saved],
    queryFn: () => api.listWebhooks(saved),
    enabled: !!saved,
  });

  const {
    data: ssoConfig,
    isLoading: ssoLoading,
    error: ssoError,
  } = useQuery({
    queryKey: ["tamga-sso", saved],
    queryFn: () => api.getSSOSettings(saved),
    enabled: !!saved && tab === "sso",
  });

  const saveSSO = useCallback(
    async (cfg: Partial<SSOSettings>) => {
      await api.updateSSOSettings(saved, cfg);
      qc.invalidateQueries({ queryKey: ["tamga-sso"] });
    },
    [saved, qc],
  );

  function saveAdminKey() {
    setAdminKey(draft);
    setSaved(draft);
    toast.success("Admin key kaydedildi");
  }

  function saveRetention() {
    if (typeof window === "undefined") return;
    window.localStorage.setItem(RETENTION_STORAGE, retention);
    toast.success("Retention updated", `${retention} days (client-side)`);
  }

  async function createKey(label: string, scope: ApiKey["scope"]) {
    if (!label.trim()) {
      toast.error("Label boş olamaz.");
      return;
    }
    try {
      const created = await api.createApiKey(saved, label.trim(), scope);
      toast.success("API key oluşturuldu", "tek seferlik gösterim");
      navigator.clipboard?.writeText(created.raw_key).catch(() => {});
      qc.invalidateQueries({ queryKey: ["tamga-apikeys"] });
      toast.success("Key copied to clipboard", created.raw_key.slice(0, 12) + "...");
    } catch (e) {
      toast.error("Anahtar oluşturulamadı", (e as Error).message);
    }
  }

  async function removeKey(id: string) {
    try {
      await api.deleteApiKey(saved, id);
      toast.success("API key iptal edildi");
      qc.invalidateQueries({ queryKey: ["tamga-apikeys"] });
    } catch (e) {
      toast.error("Silme hatası", (e as Error).message);
    }
  }

  async function createHook(payload: Omit<Webhook, "id" | "created_at">) {
    try {
      await api.createWebhook(saved, payload);
      toast.success("Webhook eklendi");
      qc.invalidateQueries({ queryKey: ["tamga-webhooks"] });
    } catch (e) {
      toast.error("Webhook eklenemedi", (e as Error).message);
    }
  }

  async function removeHook(id: string) {
    try {
      await api.deleteWebhook(saved, id);
      toast.success("Webhook silindi");
      qc.invalidateQueries({ queryKey: ["tamga-webhooks"] });
    } catch (e) {
      toast.error("Silme hatası", (e as Error).message);
    }
  }

  async function testHook(id: string) {
    try {
      const r = await api.testWebhook(saved, id);
      if (r.ok) toast.success("Webhook OK", `HTTP ${r.status_code}`);
      else toast.error("Webhook FAIL", `HTTP ${r.status_code}`);
    } catch (e) {
      toast.error("Test hatası", (e as Error).message);
    }
  }

  return {
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
  };
}
