import { describe, expect, it } from "vitest";
import { render, renderHook, screen } from "@testing-library/react";
import { createElement } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { SecurityEvent } from "@/lib/api/types-core";
import { EventTable, useFresh } from "./events";

/**
 * A row is marked as new only when it arrived on top of rows that were
 * already on screen. The first rows a list shows are not news, and neither
 * are older rows, another page or another filter.
 */

const event = (id: string): SecurityEvent => ({
  request_id: id,
  event_type: "request_scanned",
  action: "PASS",
  findings: [],
  findings_count: 0,
  timestamp: "2026-10-11T09:00:00Z",
});
/** A list read from the proxy: a new array every time, as a refetch gives. */
const list = (...ids: string[]) => ids.map(event);

function fresh(first: string[], ready = true) {
  const hook = renderHook(({ events, ready }) => useFresh(events, ready), { initialProps: { events: list(...first), ready } });
  return {
    marked: () => [...hook.result.current].sort(),
    read: (ids: string[], isReady = true) => hook.rerender({ events: list(...ids), ready: isReady }),
  };
}

describe("useFresh", () => {
  it("marks nothing on the first paint", () => {
    expect(fresh(["c", "b", "a"]).marked()).toEqual([]);
  });

  it("marks nothing when the first rows come in after the loading state", () => {
    const view = fresh([], false);
    expect(view.marked()).toEqual([]);
    view.read(["c", "b", "a"]);
    expect(view.marked()).toEqual([]);
  });

  it("marks nothing when rows come into a list that was read and found empty", () => {
    // An empty window is not a list the new rows sit on top of.
    const view = fresh([]);
    view.read(["a"]);
    expect(view.marked()).toEqual([]);
  });

  it("marks the one row that arrives on top later", () => {
    const view = fresh(["c", "b", "a"]);
    view.read(["d", "c", "b", "a"]);
    expect(view.marked()).toEqual(["d"]);
  });

  it("marks every row of a burst, and none of those that were there", () => {
    const view = fresh(["b", "a"]);
    view.read(["e", "d", "c", "b", "a"]);
    expect(view.marked()).toEqual(["c", "d", "e"]);
  });

  it("shows a new row as new once: the next read of the same rows marks nothing", () => {
    const view = fresh(["b", "a"]);
    view.read(["c", "b", "a"]);
    expect(view.marked()).toEqual(["c"]);
    view.read(["c", "b", "a"]);
    expect(view.marked()).toEqual([]);
    // And the row after it is news of its own.
    view.read(["d", "c", "b", "a"]);
    expect(view.marked()).toEqual(["d"]);
  });

  it("still marks the new row when the oldest one fell off the end of the page", () => {
    const view = fresh(["c", "b", "a"]);
    view.read(["d", "c", "b"]);
    expect(view.marked()).toEqual(["d"]);
  });

  it("does not mark older rows loaded below the ones on screen", () => {
    const view = fresh(["f", "e", "d"]);
    view.read(["f", "e", "d", "c", "b", "a"]);
    expect(view.marked()).toEqual([]);
  });

  it("does not mark the rows another filter or another window brings", () => {
    const view = fresh(["f", "e", "d"]);
    view.read(["z", "y", "x"]);
    expect(view.marked()).toEqual([]);
    // Nor when the other list holds rows that were there, with its own in between.
    view.read(["z", "q", "y", "x"]);
    expect(view.marked()).toEqual([]);
    view.read(["x", "w"]);
    expect(view.marked()).toEqual([]);
  });

  it("does not mark anything when a row in the middle is gone: that is a different list", () => {
    const view = fresh(["d", "c", "b", "a"]);
    view.read(["e", "d", "b", "a"]);
    expect(view.marked()).toEqual([]);
  });

  it("does not mark the rows that come back after a filter emptied the list", () => {
    const view = fresh(["c", "b", "a"]);
    view.read([]);
    expect(view.marked()).toEqual([]);
    view.read(["c", "b", "a"]);
    expect(view.marked()).toEqual([]);
  });

  it("does not mark what a filter brings on top of the rows that were there, once the list has been read again", () => {
    // Only stopped requests were listed. The filter is cleared: the list is
    // read anew, and comes back with a passed request above the same two.
    const view = fresh(["b", "a"]);
    view.read([], false);
    expect(view.marked()).toEqual([]);
    view.read(["z", "b", "a"], true);
    expect(view.marked()).toEqual([]);
    // From here on a row on top is news again.
    view.read(["y", "z", "b", "a"]);
    expect(view.marked()).toEqual(["y"]);
  });
});

describe("the request table", () => {
  function Rows({ ids }: { ids: string[] }) {
    const events = list(...ids);
    return (
      <EventTable
        events={events}
        label="Events"
        swapKey="|7d"
        fresh={useFresh(events, true)}
        openId=""
        onToggle={() => undefined}
        onClose={() => undefined}
        adminKey="test-key"
        empty="No request went through in this window."
      />
    );
  }
  const client = new QueryClient();
  const tree = (ids: string[]) => createElement(QueryClientProvider, { client }, <Rows ids={ids} />);
  /** The request ids of the rows that carry the mark the stylesheet flashes on. */
  const flashed = () =>
    screen
      .getAllByRole("row")
      .filter((row) => row.getAttribute("data-new") === "true")
      .map((row) => row.getAttribute("data-row"));

  it("flashes no row on the first paint, and only the row that arrives later", () => {
    const view = render(tree(["c", "b", "a"]));
    expect(screen.getAllByRole("row")).toHaveLength(4);
    expect(flashed()).toEqual([]);

    view.rerender(tree(["d", "c", "b", "a"]));
    expect(screen.getAllByRole("row")).toHaveLength(5);
    expect(flashed()).toEqual(["d"]);

    view.rerender(tree(["d", "c", "b", "a"]));
    expect(flashed()).toEqual([]);
  });
});
