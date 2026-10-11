import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createElement } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ssoMock } from "@/__mocks__/handlers";
import { ADMIN_KEY_STORAGE } from "@/hooks/useAdminKey";
import SettingsPage from "./page";

// The key built into a development build. Empty, as here by default, is a production build.
const build = vi.hoisted(() => ({ key: "" }));
vi.mock("@/lib/built-in-key", async (original) => ({
  ...(await original<typeof import("@/lib/built-in-key")>()),
  get BUILT_IN_ADMIN_KEY() {
    return build.key;
  },
}));

function showAccess() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(createElement(QueryClientProvider, { client: qc }, createElement(SettingsPage)));
  return userEvent.setup();
}

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
  build.key = "";
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

describe("Settings: which admin key is in use", () => {
  const FORGET = "Forget the key stored in this browser";
  /** The section's note, beside its heading. */
  const note = () => screen.getByRole("heading", { name: "Admin key" }).parentElement!;
  /** What is on the page to read. The key sits in the password field, as it always has, and nowhere else. */
  const words = () => document.body.textContent ?? "";

  it("none: says so, and offers nothing to forget", async () => {
    showAccess();
    await waitFor(() => expect(note()).toHaveTextContent("None in use"));
    expect(screen.queryByRole("button", { name: FORGET })).toBeNull();
    expect(screen.queryByText(/built into the page/)).toBeNull();
  });

  it("stored, in a production build: says so, and that forgetting it leaves no key", async () => {
    localStorage.setItem(ADMIN_KEY_STORAGE, "stored-key-value-1");
    const user = showAccess();
    await waitFor(() => expect(note()).toHaveTextContent("In use: the one stored in this browser"));
    expect(screen.getByText(/Forgetting it leaves the console without a key until one is entered here\./)).toBeInTheDocument();
    expect(words()).not.toContain("stored-key-value-1");

    await user.click(screen.getByRole("button", { name: FORGET }));
    expect(localStorage.getItem(ADMIN_KEY_STORAGE)).toBeNull();
    await waitFor(() => expect(note()).toHaveTextContent("None in use"));
    expect(screen.queryByRole("button", { name: FORGET })).toBeNull();
  });

  it("stored, in a development build: says it takes the place of the built-in key, and forgetting it brings that one back", async () => {
    build.key = "built-in-test-value";
    localStorage.setItem(ADMIN_KEY_STORAGE, "stored-key-value-1");
    const user = showAccess();
    await waitFor(() => expect(note()).toHaveTextContent("In use: the one stored in this browser"));
    expect(screen.getByText(/It takes the place of the key built into this page for development; forgetting it makes the console use that one again\./)).toBeInTheDocument();
    expect(screen.queryByText("This key is built into the page, for local development.")).toBeNull();

    await user.click(screen.getByRole("button", { name: FORGET }));
    expect(localStorage.getItem(ADMIN_KEY_STORAGE)).toBeNull();
    await waitFor(() => expect(note()).toHaveTextContent("In use: the one built into this page for development"));
    expect(screen.getByText("This key is built into the page, for local development.")).toBeInTheDocument();
    expect(words()).not.toContain("built-in-test-value");
    expect(words()).not.toContain("stored-key-value-1");
  });

  it("built in: says so, with nothing stored to forget", async () => {
    build.key = "built-in-test-value";
    showAccess();
    await waitFor(() => expect(note()).toHaveTextContent("In use: the one built into this page for development"));
    expect(screen.queryByRole("button", { name: FORGET })).toBeNull();
    expect(words()).not.toContain("built-in-test-value");
  });
});
