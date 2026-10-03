import { createElement } from "react";
import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { BarList } from "./bar-list";
import { DiffView, diffStats, type DiffLine } from "./diff-view";
import { CircuitBadge } from "./status-badge";

describe("diffStats", () => {
  it("counts added and removed lines and ignores unchanged ones", () => {
    const lines: DiffLine[] = [
      { type: " ", text: "a" },
      { type: "+", text: "b" },
      { type: "+", text: "c" },
      { type: "-", text: "d" },
    ];
    expect(diffStats(lines)).toEqual({ added: 2, removed: 1 });
    expect(diffStats([])).toEqual({ added: 0, removed: 0 });
  });
});

describe("DiffView", () => {
  it("marks changes with a +/- sign, not only with color", () => {
    const lines: DiffLine[] = [
      { type: "-", text: "action: WARN" },
      { type: "+", text: "action: BLOCK" },
    ];
    render(createElement(DiffView, { lines }));
    expect(screen.getByText("-")).toBeInTheDocument();
    expect(screen.getByText("+")).toBeInTheDocument();
    expect(screen.getByText("action: BLOCK")).toBeInTheDocument();
  });
});

describe("BarList", () => {
  const items = [
    { label: "OpenAI", value: 150 },
    { label: "Anthropic", value: 50 },
  ];

  it("shows each item's share of the total by default", () => {
    render(createElement(BarList, { items }));
    expect(screen.getByText("150 · 75%")).toBeInTheDocument();
    expect(screen.getByText("50 · 25%")).toBeInTheDocument();
  });

  it("ranks against the largest item without printing a share", () => {
    render(createElement(BarList, { items, scale: "max", formatValue: (n: number) => `${n} ms` }));
    expect(screen.getByText("150 ms")).toBeInTheDocument();
    expect(screen.queryByText(/%/)).not.toBeInTheDocument();
  });

  it("does not divide by zero when every value is zero", () => {
    render(createElement(BarList, { items: [{ label: "none", value: 0 }] }));
    expect(screen.getByText("0 · 0%")).toBeInTheDocument();
  });
});

describe("CircuitBadge", () => {
  it("treats a closed circuit as healthy and an open one as failing", () => {
    const { rerender } = render(createElement(CircuitBadge, { state: "CLOSED" }));
    expect(screen.getByText("closed").className).toContain("status-pass");

    rerender(createElement(CircuitBadge, { state: "open" }));
    expect(screen.getByText("open").className).toContain("status-critical");

    rerender(createElement(CircuitBadge, { state: "half-open" }));
    expect(screen.getByText("half-open").className).toContain("status-medium");
  });

  it("is neutral for an unknown or missing state", () => {
    render(createElement(CircuitBadge, {}));
    expect(screen.getByText("unknown").className).toContain("text-muted-foreground");
  });
});
