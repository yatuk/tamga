/**
 * Locale-independent case conversion.
 *
 * `toUpperCase()` / `toLowerCase()` follow the browser's locale. In a Turkish
 * locale "block".toUpperCase() is fine but "critical".toUpperCase() becomes
 * "CRİTİCAL" and "INPUT".toLowerCase() becomes "ınput", so comparisons against
 * API enums silently fail. Always go through these helpers for identifiers.
 */

const LOCALE = "en-US";

export function equalsIgnoreCase(a: string, b: string): boolean {
  return a.toLocaleUpperCase(LOCALE) === b.toLocaleUpperCase(LOCALE);
}

export function toUpperEn(s: string): string {
  return s.toLocaleUpperCase(LOCALE);
}

export function toLowerEn(s: string): string {
  return s.toLocaleLowerCase(LOCALE);
}
