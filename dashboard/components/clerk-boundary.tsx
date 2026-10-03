"use client";

import dynamic from "next/dynamic";

// Clerk's provider sits in a chunk of its own, requested only when this
// component renders. Importing it from the root layout directly put about
// 37 kB (gzip) of Clerk into every page, with or without Clerk configured.
const ClerkProvider = dynamic(() => import("@clerk/nextjs").then((m) => m.ClerkProvider));

/** Wraps the app in Clerk. Rendered by the root layout only when a real key is configured. */
export function ClerkBoundary({ children }: { children: React.ReactNode }) {
  return <ClerkProvider>{children}</ClerkProvider>;
}
