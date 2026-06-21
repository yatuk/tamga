"use client";

import { DashboardLayoutShell } from "@/components/dashboard/DashboardLayoutShell";

export default function DashboardLayout({ children }: { children: React.ReactNode }) {
  return <DashboardLayoutShell>{children}</DashboardLayoutShell>;
}
