import { test } from "@playwright/test";

/**
 * The tests in this folder drive the console against a real proxy, to prove
 * that saving, creating and deleting work end to end. They change the proxy's
 * state, so they only run when asked for, never in CI:
 *
 *   E2E_LIVE=1 E2E_BROWSER_CHANNEL=msedge npx playwright test e2e/live
 *
 * Every test cleans up what it creates. Names it creates start with PREFIX.
 */
export const PREFIX = "e2e-live";

export function liveOnly() {
  test.skip(!process.env.E2E_LIVE, "Needs a running proxy. Set E2E_LIVE=1 to run.");
}

/** A name no earlier run can have left behind. */
export const unique = (what: string) => `${PREFIX}-${what}-${Date.now().toString(36)}`;
