import { describe, expect, it } from "vitest";
import { render, screen, within } from "@testing-library/react";
import { Diff, diffLines, diffWords } from "./diff";

/** A diff written the way it is read: one line each, with its sign in front. */
const signed = (a: string, b: string) => diffLines(a, b).map((line) => `${line.type}${line.text}`);

describe("diffLines", () => {
  it("has nothing to show for two texts that are the same", () => {
    expect(diffLines("rules:\n  pii: block", "rules:\n  pii: block")).toEqual([]);
    expect(diffLines("", "")).toEqual([]);
  });

  it("marks a line that was added, and keeps the lines around it", () => {
    expect(signed("a\nc", "a\nb\nc")).toEqual([" a", "+b", " c"]);
  });

  it("marks a line that was removed", () => {
    expect(signed("a\nb\nc", "a\nc")).toEqual([" a", "-b", " c"]);
  });

  it("shows a changed line as the old one removed and then the new one added", () => {
    expect(signed("name: x\naction: block\nend", "name: x\naction: redact\nend")).toEqual([
      " name: x",
      "-action: block",
      "+action: redact",
      " end",
    ]);
  });

  it("shows changes at the first and the last line", () => {
    expect(signed("old top\nmiddle\nold end", "new top\nmiddle\nnew end")).toEqual([
      "-old top",
      "+new top",
      " middle",
      "-old end",
      "+new end",
    ]);
  });

  it("keeps a line that only moved as one removal and one addition, not a rewrite of everything", () => {
    expect(signed("a\nb\nc\nd", "b\nc\nd\na")).toEqual(["-a", " b", " c", " d", "+a"]);
  });

  it("tells a repeated line from its neighbours", () => {
    expect(signed("x\nx", "x\nx\nx")).toEqual([" x", " x", "+x"]);
  });

  it("an empty document has no line to remove: everything in the other one was added", () => {
    expect(signed("", "a\nb")).toEqual(["+a", "+b"]);
    expect(signed("a\nb", "")).toEqual(["-a", "-b"]);
    expect(diffWords(diffLines("", "a\nb"))).toBe("2 lines added, 0 removed");
  });

  it("does not count a line ending as a change", () => {
    const lines = diffLines("a\r\nb\r\nc", "a\nb\nd");
    expect(lines.map((line) => `${line.type}${line.text}`)).toEqual([" a", " b", "-c", "+d"]);
    expect(diffWords(lines)).toBe("1 line added, 1 removed");
  });

  it("compares indentation and case exactly, as YAML reads them", () => {
    expect(signed("  action: block", "action: block")).toEqual(["-  action: block", "+action: block"]);
    expect(signed("Action: block", "action: block")).toEqual(["-Action: block", "+action: block"]);
  });

  it("a blank line added between two others is one added line", () => {
    expect(signed("a\nb", "a\n\nb")).toEqual([" a", "+", " b"]);
  });
});

describe("diffWords", () => {
  it("counts what was added and removed, and not what stayed", () => {
    expect(diffWords(diffLines("a\nb\nc", "a\nx\ny\nz"))).toBe("3 lines added, 2 removed");
  });

  it("says one line as one line", () => {
    expect(diffWords(diffLines("a", "a\nb"))).toBe("1 line added, 0 removed");
  });

  it("says nothing was added when lines were only removed", () => {
    expect(diffWords(diffLines("a\nb\nc", "a"))).toBe("0 lines added, 2 removed");
  });
});

describe("Diff", () => {
  it("is a named region that the keyboard can reach, to scroll a long diff", () => {
    render(<Diff lines={diffLines("a", "b")} />);
    const region = screen.getByRole("region", { name: "Line diff" });
    expect(region).toHaveAttribute("tabindex", "0");
  });

  it("writes the sign beside each changed line, so that the colour is not the only thing that says it", () => {
    render(<Diff lines={diffLines("keep\naction: block", "keep\naction: redact")} />);
    const region = screen.getByRole("region", { name: "Line diff" });
    const row = (text: string) => within(region).getByText(text).parentElement;
    expect(row("action: block")).toHaveTextContent("-action: block");
    expect(row("action: redact")).toHaveTextContent("+action: redact");
    // An unchanged line carries no sign at all.
    expect(row("keep")?.textContent).toBe("keep");
  });

  it("writes the lines in the order of the diff, with their text as it was typed", () => {
    render(<Diff lines={diffLines("a\n  <b> & c", "a\n  <b> & d")} />);
    const region = screen.getByRole("region", { name: "Line diff" });
    expect([...region.querySelectorAll("pre")].map((pre) => pre.textContent)).toEqual(["a", "  <b> & c", "  <b> & d"]);
    // Markup in a policy is text, never an element.
    expect(region.querySelector("b")).toBeNull();
  });

  it("gives a blank line its height, so that it is not lost between two others", () => {
    render(<Diff lines={diffLines("a\nb", "a\n\nb")} />);
    const region = screen.getByRole("region", { name: "Line diff" });
    expect([...region.querySelectorAll("pre")].map((pre) => pre.textContent)).toEqual(["a", " ", "b"]);
  });
});
