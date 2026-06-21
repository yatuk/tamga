"use client";

import { ScannerPoolBody } from "./ScannerPoolBody";
import { useScannerPoolPage } from "./useScannerPoolPage";

export default function ScannerPoolPage() {
  const p = useScannerPoolPage();
  return <ScannerPoolBody {...p} />;
}
