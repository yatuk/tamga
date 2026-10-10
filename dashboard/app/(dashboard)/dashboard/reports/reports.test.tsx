import { beforeEach, describe, expect, it } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createElement } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { delay, http, HttpResponse } from "msw";
import { reportMock } from "@/__mocks__/handlers";
import { server } from "@/__mocks__/server";
import { ADMIN_KEY_STORAGE } from "@/hooks/useAdminKey";
import { ApiError } from "@/lib/api/fetch-core";
import ReportsPage from "./page";
import { reportFailure } from "./reportFailure";

function show(key = "test-key") {
  localStorage.setItem(ADMIN_KEY_STORAGE, key);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const view = render(createElement(QueryClientProvider, { client: qc }, createElement(ReportsPage)));
  return { ...view, user: userEvent.setup() };
}

const fails = (path: string, status: number, code?: string) =>
  server.use(http.get(`*/api/v1/reports/${path}/pdf`, () => HttpResponse.json(code ? { error: code, code } : { error: "failed" }, { status })));

beforeEach(() => {
  localStorage.clear();
  reportMock.queries.length = 0;
  // jsdom has no object URLs; the download itself is not what is tested here.
  URL.createObjectURL = () => "blob:report";
  URL.revokeObjectURL = () => undefined;
});

describe("reportFailure", () => {
  const failure = (status: number, code?: string) => reportFailure(new ApiError("x", status, code ? { code } : {}));

  it("says each way the report service can fail for what it is", () => {
    expect(failure(503, "analyzer_not_configured")).toBe(
      "The proxy has no report service set up, so it cannot make this PDF. Point it at an analyzer and try again.",
    );
    expect(failure(503, "analyzer_unavailable")).toBe("The report service is not reachable from the proxy.");
    expect(failure(504, "analyzer_timeout")).toBe("The report service did not answer the proxy in time. Try again.");
    expect(failure(502, "analyzer_error")).toBe("The report service failed while making the PDF.");
  });

  it("says a 503 with another code is the counts, which the proxy reads itself", () => {
    expect(failure(503, "stats_unavailable")).toBe("The proxy could not read the counts for this window, so the report was not made.");
  });

  it("says a role that is too low as it is said everywhere else", () => {
    const denied = new ApiError("role viewer lacks reports:read", 403, { code: "permission_denied", required_permission: "reports:read" });
    expect(reportFailure(denied)).toBe("This key's role cannot do this. The proxy asks for the reports:read permission.");
  });

  it("falls back to the status, and to the proxy not answering, and never to a bare 'could not generate'", () => {
    expect(failure(500)).toBe("The report was not made. The proxy answered 500.");
    expect(failure(503)).toBe("The report was not made. The proxy answered 503.");
    expect(reportFailure(new TypeError("Failed to fetch"))).toBe("The proxy did not answer, so the report was not made.");
  });
});

describe("Reports: the PDF reports", () => {
  it("asks for both by the window alone: the proxy counts for itself", async () => {
    const { user } = show();
    await user.click(await screen.findByRole("button", { name: "Incident report" }));
    await waitFor(() => expect(reportMock.queries).toEqual(["range=7d"]));
    await waitFor(() => expect(screen.getByRole("button", { name: "OWASP report" })).toBeEnabled());
    await user.click(screen.getByRole("button", { name: "OWASP report" }));
    await waitFor(() => expect(reportMock.queries).toEqual(["range=7d", "range=7d"]));
    expect(document.querySelector(".t-pagehead .t-refused")).toBeNull();
  });

  it.each([
    ["incident", 503, "analyzer_unavailable", "Incident report: The report service is not reachable from the proxy."],
    ["incident", 503, "analyzer_not_configured", /^Incident report: The proxy has no report service set up/],
    ["owasp", 504, "analyzer_timeout", "OWASP report: The report service did not answer the proxy in time. Try again."],
    ["owasp", 502, "analyzer_error", "OWASP report: The report service failed while making the PDF."],
    ["incident", 503, "stats_unavailable", "Incident report: The proxy could not read the counts for this window, so the report was not made."],
  ] as const)("says beside the buttons why the %s report was not made (%i %s)", async (which, status, code, said) => {
    fails(which, status, code);
    const { user } = show();
    await user.click(await screen.findByRole("button", { name: which === "owasp" ? "OWASP report" : "Incident report" }));
    const note = await screen.findByRole("alert");
    expect(note).toHaveTextContent(said);
    expect(note.closest(".t-pagehead")).not.toBeNull();
    expect(screen.queryByText("Could not generate the PDF")).toBeNull();
    // The page stays usable, and a report that then succeeds takes the sentence away.
    await waitFor(() => expect(screen.getByRole("button", { name: "Incident report" })).toBeEnabled());
  });

  it("drops the sentence when the next report is made", async () => {
    fails("incident", 503, "analyzer_unavailable");
    const { user } = show();
    await user.click(await screen.findByRole("button", { name: "Incident report" }));
    await screen.findByRole("alert");
    await waitFor(() => expect(screen.getByRole("button", { name: "OWASP report" })).toBeEnabled());
    await user.click(screen.getByRole("button", { name: "OWASP report" }));
    await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
  });
});

describe("Reports: the figures", () => {
  it("draws no figure while they are on their way", async () => {
    server.use(
      http.get("*/api/v1/stats", async () => {
        await delay(150);
        return HttpResponse.json({ total_requests: 1204, blocked_requests: 31, redacted_requests: 7, passed_requests: 1166, top_finding_types: {} });
      }),
    );
    const { container } = show();
    await screen.findByRole("status", { name: "Loading" });
    // Not "0 requests went through the proxy", and no facts made of defaults.
    expect(container.querySelector(".t-prose")).toBeNull();
    expect(container.querySelector(".t-facts")).toBeNull();
    expect(screen.queryByText(/requests went through the proxy/)).toBeNull();

    await waitFor(() => expect(container.querySelector(".t-prose")).toHaveTextContent("1,204 requests went through the proxy."));
    expect(screen.queryByRole("status", { name: "Loading" })).toBeNull();
  });

  it("shows the notice and no figures when the read fails", { timeout: 20_000 }, async () => {
    server.use(http.get("*/api/v1/stats", () => HttpResponse.json({ error: "boom" }, { status: 500 })));
    const { container } = show();
    // The page retries the read once before it gives up on it.
    expect(await screen.findByText("The proxy is up, but it did not return this page's data.", {}, { timeout: 8000 })).toBeInTheDocument();
    expect(container.querySelector(".t-prose")).toBeNull();
    expect(container.querySelector(".t-facts")).toBeNull();
  });
});
