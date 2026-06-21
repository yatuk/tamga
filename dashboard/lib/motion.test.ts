import { describe, it, expect } from "vitest";
import {
  springDefault,
  viewportOnce,
  fadeUpTransition,
  fadeOnlyTransition,
  staggerFast,
  staggerCards,
  containerStagger,
  itemFadeUp,
} from "./motion";

describe("springDefault", () => {
  it("has spring type with damping and stiffness", () => {
    expect(springDefault.type).toBe("spring");
    expect(springDefault.damping).toBe(25);
    expect(springDefault.stiffness).toBe(250);
  });
});

describe("viewportOnce", () => {
  it("has once true with margin and amount", () => {
    expect(viewportOnce.once).toBe(true);
    expect(viewportOnce.margin).toBe("-50px");
    expect(viewportOnce.amount).toBe(0.2);
  });
});

describe("fadeUpTransition", () => {
  it("returns spring defaults when not reduced", () => {
    expect(fadeUpTransition(false)).toEqual(springDefault);
  });

  it("returns a fast ease-out duration when reduced motion preferred", () => {
    const t = fadeUpTransition(true);
    expect(t).toEqual({ duration: 0.15, ease: "easeOut" });
  });
});

describe("fadeOnlyTransition", () => {
  it("returns shorter duration when reduced", () => {
    expect(fadeOnlyTransition(true)).toEqual({ duration: 0.12, ease: "easeOut" });
  });

  it("returns longer duration when not reduced", () => {
    expect(fadeOnlyTransition(false)).toEqual({ duration: 0.2, ease: "easeOut" });
  });
});

describe("staggerFast", () => {
  it("returns 0 when reduced", () => {
    expect(staggerFast(true)).toBe(0);
  });

  it("returns 0.08 when not reduced", () => {
    expect(staggerFast(false)).toBe(0.08);
  });
});

describe("staggerCards", () => {
  it("returns 0 when reduced", () => {
    expect(staggerCards(true)).toBe(0);
  });

  it("returns 0.12 when not reduced", () => {
    expect(staggerCards(false)).toBe(0.12);
  });
});

describe("containerStagger", () => {
  it("returns no stagger when reduced", () => {
    const v = containerStagger(true);
    expect((v.show as { transition: unknown }).transition).toEqual({ staggerChildren: 0, delayChildren: 0 });
  });

  it("returns stagger and delay when not reduced", () => {
    const v = containerStagger(false, 0.1);
    expect((v.show as { transition: unknown }).transition).toEqual({ staggerChildren: 0.1, delayChildren: 0.02 });
  });
});

describe("itemFadeUp", () => {
  it("returns no y offset when reduced", () => {
    const v = itemFadeUp(true);
    expect(v.hidden).toEqual({ opacity: 0, y: 0 });
  });

  it("returns y offset when not reduced", () => {
    const v = itemFadeUp(false);
    expect(v.hidden).toEqual({ opacity: 0, y: 20 });
  });
});
