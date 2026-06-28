import "@testing-library/jest-dom/vitest";
import { beforeAll, afterEach, afterAll, vi } from "vitest";
import { cleanup } from "@testing-library/react";
import { server } from "./__mocks__/server";

// Override API_BASE to localhost so MSW handlers match (fetch-core defaults to :8443)
process.env.NEXT_PUBLIC_API_URL = "http://localhost:3000";

// MSW lifecycle
beforeAll(() => server.listen({ onUnhandledRequest: "warn" }));
afterEach(() => {
  cleanup();
  server.resetHandlers();
});
afterAll(() => server.close());

// ── Next.js navigation mocks ────────────────────────────────────────────
vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: vi.fn(),
    replace: vi.fn(),
    back: vi.fn(),
    refresh: vi.fn(),
    prefetch: vi.fn(),
  }),
  usePathname: () => "/",
  useSearchParams: () => new URLSearchParams(),
  useParams: () => ({}),
}));

// ── Next.js image mock ──────────────────────────────────────────────────
vi.mock("next/image", () => ({
  default: "img",
}));

// ── Next.js link mock ───────────────────────────────────────────────────
vi.mock("next/link", () => ({
  default: "a",
}));

// ── localStorage mock (node --localstorage-file not needed) ────────────
const store: Record<string, string> = {};
Object.defineProperty(window, "localStorage", {
  value: {
    getItem: vi.fn((key: string) => store[key] ?? null),
    setItem: vi.fn((key: string, value: string) => {
      store[key] = value;
    }),
    removeItem: vi.fn((key: string) => {
      delete store[key];
    }),
    clear: vi.fn(() => {
      Object.keys(store).forEach((k) => delete store[k]);
    }),
    get length() {
      return Object.keys(store).length;
    },
    key: vi.fn((index: number) => Object.keys(store)[index] ?? null),
  },
  writable: true,
});

// ── matchMedia mock (Tailwind dark mode) ────────────────────────────────
Object.defineProperty(window, "matchMedia", {
  writable: true,
  value: vi.fn().mockImplementation((query) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
});

// ── IntersectionObserver mock (lazy loading / virtual scroll) ───────────
global.IntersectionObserver = vi.fn().mockImplementation(() => ({
  observe: vi.fn(),
  unobserve: vi.fn(),
  disconnect: vi.fn(),
}));

// ── ResizeObserver mock (Radix UI) ──────────────────────────────────────
global.ResizeObserver = vi.fn().mockImplementation(() => ({
  observe: vi.fn(),
  unobserve: vi.fn(),
  disconnect: vi.fn(),
}));

// ── scrollTo mock (virtual scroller) ────────────────────────────────────
Element.prototype.scrollTo = vi.fn();
Element.prototype.scrollIntoView = vi.fn();
