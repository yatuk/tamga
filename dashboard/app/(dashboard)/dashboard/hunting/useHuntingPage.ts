"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, type GetEventsQuery } from "@/lib/api";
import type { SavedHunt } from "@/lib/api/types-extended";
import { toast } from "@/lib/toast";
import { toLowerEn } from "@/lib/utils/tr-string";
import { PAGE_SIZE } from "./_constants";
import { useAdminKey } from "@/hooks/useAdminKey";
import type { TimeRange } from "@/lib/types";
import { loadHunts, saveHunt as apiSaveHunt, deleteHunt as apiDeleteHunt } from "./huntingStorage";

export function useHuntingPage() {
  const [adminKey] = useAdminKey();
  const [page, setPage] = useState(1);
  const [action, setAction] = useState("");
  const [provider, setProvider] = useState("");
  const [shadow, setShadow] = useState(false);
  const [findingType, setFindingType] = useState("");
  const [severity, setSeverity] = useState("");
  const [category, setCategory] = useState("");
  const [technique, setTechnique] = useState("");
  const [q, setQ] = useState("");
  const [range, setRange] = useState<TimeRange>("7d");
  const [savedHunts, setSavedHunts] = useState<SavedHunt[]>([]);
  const [huntsLoading, setHuntsLoading] = useState(true);

  useEffect(() => {
    // Load saved hunts from API (primary) or localStorage (fallback).
    if (adminKey) {
      loadHunts(adminKey)
        .then(setSavedHunts)
        .catch(() => setSavedHunts([]))
        .finally(() => setHuntsLoading(false));
    } else {
      loadHunts("")
        .then(setSavedHunts)
        .finally(() => setHuntsLoading(false));
    }
  }, [adminKey]);

  const queryParams = useMemo((): GetEventsQuery & { page: number; limit: number } => {
    const base: GetEventsQuery & { page: number; limit: number } = {
      page,
      limit: PAGE_SIZE,
      range,
    };
    if (action.trim()) base.action = action.trim();
    if (shadow) {
      base.shadow = true;
    } else if (provider.trim()) {
      base.provider = toLowerEn(provider.trim());
    }
    if (findingType.trim()) base.finding_type = findingType.trim();
    if (severity.trim()) base.severity = severity.trim();
    if (category.trim()) base.category = category.trim();
    if (technique.trim()) base.technique = technique.trim();
    if (q.trim()) base.q = q.trim();
    return base;
  }, [page, action, provider, shadow, findingType, severity, category, technique, q, range]);

  const { data, isLoading, error, refetch, isFetching } = useQuery({
    queryKey: ["tamga-hunting-events", adminKey, queryParams],
    queryFn: () => api.getEvents(adminKey, queryParams),
    enabled: !!adminKey,
    staleTime: 15_000,
    retry: 1,
  });

  const applyHunt = useCallback((h: SavedHunt) => {
    setAction(h.query?.action || "");
    setProvider(h.query?.provider || "");
    setShadow(!!h.query?.shadow);
    setFindingType(h.query?.finding_type || "");
    setSeverity(h.query?.severity || "");
    setCategory(h.query?.category || "");
    setTechnique(h.query?.technique || "");
    setQ(h.query?.q || "");
    if (h.query?.range) setRange(h.query.range);
    setPage(1);
  }, []);

  const saveHunt = async (name?: string) => {
    const n = (name || "Suspicious PII + shadow").trim();
    if (!n) return;
    const query: GetEventsQuery = {};
    if (action.trim()) query.action = action.trim();
    if (!shadow && provider.trim()) query.provider = toLowerEn(provider.trim());
    if (shadow) query.shadow = true;
    if (findingType.trim()) query.finding_type = findingType.trim();
    if (severity.trim()) query.severity = severity.trim();
    if (category.trim()) query.category = category.trim();
    if (technique.trim()) query.technique = technique.trim();
    if (q.trim()) query.q = q.trim();
    query.range = range;

    const created = await apiSaveHunt(adminKey, n, query);
    if (created) {
      setSavedHunts((prev) => [created, ...prev].slice(0, 16));
      toast.success("Hunt kaydedildi");
    }
  };

  const deleteHunt = async (id: string) => {
    await apiDeleteHunt(adminKey, id);
    setSavedHunts((prev) => prev.filter((x) => x.id !== id));
  };

  return {
    page,
    setPage,
    action,
    setAction,
    provider,
    setProvider,
    shadow,
    setShadow,
    findingType,
    setFindingType,
    severity,
    setSeverity,
    category,
    setCategory,
    technique,
    setTechnique,
    q,
    setQ,
    range,
    setRange,
    savedHunts,
    huntsLoading,
    data,
    isLoading,
    error,
    refetch,
    isFetching,
    applyHunt,
    saveHunt,
    deleteHunt,
  };
}
