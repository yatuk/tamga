import type { Metadata } from "next";
import { notFound } from "next/navigation";

export const metadata: Metadata = { title: "Page not found" };

// An address under /dashboard that no page answers. Sending it to notFound
// here draws the console's own "no page" inside the shell, not the bare one.
export default function Missing() {
  notFound();
}
