import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createElement } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ssoMock } from "@/__mocks__/handlers";
import { ADMIN_KEY_STORAGE } from "@/hooks/useAdminKey";
import SettingsPage from "./page";

// Carbon's tabs construct a ResizeObserver; the stand-in in vitest.setup.ts cannot be called with `new`.
vi.stubGlobal(
  "ResizeObserver",
  class {
    observe() {}
    unobserve() {}
    disconnect() {}
  },
);

async function showSSO() {
  localStorage.setItem(ADMIN_KEY_STORAGE, "test-key");
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(createElement(QueryClientProvider, { client: qc }, createElement(SettingsPage)));
  const user = userEvent.setup();
  await user.click(await screen.findByRole("tab", { name: "SSO" }));
  return user;
}

beforeEach(() => {
  localStorage.clear();
  ssoMock.config = { provider_type: "", metadata_url: "", attribute_mapping: {}, enabled: false, domain: "" };
});

describe("Settings: single sign-on", () => {
  it("says the settings are stored and used by no sign-in, and how people sign in today", async () => {
    await showSSO();
    expect(await screen.findByText("No sign-in uses these settings yet.")).toBeInTheDocument();
    expect(screen.getByText(/The proxy stores them and reads them nowhere/)).toBeInTheDocument();
    expect(screen.getByText(/Today people sign in through GitHub OAuth, which is configured on the proxy, or use a key\./)).toBeInTheDocument();
    expect(screen.getByText("Stored, not used by any sign-in")).toBeInTheDocument();
  });

  it("does not present a stored configuration as one in force", async () => {
    ssoMock.config = { provider_type: "saml", metadata_url: "https://idp.example.com/metadata", attribute_mapping: {}, enabled: true, domain: "example.com" };
    await showSSO();
    expect(await screen.findByLabelText("Email domain")).toHaveValue("example.com");
    expect(screen.getByText("No sign-in uses these settings yet.")).toBeInTheDocument();
    expect(screen.queryByText(/^Required, over/)).toBeNull();
    expect(screen.queryByText(/^People with an address on this domain sign in/)).toBeNull();
    expect(screen.queryByText(/needs a plan with your identity provider/)).toBeNull();
  });

  it("still saves the form", async () => {
    const user = await showSSO();
    await user.type(await screen.findByLabelText("Email domain"), "example.com");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(ssoMock.config.domain).toBe("example.com"));
  });
});
