"use client";

import * as React from "react";
import { ThemeProvider as NextThemesProvider } from "next-themes";

type ThemeMode = "light" | "dark" | "system";

export function ThemeProvider({
  children,
  defaultTheme = "system",
  attribute = "class",
  enableSystem = true,
}: {
  children: React.ReactNode;
  defaultTheme?: ThemeMode;
  attribute?: "class" | "data-theme";
  enableSystem?: boolean;
}) {
  return (
    <NextThemesProvider
      attribute={attribute}
      defaultTheme={defaultTheme}
      enableSystem={enableSystem}
      // The console has one theme for now: paper under an ink header.
      forcedTheme="light"
      disableTransitionOnChange
    >
      {children}
    </NextThemesProvider>
  );
}
