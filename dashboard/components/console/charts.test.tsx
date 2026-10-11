import { afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Columns, keptLabels, LABEL_GAP, labelWidth, lineLabels, Trace, underColumns, underPoints, type Column, type TracePoint } from "./charts";

/**
 * What the two charts say to someone who cannot see them or cannot point at
 * them: their name, and each bucket read out as the arrow keys walk it.
 */

const IDLE = "Point at a column, or use the arrow keys, to read that day.";
const ms = (n: number) => `${n} ms`;

const days = (count: number): Column[] =>
  Array.from({ length: count }, (_, i) => ({
    key: `d${i}`,
    label: `${i + 1} Oct`,
    says: `${i + 1} Oct: ${i * 10} requests`,
    parts: [
      { kind: "blocked" as const, value: i },
      { kind: "passed" as const, value: i * 9 },
    ],
  }));

const trace = (values: (number | null)[]): TracePoint[] => values.map((value, i) => ({ key: `h${i}`, name: `${10 + i}:00`, value }));

/** The line above a chart that reads a bucket out. */
const readout = (text: string) => screen.getByText(text);

/** A chart `width` pixels wide. Nothing is laid out here, so without this a chart has no width: no line, and only the two end labels. */
function measured(width: number) {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      constructor(private report: (entries: { contentRect: { width: number } }[]) => void) {}
      observe() {
        this.report([{ contentRect: { width } }]);
      }
      unobserve() {}
      disconnect() {}
    },
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

/** The labels written under a chart, in order. */
const written = (container: HTMLElement) => [...container.querySelectorAll("[aria-hidden='true'] [data-label]")].map((el) => el.textContent);

describe("keptLabels", () => {
  const EM = 12;
  // What the console writes under a chart: an hour, a day, and a bucket's full name.
  const hourly = (i: number) => `${String(i % 24).padStart(2, "0")}:00`;
  const daily = (i: number) => `${i + 1} Oct`;
  const named = (i: number) => `Wed ${i + 1} Oct, ${hourly(i)}`;
  const underColumnsKept = (widths: number[], room: number) => keptLabels(widths.length, underColumns(widths, room), 13);
  const AXES = [
    { name: "columns of hours", text: hourly, span: underColumns, kept: underColumnsKept, most: 13 },
    { name: "columns of days", text: daily, span: underColumns, kept: underColumnsKept, most: 13 },
    { name: "a line of hours", text: named, span: underPoints, kept: lineLabels, most: 3 },
    { name: "a line of days", text: daily, span: underPoints, kept: lineLabels, most: 3 },
  ];
  // The width of the chart, not of the window: 288px is what a 320px phone leaves it.
  const ROOMS = Array.from({ length: (1440 - 280) / 8 + 1 }, (_, i) => 280 + i * 8);
  const COUNTS = Array.from({ length: 48 }, (_, i) => i + 1);

  for (const axis of AXES) {
    it(`keeps the first and the last label of ${axis.name}, and no two that would touch, at every width and count`, () => {
      // Some 200,000 checks: each is a plain comparison and only a failure is kept, because one `expect` per check
      // took longer than the test's five seconds on a CI runner.
      const wrong: string[] = [];
      for (const room of ROOMS) {
        for (const count of COUNTS) {
          const widths = Array.from({ length: count }, (_, i) => labelWidth(axis.text(i)) * EM);
          const span = axis.span(widths, room);
          const kept = axis.kept(widths, room);
          const fail = (what: string) => wrong.push(`${what}: ${count} labels in ${room}px: ${kept.join(",")}`);

          if (kept[0] !== 0) fail("the first label is not kept");
          if (kept[kept.length - 1] !== count - 1) fail("the last label is not kept");
          if (kept.length > Math.min(count, axis.most)) fail("more labels than there may be");
          for (let k = 1; k < kept.length; k++) {
            if (!(kept[k] > kept[k - 1])) fail("labels out of order");
            // The next label starts no nearer than the gap after the one before it ends.
            if (!(span(kept[k])[0] - span(kept[k - 1])[1] >= LABEL_GAP)) fail("two labels nearer than the gap");
          }
          // Nothing is written outside the axis.
          for (const i of kept) {
            if (!(span(i)[0] >= 0)) fail("a label starts before the axis");
            if (count > 1 && !(span(i)[1] <= room)) fail("a label ends past the axis");
          }
        }
      }
      expect(wrong.slice(0, 5)).toEqual([]);
    });
  }

  it("steps evenly from the first label, and a stepped label nearer to the last than a step gives way to it", () => {
    const widths = (count: number) => Array.from({ length: count }, (_, i) => labelWidth(hourly(i)) * EM);
    // 48 hours across a phone: every sixth, and the 43rd hour gives way to the 48th.
    expect(underColumnsKept(widths(48), 343)).toEqual([0, 6, 12, 18, 24, 30, 36, 47]);
    // The same across a desk: every fourth, which is as many as a dozen and the last allow.
    expect(underColumnsKept(widths(48), 960)).toEqual([0, 4, 8, 12, 16, 20, 24, 28, 32, 36, 40, 47]);
    // 24 hours across a desk: every second, and 23:00 instead of 22:00.
    expect(underColumnsKept(widths(24), 960)).toEqual([0, 2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 23]);
    // A week has room for every day on a desk and on a 375px phone, and for every second day on a 320px one.
    const week = Array.from({ length: 7 }, (_, i) => labelWidth(daily(i + 4)) * EM);
    expect(underColumnsKept(week, 960)).toEqual([0, 1, 2, 3, 4, 5, 6]);
    expect(underColumnsKept(week, 343)).toEqual([0, 1, 2, 3, 4, 5, 6]);
    expect(underColumnsKept(week, 288)).toEqual([0, 2, 4, 6]);
  });

  it("leaves the same number of columns between every two labels, and at least as many before the last", () => {
    for (const room of ROOMS) {
      for (const count of COUNTS) {
        const kept = underColumnsKept(Array.from({ length: count }, (_, i) => labelWidth(hourly(i)) * EM), room);
        if (kept.length < 3) continue;
        const where = `${count} labels in ${room}px: ${kept.join(",")}`;
        const step = kept[1] - kept[0];
        for (let k = 2; k < kept.length - 1; k++) expect(kept[k] - kept[k - 1], where).toBe(step);
        const end = kept[kept.length - 1] - kept[kept.length - 2];
        expect(end, where).toBeGreaterThanOrEqual(step);
        expect(end, where).toBeLessThan(2 * step);
      }
    }
  });

  it("writes more labels as the axis widens, never fewer", () => {
    for (const count of [7, 24, 30, 48]) {
      const widths = Array.from({ length: count }, (_, i) => labelWidth(hourly(i)) * EM);
      let before = 0;
      for (const room of ROOMS) {
        const kept = underColumnsKept(widths, room).length;
        expect(kept, `${count} labels in ${room}px`).toBeGreaterThanOrEqual(before);
        before = kept;
      }
    }
  });

  it("under a line writes the middle only where three labels fit", () => {
    const widths = Array.from({ length: 48 }, (_, i) => labelWidth(named(i)) * EM);
    expect(lineLabels(widths, 288)).toEqual([0, 47]);
    expect(lineLabels(widths, 343)).toEqual([0, 23, 47]);
    expect(lineLabels(widths, 504)).toEqual([0, 23, 47]);
    // Two points have no middle, and four have theirs at the second.
    expect(lineLabels(widths.slice(0, 2), 504)).toEqual([0, 1]);
    expect(lineLabels(widths.slice(0, 4), 504)).toEqual([0, 1, 3]);
  });

  it("with no width keeps the two ends, and with one label or none has nothing to thin", () => {
    const widths = Array.from({ length: 30 }, (_, i) => labelWidth(daily(i)) * EM);
    expect(keptLabels(30, underColumns(widths, 0), 13)).toEqual([0, 29]);
    expect(lineLabels(widths, 0)).toEqual([0, 29]);
    expect(lineLabels(widths.slice(0, 1), 300)).toEqual([0]);
    expect(lineLabels([], 300)).toEqual([]);
    expect(keptLabels(2, underColumns(widths.slice(0, 2), 0), 13)).toEqual([0, 1]);
    expect(keptLabels(1, underColumns(widths.slice(0, 1), 300), 13)).toEqual([0]);
    expect(keptLabels(0, underColumns([], 300), 13)).toEqual([]);
  });

  it("takes a label to be at least as wide as it is drawn in the text face", () => {
    // Measured in the console at 12px: Atkinson Hyperlegible Next, tabular figures.
    for (const [text, drawn] of [["13:00", 32.9], ["10 Oct", 37.1], ["Mon 5 Oct", 55.9], ["Sun 11 Oct, 12:00", 99.4]] as const) {
      expect(labelWidth(text) * 12, text).toBeGreaterThanOrEqual(drawn);
    }
  });
});

describe("Columns", () => {
  it("names itself with the first and the last column, and says how to read it", () => {
    render(<Columns columns={days(3)} idle={IDLE} label="Requests by day" />);
    expect(screen.getByRole("img")).toHaveAccessibleName("Requests by day, 1 Oct to 3 Oct. The arrow keys read each one.");
    expect(readout(IDLE)).toHaveAttribute("aria-live", "polite");
  });

  it("can be reached with Tab, and the arrow keys, Home and End read each column out", async () => {
    const user = userEvent.setup();
    render(<Columns columns={days(4)} idle={IDLE} label="Requests by day" />);
    await user.tab();
    expect(screen.getByRole("img")).toHaveFocus();

    await user.keyboard("{ArrowRight}");
    expect(readout("1 Oct: 0 requests")).toHaveAttribute("aria-live", "polite");
    await user.keyboard("{ArrowRight}");
    expect(readout("2 Oct: 10 requests")).toBeInTheDocument();
    await user.keyboard("{End}");
    expect(readout("4 Oct: 30 requests")).toBeInTheDocument();
    // The last column is the end: another step stays on it.
    await user.keyboard("{ArrowRight}");
    expect(readout("4 Oct: 30 requests")).toBeInTheDocument();
    await user.keyboard("{ArrowLeft}");
    expect(readout("3 Oct: 20 requests")).toBeInTheDocument();
    await user.keyboard("{Home}");
    expect(readout("1 Oct: 0 requests")).toBeInTheDocument();
    await user.keyboard("{ArrowLeft}");
    expect(readout("1 Oct: 0 requests")).toBeInTheDocument();
  });

  it("starts from the newest column when the first key is the left arrow", async () => {
    const user = userEvent.setup();
    render(<Columns columns={days(4)} idle={IDLE} label="Requests by day" />);
    await user.tab();
    await user.keyboard("{ArrowLeft}");
    expect(readout("4 Oct: 30 requests")).toBeInTheDocument();
  });

  it("goes back to the idle line when the focus leaves it", async () => {
    const user = userEvent.setup();
    render(<Columns columns={days(2)} idle={IDLE} label="Requests by day" />);
    await user.tab();
    await user.keyboard("{ArrowRight}");
    expect(screen.queryByText(IDLE)).toBeNull();
    await user.tab();
    expect(readout(IDLE)).toBeInTheDocument();
  });

  it("reads out the column that is pointed at, and the idle line again once the pointer leaves", async () => {
    const user = userEvent.setup();
    render(<Columns columns={days(3)} idle={IDLE} label="Requests by day" />);
    const chart = screen.getByRole("img");
    await user.hover(chart.children[1]);
    expect(readout("2 Oct: 10 requests")).toBeInTheDocument();
    await user.unhover(chart);
    expect(readout(IDLE)).toBeInTheDocument();
  });

  it("leaves other keys to the page", async () => {
    const user = userEvent.setup();
    render(<Columns columns={days(3)} idle={IDLE} label="Requests by day" />);
    await user.tab();
    await user.keyboard("{ArrowDown}a{Enter}");
    expect(readout(IDLE)).toBeInTheDocument();
  });

  it("with no columns keeps its name and its idle line, and the keys do nothing", async () => {
    const user = userEvent.setup();
    render(<Columns columns={[]} idle={IDLE} label="Requests by day" />);
    expect(screen.getByRole("img")).toHaveAccessibleName("Requests by day");
    await user.tab();
    await user.keyboard("{ArrowRight}{End}{Home}{ArrowLeft}");
    expect(readout(IDLE)).toBeInTheDocument();
  });

  it("with one column reads that one, whichever key is pressed", async () => {
    const user = userEvent.setup();
    render(<Columns columns={days(1)} idle={IDLE} label="Requests by day" />);
    expect(screen.getByRole("img")).toHaveAccessibleName("Requests by day, 1 Oct to 1 Oct. The arrow keys read each one.");
    await user.tab();
    for (const key of ["{ArrowRight}", "{ArrowRight}", "{ArrowLeft}", "{End}", "{Home}"]) {
      await user.keyboard(key);
      expect(readout("1 Oct: 0 requests")).toBeInTheDocument();
    }
  });

  it("drops labels evenly when there are too many to fit, and still reads every column", async () => {
    measured(960);
    const user = userEvent.setup();
    const { container } = render(<Columns columns={days(24)} idle={IDLE} label="Requests by day" />);
    // Every second label of 24 is written, and the last; the readout is not thinned.
    expect(written(container)).toEqual(["1 Oct", "3 Oct", "5 Oct", "7 Oct", "9 Oct", "11 Oct", "13 Oct", "15 Oct", "17 Oct", "19 Oct", "21 Oct", "24 Oct"]);
    await user.tab();
    await user.keyboard("{ArrowRight}{ArrowRight}");
    expect(readout("2 Oct: 10 requests")).toBeInTheDocument();
  });

  it("writes fewer labels on a narrow chart, the first and the last among them, and names itself the same", () => {
    measured(288);
    const { container } = render(<Columns columns={days(24)} idle={IDLE} label="Requests by day" />);
    expect(written(container)).toEqual(["1 Oct", "6 Oct", "11 Oct", "16 Oct", "24 Oct"]);
    // The last label ends with the axis: its column is narrower than it is.
    expect(container.querySelector("[data-end='true']")).toHaveTextContent("24 Oct");
    expect(screen.getByRole("img")).toHaveAccessibleName("Requests by day, 1 Oct to 24 Oct. The arrow keys read each one.");
  });

  it("before its width is known writes the first and the last label only", () => {
    const { container } = render(<Columns columns={days(24)} idle={IDLE} label="Requests by day" />);
    expect(written(container)).toEqual(["1 Oct", "24 Oct"]);
  });

  it("keeps one cell under every column, so a label stands under its own", () => {
    measured(288);
    const { container } = render(<Columns columns={days(24)} idle={IDLE} label="Requests by day" />);
    const cells = [...container.querySelectorAll(".t-daylabels > span")];
    expect(cells).toHaveLength(24);
    expect(cells[5]).toHaveTextContent("6 Oct");
    expect(cells[6]).toBeEmptyDOMElement();
  });

  it("draws columns that are all zero without dividing by nothing", () => {
    const columns = days(2).map((c) => ({ ...c, parts: [{ value: 0 }] }));
    render(<Columns columns={columns} idle={IDLE} label="Cost by day" />);
    const bars = [...screen.getByRole("img").querySelectorAll<HTMLElement>("span")];
    expect(bars).toHaveLength(2);
    for (const bar of bars) expect(bar.style.height).not.toContain("NaN");
  });
});

describe("Trace", () => {
  const TRACE_IDLE = "Point at the line, or use the arrow keys, to read an hour.";

  it("names itself with its window and its highest value", () => {
    render(<Trace points={trace([20, 80, 40])} format={ms} idle={TRACE_IDLE} label="Scan time, 95th percentile" />);
    expect(screen.getByRole("img")).toHaveAccessibleName(
      "Scan time, 95th percentile, 10:00 to 12:00, highest 80 ms. The arrow keys read each one.",
    );
    expect(readout(TRACE_IDLE)).toHaveAttribute("aria-live", "polite");
  });

  it("writes the top of its scale and the half of it, rounded up to a round number", () => {
    render(<Trace points={trace([20, 80, 40])} format={ms} idle={TRACE_IDLE} label="Scan time" />);
    expect(screen.getByText("100 ms")).toBeInTheDocument();
    expect(screen.getByText("50 ms")).toBeInTheDocument();
  });

  it("the arrow keys, Home and End read each point out, and a gap is said to be one", async () => {
    const user = userEvent.setup();
    render(<Trace points={trace([20, null, 40])} format={ms} idle={TRACE_IDLE} label="Scan time" />);
    await user.tab();
    expect(screen.getByRole("img")).toHaveFocus();
    await user.keyboard("{ArrowRight}");
    expect(readout("10:00: 20 ms")).toHaveAttribute("aria-live", "polite");
    await user.keyboard("{ArrowRight}");
    expect(readout("11:00: nothing measured")).toBeInTheDocument();
    await user.keyboard("{End}");
    expect(readout("12:00: 40 ms")).toBeInTheDocument();
    await user.keyboard("{ArrowRight}");
    expect(readout("12:00: 40 ms")).toBeInTheDocument();
    await user.keyboard("{Home}");
    expect(readout("10:00: 20 ms")).toBeInTheDocument();
    await user.tab();
    expect(readout(TRACE_IDLE)).toBeInTheDocument();
  });

  it("a zero is a measurement, not a gap", async () => {
    const user = userEvent.setup();
    render(<Trace points={trace([0, 0, 5])} format={ms} idle={TRACE_IDLE} label="Scan time" />);
    await user.tab();
    await user.keyboard("{ArrowRight}");
    expect(readout("10:00: 0 ms")).toBeInTheDocument();
  });

  it("with no points keeps its name and its idle line, and the keys do nothing", async () => {
    const user = userEvent.setup();
    render(<Trace points={[]} format={ms} idle={TRACE_IDLE} label="Scan time" />);
    expect(screen.getByRole("img")).toHaveAccessibleName("Scan time");
    await user.tab();
    await user.keyboard("{ArrowRight}{End}{Home}{ArrowLeft}");
    expect(readout(TRACE_IDLE)).toBeInTheDocument();
  });

  it("with nothing measured in any bucket claims no highest value, and says so bucket by bucket", async () => {
    const user = userEvent.setup();
    render(<Trace points={trace([null, null])} format={ms} idle={TRACE_IDLE} label="Scan time" />);
    expect(screen.getByRole("img")).toHaveAccessibleName("Scan time");
    await user.tab();
    await user.keyboard("{ArrowRight}");
    expect(readout("10:00: nothing measured")).toBeInTheDocument();
  });

  it("one measurement is said as a sentence, not drawn as a line", () => {
    render(<Trace points={trace([42])} format={ms} idle={TRACE_IDLE} label="Scan time" />);
    expect(screen.getByText("One measurement so far, at 10:00: 42 ms. A line needs two.")).toBeInTheDocument();
    expect(screen.queryByRole("img")).toBeNull();
    expect(screen.queryByText(TRACE_IDLE)).toBeNull();
  });

  it("one measurement among empty buckets is named by the bucket it was taken in", () => {
    render(<Trace points={trace([null, null, 7, null])} format={ms} idle={TRACE_IDLE} label="Scan time" />);
    expect(screen.getByText("One measurement so far, at 12:00: 7 ms. A line needs two.")).toBeInTheDocument();
  });

  it("names the limit only when it is within the scale", () => {
    const limit = { value: 100, name: "Target, 100 ms" };
    const { unmount } = render(<Trace points={trace([20, 80])} format={ms} idle={TRACE_IDLE} label="Scan time" limit={limit} />);
    expect(screen.getByText("Target, 100 ms")).toBeInTheDocument();
    unmount();
    // The scale ends at 10 ms here: a target of 100 ms is off the chart.
    render(<Trace points={trace([2, 8])} format={ms} idle={TRACE_IDLE} label="Scan time" limit={limit} />);
    expect(screen.queryByText("Target, 100 ms")).toBeNull();
  });

  it("writes the two ends of its window under it, and the middle where there is room, each under its point", () => {
    const hours = trace(Array.from({ length: 9 }, (_, i) => i));
    measured(300);
    const wide = render(<Trace points={hours} format={ms} idle={TRACE_IDLE} label="Scan time" />);
    expect(written(wide.container)).toEqual(["10:00", "14:00", "18:00"]);
    const [first, middle, last] = wide.container.querySelectorAll<HTMLElement>(".t-tracelabels span");
    expect(middle.style.left).toBe("50%");
    // The ends are set against the axis by the stylesheet.
    expect(first.style.left).toBe("");
    expect(last.style.left).toBe("");
    wide.unmount();

    measured(100);
    const narrow = render(<Trace points={hours} format={ms} idle={TRACE_IDLE} label="Scan time" />);
    expect(written(narrow.container)).toEqual(["10:00", "18:00"]);
    expect(screen.getByRole("img")).toHaveAccessibleName("Scan time, 10:00 to 18:00, highest 8 ms. The arrow keys read each one.");
  });

  it("measures itself when its line appears after a single measurement", () => {
    measured(300);
    const { rerender } = render(<Trace points={trace([42])} format={ms} idle={TRACE_IDLE} label="Scan time" />);
    expect(screen.queryByRole("img")).toBeNull();
    rerender(<Trace points={trace([42, 84])} format={ms} idle={TRACE_IDLE} label="Scan time" />);
    expect(screen.getByRole("img").querySelector("path")).not.toBeNull();
  });

  it("reads out the point nearest the pointer", () => {
    measured(300);
    render(<Trace points={trace([20, 80, 40])} format={ms} idle={TRACE_IDLE} label="Scan time" />);
    const chart = screen.getByRole("img");
    fireEvent.pointerMove(chart, { clientX: 140 });
    expect(readout("11:00: 80 ms")).toBeInTheDocument();
    fireEvent.pointerMove(chart, { clientX: 299 });
    expect(readout("12:00: 40 ms")).toBeInTheDocument();
    // Past either edge is the first or the last point, never none.
    fireEvent.pointerMove(chart, { clientX: -40 });
    expect(readout("10:00: 20 ms")).toBeInTheDocument();
    fireEvent.pointerLeave(chart);
    expect(readout(TRACE_IDLE)).toBeInTheDocument();
  });

  it("draws the line where it is on the first paint, with nothing to play", () => {
    measured(300);
    render(<Trace points={trace([0, 100, 50])} format={ms} idle={TRACE_IDLE} label="Scan time" />);
    const line = screen.getByRole("img").querySelector("path");
    // 300 wide, 176 high, 6 kept clear above: 0 is the floor, 100 the top of the scale.
    expect(line).toHaveAttribute("d", "M0.0 176.0L150.0 6.0L300.0 91.0");
    expect(line?.getAttribute("style") ?? "").not.toMatch(/opacity|transform/);
  });
});
