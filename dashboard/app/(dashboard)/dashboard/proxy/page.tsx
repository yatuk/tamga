"use client";

import { ProxyBody } from "./ProxyBody";
import { useProxyPage } from "./useProxyPage";

export default function ProxyPage() {
  const p = useProxyPage();
  return <ProxyBody {...p} />;
}
