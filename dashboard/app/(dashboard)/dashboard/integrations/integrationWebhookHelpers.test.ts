import { describe, it, expect } from "vitest";
import { defaultHeadersForIntegration } from "./integrationWebhookHelpers";

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
