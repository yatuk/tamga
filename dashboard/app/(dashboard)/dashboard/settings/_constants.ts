export const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8443";
export const RETENTION_STORAGE = "tamga_retention_days_v1";

export const SETTINGS_TAB_IDS = ["access", "retention", "providers", "runtime", "sso"] as const;
export type SettingsTabKey = (typeof SETTINGS_TAB_IDS)[number];

export const SETTINGS_TABS: { id: SettingsTabKey; label: string }[] = [
  { id: "access", label: "Access" },
  { id: "retention", label: "Retention" },
  { id: "providers", label: "Providers" },
  { id: "runtime", label: "Runtime" },
  { id: "sso", label: "SSO" },
];
