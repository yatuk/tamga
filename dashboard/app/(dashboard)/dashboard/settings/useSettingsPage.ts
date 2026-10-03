"use client";

import { useEffect, useState, useCallback } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { type SSOSettings } from "@/lib/api/client";
import { toast } from "@/lib/toast";
import { RETENTION_STORAGE, SETTINGS_TAB_IDS } from "./_constants";
import { useAdminKey } from "@/hooks/useAdminKey";
import { useEnumParam } from "@/hooks/useUrlState";

export function useSettingsPage() {
  const [tab, setTab] = useEnumParam("tab", SETTINGS_TAB_IDS, "access");
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
    toast.success("Admin key saved");
  }

  function saveRetention() {
    if (typeof window === "undefined") return;
    window.localStorage.setItem(RETENTION_STORAGE, retention);
    toast.success("Dashboard window saved", `${retention} days`);
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
    ssoConfig,
    ssoLoading,
    ssoError,
    saveSSO,
    saveAdminKey,
    saveRetention,
  };
}
