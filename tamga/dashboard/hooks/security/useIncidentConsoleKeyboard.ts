"use client";

import { useEffect } from "react";
import type { SecurityEvent } from "@/lib/api";
import type { IncidentOpsState } from "@/lib/security/security-events-model";
import type { MutableRefObject } from "react";
import { toLowerEn } from "@/lib/utils/tr-string";

type Args = {
  router: { push: (href: string) => void };
  tableRows: SecurityEvent[];
  selectedRow: number;
  showShortcuts: boolean;
  setSelectedRow: React.Dispatch<React.SetStateAction<number>>;
  setSelected: React.Dispatch<React.SetStateAction<SecurityEvent | null>>;
  setSelectedRequestId: React.Dispatch<React.SetStateAction<string>>;
  setShowShortcuts: React.Dispatch<React.SetStateAction<boolean>>;
  searchInputRef: MutableRefObject<HTMLInputElement | null>;
  goPrefixAtRef: MutableRefObject<number>;
  toggleRowSelection: (requestId: string) => void;
  setIncidentState: (requestId: string, next: Partial<IncidentOpsState>) => void;
  markFalsePositive: (requestId: string) => void;
};

export function useIncidentConsoleKeyboard({
  router,
  tableRows,
  selectedRow,
  showShortcuts,
  setSelectedRow,
  setSelected,
  setSelectedRequestId,
  setShowShortcuts,
  searchInputRef,
  goPrefixAtRef,
  toggleRowSelection,
  setIncidentState,
  markFalsePositive,
}: Args) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement | null;
      if (t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA")) return;
      if (e.key === "j") {
        e.preventDefault();
        setSelectedRow((p) => Math.min(tableRows.length - 1, p + 1));
      } else if (e.key === "k") {
        e.preventDefault();
        setSelectedRow((p) => Math.max(0, p - 1));
      } else if (toLowerEn(e.key) === "g") {
        goPrefixAtRef.current = Date.now();
      } else if (e.key === "Enter") {
        if (!tableRows[selectedRow]) return;
        e.preventDefault();
        const ev = tableRows[selectedRow];
        setSelected(ev);
        setSelectedRequestId(ev.request_id);
      } else if (e.key === "x") {
        if (!tableRows[selectedRow]) return;
        e.preventDefault();
        toggleRowSelection(tableRows[selectedRow].request_id);
      } else if (e.key === "a") {
        if (!tableRows[selectedRow]) return;
        e.preventDefault();
        const ev = tableRows[selectedRow];
        setIncidentState(ev.request_id, { assignee: "me", status: "In Progress" });
      } else if (e.key === "c") {
        if (!tableRows[selectedRow]) return;
        e.preventDefault();
        setIncidentState(tableRows[selectedRow].request_id, { status: "Closed" });
      } else if (e.key === "f") {
        if (!tableRows[selectedRow]) return;
        e.preventDefault();
        markFalsePositive(tableRows[selectedRow].request_id);
      } else if (e.key === "/") {
        e.preventDefault();
        searchInputRef.current?.focus();
      } else if (e.key === "?") {
        e.preventDefault();
        setShowShortcuts((v) => !v);
      } else if (toLowerEn(e.key) === "o") {
        if (Date.now() - goPrefixAtRef.current < 1200) {
          e.preventDefault();
          router.push("/dashboard");
        }
      } else if (toLowerEn(e.key) === "i") {
        if (Date.now() - goPrefixAtRef.current < 1200) {
          e.preventDefault();
          router.push("/dashboard/security");
        }
      } else if (e.key === "Escape") {
        setSelected(null);
        setSelectedRequestId("");
        setShowShortcuts(false);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [
    router,
    tableRows,
    selectedRow,
    showShortcuts,
    setSelectedRow,
    setSelected,
    setSelectedRequestId,
    setShowShortcuts,
    searchInputRef,
    goPrefixAtRef,
    toggleRowSelection,
    setIncidentState,
    markFalsePositive,
  ]);
}
