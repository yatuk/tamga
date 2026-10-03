import type { PatternKind, PatternSeverity } from "@/lib/api";

export type Draft = {
  id?: string;
  name: string;
  kind: PatternKind;
  pattern: string;
  severity: PatternSeverity;
  enabled: boolean;
};

export const EMPTY_DRAFT: Draft = {
  name: "",
  kind: "regex",
  pattern: "",
  severity: "medium",
  enabled: true,
};
