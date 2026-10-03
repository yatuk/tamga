"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, type GetEventsQuery } from "@/lib/api";
import type { SavedHunt } from "@/lib/api/types-extended";
import { useAdminKey } from "@/hooks/useAdminKey";
import { useDebouncedValue } from "@/hooks/useDebouncedValue";
import { useRangeParam } from "@/hooks/useRangeParam";
import { useFlagParam, useStringParam } from "@/hooks/useUrlState";
import { toast } from "@/lib/toast";
import { toLowerEn } from "@/lib/utils/case";
import { PAGE_SIZE } from "./_constants";
import { deleteHunt as apiDeleteHunt, loadHunts, saveHunt as apiSaveHunt } from "./huntingStorage";

/** The text filters of a hunt. Each one is a URL parameter of the same name. */
export type HuntFilters = {
  action: string;
  provider: string;
  finding_type: string;
  severity: string;
  category: string;
  technique: string;
  q: string;
};
export type HuntFilterKey = keyof HuntFilters;

export function useHuntingPage() {
  const [adminKey] = useAdminKey();
  const [page, setPage] = useState(1);
  const [action, setAction] = useStringParam("action");
  const [provider, setProvider] = useStringParam("provider");
  const [findingType, setFindingType] = useStringParam("finding_type");
  const [severity, setSeverity] = useStringParam("severity");
  const [category, setCategory] = useStringParam("category");
  const [technique, setTechnique] = useStringParam("technique");
  const [q, setQ] = useStringParam("q");
  const [shadow, setShadowFlag] = useFlagParam("shadow");
  const [range, setRangeParam] = useRangeParam("7d");
  const [savedHunts, setSavedHunts] = useState<SavedHunt[]>([]);

  const filters: HuntFilters = useMemo(
    () => ({ action, provider, finding_type: findingType, severity, category, technique, q }),
    [action, provider, findingType, severity, category, technique, q],
  );

  const setFilter = (key: HuntFilterKey, value: string) => {
    const setters: Record<HuntFilterKey, (v: string) => void> = {
      action: setAction,
      provider: setProvider,
      finding_type: setFindingType,
      severity: setSeverity,
      category: setCategory,
      technique: setTechnique,
      q: setQ,
    };
    setters[key](value);
    setPage(1);
  };

  const setShadow = (on: boolean) => {
    setShadowFlag(on);
    // "Shadow only" replaces a provider filter; the two cannot combine.
    if (on) setProvider("");
    setPage(1);
  };

  const setRange = (next: typeof range) => {
    setRangeParam(next);
    setPage(1);
  };

  useEffect(() => {
    let cancelled = false;
    loadHunts(adminKey)
      .then((hunts) => {
        if (!cancelled) setSavedHunts(hunts);
      })
      .catch(() => {
        if (!cancelled) setSavedHunts([]);
      });
    return () => {
      cancelled = true;
    };
  }, [adminKey]);

  /** The filters as an events query, without paging. Empty filters are left out. */
  const buildQuery = (f: HuntFilters): GetEventsQuery => {
    const query: GetEventsQuery = { range };
    if (f.action.trim()) query.action = f.action.trim();
    if (shadow) query.shadow = true;
    else if (f.provider.trim()) query.provider = toLowerEn(f.provider.trim());
    if (f.finding_type.trim()) query.finding_type = f.finding_type.trim();
    if (f.severity.trim()) query.severity = f.severity.trim();
    if (f.category.trim()) query.category = f.category.trim();
    if (f.technique.trim()) query.technique = f.technique.trim();
    if (f.q.trim()) query.q = f.q.trim();
    return query;
  };

  // Typing should not send a request per keystroke.
  const debouncedFilters = useDebouncedValue(filters);
  const queryParams = { ...buildQuery(debouncedFilters), page, limit: PAGE_SIZE };

  const { data, isLoading, error, refetch, isFetching } = useQuery({
    queryKey: ["tamga-hunting-events", adminKey, queryParams],
    queryFn: () => api.getEvents(adminKey, queryParams),
    enabled: !!adminKey,
    staleTime: 15_000,
    retry: 1,
  });

  const activeFilterCount = Object.values(filters).filter((v) => v.trim()).length + (shadow ? 1 : 0);

  const clearFilters = () => {
    (Object.keys(filters) as HuntFilterKey[]).forEach((key) => setFilter(key, ""));
    setShadowFlag(false);
  };

  const applyHunt = useCallback(
    (h: SavedHunt) => {
      setAction(h.query?.action || "");
      setProvider(h.query?.provider || "");
      setShadowFlag(!!h.query?.shadow);
      setFindingType(h.query?.finding_type || "");
      setSeverity(h.query?.severity || "");
      setCategory(h.query?.category || "");
      setTechnique(h.query?.technique || "");
      setQ(h.query?.q || "");
      if (h.query?.range) setRangeParam(h.query.range);
      setPage(1);
    },
    // The URL setters are stable for a given key; listing them would only
    // rebuild this callback on every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  );

  const saveHunt = async (name: string) => {
    const trimmed = name.trim();
    if (!trimmed) return;
    const created = await apiSaveHunt(adminKey, trimmed, buildQuery(filters));
    if (created) {
      setSavedHunts((prev) => [created, ...prev].slice(0, 16));
      toast.success("Hunt saved", trimmed);
    }
  };

  const deleteHunt = async (id: string) => {
    await apiDeleteHunt(adminKey, id);
    setSavedHunts((prev) => prev.filter((x) => x.id !== id));
  };

  return {
    adminKey,
    page,
    setPage,
    filters,
    setFilter,
    shadow,
    setShadow,
    range,
    setRange,
    activeFilterCount,
    clearFilters,
    savedHunts,
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
