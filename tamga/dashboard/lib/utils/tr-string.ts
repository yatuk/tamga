/**
 * Locale-safe string transformations for Turkish UI.
 *
 * ## The Problem
 *
 * JavaScript's `String.prototype.toUpperCase()` and `.toLowerCase()` use the
 * **browser's default locale**, not the string's language. In a Turkish browser
 * (or any browser with Turkish as the preferred language), these methods apply
 * Turkish casing rules:
 *
 *   "INPUT".toLowerCase()   → "ınput"   (dotless ı, NOT "input")
 *   "critical".toUpperCase() → "CRİTİCAL" (dotted İ, NOT "CRITICAL")
 *   "istanbul".toUpperCase() → "İSTANBUL" (capital İ with dot)
 *
 * This means ANY `.toUpperCase() === "BLOCK"` comparison silently breaks in
 * a Turkish environment when the string contains 'I' or 'i'. The proxy sends
 * actions like "block", "redact", "warn", "pass" — all ASCII-safe — but
 * provider names ("openai"), model names ("claude-sonnet"), request IDs, and
 * user input may contain these characters.
 *
 * ## The Solution
 *
 * Always use an EXPLICIT locale:
 * - `"en-US"` for data comparison (ASCII-safe, deterministic across all
 *   environments)
 * - `"tr-TR"` for display text shown to Turkish-speaking users
 *
 * CSS `text-transform: uppercase` (Tailwind `uppercase` class) is locale-aware
 * in all modern browsers and correctly handles Turkish İ/ı. Those 220+ usages
 * are safe and require no changes.
 */

const COMPARE_LOCALE = "en-US";
const DISPLAY_LOCALE = "tr-TR";

/** Case-insensitive equality check that is always locale-safe. */
export function equalsIgnoreCase(a: string, b: string): boolean {
  return a.toLocaleUpperCase(COMPARE_LOCALE) === b.toLocaleUpperCase(COMPARE_LOCALE);
}

/** Convert to uppercase using English locale. Use for DATA COMPARISON. */
export function toUpperEn(s: string): string {
  return s.toLocaleUpperCase(COMPARE_LOCALE);
}

/** Convert to lowercase using English locale. Use for DATA COMPARISON. */
export function toLowerEn(s: string): string {
  return s.toLocaleLowerCase(COMPARE_LOCALE);
}

/** Convert to uppercase using Turkish locale. Use for DISPLAY TEXT. */
export function toUpperLocale(s: string): string {
  return s.toLocaleUpperCase(DISPLAY_LOCALE);
}

/** Convert to lowercase using Turkish locale. Use for DISPLAY TEXT. */
export function toLowerLocale(s: string): string {
  return s.toLocaleLowerCase(DISPLAY_LOCALE);
}
