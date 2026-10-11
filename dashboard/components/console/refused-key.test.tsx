import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createElement, type ReactElement } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ADMIN_KEY_STORAGE, useAdminKey, useAdminKeySource } from "@/hooks/useAdminKey";
import { ApiError } from "@/lib/api/fetch-core";
import { HealthDetailNotice } from "./health";
import { DataNotice } from "./page";
import { FORGET_STORED_KEY, forgottenWords, KEY_IN_USE, REFUSED_KEY_TITLE, refusedKeyWords, storedKeyNote } from "./refused-key";

// The key built into a development build, which a test build does not have.
// Empty is a production build; the mocked proxy rejects a key starting "wrong".
const build = vi.hoisted(() => ({ key: "" }));
vi.mock("@/lib/built-in-key", async (original) => ({
  ...(await original<typeof import("@/lib/built-in-key")>()),
  get BUILT_IN_ADMIN_KEY() {
    return build.key;
  },
}));

const STORED = "wrong-stored-key-0000";
const BUILT_IN = "built-in-test-value";
const unauthorized = new ApiError("Admin key is wrong or missing", 401, {});

function show(node: ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(createElement(QueryClientProvider, { client: qc }, node));
}

/** A page as the console builds one: the key in use decides what the notice says. */
function Page() {
  const [adminKey] = useAdminKey();
  const { source } = useAdminKeySource();
  return (
    <>
      <p data-testid="source">{source}</p>
      <DataNotice adminKey={adminKey} failed error={adminKey ? unauthorized : undefined} />
    </>
  );
}

beforeEach(() => {
  localStorage.clear();
  build.key = "";
});

describe("refusedKeyWords", () => {
  it("names the stored key and the built-in one it falls back to, in a development build", () => {
    const words = refusedKeyWords("stored", true);
    expect(words.forget).toBe(true);
    expect(words.detail).toBe(
      "The key it refused is the one stored in this browser. Forgetting it makes this console use the key built into this page for development.",
    );
  });

  it("names the stored key and promises no fallback in a production build", () => {
    const words = refusedKeyWords("stored", false);
    expect(words.forget).toBe(true);
    expect(words.detail).toBe("The key it refused is the one stored in this browser. Forget it, then enter a valid one under Settings.");
    expect(words.detail).not.toMatch(/built into/);
  });

  it("says a refused built-in key is the build's, and offers nothing to forget", () => {
    const words = refusedKeyWords("built-in", true);
    expect(words.forget).toBe(false);
    expect(words.detail).toMatch(/the one built into this page for development, from NEXT_PUBLIC_ADMIN_KEY/);
  });

  it("keeps the plain sentence when the key came from neither place", () => {
    expect(refusedKeyWords("none", false)).toEqual({ detail: "Check the key under Settings.", forget: false });
  });

  it("says what is in use and what forgetting leads to, for a build with a key and one without", () => {
    expect(Object.keys(KEY_IN_USE).sort()).toEqual(["built-in", "none", "stored"]);
    expect(storedKeyNote(true)).toMatch(/forgetting it makes the console use that one again/);
    expect(storedKeyNote(false)).toMatch(/leaves the console without a key/);
    expect(forgottenWords(true)).toMatch(/now uses the key built into this page/);
    expect(forgottenWords(false)).toMatch(/has no admin key now/);
  });
});

describe("a refused key stored in this browser", () => {
  it("in a production build: the notice names the stored key, and forgetting it leaves no key", async () => {
    localStorage.setItem(ADMIN_KEY_STORAGE, STORED);
    show(<Page />);
    await waitFor(() => expect(screen.getByText(REFUSED_KEY_TITLE)).toBeInTheDocument());
    expect(screen.getByText("It is up. The key it refused is the one stored in this browser. Forget it, then enter a valid one under Settings.")).toBeInTheDocument();
    expect(screen.queryByText(/built into this page/)).toBeNull();
    // No part of the key is on the page.
    expect(document.body.textContent).not.toContain(STORED);
    expect(document.body.textContent).not.toContain(STORED.slice(0, 8));

    await userEvent.setup().click(screen.getByRole("button", { name: FORGET_STORED_KEY }));
    expect(localStorage.getItem(ADMIN_KEY_STORAGE)).toBeNull();
    await waitFor(() => expect(screen.getByText("No admin key in this browser.")).toBeInTheDocument());
    expect(screen.queryByText(REFUSED_KEY_TITLE)).toBeNull();
    expect(screen.getByTestId("source")).toHaveTextContent("none");
  });

  it("in a development build: the notice says the built-in key takes over, and it does", async () => {
    build.key = BUILT_IN;
    localStorage.setItem(ADMIN_KEY_STORAGE, STORED);
    show(<Page />);
    await waitFor(() => expect(screen.getByText(REFUSED_KEY_TITLE)).toBeInTheDocument());
    expect(
      screen.getByText(
        "It is up. The key it refused is the one stored in this browser. Forgetting it makes this console use the key built into this page for development.",
      ),
    ).toBeInTheDocument();
    expect(document.body.textContent).not.toContain(STORED);
    expect(document.body.textContent).not.toContain(BUILT_IN);

    await userEvent.setup().click(screen.getByRole("button", { name: FORGET_STORED_KEY }));
    expect(localStorage.getItem(ADMIN_KEY_STORAGE)).toBeNull();
    expect(screen.getByTestId("source")).toHaveTextContent("built-in");
    // The proxy accepts the built-in key: the key is no longer blamed.
    await waitFor(() => expect(screen.queryByText(REFUSED_KEY_TITLE)).toBeNull());
    expect(screen.queryByText("No admin key in this browser.")).toBeNull();
    expect(document.body.textContent).not.toContain(BUILT_IN);
  });

  it("does not take the focus when it appears", async () => {
    localStorage.setItem(ADMIN_KEY_STORAGE, STORED);
    show(<Page />);
    const button = await screen.findByRole("button", { name: FORGET_STORED_KEY });
    expect(button).not.toHaveFocus();
  });

  it("is said the same way where the health detail is kept back", async () => {
    build.key = BUILT_IN;
    localStorage.setItem(ADMIN_KEY_STORAGE, STORED);
    show(<HealthDetailNotice state="rejected" />);
    expect(screen.getByText(REFUSED_KEY_TITLE)).toBeInTheDocument();
    expect(screen.getByText(/only to a valid admin key\. The key it refused is the one stored in this browser\./)).toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: FORGET_STORED_KEY }));
    expect(localStorage.getItem(ADMIN_KEY_STORAGE)).toBeNull();
  });
});

describe("a refused key that is not stored in this browser", () => {
  it("the built-in key refused: says where it comes from, with nothing to forget", async () => {
    build.key = "wrong-built-in-value";
    show(<Page />);
    expect(screen.getByTestId("source")).toHaveTextContent("built-in");
    await waitFor(() => expect(screen.getByText(REFUSED_KEY_TITLE)).toBeInTheDocument());
    expect(screen.getByText(/It is up\. The key it refused is the one built into this page for development, from NEXT_PUBLIC_ADMIN_KEY\./)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: FORGET_STORED_KEY })).toBeNull();
    expect(document.body.textContent).not.toContain("wrong-built-in-value");
  });

  it("no key at all is not a refused key", () => {
    show(<Page />);
    expect(screen.getByTestId("source")).toHaveTextContent("none");
    expect(screen.getByText("No admin key in this browser.")).toBeInTheDocument();
    expect(screen.queryByText(REFUSED_KEY_TITLE)).toBeNull();
    expect(screen.queryByRole("button", { name: FORGET_STORED_KEY })).toBeNull();
  });

  it("a stored key that works shows no notice, whatever the build holds", async () => {
    build.key = BUILT_IN;
    localStorage.setItem(ADMIN_KEY_STORAGE, "test-key");
    show(<HealthDetailNotice state="full" />);
    expect(screen.queryByText(REFUSED_KEY_TITLE)).toBeNull();
    expect(screen.queryByRole("button", { name: FORGET_STORED_KEY })).toBeNull();
  });
});
