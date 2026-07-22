"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type CustomEntity, type PolicySimulateResult } from "@/lib/api";
import { toast } from "@/lib/toast";
import { useAdminKey } from "@/hooks/useAdminKey";

export type EntityDraft = {
  name: string;
  pattern: string;
  description: string;
  severity: CustomEntity["severity"];
  action: string;
  confidence: number;
};

export const EMPTY_ENTITY: EntityDraft = {
  name: "",
  pattern: "",
  description: "",
  severity: "high",
  action: "REDACT",
  confidence: 0.9,
};

// Policy custom entities carry an action (BLOCK/REDACT/WARN) and confidence,
// unlike the runtime regex/literal patterns. They are tested against the active
// policy via the simulate endpoint (empty YAML = active policy).
export function usePolicyEntities() {
  const qc = useQueryClient();
  const [adminKey] = useAdminKey();
  const [draft, setDraft] = useState<EntityDraft>(EMPTY_ENTITY);
  const [sampleText, setSampleText] = useState("");
  const [simResult, setSimResult] = useState<PolicySimulateResult | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ["tamga-custom-entities", adminKey],
    queryFn: () => api.listCustomEntities(adminKey),
    enabled: !!adminKey,
  });

  const createMut = useMutation({
    mutationFn: (d: EntityDraft) =>
      api.createCustomEntity(adminKey, {
        name: d.name,
        pattern: d.pattern,
        description: d.description || undefined,
        severity: d.severity,
        action: d.action,
        confidence: d.confidence,
      }),
    onSuccess: () => {
      toast.success("Custom entity created");
      setDraft(EMPTY_ENTITY);
      qc.invalidateQueries({ queryKey: ["tamga-custom-entities", adminKey] });
    },
    onError: (e: Error) => toast.error("Create failed", e.message),
  });

  const deleteMut = useMutation({
    mutationFn: (name: string) => api.deleteCustomEntity(adminKey, name),
    onSuccess: () => {
      toast.success("Custom entity deleted");
      qc.invalidateQueries({ queryKey: ["tamga-custom-entities", adminKey] });
    },
    onError: (e: Error) => toast.error("Delete failed", e.message),
  });

  const simulateMut = useMutation({
    // Empty YAML simulates against the active policy, so freshly created
    // entities are included.
    mutationFn: () => api.simulatePolicy(adminKey, "", sampleText),
    onSuccess: (res) => setSimResult(res),
    onError: (e: Error) => toast.error("Simulate failed", e.message),
  });

  function onSubmit() {
    if (!draft.name.trim() || !draft.pattern.trim()) {
      toast.error("Name and pattern required");
      return;
    }
    try {
      new RegExp(draft.pattern);
    } catch {
      toast.error("Invalid regex pattern");
      return;
    }
    createMut.mutate(draft);
  }

  function onSimulate() {
    if (!sampleText.trim()) {
      toast.error("Sample text is empty");
      return;
    }
    simulateMut.mutate();
  }

  return {
    adminKey,
    draft,
    setDraft,
    items: data?.items ?? [],
    isLoading,
    createMut,
    deleteMut,
    onSubmit,
    sampleText,
    setSampleText,
    simResult,
    onSimulate,
    simulating: simulateMut.isPending,
  };
}
