export type KeyScope = "read" | "write" | "admin";

export const SCOPE_LABELS: Record<KeyScope, string> = {
  read: "Read-only",
  write: "Read & Write",
  admin: "Full Admin",
};
