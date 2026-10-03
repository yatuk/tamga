export const POLICY_DRAFT_STORAGE = "tamga_policy_draft_v1";
export const POLICY_SAMPLE_STORAGE = "tamga_policy_simulate_sample_v1";

export const POLICY_TAB_IDS = ["editor", "diff", "simulate", "history", "entities", "competitors"] as const;
export type PolicyTabKey = (typeof POLICY_TAB_IDS)[number];

export const POLICY_TABS: { id: PolicyTabKey; label: string }[] = [
  { id: "editor", label: "Editor" },
  { id: "diff", label: "Changes" },
  { id: "simulate", label: "Simulate" },
  { id: "history", label: "History" },
  { id: "entities", label: "Custom entities" },
  { id: "competitors", label: "Competitors" },
];
