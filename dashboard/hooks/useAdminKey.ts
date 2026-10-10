"use client";

import { useCallback, useSyncExternalStore } from "react";
import { BUILT_IN_ADMIN_KEY } from "@/lib/built-in-key";

export const ADMIN_KEY_STORAGE = "tamga_admin_key";

const CHANGE_EVENT = "tamga:admin-key";

function subscribe(onChange: () => void) {
  window.addEventListener("storage", onChange);
  window.addEventListener(CHANGE_EVENT, onChange);
  return () => {
    window.removeEventListener("storage", onChange);
    window.removeEventListener(CHANGE_EVENT, onChange);
  };
}

function readStored(): string {
  try {
    return window.localStorage.getItem(ADMIN_KEY_STORAGE) || "";
  } catch {
    return "";
  }
}

/**
 * The admin key used for the proxy management API.
 *
 * The key saved in this browser wins; without one, `fallback` and then, in
 * development only, the key built into the page apply. In a production build
 * there is no built-in key, so the console starts with none. Every component
 * using the hook sees a change immediately, including one made in another tab.
 */
export function useAdminKey(fallback?: string): [string, (key: string) => void] {
  // Empty on the server and during hydration, so markup matches; the stored
  // key arrives on the first client render after that.
  const stored = useSyncExternalStore(subscribe, readStored, () => "");
  const adminKey = stored || fallback || BUILT_IN_ADMIN_KEY;

  const setAdminKey = useCallback((key: string) => {
    try {
      if (key) window.localStorage.setItem(ADMIN_KEY_STORAGE, key);
      else window.localStorage.removeItem(ADMIN_KEY_STORAGE);
    } catch {
      /* storage disabled: the key cannot be remembered */
    }
    window.dispatchEvent(new Event(CHANGE_EVENT));
  }, []);

  return [adminKey, setAdminKey];
}
