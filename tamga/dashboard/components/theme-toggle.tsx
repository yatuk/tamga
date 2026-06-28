"use client";

import { Moon, Sun } from "lucide-react";
import { motion } from "framer-motion";
import { useTheme } from "next-themes";
import { useEffect, useState } from "react";

// Strict 2-state theme toggle: dark | light. Tamga is dark-first, but light
// mode is fully themed via OKLCH tokens in globals.css. System preference
// is honored for first paint (via next-themes), then the user's explicit
// choice takes over.
//
// Uses `document.startViewTransition` when available so the flip doesn't
// snap — chart axes, hash-chain rows, etc. cross-fade instead of jumping.

type ResolvedTheme = "dark" | "light";

function applyThemeWithTransition(next: ResolvedTheme, apply: () => void) {
  if (typeof document === "undefined") {
    apply();
    return;
  }
  const prefersReduced = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches;
  const doc = document as Document & {
    startViewTransition?: (cb: () => void) => { finished: Promise<void> };
  };
  if (prefersReduced || typeof doc.startViewTransition !== "function") {
    apply();
    return;
  }
  doc.startViewTransition(() => apply());
}

export function ThemeToggle() {
  const { setTheme, resolvedTheme } = useTheme();
  const [mounted, setMounted] = useState(false);

  useEffect(() => setMounted(true), []);

  const dark = resolvedTheme !== "light";

  return (
    <button
      type="button"
      aria-label={dark ? "Switch to light theme" : "Switch to dark theme"}
      aria-pressed={!dark}
      onClick={() => {
        if (!mounted) return;
        const next: ResolvedTheme = dark ? "light" : "dark";
        applyThemeWithTransition(next, () => {
          setTheme(next);
          try {
            document.cookie = `tamga-theme=${next}; path=/; max-age=31536000; samesite=lax`;
          } catch {
            /* cookies disabled — resolution still lives in localStorage via next-themes */
          }
        });
      }}
      className="inline-flex h-8 w-8 cursor-pointer items-center justify-center rounded-sm border border-border bg-surface-subtle text-fg-muted transition-colors duration-200 hover:border-border-strong hover:bg-surface-card hover:text-fg"
    >
      <motion.span
        key={mounted ? (dark ? "sun" : "moon") : "placeholder"}
        initial={{ rotate: -45, opacity: 0.4 }}
        animate={{ rotate: 0, opacity: 1 }}
        transition={{ type: "spring", damping: 22, stiffness: 320, duration: 0.2 }}
        className="inline-flex"
      >
        {mounted ? (
          dark ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />
        ) : (
          <Sun className="h-4 w-4" />
        )}
      </motion.span>
    </button>
  );
}
