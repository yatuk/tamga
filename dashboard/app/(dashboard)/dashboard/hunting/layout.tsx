import type { Metadata } from "next";
import type { ReactNode } from "react";

// The page is a client component, which cannot name its own tab.
export const metadata: Metadata = { title: "Threat hunting" };

export default function Layout({ children }: { children: ReactNode }) {
  return children;
}
