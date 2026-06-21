"use client";

import { LatencyBody } from "./LatencyBody";
import { useLatencyPage } from "./useLatencyPage";

export default function LatencyPage() {
  const p = useLatencyPage();
  return <LatencyBody {...p} />;
}
