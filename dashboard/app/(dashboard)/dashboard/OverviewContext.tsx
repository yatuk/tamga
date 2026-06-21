"use client";

import { createContext, useContext, type ReactNode } from "react";
import type { useOverviewPage } from "./useOverviewPage";

type OverviewCtx = ReturnType<typeof useOverviewPage>;

const OverviewContext = createContext<OverviewCtx | null>(null);

export function OverviewProvider({ value, children }: { value: OverviewCtx; children: ReactNode }) {
  return <OverviewContext.Provider value={value}>{children}</OverviewContext.Provider>;
}

export function useOverviewContext() {
  const v = useContext(OverviewContext);
  if (!v) throw new Error("useOverviewContext must be used within OverviewProvider");
  return v;
}
