import { describe, it, expect } from "vitest";
import { toast } from "./toast";

// We test the shape of the API — sonner is mocked by vitest setup
// so we just verify the wrapper doesn't throw.
describe("toast", () => {
  it("exposes success method", () => {
    expect(typeof toast.success).toBe("function");
  });

  it("exposes error method", () => {
    expect(typeof toast.error).toBe("function");
  });

  it("exposes info method", () => {
    expect(typeof toast.info).toBe("function");
  });

  it("exposes warning method", () => {
    expect(typeof toast.warning).toBe("function");
  });

  it("exposes message method", () => {
    expect(typeof toast.message).toBe("function");
  });

  it("exposes promise method (passthrough)", () => {
    expect(typeof toast.promise).toBe("function");
  });

  it("exposes dismiss method (passthrough)", () => {
    expect(typeof toast.dismiss).toBe("function");
  });

  it("does not throw when calling success", () => {
    expect(() => toast.success("Test title")).not.toThrow();
  });

  it("does not throw when calling error with detail", () => {
    expect(() => toast.error("Failed", "Something went wrong")).not.toThrow();
  });

  it("does not throw when calling info without detail", () => {
    expect(() => toast.info("Info")).not.toThrow();
  });
});
