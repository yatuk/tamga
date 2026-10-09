import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test, type APIRequestContext, type Page } from "@playwright/test";
import { liveOnly, PREFIX, unique } from "./live";

/**
 * API keys, Team and Settings against a real proxy.
 *
 * The admin key is read from .env.local, the same file the console is built
 * from. It is never passed to an assertion or typed into the page, so a
 * failure cannot print it.
 */
function env(name: string): string {
  if (process.env[name]) return process.env[name] as string;
  try {
    const line = readFileSync(join(process.cwd(), ".env.local"), "utf8")
      .split(/\r?\n/)
      .find((l) => l.startsWith(`${name}=`));
    return line ? line.slice(name.length + 1).trim().replace(/^"|"$/g, "") : "";
  } catch {
    return "";
  }
}

const API = (env("NEXT_PUBLIC_API_URL") || "http://localhost:8443").replace(/\/$/, "");
const ADMIN = env("NEXT_PUBLIC_ADMIN_KEY");
const auth = { "X-Tamga-Admin-Key": ADMIN };

const ADMIN_KEY_STORAGE = "tamga_admin_key";
const RETENTION_STORAGE = "tamga_retention_days_v1";

type Key = { id: string; label: string; scope: string; prefix: string; org_id?: string; last_used?: string };
type Sso = { provider_type: string; metadata_url: string; attribute_mapping: Record<string, string>; enabled: boolean; domain: string };

async function getJSON<T>(request: APIRequestContext, path: string): Promise<T> {
  const res = await request.get(`${API}${path}`, { headers: auth });
  expect(res.ok(), `GET ${path} answered ${res.status()}`).toBe(true);
  return (await res.json()) as T;
}

const listKeys = async (request: APIRequestContext) => (await getJSON<{ items: Key[] }>(request, "/api/v1/apikeys")).items ?? [];

const visibleModal = (page: Page) => page.locator(".cds--modal.is-visible");
const toastWith = (page: Page, text: string) => page.locator("[data-sonner-toast]").filter({ hasText: text });

test.describe("API keys", () => {
  const KEY_PREFIX = `${PREFIX}-key-`;

  test.beforeEach(() => liveOnly());

  // Revokes whatever a test left behind, also when it failed half way.
  test.afterEach(async ({ request }) => {
    if (!process.env.E2E_LIVE) return;
    for (const key of await listKeys(request)) {
      if (key.label.startsWith(KEY_PREFIX)) {
        await request.delete(`${API}/api/v1/apikeys/${encodeURIComponent(key.id)}`, { headers: auth });
      }
    }
  });

  /** The sentence under the title, as the page builds it from the list. */
  async function expectLede(page: Page, request: APIRequestContext) {
    await expect(async () => {
      const keys = await listKeys(request);
      const apps = keys.filter((k) => k.scope === "proxy").length;
      const admins = keys.filter((k) => k.scope === "admin").length;
      const start =
        keys.length === 0
          ? "An application key identifies a caller on the proxy"
          : `${keys.length} ${keys.length === 1 ? "key" : "keys"}: ${apps} identify applications on the proxy, ${admins} can do everything.`;
      await expect(page.locator(".t-lede").filter({ hasText: start })).toBeVisible({ timeout: 1_000 });
    }).toPass({ timeout: 10_000 });
  }

  async function openKeys(page: Page) {
    await page.goto("/dashboard/keys");
    await expect(page.getByRole("heading", { name: "API keys" })).toBeVisible();
    await expect(page.getByRole("button", { name: "New key" })).toBeEnabled();
  }

  /** Fills the "New API key" modal and submits it. Leaves the reveal modal open. */
  async function createKey(page: Page, name: string, scope: "proxy" | "read", org?: string) {
    await page.getByRole("button", { name: "New key" }).click();
    const modal = visibleModal(page);
    await expect(modal.getByRole("heading", { name: "New API key" })).toBeVisible();
    await expect(modal.getByRole("button", { name: "Create key" })).toBeDisabled();
    await modal.locator("#new-key-name").fill(name);
    if (scope !== "proxy") {
      await modal.locator(`label[for="scope-${scope}"]`).click();
      await expect(modal.locator("#new-key-org")).toHaveCount(0);
    }
    if (org) await modal.locator("#new-key-org").fill(org);
    await modal.getByRole("button", { name: "Create key" }).click();
    await expect(visibleModal(page).getByRole("heading", { name: "Copy the key now" })).toBeVisible();
  }

  /** Checks the one-time reveal and that only its own button closes it. Returns the full key. */
  async function answerReveal(page: Page, name: string): Promise<string> {
    const modal = visibleModal(page);
    await expect(modal).toContainText(name);
    const raw = ((await modal.locator(".cds--snippet pre, .cds--snippet code").first().textContent()) ?? "").trim();
    expect(raw.length).toBeGreaterThan(20);
    expect(raw).not.toContain("…");

    await page.keyboard.press("Escape");
    await expect(modal.getByRole("heading", { name: "Copy the key now" })).toBeVisible();
    await page.mouse.click(5, 5);
    await expect(modal.getByRole("heading", { name: "Copy the key now" })).toBeVisible();
    await expect(modal.locator(".cds--modal-close-button")).toBeHidden();

    await modal.getByRole("button", { name: "I have saved it" }).click();
    await expect(visibleModal(page)).toHaveCount(0);
    return raw;
  }

  const rowOf = (page: Page, name: string) => page.getByRole("row").filter({ hasText: name });

  test("an application key without an organisation is created, shown once and revoked", async ({ page, request }) => {
    const name = unique("key-app");
    await openKeys(page);
    await expectLede(page, request);

    await createKey(page, name, "proxy");
    const raw = await answerReveal(page, name);

    const row = rowOf(page, name);
    await expect(row).toBeVisible();
    await expect(row.getByRole("cell").nth(0)).toContainText("Organisation default");
    await expect(row.getByRole("cell").nth(1)).toHaveText("Identify an application");
    const prefix = ((await row.getByRole("cell").nth(2).textContent()) ?? "").replace(/[….*]+$/, "").trim();
    expect(prefix.length).toBeGreaterThan(3);
    expect(raw.startsWith(prefix)).toBe(true);
    await expect(row.getByRole("cell").nth(4)).toContainText("Never");
    await expectLede(page, request);

    const stored = (await listKeys(request)).find((k) => k.label === name);
    expect(stored?.scope).toBe("proxy");
    expect(stored?.org_id ?? "").toBe("");
    // As the form says, an application key opens nothing in the management API.
    const asAdmin = await request.get(`${API}/api/v1/stats`, { headers: { "X-Tamga-Admin-Key": raw } });
    expect(asAdmin.status()).toBe(403);

    // Backing out keeps the key.
    await row.getByRole("button", { name: "Revoke", exact: true }).click();
    await expect(row.getByRole("button", { name: "Revoke for good" })).toBeVisible();
    await row.getByRole("button", { name: "Keep" }).click();
    await expect(row.getByRole("button", { name: "Revoke for good" })).toHaveCount(0);
    await expect(row.getByRole("button", { name: "Revoke", exact: true })).toBeVisible();
    expect((await listKeys(request)).some((k) => k.label === name)).toBe(true);

    await row.getByRole("button", { name: "Revoke", exact: true }).click();
    await row.getByRole("button", { name: "Revoke for good" }).click();
    await expect(toastWith(page, "API key revoked")).toBeVisible();
    await expect(rowOf(page, name)).toHaveCount(0);
    expect((await listKeys(request)).some((k) => k.label === name)).toBe(false);
    await expectLede(page, request);
  });

  test("an application key keeps the organisation it was created for", async ({ page, request }) => {
    const name = unique("key-org");
    const org = `${PREFIX}-org`;
    await openKeys(page);

    await createKey(page, name, "proxy", org);
    await answerReveal(page, name);

    const row = rowOf(page, name);
    await expect(row.getByRole("cell").nth(0)).toContainText(`Organisation ${org}`);
    await expect(row.getByRole("cell").nth(1)).toHaveText("Identify an application");
    expect((await listKeys(request)).find((k) => k.label === name)?.org_id).toBe(org);
    await expectLede(page, request);

    await row.getByRole("button", { name: "Revoke", exact: true }).click();
    await row.getByRole("button", { name: "Revoke for good" }).click();
    await expect(rowOf(page, name)).toHaveCount(0);
    await expectLede(page, request);
  });

  test("a read key is created with the read wording and no organisation", async ({ page, request }) => {
    const name = unique("key-read");
    await openKeys(page);

    await createKey(page, name, "read");
    const raw = await answerReveal(page, name);

    const row = rowOf(page, name);
    await expect(row.getByRole("cell").nth(1)).toHaveText("Read");
    await expect(row.getByRole("cell").nth(0)).not.toContainText("Organisation");
    expect((await listKeys(request)).find((k) => k.label === name)?.scope).toBe("read");
    await expectLede(page, request);

    // The key really opens the management API for reading, and nothing more.
    const read = await request.get(`${API}/api/v1/stats`, { headers: { "X-Tamga-Admin-Key": raw } });
    expect(read.status()).toBe(200);
    const write = await request.delete(`${API}/api/v1/apikeys/${PREFIX}-no-such-key`, { headers: { "X-Tamga-Admin-Key": raw } });
    expect(write.status()).toBe(403);

    // The form starts clean the next time it is opened.
    await page.getByRole("button", { name: "New key" }).click();
    await expect(visibleModal(page).locator("#new-key-name")).toHaveValue("");
    await expect(visibleModal(page).locator("#scope-proxy")).toBeChecked();
    await visibleModal(page).getByRole("button", { name: "Cancel" }).click();
    await expect(visibleModal(page)).toHaveCount(0);

    await row.getByRole("button", { name: "Revoke", exact: true }).click();
    await row.getByRole("button", { name: "Revoke for good" }).click();
    await expect(rowOf(page, name)).toHaveCount(0);
    const gone = await request.get(`${API}/api/v1/stats`, { headers: { "X-Tamga-Admin-Key": raw } });
    expect(gone.status()).toBe(401);
  });
});

test.describe("Team", () => {
  test.beforeEach(() => liveOnly());

  type Member = { user_id: string; name?: string; role: string };
  const team = (request: APIRequestContext) => getJSON<{ items: Member[] | null; clerk: boolean }>(request, "/api/v1/team");

  test("says what the proxy knows about the team and nothing more", async ({ page, request }) => {
    const errors: string[] = [];
    page.on("pageerror", (e) => errors.push(e.message));
    const { items, clerk } = await team(request);
    const members = items ?? [];

    await page.goto("/dashboard/team");
    await expect(page.getByRole("heading", { name: "Team", exact: true })).toBeVisible();
    await expect(page.getByRole("heading", { name: "What each role can do" })).toBeVisible();
    await expect(page.getByText("The proxy did not answer.")).toHaveCount(0);

    const warning = page.getByText("The identity provider is not connected.");
    if (clerk) await expect(warning).toHaveCount(0);
    else await expect(warning).toBeVisible();
    await expect(page.getByRole("link", { name: "Invite someone in Clerk" })).toHaveCount(clerk ? 1 : 0);

    if (members.length === 0) {
      await expect(page.getByText("Nobody yet. People appear after they sign in")).toBeVisible();
      await expect(page.getByRole("table", { name: "Members" })).toHaveCount(0);
    } else {
      await expect(page.getByRole("table", { name: "Members" }).getByRole("row")).toHaveCount(members.length + 1);
      await expect(page.getByText(`${members.length} ${members.length === 1 ? "person" : "people"} can sign in`)).toBeVisible();
    }
    expect(errors).toEqual([]);
  });

  test("a member's role is changed and changed back", async ({ page, request }) => {
    const { items } = await team(request);
    const member = (items ?? [])[0];
    // Setting a role is the only way to add a member, and the proxy has no call to remove one.
    test.skip(!member, "The proxy has no members, and a test member could not be removed again.");

    const other = member.role === "viewer" ? "analyst" : "viewer";
    try {
      await page.goto("/dashboard/team");
      const select = page.locator(`[id="role-${member.user_id}"]`);
      const saveRole = page.getByRole("button", { name: /^Save/ });
      await expect(select).toHaveValue(member.role);

      await select.selectOption(other);
      await saveRole.click();
      await expect(toastWith(page, "Role updated")).toBeVisible();
      await expect(select).toHaveValue(other);
      expect((await team(request)).items?.find((m) => m.user_id === member.user_id)?.role).toBe(other);

      await expect(select).toBeEnabled();
      await select.selectOption(member.role);
      await saveRole.click();
      await expect(select).toHaveValue(member.role);
    } finally {
      await request.put(`${API}/api/v1/team/${encodeURIComponent(member.user_id)}/role`, {
        headers: auth,
        data: { role: member.role },
      });
    }
    expect((await team(request)).items?.find((m) => m.user_id === member.user_id)?.role).toBe(member.role);
  });
});

test.describe("Settings", () => {
  test.beforeEach(() => liveOnly());

  const stored = (page: Page, key: string) => page.evaluate((k) => window.localStorage.getItem(k), key);

  test("access: the admin key is saved in the browser and used from then on", async ({ page }) => {
    const wrong = `${PREFIX}-not-a-key`;
    await page.goto("/dashboard/settings?tab=access");
    const field = page.locator("#admin-key-input");
    const save = page.getByRole("button", { name: "Save the key" });
    await expect(field).toHaveAttribute("type", "password");
    await expect(field).not.toHaveValue("");
    await expect(save).toBeDisabled();
    expect(await stored(page, ADMIN_KEY_STORAGE)).toBeNull();

    try {
      await field.fill(wrong);
      await expect(save).toBeEnabled();
      await save.click();
      await expect(toastWith(page, "Admin key saved")).toBeVisible();
      await expect(save).toBeDisabled();
      expect(await stored(page, ADMIN_KEY_STORAGE)).toBe(wrong);

      // The saved key is the one the console now sends: the proxy turns it away.
      await page.goto("/dashboard/keys");
      await expect(page.getByText("The proxy did not answer.")).toBeVisible({ timeout: 15_000 });

      // Emptying the field removes the saved key, and the built-in one applies again.
      await page.goto("/dashboard/settings?tab=access");
      await expect(field).toHaveValue(wrong);
      await field.fill("");
      await expect(save).toBeEnabled();
      await save.click();
      await expect(toastWith(page, "Admin key saved")).toBeVisible();
      expect(await stored(page, ADMIN_KEY_STORAGE)).toBeNull();
      await expect(field).not.toHaveValue("");
      await expect(field).not.toHaveValue(wrong);
      await expect(save).toBeDisabled();
    } finally {
      await page.evaluate((k) => window.localStorage.removeItem(k), ADMIN_KEY_STORAGE);
    }

    await page.goto("/dashboard/keys");
    await expect(page.getByRole("button", { name: "New key" })).toBeEnabled();
    await expect(page.getByText("The proxy did not answer.")).toHaveCount(0);
  });

  test("retention: the number of days is saved and still there after a reload", async ({ page }) => {
    await page.goto("/dashboard/settings?tab=retention");
    const days = page.locator("#retention-days-input");
    await expect(days).toHaveValue("30");

    // The markup arrives before React does; typing earlier than that is thrown away.
    await page.waitForFunction(() => {
      const input = document.querySelector("#retention-days-input");
      return !!input && Object.keys(input).some((k) => k.startsWith("__reactProps"));
    });
    const save = page.getByRole("button", { name: "Save", exact: true });

    // Typed the way a person does it: clear the field, then type.
    await days.click();
    await days.press("ControlOrMeta+a");
    await days.press("Backspace");
    await expect(days).toHaveValue("");
    await expect(page.getByText("Enter a whole number of days from 1 to 365.")).toBeVisible();
    await expect(save).toBeDisabled();
    await days.pressSequentially("45", { delay: 120 });
    await expect(days).toHaveValue("45");
    await save.click();
    await expect(toastWith(page, "Dashboard window saved")).toBeVisible();
    await expect(toastWith(page, "45 days")).toBeVisible();
    expect(await stored(page, RETENTION_STORAGE)).toBe("45");

    await page.reload();
    await expect(page.locator("#retention-days-input")).toHaveValue("45");

    await page.evaluate((k) => window.localStorage.removeItem(k), RETENTION_STORAGE);
  });

  test("runtime: shows what the proxy reports about itself", async ({ page, request }) => {
    const detail = await getJSON<{
      version?: string;
      policy_name?: string;
      policy_path?: string;
      scanner_count?: number;
      tls_enabled?: boolean;
      mtls_enabled?: boolean;
      redis_enabled?: boolean;
      database?: string;
    }>(request, "/api/v1/health/detail");

    await page.goto("/dashboard/settings?tab=runtime");
    const fact = (label: string) => page.locator(".t-facts div").filter({ has: page.locator("dt", { hasText: new RegExp(`^${label}$`) }) }).locator("dd");

    await expect(fact("Proxy")).toHaveText("Up");
    if (detail.version) await expect(fact("Version")).toHaveText(detail.version);
    if (detail.policy_name) await expect(fact("Policy")).toHaveText(detail.policy_name);
    if (detail.policy_path) await expect(fact("Policy file")).toHaveText(detail.policy_path);
    await expect(fact("Scanners")).toHaveText(String(detail.scanner_count ?? 0));
    await expect(fact("TLS")).toHaveText(detail.tls_enabled ? "On" : "Off, plain HTTP");
    await expect(fact("Mutual TLS")).toHaveText(detail.mtls_enabled ? "Required" : "Off");
    await expect(fact("Rate limit state")).toHaveText(detail.redis_enabled ? "In Redis, shared between nodes" : "In memory, this node only");
    if (detail.database === "connected") await expect(fact("Database")).toHaveText("Connected");
    await expect(fact("Running for")).toHaveText(/^\d+ (d \d+ h|h \d+ min|min)$/);
  });

  test("providers: lists the pools the proxy has, or says there are none", async ({ page, request }) => {
    const health = await getJSON<{ providers?: { pool: string; healthy_count: number; total_count: number }[] | null }>(
      request,
      "/api/v1/health/detailed",
    );
    const pools = health.providers ?? [];

    await page.goto("/dashboard/settings?tab=providers");
    await expect(page.getByRole("tab", { name: "Providers" })).toHaveAttribute("aria-selected", "true");
    if (pools.length === 0) {
      await expect(page.getByText("No provider pool is defined.")).toBeVisible();
      await expect(page.getByRole("button", { name: "Send traffic again" })).toHaveCount(0);
    } else {
      for (const pool of pools) {
        await expect(page.getByRole("heading", { name: `${pool.pool} pool` })).toBeVisible();
      }
      await expect(page.getByText("No provider pool is defined.")).toHaveCount(0);
    }
  });

  test.describe("sign-in configuration", () => {
    let original: Sso | null = null;

    test.beforeEach(async ({ request }) => {
      if (!process.env.E2E_LIVE) return;
      original = await getJSON<Sso>(request, "/api/v1/settings/sso");
    });

    test.afterEach(async ({ request }) => {
      if (!original) return;
      await request.put(`${API}/api/v1/settings/sso`, { headers: auth, data: original });
      original = null;
    });

    test("sso: the configuration is saved, survives a reload and is put back", async ({ page, request }) => {
      const before = original as Sso;
      const protocol = before.provider_type === "oidc" ? "saml" : "oidc";
      const url = `https://${PREFIX}.example.com/${unique("idp")}`;
      const domain = `${PREFIX}.example.com`;

      await page.goto("/dashboard/settings?tab=sso");
      const type = page.locator("#sso-provider-type");
      const metadata = page.locator("#sso-metadata-url");
      const mail = page.locator("#sso-domain");
      const required = page.locator("#sso-enabled");
      // Carbon draws the switch inside the label; the button itself sits under it.
      const flip = page.locator('label[for="sso-enabled"] .cds--toggle__switch');
      const save = page.getByRole("button", { name: "Save", exact: true });

      // The form shows what the proxy has.
      await expect(type).toHaveValue(before.provider_type);
      await expect(metadata).toHaveValue(before.metadata_url);
      await expect(mail).toHaveValue(before.domain);
      await expect(required).toHaveAttribute("aria-checked", String(before.enabled));

      await type.selectOption(protocol);
      await metadata.fill(url);
      await mail.fill(domain);
      await flip.click();
      await expect(required).toHaveAttribute("aria-checked", String(!before.enabled));
      await save.click();
      await expect(toastWith(page, "Sign-in configuration saved")).toBeVisible();

      const after = await getJSON<Sso>(request, "/api/v1/settings/sso");
      expect(after).toMatchObject({ provider_type: protocol, metadata_url: url, domain, enabled: !before.enabled });
      // Saving the form must not rewrite what the form does not show.
      expect(after.attribute_mapping).toEqual(before.attribute_mapping);

      await page.reload();
      await expect(type).toHaveValue(protocol);
      await expect(metadata).toHaveValue(url);
      await expect(mail).toHaveValue(domain);
      await expect(required).toHaveAttribute("aria-checked", String(!before.enabled));

      // Put back through the form as well.
      await type.selectOption(before.provider_type);
      await metadata.fill(before.metadata_url);
      await mail.fill(before.domain);
      await flip.click();
      await save.click();
      await expect(toastWith(page, "Sign-in configuration saved")).toBeVisible();
      await expect(async () => {
        expect(await getJSON<Sso>(request, "/api/v1/settings/sso")).toEqual(before);
      }).toPass({ timeout: 5_000 });
    });

    test("sso: what the proxy refuses is reported and not saved", async ({ page, request }) => {
      const before = original as Sso;
      await page.goto("/dashboard/settings?tab=sso");
      const mail = page.locator("#sso-domain");
      await expect(mail).toHaveValue(before.domain);

      await mail.fill(`${PREFIX}-no-dot`);
      await page.getByRole("button", { name: "Save", exact: true }).click();
      await expect(toastWith(page, "Could not save the sign-in configuration")).toBeVisible();
      expect(await getJSON<Sso>(request, "/api/v1/settings/sso")).toEqual(before);
    });
  });
});
