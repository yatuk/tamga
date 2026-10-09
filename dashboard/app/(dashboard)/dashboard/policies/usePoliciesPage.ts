"use client";

import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { toast } from "@/lib/toast";
import { api, type PolicyIssue, type PolicySimulateResult, type TamgaPolicy } from "@/lib/api";
import { POLICY_DRAFT_STORAGE, POLICY_SAMPLE_STORAGE, POLICY_TAB_IDS } from "./_constants";
import { useAdminKey } from "@/hooks/useAdminKey";
import { useEnumParam } from "@/hooks/useUrlState";
import { stringifyPolicy } from "./policyUtils";
import { explainRefusal, type Refusal } from "./refusal";

/** How the last save or reload ended, shown under the page head until the next one. */
export type WriteOutcome =
  | { kind: "refused"; refusal: Refusal }
  | { kind: "saved" | "reloaded"; changed: string[]; warnings: PolicyIssue[] };

export function usePoliciesPage() {
  const [adminKey] = useAdminKey();
  const [draft, setDraft] = useState("");
  const [originalYaml, setOriginalYaml] = useState("");
  const [sample, setSample] = useState('Hi, my credit card is 4242 4242 4242 4242');
  const [saving, setSaving] = useState(false);
  const [outcome, setOutcome] = useState<WriteOutcome | null>(null);
  const [simulating, setSimulating] = useState(false);
  const [simResult, setSimResult] = useState<PolicySimulateResult | null>(null);
  const [tab, setTab] = useEnumParam("tab", POLICY_TAB_IDS, "editor");

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
    setOutcome(null);
    try {
      const res = await api.reloadPolicies(adminKey);
      setOutcome({ kind: "reloaded", changed: res.changed ?? [], warnings: [] });
      await refetch();
    } catch (e) {
      setOutcome({ kind: "refused", refusal: explainRefusal(e, "reload") });
    }
  }

  async function onSave() {
    if (!draft.trim()) {
      toast.error("The draft is empty");
      return;
    }
    try {
      JSON.parse(draft);
    } catch {
      toast.error("The draft is not valid JSON", "Fix the syntax error and save again.");
      return;
    }
    setSaving(true);
    setOutcome(null);
    try {
      const validation = await api.validatePolicy(adminKey, draft);
      const res = await api.putPolicy(adminKey, draft);
      setOutcome({ kind: "saved", changed: res.changed ?? [], warnings: validation.warnings ?? [] });
      // The proxy writes the document its own way, so take the draft from what it now runs.
      const saved = stringifyPolicy((await refetch()).data?.[0] ?? null);
      if (saved) setDraft(saved);
    } catch (e) {
      // Said under the page head, where it stays to be read: which sections, which fields.
      setOutcome({ kind: "refused", refusal: explainRefusal(e, "save") });
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
      toast.error("Could not run the simulation", (e as Error).message);
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
    outcome,
    setOutcome,
    refetch,
    simulating,
    simResult,
    tab,
    setTab,
    data,
    isLoading,
    error,
    activePolicy,
    onReload,
    onSave,
    onSimulate,
  };
}
