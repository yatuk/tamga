import type { WebhookKind } from "@/lib/api";

// IntegrationGuide is the data contract backing every per-provider
// tutorial page under `/dashboard/integrations/[kind]`. Keep it UI-free —
// the rendering is done by `GuideView`. Each guide carries a
// `lastVerified` stamp so stale instructions can be spotted quickly.
export type IntegrationGuide = {
  kind: WebhookKind;
  name: string;
  badge: string;
  overview: string;
  lastVerified: string;
  docsLinks: { label: string; href: string }[];
  urlHint: string;
  prerequisites: string[];
  steps: {
    title: string;
    body: string;
    code?: { lang: "bash" | "json" | "yaml" | "http" | "text"; content: string };
    note?: string;
  }[];
  payloadPreview: { lang: "json" | "text"; content: string };
  headers?: { key: string; valueHint: string; note?: string }[];
  gotchas: { title: string; body: string }[];
};

export const LAST_VERIFIED = "2026-04-17";
