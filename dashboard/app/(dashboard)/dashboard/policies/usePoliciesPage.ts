"use client";

import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { toast } from "@/lib/toast";
import { api, type PolicySimulateResult, type TamgaPolicy } from "@/lib/api";
import { POLICY_DRAFT_STORAGE, POLICY_SAMPLE_STORAGE } from "./_constants";
import { useAdminKey } from "@/hooks/useAdminKey";
import { stringifyPolicy } from "./policyUtils";

export function usePoliciesPage() {
  const [adminKey] = useAdminKey();
  const [draft, setDraft] = useState("");
  const [originalYaml, setOriginalYaml] = useState("");
  const [sample, setSample] = useState('Merhaba, kredi kartım: 4242 4242 4242 4242');
  const [saving, setSaving] = useState(false);
  const [simulating, setSimulating] = useState(false);
  const [simResult, setSimResult] = useState<PolicySimulateResult | null>(null);
  const [tab, setTab] = useState<"editor" | "diff" | "simulate" | "history" | "entities" | "competitors">("editor");

  useEffect(() => {
    if (typeof window === "undefined") return;
    const savedDraft = window.localStorage.getItem(POLICY_DRAFT_STORAGE) || "";
    if (savedDraft) setDraft(savedDraft);
    const savedSample = window.localStorage.getItem(POLICY_SAMPLE_STORAGE);
    if (savedSample) setSample(savedSample);
  }, []);

  useEffect(() => {
    if (typeof window === "undefined") return;
    window.localStorage.setItem(POLICY_DRAFT_STORAGE, draft);
  }, [draft]);

  useEffect(() => {
    if (typeof window === "undefined") return;
    window.localStorage.setItem(POLICY_SAMPLE_STORAGE, sample);
  }, [sample]);

  const { data, isLoading, error, refetch } = useQuery({
    queryKey: ["tamga-policies", adminKey],
    queryFn: () => api.getPolicies(adminKey),
    enabled: !!adminKey,
    retry: 1,
    staleTime: 60 * 1000,
  });

  const activePolicy = useMemo<TamgaPolicy | null>(() => {
    if (!data || data.length === 0) return null;
    return data[0];
  }, [data]);

  useEffect(() => {
    const serialized = stringifyPolicy(activePolicy);
    setOriginalYaml(serialized);
    if (!draft && serialized) setDraft(serialized);
  }, [activePolicy, draft]);

  async function onReload() {
    try {
      const res = await api.reloadPolicies(adminKey);
      toast.success(`Policy reload`, res.name || "default");
      await refetch();
    } catch (e) {
      toast.error("Reload hatası", (e as Error).message);
    }
  }

  async function onSave() {
    if (!draft.trim()) {
      toast.error("Draft boş");
      return;
    }
    try {
      JSON.parse(draft);
    } catch {
      toast.error("Geçersiz JSON", "Policy geçerli bir JSON olmalı.");
      return;
    }
    setSaving(true);
    try {
      const validation = await api.validatePolicy(adminKey, draft);
      await api.putPolicy(adminKey, draft);
      const subtitle =
        validation.warnings?.length ? `${validation.warnings.length} uyarı · reload sonrası aktif` : "reload sonrası aktif";
      toast.success("Policy kaydedildi", subtitle);
      await refetch();
    } catch (e) {
      toast.error("Kaydetme hatası", (e as Error).message);
    } finally {
      setSaving(false);
    }
  }

  async function onSimulate() {
    setSimulating(true);
    try {
      const res = await api.simulatePolicy(adminKey, draft, sample);
      setSimResult(res);
    } catch (e) {
      toast.error("Simulate hatası", (e as Error).message);
    } finally {
      setSimulating(false);
    }
  }

  return {
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
    data,
    isLoading,
    error,
    refetch,
    activePolicy,
    onReload,
    onSave,
    onSimulate,
  };
}
