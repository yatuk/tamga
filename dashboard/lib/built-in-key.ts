/**
 * The admin key built into the page, for local development only.
 *
 * Next.js writes the value of a NEXT_PUBLIC_ variable into the JavaScript
 * every visitor downloads, wherever the variable is named. So it is named
 * here and nowhere else, behind a test the bundler settles at build time: in
 * a production build `process.env.NODE_ENV` is the literal "production", the
 * first branch is dead code and is dropped, and the constant is "". The
 * production build also stops before it starts when the variable is set: see
 * built-in-key-guard.js.
 */
export const BUILT_IN_ADMIN_KEY: string =
  process.env.NODE_ENV !== "production" ? process.env.NEXT_PUBLIC_ADMIN_KEY || "" : "";

/**
 * Where the admin key this console sends comes from. A key a person saved
 * under Settings is kept in this browser's storage and wins over the built-in
 * one; "none" is the only other state in a production build. The console says
 * which of the three applies and never shows any part of the key.
 */
export type AdminKeySource = "stored" | "built-in" | "none";

export function adminKeySource(storedKey: string, builtInKey: string = BUILT_IN_ADMIN_KEY): AdminKeySource {
  return storedKey ? "stored" : builtInKey ? "built-in" : "none";
}

if (BUILT_IN_ADMIN_KEY && typeof window !== "undefined") {
  console.warn(
    "Tamga console: NEXT_PUBLIC_ADMIN_KEY is built into this page. That is for local development only; a production build refuses it. Enter a key under Settings instead.",
  );
}
