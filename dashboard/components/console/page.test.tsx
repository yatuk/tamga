import { beforeEach, describe, expect, it } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { createElement, type ReactElement } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ADMIN_KEY_STORAGE } from "@/hooks/useAdminKey";
import { ApiError } from "@/lib/api/fetch-core";
import { DataNotice, NoData, Refused } from "./page";

function show(node: ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(createElement(QueryClientProvider, { client: qc }, node));
}

const denied = new ApiError("role viewer lacks apikeys:read", 403, {
  error: "role viewer lacks apikeys:read",
  code: "permission_denied",
  required_permission: "apikeys:read",
});

beforeEach(() => {
  localStorage.removeItem(ADMIN_KEY_STORAGE);
});

describe("DataNotice", () => {
  it("renders nothing when there is a key and nothing failed", () => {
    const { container } = show(<DataNotice adminKey="test-key" />);
    expect(container).toBeEmptyDOMElement();
  });

  it("without a key points at Settings", () => {
    show(<DataNotice adminKey="" />);
    expect(screen.getByText("No admin key in this browser.")).toBeInTheDocument();
    expect(screen.getByText(/Add one under Settings/)).toBeInTheDocument();
  });

  it("says the key's role cannot read the list, and what the proxy asked for", () => {
    localStorage.setItem(ADMIN_KEY_STORAGE, "viewer-key");
    show(<DataNotice adminKey="viewer-key" error={denied} what="API keys" />);
    expect(screen.getByText("This key's role cannot read API keys.")).toBeInTheDocument();
    expect(screen.getByText(/The proxy asks for the apikeys:read permission\. Use a key with a higher role under Settings\./)).toBeInTheDocument();
    expect(screen.queryByText("The proxy did not answer.")).toBeNull();
    expect(screen.queryByText("The proxy did not accept the admin key.")).toBeNull();
  });

  it("keeps saying so once the health check has answered", async () => {
    localStorage.setItem(ADMIN_KEY_STORAGE, "viewer-key");
    show(<DataNotice adminKey="viewer-key" error={denied} />);
    // Long enough for the health answer to be in.
    await waitFor(() => expect(screen.getByText("This key's role cannot read this page's data.")).toBeInTheDocument());
    await new Promise((done) => setTimeout(done, 100));
    expect(screen.getByText("This key's role cannot read this page's data.")).toBeInTheDocument();
    expect(screen.queryByText(/The proxy is up, but/)).toBeNull();
  });

  it("for any other error says what the health check knows", async () => {
    localStorage.setItem(ADMIN_KEY_STORAGE, "wrong-key");
    show(<DataNotice adminKey="wrong-key" error={new ApiError("Admin key is wrong or missing", 401, {})} />);
    await waitFor(() => expect(screen.getByText("The proxy did not accept the admin key.")).toBeInTheDocument());
    expect(screen.queryByText(/role cannot read/)).toBeNull();
  });

  it("without a key says that, not that a role is too low", () => {
    show(<DataNotice adminKey="" error={denied} what="API keys" />);
    expect(screen.getByText("No admin key in this browser.")).toBeInTheDocument();
    expect(screen.queryByText(/role cannot read/)).toBeNull();
  });
});

describe("NoData", () => {
  it("is the page's title, what it is for, and why there is nothing", () => {
    show(<NoData title="Traffic" lede="How much went through the proxy." adminKey="" />);
    expect(screen.getByRole("heading", { name: "Traffic" })).toBeInTheDocument();
    expect(screen.getByText("How much went through the proxy.")).toBeInTheDocument();
    expect(screen.getByText("No admin key in this browser.")).toBeInTheDocument();
  });
});

describe("Refused", () => {
  it("puts the refusal beside the button that asked", () => {
    show(<Refused error={denied} />);
    expect(screen.getByRole("alert")).toHaveTextContent("This key's role cannot do this. The proxy asks for the apikeys:read permission.");
  });

  it("renders nothing for an error that is not about the role", () => {
    const { container } = show(<Refused error={new Error("network")} />);
    expect(container).toBeEmptyDOMElement();
  });
});
