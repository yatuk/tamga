import fs from "node:fs";
import path from "node:path";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createElement, type ReactElement } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { CostsBody } from "@/app/(dashboard)/dashboard/costs/CostsBody";
import { LatencyBody } from "@/app/(dashboard)/dashboard/latency/LatencyBody";
import { ProxyBody } from "@/app/(dashboard)/dashboard/proxy/ProxyBody";
import { highlight, theme } from "@/app/(dashboard)/dashboard/policies/policy-code-editor";
import { Scale } from "./charts";
import { EventTable } from "./events";
import { Bar, Count } from "./motion";
import { Remove } from "./page";
import { useSwap } from "./rows";

/**
 * The rules DESIGN.md states, held against the code: red is a stopped
 * request, ochre a changed one, colours are tokens, nothing animates for
 * arriving, and reduced motion means no tween at all.
 */

const reduced = vi.hoisted(() => ({ value: false as boolean | null }));
vi.mock("motion/react", async (original) => ({
  ...(await original<typeof import("motion/react")>()),
  useReducedMotion: () => reduced.value,
}));

const ROOT = path.resolve(__dirname, "../..");
const CSS = fs.readFileSync(path.join(ROOT, "app/tamga.css"), "utf8");
const COLOUR = /#[0-9a-fA-F]{3,8}\b|\b(?:rgba?|hsla?|oklch|oklab|lab|lch|hwb|color)\(/;

/** Every rule of the stylesheet as [selector, body], with the at-rule wrappers opened up. */
function rules(css: string): [string, string][] {
  const plain = css.replace(/\/\*[\s\S]*?\*\//g, "");
  const out: [string, string][] = [];
  const re = /([^{}]+)\{([^{}]*)\}/g;
  for (let m = re.exec(plain); m; m = re.exec(plain)) {
    const selector = m[1].replace(/^[\s\S]*@(?:media|supports)[^{]*\{/, "").replace(/@import[^;]*;/, "").replace(/\s+/g, " ").trim();
    out.push([selector, m[2]]);
  }
  return out;
}

function sources(dir: string): string[] {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) return sources(full);
    return /\.tsx?$/.test(entry.name) && !/\.test\.tsx?$/.test(entry.name) ? [full] : [];
  });
}

function show(node: ReactElement) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(createElement(QueryClientProvider, { client: qc }, node));
}

beforeEach(() => {
  reduced.value = false;
});

describe("colour", () => {
  it("is written as a literal only where the tokens are defined", () => {
    const root = CSS.slice(CSS.indexOf(":root {"), CSS.indexOf("}", CSS.indexOf(":root {")));
    const rest = CSS.replace(root, "").replace(/\/\*[\s\S]*?\*\//g, "");
    const stray = rest.split("\n").filter((line) => COLOUR.test(line));
    expect(stray).toEqual([]);
  });

  it("is not written as a literal in any component", () => {
    const stray: string[] = [];
    for (const file of [...sources(path.join(ROOT, "app")), ...sources(path.join(ROOT, "components"))]) {
      fs.readFileSync(file, "utf8")
        .split("\n")
        .forEach((line, i) => {
          // The theme-color meta tag cannot read a CSS variable; it repeats `paper`.
          if (COLOUR.test(line) && !line.includes("themeColor")) stray.push(`${path.relative(ROOT, file).replace(/\\/g, "/")}:${i + 1}`);
        });
    }
    expect(stray).toEqual([]);
  });

  it("red is drawn only for a stopped request, the mark and the diff", () => {
    const red = rules(CSS)
      .filter(([selector, body]) => selector !== ":root" && /--tamga-(stamp|diff-removed)/.test(body))
      .map(([selector]) => selector.replace(/\.tamga /g, ""));
    expect(red).toEqual([
      ".t-brand i",
      ".cds--header .t-brand i",
      '.t-figure[data-stopped="true"]',
      '.t-split span[data-kind="blocked"]',
      '.t-splitkey span[data-kind="blocked"]::before',
      '.t-day span[data-kind="blocked"]',
      '.t-action[data-action="BLOCK"]',
      '.t-chart .t-day span[data-kind="blocked"]',
      '.t-sample mark[data-action="BLOCK"]',
      '.t-verdict[data-action="BLOCK"]',
      '.t-diff > div[data-type="-"]',
    ]);
  });

  it("ochre is drawn only for something that was changed", () => {
    const ochre = rules(CSS)
      .filter(([selector, body]) => selector !== ":root" && /--tamga-altered/.test(body))
      .map(([selector]) => selector.replace(/\.tamga /g, ""));
    expect(ochre).toEqual([
      '.t-split span[data-kind="altered"]',
      '.t-splitkey span[data-kind="altered"]::before',
      '.t-day span[data-kind="altered"]',
      '.t-action[data-action="REDACT"], .t-action[data-action="STRIP"]',
      '.t-chart .t-day span[data-kind="altered"]',
      '.t-verdict[data-action="REDACT"], .t-verdict[data-action="STRIP"]',
      '.t-sample mark[data-action="REDACT"], .t-sample mark[data-action="STRIP"]',
      ".t-unsaved",
      ".t-changed li",
    ]);
  });

  it("the policy editor reads the tokens, and none of them is red or ochre", () => {
    const values = [...Object.values(theme).flatMap((spec) => Object.values(spec)), ...highlight.flatMap((style) => [style.color])];
    for (const value of values) expect(value).not.toMatch(COLOUR);
    const colours = values.filter((value) => value.includes("var("));
    expect(colours.length).toBeGreaterThan(8);
    for (const value of colours) expect(value).toMatch(/var\(--tamga-(ink|ink-2|sheet|hover|rule|font-data)\)/);
    expect(theme["&"].fontSize).toBe("0.8125rem");
  });
});

describe("shape and depth", () => {
  it("no corner is rounded but the status dot's, and no shadow is dropped", () => {
    for (const [selector, body] of rules(CSS)) {
      const radius = /border-radius:\s*([^;]+)/.exec(body)?.[1].trim();
      if (radius && radius !== "0") expect(`${selector}: ${radius}`).toMatch(/t-dot.*: 50%$/);
      for (const shadow of body.match(/box-shadow:\s*[^;]+/g) ?? []) {
        const layers = shadow.replace(/box-shadow:\s*/, "").split(/,(?![^(]*\))/);
        for (const layer of layers) expect(layer.trim()).toMatch(/^(none|inset )/);
      }
    }
  });

  it("no selector waits for an attribute the code never sets", () => {
    expect(CSS).not.toContain(".t-table[data-swap]");
    expect(CSS).not.toContain("data-quiet");
  });
});

describe("what is not a stopped request is not red", () => {
  const noop = () => undefined;

  it("a 95th percentile above the target is said in words and weight", () => {
    const { container } = show(
      <LatencyBody
        {...({
          adminKey: "test-key",
          range: "7d",
          setRange: noop,
          isLoading: false,
          hasError: false,
          p50: 900,
          p95: 3200,
          p99: 4100,
          points: [],
          hourly: false,
          scannerCount: 4,
          detailLocked: false,
          detailState: "ok",
          providerPools: [],
          circuitReset: { mutate: noop, isPending: false },
          uptimeSeconds: 120,
        } as unknown as Parameters<typeof LatencyBody>[0])}
      />,
    );
    expect(container.querySelector(".t-figure")).not.toBeNull();
    expect(container.querySelector("[data-stopped]")).toBeNull();
    expect(screen.getByText(/That is above the .* target\./).tagName).toBe("B");
  });

  it("the proxy being down is said in ink", () => {
    const { container } = show(
      <ProxyBody
        {...({
          adminKey: "test-key",
          isLoading: false,
          hasError: true,
          isOnline: false,
          detailState: "pending",
          health: undefined,
          detail: undefined,
          componentRows: [],
        } as unknown as Parameters<typeof ProxyBody>[0])}
      />,
    );
    expect(container.querySelector(".t-figure")).toHaveTextContent("Down");
    expect(container.querySelector("[data-stopped]")).toBeNull();
    expect(container.querySelector('[data-action="BLOCK"]')).toBeNull();
  });

  it("a token budget with nothing left is an ink bar and a bold sentence", () => {
    const { container } = show(
      <CostsBody
        {...({
          adminKey: "test-key",
          range: "7d",
          setRange: noop,
          isLoading: false,
          hasError: false,
          tokensToday: 5000,
          costToday: 1,
          limitTokens: 5000,
          limitCost: 1,
          modelCostRows: [],
          dailyRows: [],
          totalCostEstimate: 1,
          mtdTotalUSD: 1,
          projectedMonthlyUSD: 2,
          exportCsv: noop,
          costPerRequest: 0.1,
          avgTokensPerRequest: 10,
          modelFamilyBars: [],
          unpriced: { models: 0, tokens: 0 },
        } as unknown as Parameters<typeof CostsBody>[0])}
      />,
    );
    expect(screen.getByText("Nothing left: requests are being refused").tagName).toBe("B");
    expect(container.querySelectorAll('.t-split [data-kind="ink"]')).toHaveLength(1);
    expect(container.querySelector('[data-kind="blocked"]')).toBeNull();
    expect(container.querySelector("[data-stopped]")).toBeNull();
  });
});

describe("Remove", () => {
  it("asks for something that is gone for good with the solid button, and never a danger one", async () => {
    const user = userEvent.setup();
    const onConfirm = vi.fn();
    render(<Remove label="Delete card numbers" verb="Delete the pattern" onConfirm={onConfirm} />);
    await user.click(screen.getByRole("button", { name: "Delete card numbers" }));
    const answer = screen.getByRole("button", { name: "Delete the pattern" });
    expect(answer).toHaveClass("cds--btn--primary");
    expect(answer).not.toHaveClass("cds--btn--danger");
    expect(answer).toHaveAttribute("data-answer", "irreversible");
    // The way out has the focus, so a second Enter keeps the thing.
    expect(screen.getByRole("button", { name: "Keep" })).toHaveFocus();
    expect(screen.getByRole("button", { name: "Keep" })).toHaveClass("cds--btn--ghost");
    await user.click(answer);
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it("asks for something that can be undone with the outlined button", async () => {
    const user = userEvent.setup();
    render(<Remove label="Sign out Ada everywhere" text="Sign out everywhere" verb="Sign out" keep="Keep signed in" recoverable onConfirm={() => undefined} />);
    await user.click(screen.getByRole("button", { name: "Sign out everywhere" }));
    const answer = screen.getByRole("button", { name: "Sign out" });
    expect(answer).toHaveClass("cds--btn--tertiary");
    expect(answer).not.toHaveClass("cds--btn--primary");
    expect(answer).toHaveAttribute("data-answer", "recoverable");
    expect(screen.getByRole("button", { name: "Keep signed in" })).toHaveFocus();
  });
});

describe("reduced motion", () => {
  it("draws a bar at its share, with nothing left to animate", () => {
    reduced.value = true;
    const { container, rerender } = render(<Bar share={0.25} data-kind="ink" />);
    const bar = container.firstElementChild as HTMLElement;
    expect(bar.style.width).toBe("25%");
    expect(bar).toHaveAttribute("data-kind", "ink");
    rerender(<Bar share={0.8} data-kind="ink" />);
    expect((container.firstElementChild as HTMLElement).style.width).toBe("80%");
    rerender(<Bar share={0.5} axis="y" />);
    expect((container.firstElementChild as HTMLElement).style.height).toBe("50%");
  });

  it("puts a scale mark at its value", () => {
    reduced.value = true;
    const format = (n: number) => `${n} ms`;
    const { container, rerender } = render(<Scale label="Scan time" format={format} marks={[{ name: "Median", value: 50 }, { name: "95th", value: 100 }]} />);
    const lefts = () => [...container.querySelectorAll<HTMLElement>(".t-mark")].map((mark) => Math.round(parseFloat(mark.style.left)));
    expect(lefts()).toEqual([46, 93]);
    rerender(<Scale label="Scan time" format={format} marks={[{ name: "Median", value: 100 }, { name: "95th", value: 100 }]} />);
    expect(lefts()).toEqual([93, 93]);
  });

  it("shows a figure's new value at once", () => {
    reduced.value = true;
    const { container, rerender } = render(<Count value={12} />);
    expect(container).toHaveTextContent("12");
    rerender(<Count value={1204} />);
    expect(container.textContent).toBe("1,204");
  });
});

describe("rows", () => {
  it("are still the first time they are shown, and fade when a later change swaps them", () => {
    const { result, rerender } = renderSwap("7d");
    expect(result()).toBeUndefined();
    rerender("7d");
    expect(result()).toBeUndefined();
    rerender("30d");
    expect(result()).toBe("");
    // Back to the first window is a change too.
    rerender("7d");
    expect(result()).toBe("");
  });

  it("do not count a filter changed before anything was shown", () => {
    const { result, rerender } = renderSwap("7d", false);
    rerender("30d", false);
    rerender("30d", true);
    expect(result()).toBeUndefined();
    rerender("24h", true);
    expect(result()).toBe("");
  });

  it("the request table sets the attribute only on a later set", () => {
    const table = (swapKey: string) => (
      <EventTable
        events={[]}
        label="Events"
        swapKey={swapKey}
        openId=""
        onToggle={() => undefined}
        onClose={() => undefined}
        adminKey="test-key"
        empty="No request went through in this window."
      />
    );
    const { container, rerender } = show(table("|7d"));
    expect(container.querySelector("tbody")).not.toHaveAttribute("data-swap");
    const qc = new QueryClient();
    rerender(createElement(QueryClientProvider, { client: qc }, table("block|7d")));
    expect(container.querySelector("tbody")).toHaveAttribute("data-swap");
  });
});

function renderSwap(first: string, ready = true) {
  let value: "" | undefined;
  function Probe({ swapKey, ready }: { swapKey: string; ready: boolean }) {
    value = useSwap(swapKey, ready);
    return null;
  }
  const view = render(<Probe swapKey={first} ready={ready} />);
  return {
    result: () => value,
    rerender: (swapKey: string, isReady = true) => view.rerender(<Probe swapKey={swapKey} ready={isReady} />),
  };
}
