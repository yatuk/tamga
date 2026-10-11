import { afterEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Columns, Trace, type Column, type TracePoint } from "./charts";

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
    const user = userEvent.setup();
    render(<Columns columns={days(24)} idle={IDLE} label="Requests by day" />);
    // Every second label of 24 is written; the readout is not thinned.
    expect(screen.getByText("1 Oct")).toBeInTheDocument();
    expect(screen.queryByText("2 Oct")).toBeNull();
    expect(screen.getByText("3 Oct")).toBeInTheDocument();
    await user.tab();
    await user.keyboard("{ArrowRight}{ArrowRight}");
    expect(readout("2 Oct: 10 requests")).toBeInTheDocument();
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

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  /** A chart 300 pixels wide: without a width the line is not drawn at all. */
  function measured() {
    vi.stubGlobal(
      "ResizeObserver",
      class {
        constructor(private report: (entries: { contentRect: { width: number } }[]) => void) {}
        observe() {
          this.report([{ contentRect: { width: 300 } }]);
        }
        unobserve() {}
        disconnect() {}
      },
    );
  }

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

  it("reads out the point nearest the pointer", () => {
    measured();
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
    measured();
    render(<Trace points={trace([0, 100, 50])} format={ms} idle={TRACE_IDLE} label="Scan time" />);
    const line = screen.getByRole("img").querySelector("path");
    // 300 wide, 176 high, 6 kept clear above: 0 is the floor, 100 the top of the scale.
    expect(line).toHaveAttribute("d", "M0.0 176.0L150.0 6.0L300.0 91.0");
    expect(line?.getAttribute("style") ?? "").not.toMatch(/opacity|transform/);
  });
});
