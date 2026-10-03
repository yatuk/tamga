export type KeyScope = "proxy" | "read" | "write" | "admin";

export const SCOPE_LABELS: Record<KeyScope, string> = {
  proxy: "Application",
  read: "Read-only",
  write: "Read & Write",
  admin: "Full Admin",
};
