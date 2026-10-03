"use client";

import { useRef, useState } from "react";
import type { SecurityEvent } from "@/lib/api";
import { useLocalStorageState } from "@/hooks/useLocalStorageState";
import { useStringParam } from "@/hooks/useUrlState";
import {
  INCIDENT_COMMENTS_STORAGE,
  INCIDENT_OPS_STORAGE,
  INCIDENT_TAGS_STORAGE,
  SAVED_VIEWS_STORAGE,
  type IncidentOpsState,
  type SavedView,
} from "@/lib/security/security-events-model";

export function useSecurityIncidentsLocalState() {
  // ── localStorage-persisted ──────────────────────────────────────────
  const [savedViews, setSavedViews] = useLocalStorageState<SavedView[]>(
    SAVED_VIEWS_STORAGE,
    [],
  );

  const [incidentOps, setIncidentOps] = useLocalStorageState<
    Record<string, IncidentOpsState>
  >(INCIDENT_OPS_STORAGE, {});

  const [commentsByRequest, setCommentsByRequest] = useLocalStorageState<
    Record<string, string[]>
  >(INCIDENT_COMMENTS_STORAGE, {});

  const [tagsByRequest, setTagsByRequest] = useLocalStorageState<
    Record<string, string[]>
  >(INCIDENT_TAGS_STORAGE, {});

  // ── transient UI state ──────────────────────────────────────────────
  const [selected, setSelected] = useState<SecurityEvent | null>(null);
  const [selectedRequestId, setSelectedRequestId] = useState("");
  const [selectedRow, setSelectedRow] = useState(0);
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [commentDraft, setCommentDraft] = useState("");
  const [showShortcuts, setShowShortcuts] = useState(false);
  // The free-text search is part of the view, so it lives in the URL.
  const [searchText, setSearchText] = useStringParam("q");

  // ── refs ────────────────────────────────────────────────────────────
  const searchInputRef = useRef<HTMLInputElement | null>(null);
  const goPrefixAtRef = useRef<number>(0);

  return {
    selected,
    setSelected,
    selectedRequestId,
    setSelectedRequestId,
    selectedRow,
    setSelectedRow,
    savedViews,
    setSavedViews,
    incidentOps,
    setIncidentOps,
    selectedIds,
    setSelectedIds,
    commentsByRequest,
    setCommentsByRequest,
    commentDraft,
    setCommentDraft,
    tagsByRequest,
    setTagsByRequest,
    showShortcuts,
    setShowShortcuts,
    searchText,
    setSearchText,
    searchInputRef,
    goPrefixAtRef,
  };
}
