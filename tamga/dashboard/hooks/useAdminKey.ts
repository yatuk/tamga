"use client";

import { useCallback, useEffect, useState } from "react";

export const ADMIN_KEY_STORAGE = "tamga_admin_key";

export function useAdminKey(fallback?: string): [string, (key: string) => void] {
  const [adminKey, setAdminKeyState] = useState("");

  useEffect(() => {
    if (typeof window === "undefined") return;
    const existing = window.localStorage.getItem(ADMIN_KEY_STORAGE) || "";
    const effective = existing || fallback || "";
    setAdminKeyState(effective);
  }, [fallback]);

  const setAdminKey = useCallback((key: string) => {
    if (typeof window === "undefined") return;
    if (key) {
      window.localStorage.setItem(ADMIN_KEY_STORAGE, key);
    } else {
      window.localStorage.removeItem(ADMIN_KEY_STORAGE);
    }
    setAdminKeyState(key);
  }, []);

  return [adminKey, setAdminKey];
}
