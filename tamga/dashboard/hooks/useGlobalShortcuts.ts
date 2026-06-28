"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { toLowerEn } from "@/lib/utils/tr-string";

// ── Types ──────────────────────────────────────────────────────────────────────

interface ShortcutDef {
  key: string;
  ctrl?: boolean;
  shift?: boolean;
  description: string;
  action: () => void;
}

// ── Hook ───────────────────────────────────────────────────────────────────────

export function useGlobalShortcuts(shortcuts: ShortcutDef[], enabled = true) {
  useEffect(() => {
    if (!enabled) return;

    function handleKeyDown(e: KeyboardEvent) {
      // Don't intercept when typing in inputs
      const tag = (e.target as HTMLElement)?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT" || (e.target as HTMLElement)?.isContentEditable) {
        return;
      }

      for (const s of shortcuts) {
        const ctrlMatch = s.ctrl ? (e.ctrlKey || e.metaKey) : true;
        const shiftMatch = s.shift ? e.shiftKey : !e.shiftKey;
        const keyMatch =
          s.key.length === 1
            ? toLowerEn(e.key) === toLowerEn(s.key) && !e.ctrlKey && !e.metaKey
            : e.key === s.key;

        if (ctrlMatch && shiftMatch && keyMatch) {
          e.preventDefault();
          s.action();
          return;
        }
      }
    }

    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [shortcuts, enabled]);
}

// ── Dashboard-specific shortcuts ───────────────────────────────────────────────

export function useDashboardShortcuts(
  opts: {
    onToggleFocus?: () => void;
    onTogglePalette?: () => void;
    enabled?: boolean;
  } = {},
) {
  const router = useRouter();

  const shortcuts: ShortcutDef[] = [
    {
      key: "k",
      ctrl: true,
      description: "Open command palette",
      action: () => opts.onTogglePalette?.(),
    },
    {
      key: "F",
      shift: true,
      description: "Toggle focus mode",
      action: () => opts.onToggleFocus?.(),
    },
    {
      key: "d",
      ctrl: true,
      shift: true,
      description: "Go to dashboard overview",
      action: () => router.push("/dashboard"),
    },
    {
      key: "i",
      ctrl: true,
      shift: true,
      description: "Go to security incidents",
      action: () => router.push("/dashboard/security"),
    },
    {
      key: "p",
      ctrl: true,
      shift: true,
      description: "Go to policies",
      action: () => router.push("/dashboard/policies"),
    },
    {
      key: "h",
      ctrl: true,
      shift: true,
      description: "Go to threat hunting",
      action: () => router.push("/dashboard/hunting"),
    },
    {
      key: "r",
      ctrl: true,
      shift: true,
      description: "Go to reports",
      action: () => router.push("/dashboard/reports"),
    },
    {
      key: "?",
      description: "Show shortcuts help",
      action: () => {
        // Dispatch custom event for the shortcuts overlay to listen for
        window.dispatchEvent(new CustomEvent("tamga:toggle-shortcuts"));
      },
    },
  ];

  useGlobalShortcuts(shortcuts, opts.enabled !== false);

  return shortcuts;
}
