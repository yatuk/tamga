import { describe, expect, it } from "vitest";
import { http, HttpResponse } from "msw";
import { server } from "@/__mocks__/server";
import { ApiError, permissionDenied } from "@/lib/api/fetch-core";
import { exportEndpoints } from "./export";

const API = "/api/v1";
const denied = (permission: string) =>
  HttpResponse.json({ error: `role viewer lacks ${permission}`, code: "permission_denied", required_permission: permission }, { status: 403 });

/** What the call threw, or a failure when it threw nothing. */
async function thrownBy(call: () => Promise<unknown>): Promise<unknown> {
  try {
    await call();
  } catch (e) {
    return e;
  }
  throw new Error("the call did not throw");
}

describe("the fetchers whose answer is not JSON", () => {
  it("download the events export with a key that may", async () => {
    const blob = await exportEndpoints.getEventsExport("test-key", { range: "7d" });
    expect(await blob.text()).toContain("request_id,action");
  });

  it("throw an ApiError that says the role is too low for the export", async () => {
    const error = await thrownBy(() => exportEndpoints.getEventsExport("viewer-key", { range: "7d" }));
    expect(error).toBeInstanceOf(ApiError);
    expect(error).toMatchObject({ status: 403, code: "permission_denied", message: "role viewer lacks events:export" });
    expect(permissionDenied(error)?.permission).toBe("events:export");
  });

  it("throw the same for the metrics text", async () => {
    server.use(http.get(`${API}/metrics`, () => denied("metrics:read")));
    const error = await thrownBy(() => exportEndpoints.getMetricsText("viewer-key"));
    expect(error).toBeInstanceOf(ApiError);
    expect(permissionDenied(error)?.permission).toBe("metrics:read");
  });

  it("throw the same for a PDF report", async () => {
    server.use(http.get(`${API}/reports/owasp/pdf`, () => denied("reports:read")));
    const error = await thrownBy(() => exportEndpoints.getOwaspPdfReport("viewer-key", { range: "7d" }));
    expect(error).toBeInstanceOf(ApiError);
    expect(permissionDenied(error)?.refused).toBe("This key's role cannot do this. The proxy asks for the reports:read permission.");
  });

  it("say a wrong key is a wrong key", async () => {
    server.use(http.get(`${API}/metrics`, () => HttpResponse.json({ error: "unauthorized" }, { status: 401 })));
    const error = await thrownBy(() => exportEndpoints.getMetricsText("wrong-key"));
    expect(error).toMatchObject({ status: 401, message: "Admin key is wrong or missing" });
    expect(permissionDenied(error)).toBeNull();
  });

  it("keep the status when the body is not JSON", async () => {
    server.use(http.get(`${API}/reports/incident/pdf`, () => new HttpResponse("bad gateway", { status: 502 })));
    const error = await thrownBy(() => exportEndpoints.getIncidentPdfReport("test-key"));
    expect(error).toBeInstanceOf(ApiError);
    expect(error).toMatchObject({ status: 502, code: "", message: "API error: 502" });
  });
});
