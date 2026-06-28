"use client";

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, type CustomPattern, type PatternKind } from "@/lib/api";
import { toast } from "@/lib/toast";
import { toLowerEn } from "@/lib/utils/tr-string";
import { EMPTY_DRAFT, type Draft } from "./_constants";
import { useAdminKey } from "@/hooks/useAdminKey";

export function usePatternsPage() {
  const qc = useQueryClient();
  const [adminKey] = useAdminKey();
  const [draft, setDraft] = useState<Draft>(EMPTY_DRAFT);
  const [testInput, setTestInput] = useState("");
  const [testMatch, setTestMatch] = useState<string | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ["tamga-patterns", adminKey],
    queryFn: () => api.listPatterns(adminKey),
    enabled: !!adminKey,
  });

  const createMut = useMutation({
    mutationFn: (d: Draft) =>
      api.createPattern(adminKey, {
        name: d.name,
        kind: d.kind,
        pattern: d.pattern,
        severity: d.severity,
        enabled: d.enabled,
      }),
    onSuccess: () => {
      toast.success("Pattern created");
      setDraft(EMPTY_DRAFT);
      qc.invalidateQueries({ queryKey: ["tamga-patterns", adminKey] });
    },
    onError: (e: Error) => toast.error("Create failed", e.message),
  });

  const updateMut = useMutation({
    mutationFn: ({ id, d }: { id: string; d: Draft }) =>
      api.updatePattern(adminKey, id, {
        name: d.name,
        kind: d.kind,
        pattern: d.pattern,
        severity: d.severity,
        enabled: d.enabled,
      }),
    onSuccess: () => {
      toast.success("Pattern updated");
      setDraft(EMPTY_DRAFT);
      qc.invalidateQueries({ queryKey: ["tamga-patterns", adminKey] });
    },
    onError: (e: Error) => toast.error("Update failed", e.message),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => api.deletePattern(adminKey, id),
    onSuccess: () => {
      toast.success("Pattern deleted");
      qc.invalidateQueries({ queryKey: ["tamga-patterns", adminKey] });
    },
    onError: (e: Error) => toast.error("Delete failed", e.message),
  });

  const items = data?.items ?? [];

  const compiledRegex = useMemo(() => {
    if (draft.kind !== "regex" || !draft.pattern) return null;
    try {
      return new RegExp(draft.pattern, "gi");
    } catch {
      return "invalid" as const;
    }
  }, [draft.kind, draft.pattern]);

  function onSubmit() {
    if (!draft.name.trim() || !draft.pattern.trim()) {
      toast.error("Name and pattern required");
      return;
    }
    if (draft.id) {
      updateMut.mutate({ id: draft.id, d: draft });
    } else {
      createMut.mutate(draft);
    }
  }

  function onTest() {
    if (!draft.pattern) {
      toast.error("Pattern is empty");
      return;
    }
    if (draft.kind === "regex") {
      if (compiledRegex === "invalid" || !compiledRegex) {
        toast.error("Invalid regex");
        return;
      }
      const m = testInput.match(compiledRegex);
      setTestMatch(m && m.length ? m.join(", ") : "no match");
      return;
    }
    const idx = toLowerEn(testInput).indexOf(toLowerEn(draft.pattern));
    setTestMatch(idx >= 0 ? `matched @ ${idx}` : "no match");
  }

  function editPattern(p: CustomPattern) {
    setDraft({
      id: p.id,
      name: p.name,
      kind: p.kind,
      pattern: p.pattern,
      severity: p.severity,
      enabled: p.enabled,
    });
  }

  function setDraftKind(kind: PatternKind) {
    setDraft((d) => ({ ...d, kind }));
  }

  return {
    draft,
    setDraft,
    setDraftKind,
    testInput,
    setTestInput,
    testMatch,
    items,
    isLoading,
    compiledRegex,
    createMut,
    updateMut,
    deleteMut,
    onSubmit,
    onTest,
    editPattern,
  };
}
