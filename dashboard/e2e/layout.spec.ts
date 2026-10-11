import { readdirSync } from "node:fs";
import path from "node:path";
import { expect, test, type Page } from "@playwright/test";
import { mockProxy, withAdminKey } from "./fixtures";
import { HOSTILE, SHOWN } from "./hostile";

/**
 * No page scrolls sideways, and nothing on it reaches past the right edge of
 * the screen, at a desktop width and on a phone, with every list full of the
 * long values of e2e/hostile.ts.
 *
 * The routes are read from the page tree, app/(dashboard)/dashboard, so a new
 * page is swept without a change here. On each route every tab is opened in
 * turn as well, since most of what a page holds is behind one.
 */

const TREE = path.join(process.cwd(), "app", "(dashboard)", "dashboard");

/**
 * What stands in for a segment that is a parameter. A new one has no value
 * here, and its route then fails with a sentence that says so.
 */
const PARAMS: Record<string, string[]> = {
  // One guide with a short name and one with the longest: they share a layout.
  "[kind]": ["slack", "splunk_hec"],
  // A page the console does not have.
  "[...missing]": ["no-such-page"],
};

/** Every route under /dashboard that has a page.tsx, parameters filled in. */
function routes(dir = TREE, base = "/dashboard"): string[] {
  const found: string[] = [];
  const entries = readdirSync(dir, { withFileTypes: true });
  if (entries.some((e) => e.isFile() && e.name === "page.tsx")) found.push(base);
  for (const entry of entries) {
    // A folder that starts with "_" is private to Next and holds no route.
    if (!entry.isDirectory() || entry.name.startsWith("_")) continue;
    const next = path.join(dir, entry.name);
    // "(group)" names no segment of the address.
    if (/^\(.*\)$/.test(entry.name)) found.push(...routes(next, base));
    else if (entry.name.startsWith("[")) for (const value of PARAMS[entry.name] ?? [entry.name]) found.push(...routes(next, `${base}/${value}`));
    else found.push(...routes(next, `${base}/${entry.name}`));
  }
  return found.sort();
}

const WIDTHS = [
  { name: "desktop", width: 1280, height: 800 },
  { name: "phone", width: 375, height: 812 },
];

/**
 * What may be wider than its box and scroll inside it: a table in its own
 * container, a block of code or of diff, a row of tabs. Anything else that
 * scrolls sideways is a layout that did not hold.
 */
const MAY_SCROLL = [
  ".cds--data-table-container",
  ".cds--data-table-content",
  ".cds--snippet",
  ".cds--snippet-container",
  ".cds--tab--list",
  ".cds--tabs",
  ".t-diff",
  ".t-table",
  "pre",
  "textarea",
].join(", ");

/** The elements that break the rule, each named so that it can be found: tag, classes, the start of its text, by how much. */
async function overflowing(page: Page): Promise<string[]> {
  return page.evaluate((mayScroll) => {
    const root = document.documentElement;
    const edge = root.clientWidth;
    const tag = (el: Element) => {
      const cls = typeof el.className === "string" && el.className.trim() ? "." + el.className.trim().split(/\s+/).join(".") : "";
      return `<${el.tagName.toLowerCase()}${el.id ? `#${el.id}` : ""}${cls}>`;
    };
    /** The element, the start of its text, and the three boxes around it. */
    const name = (el: Element) => {
      const text = (el.textContent ?? "").trim().replace(/\s+/g, " ").slice(0, 48);
      const around: string[] = [];
      for (let p = el.parentElement; p && p !== document.body && around.length < 3; p = p.parentElement) around.push(tag(p));
      return `${tag(el)}${text ? ` "${text}"` : ""}${around.length ? ` in ${around.join(" in ")}` : ""}`;
    };
    /**
     * Cut off sideways by a box around it: it cannot be what makes the page
     * wider. A box only cuts off what it holds, and what is placed out of the
     * flow is held by the nearest positioned box, not by its parent: text
     * hidden for screen readers in a table that scrolls is not cut off by the
     * scroll box unless that box is positioned.
     */
    const clipped = (el: Element) => {
      let position = getComputedStyle(el).position;
      for (let p = el.parentElement; p && p !== document.body && p !== root; p = p.parentElement) {
        const style = getComputedStyle(p);
        const moved = style.transform !== "none";
        if (position === "fixed" && !moved) continue;
        if (position === "absolute" && style.position === "static" && !moved) continue;
        if (style.overflowX !== "visible") return true;
        position = style.position;
      }
      return false;
    };

    const past = new Map<Element, number>();
    const problems: string[] = [];
    for (const el of document.body.querySelectorAll("*")) {
      const style = getComputedStyle(el);
      if (style.visibility === "hidden" || style.display === "none") continue;
      const box = el.getBoundingClientRect();
      // An empty box. Text kept for screen readers alone is a box of one
      // pixel, and is looked at: past the edge, it widens the page as well.
      if (box.width === 0 || box.height === 0) continue;

      if ((style.overflowX === "auto" || style.overflowX === "scroll") && el.scrollWidth > el.clientWidth + 1 && !el.matches(mayScroll)) {
        problems.push(`${name(el)} scrolls sideways (${el.scrollWidth}px of content in ${el.clientWidth}px) and is not one of the boxes that may`);
      }
      // Where the box ends, or where its content does when that runs out of
      // it: a word with no place to break leaves a box that keeps its width.
      const content = style.overflowX === "visible" && el.scrollWidth > el.clientWidth + 1 ? box.left + el.clientLeft + el.scrollWidth : 0;
      const right = Math.max(box.right, content);
      if (right > edge + 1 && !clipped(el)) past.set(el, right);
    }
    // A box is past the edge because something in it is: name the innermost.
    for (const [el, right] of past) {
      if ([...el.children].some((child) => past.has(child))) continue;
      problems.push(`${name(el)} ends ${Math.round(right - edge)}px past the right edge`);
    }
    if (root.scrollWidth > root.clientWidth) {
      problems.unshift(`the page scrolls sideways: ${root.scrollWidth}px of content in ${root.clientWidth}px`);
    }
    return problems;
  }, MAY_SCROLL);
}

/** The page has what it was going to read: no placeholder is left on it. */
async function settled(page: Page) {
  await expect(page.locator("main h1, main h2").first()).toBeVisible({ timeout: 60_000 });
  await expect(page.locator(".cds--skeleton, .cds--skeleton__text, [aria-label='Loading']")).toHaveCount(0, { timeout: 20_000 });
}

const held = (problems: string[], where: string) => expect(problems, `${where}\n${problems.join("\n")}\n`).toEqual([]);

test.beforeEach(async ({ page }) => {
  await withAdminKey(page);
  await mockProxy(page, HOSTILE);
});

// The dev server compiles a page the first time it is asked for.
test.describe.configure({ timeout: 120_000 });

for (const route of routes()) {
  test.describe(route, () => {
    test("is a route this sweep can open", () => {
      const open = route.split("/").find((segment) => segment.startsWith("["));
      expect(open, `${route} has the parameter ${open}: give it a value in PARAMS, in e2e/layout.spec.ts`).toBeUndefined();
    });

    for (const { name, width, height } of WIDTHS) {
      test(`holds at ${width}px (${name}), in every tab`, async ({ page }) => {
        await page.setViewportSize({ width, height });
        await page.goto(route, { timeout: 90_000 });
        await settled(page);
        if (SHOWN[route]) await expect(page.getByText(SHOWN[route]).first()).toBeAttached({ timeout: 15_000 });
        held(await overflowing(page), `${route} at ${width}px`);

        // A page's own tabs. The shell has none.
        const tabs = page.locator("main").getByRole("tab");
        for (let i = 0; i < (await tabs.count()); i++) {
          const tab = tabs.nth(i);
          const label = ((await tab.textContent()) ?? "").trim();
          await tab.click();
          await settled(page);
          held(await overflowing(page), `${route} at ${width}px, tab "${label}"`);
        }
      });
    }
  });
}
