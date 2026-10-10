import { ApiError, permissionDenied, sessionRefused } from "@/lib/api/fetch-core";

/**
 * Why a PDF report was not made, as a sentence to put beside its button.
 *
 * The proxy does not make the PDF itself: it reads the counts for the window
 * and hands them to its report service, the analyzer. Each way that can fail
 * comes back with a code, and each is said for what it is, so that "the
 * report service is not set up" is not read as "try again".
 */
const WORDS: Record<string, string> = {
  analyzer_not_configured: "The proxy has no report service set up, so it cannot make this PDF. Point it at an analyzer and try again.",
  analyzer_unavailable: "The report service is not reachable from the proxy.",
  analyzer_timeout: "The report service did not answer the proxy in time. Try again.",
  analyzer_error: "The report service failed while making the PDF.",
  report_counts_unavailable: "The proxy could not read the counts for this window, so the report was not made.",
  invalid_range: "The proxy does not make this report for that window. Choose 24 hours, 7 days or 30 days.",
};

export function reportFailure(error: unknown): string {
  const refused = (permissionDenied(error) ?? sessionRefused(error))?.refused;
  if (refused) return refused;
  if (!(error instanceof ApiError)) return "The proxy did not answer, so the report was not made.";
  const known = WORDS[error.code];
  if (known) return known;
  // A 503 with any other code comes from before the report service is asked: the counts.
  if (error.status === 503 && error.code) return "The proxy could not read the counts for this window, so the report was not made.";
  if (error.status === 401) return "The proxy did not accept the admin key. Check the one under Settings.";
  if (error.status === 400) return `The proxy did not accept the request for this report: ${error.message}.`;
  return `The report was not made. The proxy answered ${error.status}.`;
}
