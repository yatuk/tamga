import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";
import { MESSAGE, refuseBuiltInKey } from "./built-in-key-guard";

const SECRET = "built-in-test-value";

afterEach(() => {
  vi.unstubAllEnvs();
  vi.resetModules();
  vi.restoreAllMocks();
});

/** The module as it evaluates under the given environment. */
async function load(env: { NODE_ENV: string; NEXT_PUBLIC_ADMIN_KEY: string }) {
  vi.resetModules();
  vi.stubEnv("NODE_ENV", env.NODE_ENV);
  vi.stubEnv("NEXT_PUBLIC_ADMIN_KEY", env.NEXT_PUBLIC_ADMIN_KEY);
  return import("./built-in-key");
}

describe("the build guard", () => {
  it("stops a production build that has NEXT_PUBLIC_ADMIN_KEY set", () => {
    expect(() => refuseBuiltInKey("phase-production-build", { NEXT_PUBLIC_ADMIN_KEY: SECRET })).toThrow(MESSAGE);
  });

  it("names the variable and the way out, and never the value", () => {
    let message = "";
    try {
      refuseBuiltInKey("phase-production-build", { NEXT_PUBLIC_ADMIN_KEY: SECRET });
    } catch (e) {
      message = (e as Error).message;
    }
    expect(message).toContain("NEXT_PUBLIC_ADMIN_KEY");
    expect(message).toContain("Settings");
    expect(message).not.toContain(SECRET);
  });

  it("lets a production build through when the variable is unset or empty", () => {
    expect(() => refuseBuiltInKey("phase-production-build", {})).not.toThrow();
    expect(() => refuseBuiltInKey("phase-production-build", { NEXT_PUBLIC_ADMIN_KEY: "" })).not.toThrow();
  });

  it("leaves the development server alone", () => {
    expect(() => refuseBuiltInKey("phase-development-server", { NEXT_PUBLIC_ADMIN_KEY: SECRET })).not.toThrow();
  });

  it("knows the phase by the name Next.js gives it", async () => {
    const { PHASE_PRODUCTION_BUILD } = await import("next/constants");
    expect(() => refuseBuiltInKey(PHASE_PRODUCTION_BUILD, { NEXT_PUBLIC_ADMIN_KEY: SECRET })).toThrow(MESSAGE);
  });

  it("cannot be switched off by the environment a build runs in", () => {
    // Lint is kept away from the guard by not loading next.config.js, never
    // by teaching the guard to stand down: a build started from any npm
    // script, in CI or with any NODE_ENV, is still refused.
    for (const extra of [
      { npm_lifecycle_event: "lint" },
      { npm_lifecycle_script: "eslint app components lib" },
      { npm_lifecycle_event: "build", CI: "true" },
      { NODE_ENV: "development" },
      { NODE_ENV: "test" },
      { NEXT_PHASE: "phase-development-server" },
    ]) {
      expect(() => refuseBuiltInKey("phase-production-build", { ...extra, NEXT_PUBLIC_ADMIN_KEY: SECRET })).toThrow(MESSAGE);
    }
  });

  it("reads the phase and the variable, and nothing else", () => {
    const source = readFileSync("lib/built-in-key-guard.js", "utf8").replace(/\/\*[\s\S]*?\*\//g, "");
    expect(source).not.toMatch(/process\.argv|npm_lifecycle|NODE_ENV/);
    // Variables are upper case; the message's ".env.local" is a file name.
    expect(source.match(/\benv\.[A-Z][A-Z_]*/g)).toEqual(["env.NEXT_PUBLIC_ADMIN_KEY"]);
  });

  it("is not in the way of lint, which does not load next.config.js", () => {
    const { scripts } = JSON.parse(readFileSync("package.json", "utf8")) as { scripts: Record<string, string> };
    // `next lint` loads the configuration as a production build would, so
    // with a development key in .env.local it stopped at the guard.
    expect(scripts.lint).not.toMatch(/\bnext\b/);
    // The directories `next lint` checked by default, of those that exist, and the hooks beside them.
    expect(scripts.lint).toBe("eslint app components lib hooks");
    // The build is still `next build`, which does load it.
    expect(scripts.build).toBe("next build");
    const ci = readFileSync("../.github/workflows/dashboard-tests.yml", "utf8");
    expect(ci).not.toMatch(/next lint/);
    expect(ci).toMatch(/run: npm run lint/);
    expect(ci).toMatch(/run: npx next build/);
  });

  it("is what next.config.js runs before it hands over the configuration", () => {
    const config = readFileSync("next.config.js", "utf8");
    expect(config).toContain('require("./lib/built-in-key-guard")');
    expect(config).toMatch(/module\.exports = \(phase\) => \{\s*refuseBuiltInKey\(phase\);/);
  });
});

describe("where the key in use comes from", () => {
  it("is the stored key whenever one is stored, the built-in one otherwise, and none in a build without one", async () => {
    const { adminKeySource } = await load({ NODE_ENV: "production", NEXT_PUBLIC_ADMIN_KEY: "" });
    expect(adminKeySource("typed-in", SECRET)).toBe("stored");
    expect(adminKeySource("typed-in", "")).toBe("stored");
    expect(adminKeySource("", SECRET)).toBe("built-in");
    expect(adminKeySource("", "")).toBe("none");
  });

  it("reads the build's own key when it is given none: never built-in in production", async () => {
    vi.spyOn(console, "warn").mockImplementation(() => {});
    const production = await load({ NODE_ENV: "production", NEXT_PUBLIC_ADMIN_KEY: SECRET });
    expect(production.adminKeySource("")).toBe("none");
    const development = await load({ NODE_ENV: "development", NEXT_PUBLIC_ADMIN_KEY: SECRET });
    expect(development.adminKeySource("")).toBe("built-in");
    expect(development.adminKeySource("typed-in")).toBe("stored");
  });
});

describe("the built-in key", () => {
  it("is empty in production, whatever the environment holds", async () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const { BUILT_IN_ADMIN_KEY } = await load({ NODE_ENV: "production", NEXT_PUBLIC_ADMIN_KEY: SECRET });
    expect(BUILT_IN_ADMIN_KEY).toBe("");
    expect(warn).not.toHaveBeenCalled();
  });

  it("sends no key from authHeaders in production when none was entered", async () => {
    vi.spyOn(console, "warn").mockImplementation(() => {});
    await load({ NODE_ENV: "production", NEXT_PUBLIC_ADMIN_KEY: SECRET });
    const { authHeaders } = await import("@/lib/api/fetch-core");
    expect(authHeaders()).toEqual({});
    expect(authHeaders("typed-in")).toEqual({ "X-Tamga-Admin-Key": "typed-in" });
  });

  it("is used in development, with one warning that does not print it", async () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const { BUILT_IN_ADMIN_KEY } = await load({ NODE_ENV: "development", NEXT_PUBLIC_ADMIN_KEY: SECRET });
    expect(BUILT_IN_ADMIN_KEY).toBe(SECRET);
    expect(warn).toHaveBeenCalledTimes(1);
    expect(String(warn.mock.calls[0][0])).toContain("NEXT_PUBLIC_ADMIN_KEY");
    expect(JSON.stringify(warn.mock.calls)).not.toContain(SECRET);
  });

  it("is empty and silent in development when the variable is not set", async () => {
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const { BUILT_IN_ADMIN_KEY } = await load({ NODE_ENV: "development", NEXT_PUBLIC_ADMIN_KEY: "" });
    expect(BUILT_IN_ADMIN_KEY).toBe("");
    expect(warn).not.toHaveBeenCalled();
  });

  it("is named in one module only, so no other file can ship it", () => {
    const found: string[] = [];
    const walk = (dir: string) => {
      for (const entry of readdirSync(dir, { withFileTypes: true })) {
        const path = join(dir, entry.name);
        if (entry.isDirectory()) walk(path);
        else if (/\.(ts|tsx|js|mjs)$/.test(entry.name) && !/\.test\.tsx?$/.test(entry.name)) {
          if (readFileSync(path, "utf8").includes("process.env.NEXT_PUBLIC_ADMIN_KEY")) found.push(path.replace(/\\/g, "/"));
        }
      }
    };
    for (const dir of ["app", "components", "hooks", "lib"]) walk(dir);
    expect(found).toEqual(["lib/built-in-key.ts"]);
  });

  it("is named only behind the test for a production build", () => {
    const source = readFileSync("lib/built-in-key.ts", "utf8").replace(/\/\*[\s\S]*?\*\//g, "");
    expect(source.match(/process\.env\.NEXT_PUBLIC_ADMIN_KEY/g)).toHaveLength(1);
    expect(source).toContain('process.env.NODE_ENV !== "production" ? process.env.NEXT_PUBLIC_ADMIN_KEY || "" : ""');
  });
});
