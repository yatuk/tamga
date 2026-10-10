import { beforeEach, describe, expect, it } from "vitest";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createElement } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { teamMock } from "@/__mocks__/team-handlers";
import { clockTime, dayLabel } from "@/components/console/names";
import { ADMIN_KEY_STORAGE } from "@/hooks/useAdminKey";
import { ApiError } from "@/lib/api/fetch-core";
import { assignedRole, explainTeamRefusal } from "./access";
import TeamPage from "./page";

// See __mocks__/team-handlers.ts for what each key stands for.
const ADMIN_KEY = "test-key";

let client: QueryClient;

function show(key = ADMIN_KEY) {
  localStorage.setItem(ADMIN_KEY_STORAGE, key);
  client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(createElement(QueryClientProvider, { client }, createElement(TeamPage)));
  return userEvent.setup();
}

/** The list is read again, as it is when the window is returned to. */
const reload = () => act(() => client.invalidateQueries());

const rowOf = (name: string | RegExp) => screen.findByRole("row", { name: new RegExp(name) });
const clockAt = (unix: number) => clockTime(new Date(unix * 1000).toISOString());

async function pickRole(user: ReturnType<typeof userEvent.setup>, row: HTMLElement, role: string) {
  await user.selectOptions(within(row).getByRole("combobox"), role);
}

beforeEach(() => {
  localStorage.clear();
  teamMock.reset();
});

describe("assignedRole", () => {
  it("reads a default viewer the proxy holds no record of as not assigned", () => {
    expect(assignedRole({ user_id: "u", role: "viewer", updated_at: "0001-01-01T00:00:00Z" }, "viewer")).toBe("");
    expect(assignedRole({ user_id: "u", role: "viewer", updated_at: "2026-09-03T10:00:00Z" }, "viewer")).toBe("viewer");
    expect(assignedRole({ user_id: "u", role: "", updated_at: "0001-01-01T00:00:00Z" }, "")).toBe("");
  });

  it("takes the proxy's word for it where the proxy says", () => {
    // A role that was removed leaves a record with a time, and the default in its place.
    expect(assignedRole({ user_id: "u", role: "viewer", assigned: false, updated_at: "2026-09-03T10:00:00Z" }, "viewer")).toBe("");
    expect(assignedRole({ user_id: "u", role: "viewer", assigned: true, updated_at: "0001-01-01T00:00:00Z" }, "viewer")).toBe("viewer");
    expect(assignedRole({ user_id: "u", role: "", assigned: false, updated_at: "2026-09-03T10:00:00Z" }, "")).toBe("");
    expect(assignedRole({ user_id: "u", role: "analyst", assigned: true }, "")).toBe("analyst");
  });
});

describe("explainTeamRefusal", () => {
  it("says the proxy could not store a change", () => {
    const error = new ApiError("the change could not be stored, try again", 503, {});
    expect(explainTeamRefusal(error, "sign-out")).toBe("Nobody was signed out. The proxy could not store the change. Try again.");
  });

  it("says a key's role is too low for the team", () => {
    const error = new ApiError("role analyst lacks team:admin", 403, { code: "permission_denied", required_permission: "team:admin" });
    expect(explainTeamRefusal(error, "role")).toBe("This key's role cannot do this. The proxy asks for the team:admin permission.");
  });

  it("says an account has no role to remove", () => {
    expect(explainTeamRefusal(new ApiError("this account has no role", 404, {}), "remove")).toBe("This account has no role to remove.");
  });
});

describe("Team page: what each account may do", () => {
  it("lists the accounts that hold a role, and says an account needs one to sign in", async () => {
    show();
    const ayse = await rowOf("Ayse Kaya");
    expect(within(ayse).getByRole("combobox")).toHaveValue("admin");
    expect(within(await rowOf("Mehmet Demir")).getByRole("combobox")).toHaveValue("analyst");
    expect(within(await rowOf("1003")).getByRole("combobox")).toHaveValue("viewer");

    expect(screen.getByText("3 accounts hold a role on this proxy: 1 admin, 1 analyst, 1 viewer.")).toBeInTheDocument();
    expect(screen.getByText("An account must be given a role before it can sign in.")).toBeInTheDocument();
    expect(screen.queryByText("Accounts nobody gave a role are admitted as viewers.")).toBeNull();
    // When the account's sessions were last ended, where the proxy says so.
    const ended = teamMock.member("1001")?.sessions_valid_from as number;
    expect(within(ayse).getByText(`Sessions ended ${dayLabel(new Date(ended * 1000).toISOString())}, ${clockAt(ended)}`)).toBeInTheDocument();
    expect(within(await rowOf("Mehmet Demir")).queryByText(/Sessions ended/)).toBeNull();
  });

  it("says nothing of Clerk when the proxy reads no directory, and sends nobody there", async () => {
    show();
    await rowOf("Ayse Kaya");
    expect(screen.queryByText(/Clerk/)).toBeNull();
    expect(screen.queryByText("The identity provider is not connected.")).toBeNull();
    expect(screen.queryByRole("link")).toBeNull();
  });

  it("shows an account nobody gave a role as having no access", async () => {
    teamMock.reset({ directory: [{ user_id: "user_2abc", name: "Zeynep Arslan", email: "zeynep@example.com" }] });
    show();
    const row = await rowOf("Zeynep Arslan");
    const select = within(row).getByRole("combobox");
    expect(select).toHaveValue("");
    expect(within(select).getByRole("option", { name: "No access" })).toBeInTheDocument();
    // There is no role to take away, and nothing says when one was set.
    expect(within(row).queryByRole("button", { name: "Remove access" })).toBeNull();
    expect(screen.getByText("3 accounts hold a role on this proxy: 1 admin, 1 analyst, 1 viewer. 1 account has none.")).toBeInTheDocument();
    // Being in the directory is not a sign-in.
    expect(screen.getByText(/Being listed there gives no access, and\s+nobody signs in through Clerk\./)).toBeInTheDocument();
  });

  it("shows such an account as a viewer by default where the proxy admits it as one", async () => {
    teamMock.reset({ directory: [{ user_id: "user_2abc", name: "Zeynep Arslan" }], defaultRole: "viewer" });
    show();
    const row = await rowOf("Zeynep Arslan");
    const select = within(row).getByRole("combobox");
    expect(select).toHaveValue("");
    expect(within(select).getByRole("option", { name: "Viewer by default" })).toBeInTheDocument();
    expect(within(row).queryByRole("button", { name: "Remove access" })).toBeNull();

    expect(screen.getByText("Accounts nobody gave a role are admitted as viewers.")).toBeInTheDocument();
    expect(screen.getByText(/TAMGA_SSO_DEFAULT_ROLE=viewer is set on the proxy/)).toBeInTheDocument();
    expect(screen.queryByText("An account must be given a role before it can sign in.")).toBeNull();
    // The viewer the proxy holds a record of is a viewer, not one by default.
    expect(within(await rowOf("1003")).getByRole("combobox")).toHaveValue("viewer");
  });

  it("reads the same from a proxy that does not say which roles were assigned", async () => {
    teamMock.reset({ directory: [{ user_id: "user_2abc", name: "Zeynep Arslan" }], defaultRole: "viewer", older: true });
    show();
    const select = within(await rowOf("Zeynep Arslan")).getByRole("combobox");
    expect(select).toHaveValue("");
    expect(within(select).getByRole("option", { name: "Viewer by default" })).toBeInTheDocument();
    expect(within(await rowOf("1003")).getByRole("combobox")).toHaveValue("viewer");
    expect(within(await rowOf("1003")).getByRole("button", { name: "Remove access" })).toBeInTheDocument();
    expect(screen.getByText("3 accounts hold a role on this proxy: 1 admin, 1 analyst, 1 viewer. 1 account has none.")).toBeInTheDocument();
  });

  it("lists an account whose role was removed, with when its sessions ended, and gives it a role again", async () => {
    const ended = 1788256800;
    teamMock.reset({
      members: [
        { user_id: "1001", name: "Ayse Kaya", role: "admin", updated_at: "2026-09-01T10:00:00Z" },
        { user_id: "1004", name: "Deniz Yilmaz", role: "", role_removed: true, updated_at: "2026-09-04T10:00:00Z", sessions_valid_from: ended },
      ],
    });
    const user = show();
    const row = await rowOf("Deniz Yilmaz");
    const select = within(row).getByRole("combobox");
    expect(select).toHaveValue("");
    expect(within(select).getByRole("option", { name: "No access" })).toBeInTheDocument();
    expect(within(row).getByText(`Sessions ended ${dayLabel(new Date(ended * 1000).toISOString())}, ${clockAt(ended)}`)).toBeInTheDocument();
    // The time the proxy holds is when the role was taken away, not when one was set.
    expect(within(row).getByText(`Role removed ${dayLabel("2026-09-04T10:00:00Z")}`)).toBeInTheDocument();
    expect(within(row).queryByRole("button", { name: "Remove access" })).toBeNull();
    expect(screen.getByText("1 account holds a role on this proxy: 1 admin, 0 analysts, 0 viewers. 1 account has none.")).toBeInTheDocument();

    await pickRole(user, row, "analyst");
    expect(within(row).getByText("Saving lets this account sign in.")).toBeInTheDocument();
    await user.click(within(row).getByRole("button", { name: "Save role" }));
    expect(await within(await rowOf("Deniz Yilmaz")).findByRole("status")).toHaveTextContent("Now analyst. The account can sign in.");
    await waitFor(() => expect(within(screen.getByRole("row", { name: /Deniz Yilmaz/ })).getByRole("combobox")).toHaveValue("analyst"));
    expect(within(screen.getByRole("row", { name: /Deniz Yilmaz/ })).getByRole("button", { name: "Remove access" })).toBeInTheDocument();
    expect(screen.getAllByRole("row")).toHaveLength(3);
  });

  it("tells a key that is not an admin key that it cannot read the team", async () => {
    show("viewer-key");
    expect(await screen.findByText("This key's role cannot read the team.")).toBeInTheDocument();
    expect(screen.getByText(/The proxy asks for the team:admin permission\./)).toBeInTheDocument();
    expect(screen.queryByRole("table")).toBeNull();
  });
});

describe("Team page: changing a role", () => {
  it("says before the save that it signs the person out, and afterwards from when", async () => {
    const user = show();
    const row = await rowOf("Mehmet Demir");
    await pickRole(user, row, "viewer");
    expect(within(row).getByText("Saving signs this person out everywhere. They sign in again with the new role.")).toBeInTheDocument();
    await user.click(within(row).getByRole("button", { name: "Save role" }));

    const done = await within(await rowOf("Mehmet Demir")).findByRole("status");
    expect(done).toHaveTextContent(
      `Now viewer, and signed out to sign in again with that role. Tokens issued before ${clockAt(teamMock.now)} no longer work; other replicas follow within 30 seconds.`,
    );
    expect(teamMock.member("1002")).toMatchObject({ role: "viewer", sessions_valid_from: teamMock.now });
    await waitFor(() => expect(within(screen.getByRole("row", { name: /Mehmet Demir/ })).getByRole("combobox")).toHaveValue("viewer"));
  });

  it("gives a role to an account that had none, and says it can sign in", async () => {
    teamMock.reset({ directory: [{ user_id: "user_2abc", name: "Zeynep Arslan" }] });
    const user = show();
    const row = await rowOf("Zeynep Arslan");
    await pickRole(user, row, "analyst");
    expect(within(row).getByText("Saving lets this account sign in.")).toBeInTheDocument();
    await user.click(within(row).getByRole("button", { name: "Save role" }));

    expect(await within(await rowOf("Zeynep Arslan")).findByRole("status")).toHaveTextContent("Now analyst. The account can sign in.");
    expect(teamMock.member("user_2abc")?.role).toBe("analyst");
  });

  it("says at the row that the only admin keeps the role", async () => {
    const user = show();
    const row = await rowOf("Ayse Kaya");
    await pickRole(user, row, "viewer");
    await user.click(within(row).getByRole("button", { name: "Save role" }));

    expect(await within(row).findByRole("alert")).toHaveTextContent(
      "This is the only admin. The role was not changed: make someone else an admin first.",
    );
    expect(teamMock.member("1001")?.role).toBe("admin");
    // Nobody else's row says anything.
    expect(within(await rowOf("Mehmet Demir")).queryByRole("alert")).toBeNull();
  });

  it("says at the row that nobody changes their own role", async () => {
    // The proxy answers this to a signed-in user, which the console is not. The sentence is there for it.
    const user = show("session-1002");
    const row = await rowOf("Mehmet Demir");
    await pickRole(user, row, "admin");
    await user.click(within(row).getByRole("button", { name: "Save role" }));

    expect(await within(row).findByRole("alert")).toHaveTextContent(
      "You cannot change your own role. Another admin can, or whoever holds the admin key.",
    );
    expect(teamMock.member("1002")?.role).toBe("analyst");
  });

  it("says where it was typed that the proxy does not take a user id", async () => {
    const user = show();
    await rowOf("Ayse Kaya");
    await user.type(screen.getByLabelText("User id"), "not an id!");
    await user.click(screen.getByRole("button", { name: "Give the role" }));

    // Under the form, not at anyone's row. (Carbon's field has an alert of its own, so the sentence is looked up by its text.)
    const form = screen.getByLabelText("User id").closest("section") as HTMLElement;
    const said = await within(form).findByText(
      "The role was not changed. The proxy does not take this as a user id: it is letters, digits and _ - . @ | : + only, up to 128 characters.",
    );
    expect(said).toHaveAttribute("role", "alert");
    expect(screen.getAllByRole("row")).toHaveLength(4);
  });

  it("gives a role to an account named by its id", async () => {
    const user = show();
    await rowOf("Ayse Kaya");
    // Nothing is sent without an id.
    await user.click(screen.getByRole("button", { name: "Give the role" }));
    expect(await screen.findByText("Enter the account's user id.")).toBeInTheDocument();

    await user.type(screen.getByLabelText("User id"), "583231");
    await user.selectOptions(screen.getByLabelText("Role"), "analyst");
    await user.click(screen.getByRole("button", { name: "Give the role" }));

    const form = screen.getByLabelText("User id").closest("section") as HTMLElement;
    expect(await within(form).findByRole("status")).toHaveTextContent("583231: Now analyst. The account can sign in.");
    expect(within(await rowOf("583231")).getByRole("combobox")).toHaveValue("analyst");
  });
});

describe("Team page: what is said about an account is the last thing that happened to it", () => {
  const giveByForm = async (user: ReturnType<typeof userEvent.setup>, id: string) => {
    await user.type(screen.getByLabelText("User id"), id);
    await user.click(screen.getByRole("button", { name: "Give the role" }));
    const form = screen.getByLabelText("User id").closest("section") as HTMLElement;
    expect(await within(form).findByRole("status")).toHaveTextContent(`${id}: Now viewer. The account can sign in.`);
    return form;
  };

  it("takes the form's sentence away when the account's access is removed at its row", async () => {
    const user = show();
    await rowOf("Ayse Kaya");
    const form = await giveByForm(user, "900000001");

    const row = await rowOf("900000001");
    await user.click(within(row).getByRole("button", { name: "Remove access" }));
    await user.click(within(row).getByRole("button", { name: "Remove and sign out" }));

    expect(await within(await rowOf("900000001")).findByRole("status")).toHaveTextContent(
      "Access removed and signed out. The account cannot sign in until it is given a role again.",
    );
    expect(within(form).queryByRole("status")).toBeNull();
    expect(screen.queryByText(/The account can sign in\./)).toBeNull();
  });

  it("takes the form's sentence away when the account's role is changed at its row", async () => {
    const user = show();
    await rowOf("Ayse Kaya");
    const form = await giveByForm(user, "900000001");

    const row = await rowOf("900000001");
    await pickRole(user, row, "analyst");
    await user.click(within(row).getByRole("button", { name: "Save role" }));

    expect(await within(await rowOf("900000001")).findByRole("status")).toHaveTextContent(/^Now analyst, and signed out/);
    expect(within(form).queryByRole("status")).toBeNull();
  });

  it("says one thing at a row: the next change replaces the sentence before it", async () => {
    const user = show();
    const row = await rowOf("Mehmet Demir");
    await pickRole(user, row, "viewer");
    await user.click(within(row).getByRole("button", { name: "Save role" }));
    expect(await within(await rowOf("Mehmet Demir")).findByRole("status")).toHaveTextContent(/^Now viewer/);

    const again = await rowOf("Mehmet Demir");
    await user.click(within(again).getByRole("button", { name: "Remove access" }));
    await user.click(within(again).getByRole("button", { name: "Remove and sign out" }));
    await waitFor(() => expect(within(screen.getByRole("row", { name: /Mehmet Demir/ })).getByRole("status")).toHaveTextContent(/^Access removed/));
    expect(within(screen.getByRole("row", { name: /Mehmet Demir/ })).getAllByRole("status")).toHaveLength(1);
    expect(screen.queryByText(/^Now viewer/)).toBeNull();
  });

  it("says nothing at one row once something is done at another", async () => {
    const user = show();
    const row = await rowOf("Mehmet Demir");
    await pickRole(user, row, "viewer");
    await user.click(within(row).getByRole("button", { name: "Save role" }));
    expect(await within(await rowOf("Mehmet Demir")).findByRole("status")).toBeInTheDocument();

    const other = await rowOf("1003");
    await user.click(within(other).getByRole("button", { name: "Sign out everywhere" }));
    await user.click(within(other).getByRole("button", { name: "Sign out" }));
    expect(await within(await rowOf("1003")).findByRole("status")).toHaveTextContent(/^Signed out\./);
    expect(within(screen.getByRole("row", { name: /Mehmet Demir/ })).queryByRole("status")).toBeNull();
  });

  it("keeps a row's sentence over a reload that agrees with it, and drops it when someone else has changed the account", async () => {
    const user = show();
    const row = await rowOf("Mehmet Demir");
    await pickRole(user, row, "viewer");
    await user.click(within(row).getByRole("button", { name: "Save role" }));
    expect(await within(await rowOf("Mehmet Demir")).findByRole("status")).toHaveTextContent(/^Now viewer/);

    await reload();
    expect(within(screen.getByRole("row", { name: /Mehmet Demir/ })).getByRole("status")).toHaveTextContent(/^Now viewer/);

    teamMock.setElsewhere("1002", "admin");
    await reload();
    await waitFor(() => expect(within(screen.getByRole("row", { name: /Mehmet Demir/ })).getByRole("combobox")).toHaveValue("admin"));
    expect(within(screen.getByRole("row", { name: /Mehmet Demir/ })).queryByRole("status")).toBeNull();
  });

  it("drops the sentence about the only admin once a reload shows another", async () => {
    const user = show();
    const row = await rowOf("Ayse Kaya");
    await pickRole(user, row, "viewer");
    await user.click(within(row).getByRole("button", { name: "Save role" }));
    expect(await within(row).findByRole("alert")).toHaveTextContent(/^This is the only admin\./);

    teamMock.setElsewhere("1002", "admin");
    await reload();
    await waitFor(() => expect(within(screen.getByRole("row", { name: /Mehmet Demir/ })).getByRole("combobox")).toHaveValue("admin"));
    expect(within(screen.getByRole("row", { name: /Ayse Kaya/ })).queryByRole("alert")).toBeNull();
  });
});

describe("Team page: removing access", () => {
  it("asks twice, then removes the role and says the account cannot sign in", async () => {
    const user = show();
    const row = await rowOf("Mehmet Demir");
    await user.click(within(row).getByRole("button", { name: "Remove access" }));
    // The first press only asks.
    expect(teamMock.member("1002")?.role).toBe("analyst");
    await user.click(within(row).getByRole("button", { name: "Remove and sign out" }));

    // The proxy goes on listing the account, without a role. The row is there once, with what happened.
    const after = await rowOf("Mehmet Demir");
    expect(await within(after).findByRole("status")).toHaveTextContent(
      "Access removed and signed out. The account cannot sign in until it is given a role again.",
    );
    await waitFor(() => expect(within(screen.getByRole("row", { name: /Mehmet Demir/ })).getByRole("combobox")).toHaveValue(""));
    expect(within(screen.getByRole("row", { name: /Mehmet Demir/ })).queryByRole("button", { name: "Remove access" })).toBeNull();
    expect(teamMock.member("1002")).toMatchObject({ role: "", sessions_valid_from: teamMock.now });
    expect(await screen.findByText("2 accounts hold a role on this proxy: 1 admin, 0 analysts, 1 viewer. 1 account has none.")).toBeInTheDocument();
    expect(screen.getAllByRole("row", { name: /Mehmet Demir/ })).toHaveLength(1);
    expect(within(screen.getByRole("row", { name: /Mehmet Demir/ })).getByText(/^Role removed/)).toBeInTheDocument();
    expect(
      within(screen.getByRole("row", { name: /Mehmet Demir/ })).getByText(
        `Sessions ended ${dayLabel(new Date(teamMock.now * 1000).toISOString())}, ${clockAt(teamMock.now)}`,
      ),
    ).toBeInTheDocument();

    // The list read again says the same, and the row's own select gives the role back.
    await reload();
    const listed = screen.getByRole("row", { name: /Mehmet Demir/ });
    expect(within(listed).getByRole("status")).toHaveTextContent(/^Access removed and signed out\./);
    await pickRole(user, listed, "analyst");
    await user.click(within(listed).getByRole("button", { name: "Save role" }));
    expect(await within(await rowOf("Mehmet Demir")).findByRole("status")).toHaveTextContent("Now analyst. The account can sign in.");
    expect(teamMock.member("1002")?.role).toBe("analyst");
  });

  it("keeps the row of an account an older proxy stops listing", async () => {
    teamMock.reset({ older: true });
    const user = show();
    const row = await rowOf("Mehmet Demir");
    await user.click(within(row).getByRole("button", { name: "Remove access" }));
    await user.click(within(row).getByRole("button", { name: "Remove and sign out" }));

    expect(await within(await rowOf("Mehmet Demir")).findByRole("status")).toHaveTextContent(/^Access removed and signed out\./);
    expect(await screen.findByText("2 accounts hold a role on this proxy: 1 admin, 0 analysts, 1 viewer. 1 account has none.")).toBeInTheDocument();
    await reload();
    const kept = screen.getByRole("row", { name: /Mehmet Demir/ });
    expect(within(kept).getByRole("combobox")).toHaveValue("");
    expect(within(kept).getByRole("status")).toHaveTextContent(/^Access removed and signed out\./);
  });

  it("keeps the role when the question is answered with Keep", async () => {
    const user = show();
    const row = await rowOf("Mehmet Demir");
    await user.click(within(row).getByRole("button", { name: "Remove access" }));
    await user.click(within(row).getByRole("button", { name: "Keep" }));
    expect(teamMock.member("1002")?.role).toBe("analyst");
    expect(within(row).queryByRole("status")).toBeNull();
  });

  it("says the account can still sign in as a viewer where the proxy admits it as one", async () => {
    teamMock.reset({ defaultRole: "viewer" });
    const user = show();
    const row = await rowOf("Mehmet Demir");
    await user.click(within(row).getByRole("button", { name: "Remove access" }));
    await user.click(within(row).getByRole("button", { name: "Remove and sign out" }));

    expect(await within(await rowOf("Mehmet Demir")).findByRole("status")).toHaveTextContent(
      "Role removed and signed out. This proxy admits accounts without a role as viewers, so the account can still sign in as one.",
    );
    await waitFor(() =>
      expect(within(within(screen.getByRole("row", { name: /Mehmet Demir/ })).getByRole("combobox")).getByRole("option", { name: "Viewer by default" })).toBeInTheDocument(),
    );
    // The proxy lists the account as a viewer again, and says that is not a role anyone gave it.
    await reload();
    const listed = screen.getByRole("row", { name: /Mehmet Demir/ });
    expect(within(listed).getByRole("combobox")).toHaveValue("");
    expect(within(listed).getByText(`Role removed ${dayLabel(new Date(teamMock.now * 1000).toISOString())}`)).toBeInTheDocument();
    expect(within(listed).queryByRole("button", { name: "Remove access" })).toBeNull();
    expect(within(listed).getByRole("status")).toHaveTextContent(/^Role removed and signed out\./);
  });

  it("says at the row that the only admin cannot be removed", async () => {
    const user = show();
    const row = await rowOf("Ayse Kaya");
    await user.click(within(row).getByRole("button", { name: "Remove access" }));
    await user.click(within(row).getByRole("button", { name: "Remove and sign out" }));

    expect(await within(row).findByRole("alert")).toHaveTextContent(
      "This is the only admin. Access was not removed: make someone else an admin first.",
    );
    expect(teamMock.member("1001")?.role).toBe("admin");
  });

  it("says at the row that nobody removes their own access", async () => {
    const user = show("session-1002");
    const row = await rowOf("Mehmet Demir");
    await user.click(within(row).getByRole("button", { name: "Remove access" }));
    await user.click(within(row).getByRole("button", { name: "Remove and sign out" }));

    expect(await within(row).findByRole("alert")).toHaveTextContent(
      "You cannot remove your own access. Another admin can, or whoever holds the admin key.",
    );
  });
});

describe("Team page: signing someone out", () => {
  it("ends the sessions, keeps the role, and says from when the tokens stopped working", async () => {
    const user = show();
    const row = await rowOf("Mehmet Demir");
    await user.click(within(row).getByRole("button", { name: "Sign out everywhere" }));
    // The first press only asks, in ink: signing in again undoes it.
    expect(teamMock.member("1002")?.sessions_valid_from).toBeUndefined();
    const confirm = within(row).getByRole("button", { name: "Sign out" });
    expect(confirm).not.toHaveClass("cds--btn--danger");
    expect(within(row).getByRole("button", { name: "Keep signed in" })).toHaveFocus();
    await user.click(confirm);

    expect(await within(row).findByRole("status")).toHaveTextContent(
      `Signed out. Tokens issued before ${clockAt(teamMock.now)} no longer work; other replicas follow within 30 seconds.`,
    );
    expect(teamMock.member("1002")).toMatchObject({ role: "analyst", sessions_valid_from: teamMock.now });
    // The row now says when the sessions were ended.
    expect(await within(await rowOf("Mehmet Demir")).findByText(/^Sessions ended /)).toBeInTheDocument();
    expect(within(screen.getByRole("row", { name: /Mehmet Demir/ })).getByRole("combobox")).toHaveValue("analyst");
  });

  it("signs nobody out when the question is answered with Keep signed in, or with Escape", async () => {
    const user = show();
    const row = await rowOf("Mehmet Demir");
    await user.click(within(row).getByRole("button", { name: "Sign out everywhere" }));
    await user.click(within(row).getByRole("button", { name: "Keep signed in" }));
    expect(within(row).queryByRole("button", { name: "Sign out" })).toBeNull();
    // The focus is back where the question was asked, so the keyboard can ask again.
    expect(within(row).getByRole("button", { name: "Sign out everywhere" })).toHaveFocus();

    await user.keyboard("{Enter}");
    expect(within(row).getByRole("button", { name: "Keep signed in" })).toHaveFocus();
    await user.keyboard("{Escape}");
    expect(within(row).queryByRole("button", { name: "Sign out" })).toBeNull();

    expect(teamMock.member("1002")?.sessions_valid_from).toBeUndefined();
    expect(within(row).queryByRole("status")).toBeNull();
  });

  it("signs out from the keyboard", async () => {
    const user = show();
    const row = await rowOf("Mehmet Demir");
    within(row).getByRole("button", { name: "Sign out everywhere" }).focus();
    await user.keyboard("{Enter}");
    await user.tab({ shift: true });
    expect(within(row).getByRole("button", { name: "Sign out" })).toHaveFocus();
    await user.keyboard("{Enter}");
    expect(await within(row).findByRole("status")).toHaveTextContent(/^Signed out\./);
    expect(teamMock.member("1002")?.sessions_valid_from).toBe(teamMock.now);
  });

  it("signs out an account that has no role", async () => {
    teamMock.reset({ directory: [{ user_id: "user_2abc", name: "Zeynep Arslan" }] });
    const user = show();
    const row = await rowOf("Zeynep Arslan");
    await user.click(within(row).getByRole("button", { name: "Sign out everywhere" }));
    await user.click(within(row).getByRole("button", { name: "Sign out" }));
    expect(await within(row).findByRole("status")).toHaveTextContent(/^Signed out\. Tokens issued before /);
    expect(teamMock.member("user_2abc")?.sessions_valid_from).toBe(teamMock.now);
  });
});
