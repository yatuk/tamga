import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

/*
 * Nobody signs in through Clerk, and the console ships none of it. The shell
 * and the root layout are covered with everything else: a file that imported
 * the package again would put it back into the page.
 */

const ROOT = process.cwd();
const SOURCE = /\.(ts|tsx|js|jsx|mjs)$/;
const DIRS = ["app", "components", "lib", "hooks"];

function sources(dir: string): string[] {
  return readdirSync(join(ROOT, dir), { withFileTypes: true }).flatMap((entry) => {
    const path = `${dir}/${entry.name}`;
    if (entry.isDirectory()) return entry.name === "node_modules" ? [] : sources(path);
    return SOURCE.test(entry.name) ? [path] : [];
  });
}

const read = (file: string) => readFileSync(join(ROOT, file), "utf8");
// This file names what it looks for.
const files = DIRS.flatMap(sources).filter((file) => !file.endsWith("no-clerk.test.ts"));

describe("the console without Clerk", () => {
  it("imports nothing from @clerk in app, components, lib or hooks", () => {
    expect(files.length).toBeGreaterThan(50);
    expect(files).toContain("app/layout.tsx");
    expect(files).toContain("components/console/shell.tsx");
    // An import, a dynamic import or a require of the package, in any quoting.
    const importsClerk = /(?:from|import|require)\s*\(?\s*["'`]@clerk\//;
    expect(files.filter((file) => importsClerk.test(read(file)))).toEqual([]);
  });

  it("reads no Clerk key from the environment", () => {
    expect(files.filter((file) => /CLERK_(?:PUBLISHABLE|SECRET)_KEY/.test(read(file)))).toEqual([]);
  });

  it("does not depend on a Clerk package", () => {
    const pkg = JSON.parse(read("package.json")) as { dependencies?: Record<string, string>; devDependencies?: Record<string, string> };
    const all = { ...pkg.dependencies, ...pkg.devDependencies };
    expect(Object.keys(all).filter((name) => name.startsWith("@clerk/"))).toEqual([]);
  });
});
