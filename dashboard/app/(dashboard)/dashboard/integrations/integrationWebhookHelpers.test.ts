import { describe, it, expect } from "vitest";
import { integrationKindBadge, defaultHeadersForIntegration } from "./integrationWebhookHelpers";

describe("integrationKindBadge", () => {
  it("returns slack style for slack", () => {
    expect(integrationKindBadge("slack")).toContain("border");
  });

  it("returns teams style for teams", () => {
    expect(integrationKindBadge("teams")).toContain("bg");
  });

  it("returns splunk style for splunk", () => {
    expect(integrationKindBadge("splunk")).toContain("emerald");
  });

  it("returns splunk style for splunk_hec", () => {
    expect(integrationKindBadge("splunk_hec")).toContain("emerald");
  });

  it("returns sentinel style for sentinel", () => {
    expect(integrationKindBadge("sentinel")).toContain("blue");
  });

  it("returns qradar style for qradar", () => {
    expect(integrationKindBadge("qradar")).toContain("amber");
  });

  it("returns datadog style for datadog", () => {
    expect(integrationKindBadge("datadog")).toContain("zinc");
  });

  it("returns jira style for jira", () => {
    expect(integrationKindBadge("jira")).toContain("sky");
  });

  it("returns pagerduty style for pagerduty", () => {
    expect(integrationKindBadge("pagerduty")).toContain("06A94D");
  });

  it("returns opsgenie style for opsgenie", () => {
    expect(integrationKindBadge("opsgenie")).toContain("4C9AFF");
  });

  it("returns servicenow style for servicenow", () => {
    expect(integrationKindBadge("servicenow")).toContain("81B5A1");
  });

  it("returns default style for unknown", () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any -- intentional cast for testing edge case with unrecognized kind
    const cls = integrationKindBadge("unknown" as any);
    expect(cls).toContain("border-zinc");
  });
});

describe("defaultHeadersForIntegration", () => {
  it("returns Splunk header for splunk", () => {
    expect(defaultHeadersForIntegration("splunk")).toContain("Splunk");
  });

  it("returns Splunk header for splunk_hec", () => {
    expect(defaultHeadersForIntegration("splunk_hec")).toContain("Splunk");
  });

  it("returns DD-API-KEY for datadog", () => {
    expect(defaultHeadersForIntegration("datadog")).toContain("DD-API-KEY");
  });

  it("returns Basic auth for jira", () => {
    expect(defaultHeadersForIntegration("jira")).toContain("Basic");
  });

  it("returns Bearer for sentinel", () => {
    expect(defaultHeadersForIntegration("sentinel")).toContain("Bearer");
  });

  it("returns Basic for servicenow", () => {
    expect(defaultHeadersForIntegration("servicenow")).toContain("Basic");
  });

  it("returns empty string for unknown", () => {
    expect(defaultHeadersForIntegration("slack")).toBe("");
  });

  it("returns empty string for pagerduty", () => {
    expect(defaultHeadersForIntegration("pagerduty")).toBe("");
  });
});
