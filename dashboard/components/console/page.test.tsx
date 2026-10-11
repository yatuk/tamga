import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, render, screen, waitFor } from "@testing-library/react";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@carbon/react";
import { createElement, type ReactElement } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ADMIN_KEY_STORAGE } from "@/hooks/useAdminKey";
import { ApiError } from "@/lib/api/fetch-core";
import { DataNotice, NoData, Refused, ScrollTable } from "./page";

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

  it("says an account has no role, and which id an admin gives one to", async () => {
    localStorage.setItem(ADMIN_KEY_STORAGE, "test-key");
    const error = new ApiError("this account has not been given access; an admin assigns it a role first", 403, {
      code: "account_not_assigned",
      user_id: "583231",
    });
    show(<DataNotice adminKey="test-key" error={error} />);
    expect(screen.getByText("This account has not been given a role.")).toBeInTheDocument();
    expect(screen.getByText("An admin can assign one under Team: 583231.")).toBeInTheDocument();
    // Still so once the health check has answered: it is not a wrong key.
    await new Promise((done) => setTimeout(done, 100));
    expect(screen.getByText("This account has not been given a role.")).toBeInTheDocument();
    expect(screen.queryByText("The proxy did not accept the admin key.")).toBeNull();
  });

  it("says a session has ended, not that the key is wrong", async () => {
    // A key the health check rejects, so that the notice would otherwise blame the key.
    localStorage.setItem(ADMIN_KEY_STORAGE, "wrong-key");
    const error = new ApiError("this session has ended; sign in again", 401, { code: "session_revoked" });
    show(<DataNotice adminKey="wrong-key" error={error} />);
    expect(screen.getByText("This session has ended.")).toBeInTheDocument();
    expect(screen.getByText("Its role changed, or the account was signed out. Sign in again.")).toBeInTheDocument();
    await new Promise((done) => setTimeout(done, 100));
    expect(screen.queryByText("The proxy did not accept the admin key.")).toBeNull();
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

  it("says the same of an account without a role", () => {
    show(<Refused error={new ApiError("no role", 403, { code: "account_not_assigned", user_id: "583231" })} />);
    expect(screen.getByRole("alert")).toHaveTextContent("This account has not been given a role. An admin can assign one under Team: 583231.");
  });

  it("renders nothing for an error that is not about the role", () => {
    const { container } = show(<Refused error={new Error("network")} />);
    expect(container).toBeEmptyDOMElement();
  });
});

describe("ScrollTable", () => {
  /** jsdom lays nothing out: the box that scrolls is given the widths a browser would measure. */
  function widths(content: number, box: number) {
    const scrolling = (el: Element) => el.classList.contains("cds--data-table-content");
    vi.spyOn(Element.prototype, "scrollWidth", "get").mockImplementation(function (this: Element) {
      return scrolling(this) ? content : 0;
    });
    vi.spyOn(Element.prototype, "clientWidth", "get").mockImplementation(function (this: Element) {
      return scrolling(this) ? box : 0;
    });
  }
  afterEach(() => vi.restoreAllMocks());

  const table = (label: string) => (
    <ScrollTable className="t-table">
      <Table size="sm" aria-label={label}>
        <TableHead>
          <TableRow>
            <TableHeader>Endpoint</TableHeader>
            <TableHeader>Pool</TableHeader>
          </TableRow>
        </TableHead>
        <TableBody>
          <TableRow>
            <TableCell>openai</TableCell>
            <TableCell>primary</TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </ScrollTable>
  );
  const NOTE = "This table is wider than the page. Scroll it sideways to read the other columns.";

  it("says nothing of a table that fits, and adds no tab stop", () => {
    widths(600, 600);
    const { container } = render(table("Upstream endpoints"));
    const box = container.querySelector(".cds--data-table-content")!;
    expect(screen.getByText(NOTE)).not.toBeVisible();
    expect(box).not.toHaveAttribute("tabindex");
    expect(box).not.toHaveAttribute("role");
    expect(container.querySelector("[data-more]")).toBeNull();
  });

  it("says that a table wider than its box scrolls, on which side, and makes the box a named tab stop", () => {
    widths(900, 300);
    const { container } = render(table("Upstream endpoints"));
    const box = container.querySelector<HTMLElement>(".cds--data-table-content")!;
    expect(screen.getByText(NOTE)).toBeVisible();
    expect(screen.getByRole("region", { name: "Upstream endpoints, scrolls sideways" })).toBe(box);
    expect(box).toHaveAttribute("tabindex", "0");
    expect(container.querySelector(".cds--data-table-container")).toHaveAttribute("data-more", "right");

    // Scrolled part of the way, columns are out of view on both sides; at the end, on the left alone.
    act(() => {
      box.scrollLeft = 200;
      box.dispatchEvent(new Event("scroll"));
    });
    expect(container.querySelector(".cds--data-table-container")).toHaveAttribute("data-more", "both");
    act(() => {
      box.scrollLeft = 600;
      box.dispatchEvent(new Event("scroll"));
    });
    expect(container.querySelector(".cds--data-table-container")).toHaveAttribute("data-more", "left");
  });

  it("names the region by the table, without the instructions a table's name can carry", () => {
    widths(900, 300);
    render(table("Incidents. Arrow keys move between requests, Enter opens one."));
    expect(screen.getByRole("region", { name: "Incidents, scrolls sideways" })).toBeInTheDocument();
  });
});
