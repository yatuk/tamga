import type { Metadata } from "next";
import { DocsContent } from "./DocsContent";

export const metadata: Metadata = {
  title: "Docs — Tamga",
  description: "Tamga AI Security Proxy — kurulum, policy modeli, entegrasyon, admin API ve uyumluluk rehberi.",
};

export default function DocsPage() {
  return <DocsContent />;
}
