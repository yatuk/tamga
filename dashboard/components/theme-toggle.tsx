"use client";

import { Moon, Sun } from "lucide-react";
import { useTheme } from "next-themes";
import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";

/** Dark / light switch. Cross-fades through a view transition where supported. */
export function ThemeToggle() {
  const { setTheme, resolvedTheme } = useTheme();
  const [mounted, setMounted] = useState(false);
  useEffect(() => setMounted(true), []);

  // The theme is only known in the browser. Until the component has mounted,
  // render the dark state the server rendered, so hydration matches.
  const dark = !mounted || resolvedTheme !== "light";

  function toggle() {
    if (!mounted) return;
    const next = dark ? "light" : "dark";
    const apply = () => {
      setTheme(next);
      try {
        // The server reads this cookie to render the right theme on first paint.
        document.cookie = `tamga-theme=${next}; path=/; max-age=31536000; samesite=lax`;
      } catch {
        /* cookies disabled: next-themes still keeps the choice in localStorage */
      }
    };
    const doc = document as Document & { startViewTransition?: (cb: () => void) => unknown };
    const reduced = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches;
    if (reduced || typeof doc.startViewTransition !== "function") apply();
    else doc.startViewTransition(apply);
  }

  return (
    <Button
      variant="outline"
      size="icon-sm"
      onClick={toggle}
      aria-label={dark ? "Switch to light theme" : "Switch to dark theme"}
      aria-pressed={!dark}
    >
      {dark ? <Sun /> : <Moon />}
    </Button>
  );
}
