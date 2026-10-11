import { beforeEach, describe, expect, it } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { createElement } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/__mocks__/server";
import { ADMIN_KEY_STORAGE } from "@/hooks/useAdminKey";
import PatternsPage from "./page";

/**
 * "Try it on some text": the sample is written out whole, with every piece
 * the pattern catches marked, and the count beside it is the number of marks.
 */

beforeEach(() => {
  localStorage.clear();
  localStorage.setItem(ADMIN_KEY_STORAGE, "test-key");
  server.use(http.get("http://localhost:3000/api/v1/patterns", () => HttpResponse.json({ items: [], total: 0 })));
});

function show() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const { container } = render(createElement(QueryClientProvider, { client: qc }, createElement(PatternsPage)));
  const set = (label: string, value: string) => fireEvent.change(screen.getByLabelText(label), { target: { value } });
  return {
    /** Sets the pattern and the sample, and reads back what is marked. */
    try(kind: "regex" | "literal", pattern: string, sample: string) {
      set("Kind", kind);
      set(kind === "regex" ? "Regular expression" : "Literal text", pattern);
      set("Try it on some text", sample);
      const marks = [...container.querySelectorAll("mark")];
      return {
        marked: marks.map((mark) => mark.textContent),
        // The sample is the element that holds the marks and the text between them.
        written: marks[0]?.parentElement?.textContent ?? null,
      };
    },
    container,
  };
}

const catches = (count: number) => screen.queryByText(`It catches ${count} ${count === 1 ? "piece" : "pieces"} of this text.`);
const NOTHING = "It catches nothing in this text.";

describe("a regular expression tried on a sample", () => {
  it("marks every match and counts them", () => {
    const result = show().try("regex", "ACME-\\d+", "ship ACME-1234 and ACME-77 today");
    expect(result.marked).toEqual(["ACME-1234", "ACME-77"]);
    expect(result.written).toBe("ship ACME-1234 and ACME-77 today");
    expect(catches(2)).toBeInTheDocument();
  });

  it("marks a match at the very start and one at the very end", () => {
    const result = show().try("regex", "\\d+", "12 apples and 7");
    expect(result.marked).toEqual(["12", "7"]);
    expect(result.written).toBe("12 apples and 7");
  });

  it("marks a sample that is one match from end to end", () => {
    const result = show().try("regex", "^.+$", "all of it");
    expect(result.marked).toEqual(["all of it"]);
    expect(result.written).toBe("all of it");
    expect(catches(1)).toBeInTheDocument();
  });

  it("keeps matches that touch as separate marks", () => {
    const result = show().try("regex", "\\d", "a123b");
    expect(result.marked).toEqual(["1", "2", "3"]);
    expect(result.written).toBe("a123b");
    expect(catches(3)).toBeInTheDocument();
  });

  it("does not match the same text twice where matches could overlap", () => {
    // "aa" fits "aaaa" at three places; a scanner takes two, one after the other.
    const result = show().try("regex", "aa", "aaaa b");
    expect(result.marked).toEqual(["aa", "aa"]);
    expect(result.written).toBe("aaaa b");
  });

  it("says that nothing was caught, and writes the sample without a mark", () => {
    const view = show();
    const result = view.try("regex", "ACME-\\d+", "nothing like it here");
    expect(result.marked).toEqual([]);
    expect(screen.getByText(NOTHING)).toBeInTheDocument();
    expect(screen.getByText("nothing like it here", { selector: "div" })).toBeInTheDocument();
  });

  it("matches case as written, and (?i) in front switches that off as it does in the proxy", () => {
    const view = show();
    expect(view.try("regex", "acme", "ACME and acme").marked).toEqual(["acme"]);
    expect(view.try("regex", "(?i)acme", "ACME and acme").marked).toEqual(["ACME", "acme"]);
    expect(screen.queryByText("This is not a valid regular expression.")).toBeNull();
  });

  it("does not count a pattern that can match nothing as catching every position", () => {
    const result = show().try("regex", "x*", "abc");
    expect(result.marked).toEqual([]);
    expect(screen.getByText(NOTHING)).toBeInTheDocument();
  });

  it("writes markup in the sample as text", () => {
    const view = show();
    const result = view.try("regex", "<[^>]+>", 'a <img src=x onerror="alert(1)"> & <b>bold</b>');
    expect(result.marked).toEqual(['<img src=x onerror="alert(1)">', "<b>", "</b>"]);
    expect(result.written).toBe('a <img src=x onerror="alert(1)"> & <b>bold</b>');
    expect(view.container.querySelector("img, b")).toBeNull();
  });

  it("shows no result for an expression that does not compile, and says that it does not", () => {
    const view = show();
    const result = view.try("regex", "ACME-(", "ship ACME-( now");
    expect(result.marked).toEqual([]);
    expect(screen.getByText("This is not a valid regular expression.")).toBeInTheDocument();
    expect(screen.queryByText(NOTHING)).toBeNull();
    expect(catches(1)).toBeNull();
  });

  it("shows no result while the pattern or the sample is empty", () => {
    const view = show();
    view.try("regex", "", "some text");
    expect(screen.queryByText(NOTHING)).toBeNull();
    view.try("regex", "some", "");
    expect(screen.queryByText(NOTHING)).toBeNull();
    expect(view.container.querySelector("mark")).toBeNull();
  });
});

describe("literal text tried on a sample", () => {
  it("marks every place the text occurs, whatever its case", () => {
    const result = show().try("literal", "acme-secret", "ACME-SECRET, then acme-secret.");
    expect(result.marked).toEqual(["ACME-SECRET", "acme-secret"]);
    expect(result.written).toBe("ACME-SECRET, then acme-secret.");
    expect(catches(2)).toBeInTheDocument();
  });

  it("reads pattern characters as the characters they are", () => {
    const view = show();
    expect(view.try("literal", "a.b*(c)", "axb a.b*(c) a.b*(c)[d]").marked).toEqual(["a.b*(c)", "a.b*(c)"]);
    expect(view.try("literal", "$1 [x] \\d ^", "cost $1 [x] \\d ^ end").marked).toEqual(["$1 [x] \\d ^"]);
    expect(view.try("literal", "a.b", "axb").marked).toEqual([]);
    expect(screen.getByText(NOTHING)).toBeInTheDocument();
  });

  it("marks the text at the start and at the end, and occurrences that touch", () => {
    const view = show();
    expect(view.try("literal", "ab", "ab then ab").marked).toEqual(["ab", "ab"]);
    const touching = view.try("literal", "ab", "ababab");
    expect(touching.marked).toEqual(["ab", "ab", "ab"]);
    expect(touching.written).toBe("ababab");
    // Three places fit "aa" in "aaaa"; two are taken, one after the other.
    expect(view.try("literal", "aa", "aaaa").marked).toEqual(["aa", "aa"]);
  });

  it("marks the right characters after a letter whose lower case is longer", () => {
    // "İ" lowers to two characters; the mark must still sit on "secret".
    const result = show().try("literal", "secret", "İstanbul secret İ secret");
    expect(result.marked).toEqual(["secret", "secret"]);
    expect(result.written).toBe("İstanbul secret İ secret");
  });

  it("writes markup in the sample as text", () => {
    const view = show();
    const result = view.try("literal", "<b>", "x <b>y</b> z");
    expect(result.marked).toEqual(["<b>"]);
    expect(result.written).toBe("x <b>y</b> z");
    expect(view.container.querySelector("b")).toBeNull();
  });
});
