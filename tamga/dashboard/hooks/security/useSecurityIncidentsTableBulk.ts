"use client";

import { toast } from "sonner";
import { api, type IncidentStatus } from "@/lib/api";
import type { SecurityIncidentsDataLayer } from "@/hooks/security/useSecurityIncidentsDataLayer";
import type { TriageStatus } from "@/lib/security/security-events-model";

export function useSecurityIncidentsTableBulk(L: SecurityIncidentsDataLayer) {
  const {
    adminKey,
    setIncidentOps,
    selectedIds,
    setSelectedIds,
    filtered,
    selectedRequestId,
    commentDraft,
    setCommentDraft,
    setCommentsByRequest,
    setTagsByRequest,
  } = L;

  const toggleRowSelection = (requestId: string) => {
    setSelectedIds((prev) =>
      prev.includes(requestId) ? prev.filter((id) => id !== requestId) : [...prev, requestId],
    );
  };

  const toggleSelectAllVisible = () => {
    const visibleIds = filtered.map((e) => e.request_id);
    if (visibleIds.length === 0) return;
    const allSelected = visibleIds.every((id) => selectedIds.includes(id));
    if (allSelected) setSelectedIds((prev) => prev.filter((id) => !visibleIds.includes(id)));
    else setSelectedIds((prev) => Array.from(new Set([...prev, ...visibleIds])));
  };

  const applyBulkStatus = async (status: TriageStatus) => {
    if (selectedIds.length === 0) return;
    const ids = [...selectedIds];
    setIncidentOps((prev) => {
      const next = { ...prev };
      for (const id of ids) {
        const current = next[id] || { status: "Open" as TriageStatus, assignee: "unassigned" };
        next[id] = { ...current, status };
      }
      return next;
    });
    if (!adminKey) return;
    const results = await Promise.allSettled(
      ids.map((id) => api.patchIncident(adminKey, id, { status: status as IncidentStatus })),
    );
    const failed = results.filter((r) => r.status === "rejected").length;
    if (failed > 0) toast.error(`${failed}/${ids.length} olay sunucuda güncellenemedi`);
    else toast.success(`${ids.length} olay -> ${status}`);
  };

  const bulkAssignMe = async () => {
    if (selectedIds.length === 0) return;
    const ids = [...selectedIds];
    setIncidentOps((prev) => {
      const next = { ...prev };
      for (const id of ids) {
        const current = next[id] || { status: "Open" as TriageStatus, assignee: "unassigned" };
        next[id] = {
          ...current,
          assignee: "me",
          status: current.status === "Open" ? "In Progress" : current.status,
        };
      }
      return next;
    });
    if (!adminKey) return;
    const results = await Promise.allSettled(
      ids.map((id) =>
        api.patchIncident(adminKey, id, { assignee: "me", status: "In Progress" as IncidentStatus }),
      ),
    );
    const failed = results.filter((r) => r.status === "rejected").length;
    if (failed > 0) toast.error(`${failed}/${ids.length} atama başarısız`);
  };

  const addCommentToSelected = () => {
    if (!selectedRequestId || !commentDraft.trim()) return;
    const text = commentDraft.trim();
    setCommentsByRequest((prev) => ({
      ...prev,
      [selectedRequestId]: [text, ...(prev[selectedRequestId] || [])].slice(0, 20),
    }));
    if (adminKey) {
      api
        .patchIncident(adminKey, selectedRequestId, { add_comment: { author: "me", text } })
        .catch((err) => toast.error(`Yorum kaydedilemedi: ${String(err?.message || err)}`));
    }
    setCommentDraft("");
  };

  const addTagToSelected = (tagName?: string) => {
    if (!selectedRequestId) return;
    const tag = (tagName || "investigation").trim();
    if (!tag) return;
    const t = tag;
    setTagsByRequest((prev) => {
      const current = prev[selectedRequestId] || [];
      if (current.includes(t)) return prev;
      const nextTags = [t, ...current].slice(0, 8);
      if (adminKey) {
        api
          .patchIncident(adminKey, selectedRequestId, { tags: nextTags })
          .catch((err) => toast.error(`Tag kaydedilemedi: ${String(err?.message || err)}`));
      }
      return { ...prev, [selectedRequestId]: nextTags };
    });
  };

  return {
    toggleRowSelection,
    toggleSelectAllVisible,
    applyBulkStatus,
    bulkAssignMe,
    addCommentToSelected,
    addTagToSelected,
  };
}
