"use client";

import { ReportsBody } from "./ReportsBody";
import { useReportsPage } from "./useReportsPage";

export default function ReportsPage() {
  const p = useReportsPage();
  return <ReportsBody {...p} />;
}
