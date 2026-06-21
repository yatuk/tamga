export const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8443";
export const RETENTION_STORAGE = "tamga_retention_days_v1";

export type SettingsTabKey = "access" | "raw-webhooks" | "retention" | "providers" | "runtime" | "sso";

export const SETTINGS_TABS: { id: SettingsTabKey; label: string }[] = [
  { id: "access", label: "Access" },
  { id: "raw-webhooks", label: "Raw Webhooks" },
  { id: "retention", label: "Retention" },
  { id: "providers", label: "Providers" },
  { id: "runtime", label: "Runtime" },
  { id: "sso", label: "SSO" },
];
