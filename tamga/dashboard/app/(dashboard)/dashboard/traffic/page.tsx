"use client";

import { TrafficBody } from "./TrafficBody";
import { useTrafficPage } from "./useTrafficPage";

export default function TrafficPage() {
  const p = useTrafficPage();
  return <TrafficBody {...p} />;
}
