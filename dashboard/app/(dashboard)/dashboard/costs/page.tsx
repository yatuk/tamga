"use client";

import { CostsBody } from "./CostsBody";
import { useCostsPage } from "./useCostsPage";

export default function CostsPage() {
  const p = useCostsPage();
  return <CostsBody {...p} />;
}
