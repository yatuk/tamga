import { beforeEach, describe, expect, it } from "vitest";
import { render, renderHook, screen, waitFor } from "@testing-library/react";
import { createElement, useMemo } from "react";
import { keepPreviousData, QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";
import type { SecurityEvent } from "@/lib/api/types-core";
import { ActionMark, EventTable, useFresh } from "./events";

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

/** The question the rows answer when a test does not change it. */
const SAME = "|24h";

function fresh(first: string[], ready = true) {
  const hook = renderHook(({ events, question }: { events: SecurityEvent[]; question: string | null }) => useFresh(events, question), {
    initialProps: { events: list(...first), question: ready ? SAME : null },
  });
  return {
    marked: () => [...hook.result.current].sort(),
    read: (ids: string[], isReady = true) => hook.rerender({ events: list(...ids), question: isReady ? SAME : null }),
    /** The rows on screen and the question they answer; null while they are another question's rows, kept until the answer comes. */
    show: (ids: string[], question: string | null) => hook.rerender({ events: list(...ids), question }),
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

/**
 * The query client keeps the rows of the previous question on screen while
 * the next one loads, so a change of filter, window or page swaps one list
 * for another with no loading state in between. The hook is told which
 * question the rows answer, and marks a row only when the same question gets
 * a newer answer.
 */
describe("useFresh, when the question changes", () => {
  it("does not mark what clearing a filter brings on top of the rows that were there", () => {
    // Only stopped requests were listed, and they are the oldest.
    const view = fresh([]);
    view.show(["b", "a"], "block|24h");
    // The filter is cleared: the old rows stay while the new list is read.
    view.show(["b", "a"], null);
    expect(view.marked()).toEqual([]);
    view.show(["d", "c", "b", "a"], "|24h");
    expect(view.marked()).toEqual([]);
  });

  it("does not mark them when the other list was already read and comes back at once", () => {
    // An answer still in the cache is shown without the old rows being kept first.
    const view = fresh([]);
    view.show(["b", "a"], "block|24h");
    view.show(["d", "c", "b", "a"], "|24h");
    expect(view.marked()).toEqual([]);
  });

  it("does not mark the rows a longer window brings on top", () => {
    const view = fresh([]);
    view.show(["b", "a"], "|24h");
    view.show(["b", "a"], null);
    view.show(["e", "d", "c", "b", "a"], "|7d");
    expect(view.marked()).toEqual([]);
    // Nor going back to the shorter one, which is in the cache.
    view.show(["b", "a"], "|24h");
    expect(view.marked()).toEqual([]);
  });

  it("does not mark the rows of another page", () => {
    const view = fresh([]);
    view.show(["f", "e", "d"], "|24h|1");
    view.show(["f", "e", "d"], null);
    view.show(["c", "b", "a"], "|24h|2");
    expect(view.marked()).toEqual([]);
    // Back to the first page, where a request came in meanwhile: the page is
    // read anew, so its rows are the first this question shows.
    view.show(["c", "b", "a"], null);
    view.show(["g", "f", "e"], "|24h|1");
    expect(view.marked()).toEqual([]);
  });

  it("marks a row that arrives after the question changed, and only that row", () => {
    const view = fresh([]);
    view.show(["b", "a"], "block|24h");
    view.show(["b", "a"], null);
    view.show(["d", "c", "b", "a"], "|24h");
    expect(view.marked()).toEqual([]);
    // The same question, read again, with one more request on top.
    view.show(["e", "d", "c", "b", "a"], "|24h");
    expect(view.marked()).toEqual(["e"]);
    view.show(["e", "d", "c", "b", "a"], "|24h");
    expect(view.marked()).toEqual([]);
  });

  it("marks nothing through two filter changes made one after the other", () => {
    const view = fresh([]);
    view.show(["a"], "block|24h");
    // The second change is made before the first one's rows are there.
    view.show(["a"], null);
    view.show(["a"], null);
    expect(view.marked()).toEqual([]);
    view.show(["c", "b", "a"], "block,redact|openai|24h");
    expect(view.marked()).toEqual([]);
    view.show(["d", "c", "b", "a"], "block,redact|openai|24h");
    expect(view.marked()).toEqual(["d"]);
  });

  it("marks nothing through two changes that each find their answer in the cache", () => {
    const view = fresh([]);
    view.show(["a"], "block|24h");
    view.show(["b", "a"], "block,redact|24h");
    expect(view.marked()).toEqual([]);
    view.show(["c", "b", "a"], "|24h");
    expect(view.marked()).toEqual([]);
  });

  it("does not take a row that came in while another question was on screen for news of the first", () => {
    const view = fresh(["b", "a"]);
    view.show(["z"], "block|24h");
    // Back to the first question: it has one more row than when it was left.
    view.show(["c", "b", "a"], SAME);
    expect(view.marked()).toEqual([]);
  });
});

describe("useFresh, behind a query that keeps the previous rows while the next ones load", () => {
  /** What the proxy holds, per filter. A test adds to it to make a request arrive. */
  let held: Record<string, string[]>;
  /** Every row that was marked in any render, in the order it was. */
  let marked: string[];

  function Asked({ filter }: { filter: string }) {
    const rows = useQuery({
      queryKey: ["rows", filter],
      queryFn: async () => list(...held[filter]),
      placeholderData: keepPreviousData,
    });
    const events = useMemo(() => rows.data ?? [], [rows.data]);
    const fresh = useFresh(events, rows.data && !rows.isPlaceholderData ? filter : null);
    marked.push(...fresh);
    return (
      <ul aria-label={rows.isPlaceholderData ? "kept" : filter}>
        {events.map((e) => (
          <li key={e.request_id}>{e.request_id}</li>
        ))}
      </ul>
    );
  }

  function ask(first: string) {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    const tree = (filter: string) => createElement(QueryClientProvider, { client }, <Asked filter={filter} />);
    const view = render(tree(first));
    const shown = () => screen.getAllByRole("listitem").map((item) => item.textContent);
    return {
      /** Changes the filter and waits for its rows. */
      async change(filter: string) {
        view.rerender(tree(filter));
        await screen.findByRole("list", { name: filter });
        await waitFor(() => expect(shown()).toEqual(held[filter]));
      },
      /** Reads the same question again, as the refetch interval or "Show new requests" does. */
      async again(filter: string) {
        await client.refetchQueries({ queryKey: ["rows", filter] });
        await waitFor(() => expect(shown()).toEqual(held[filter]));
      },
      rerender: (filter: string) => view.rerender(tree(filter)),
    };
  }

  beforeEach(() => {
    held = { block: ["b", "a"], "": ["d", "c", "b", "a"], redact: ["c"] };
    marked = [];
  });

  it("marks nothing when a filter is cleared, and then the one request that arrives", async () => {
    const view = ask("block");
    await view.change("block");
    await view.change("");
    expect(marked).toEqual([]);

    held[""] = ["e", "d", "c", "b", "a"];
    await view.again("");
    expect(marked).toEqual(["e"]);
  });

  it("marks nothing going back to a filter whose rows are still in the cache", async () => {
    const view = ask("block");
    await view.change("block");
    await view.change("");
    await view.change("block");
    await view.change("");
    expect(marked).toEqual([]);
  });

  it("marks nothing through two filter changes made before the first has answered", async () => {
    const view = ask("block");
    await view.change("block");
    view.rerender("redact");
    await view.change("");
    expect(marked).toEqual([]);
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
        fresh={useFresh(events, "|7d")}
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

describe("the action of a request", () => {
  it("names a routed request, with the mark the stylesheet draws in ochre", () => {
    render(createElement(ActionMark, { action: "ROUTE" }));
    const mark = screen.getByText("Routed");
    expect(mark).toHaveAttribute("data-action", "ROUTE");
  });
});
