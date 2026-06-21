import { api, type SavedHunt } from "@/lib/api";
import { SAVED_HUNTS_KEY } from "./_constants";

/**
 * Load saved hunts from the server API. Falls back to localStorage when the
 * API key is unavailable or the server returns an error.
 */
export async function loadHunts(adminKey: string): Promise<SavedHunt[]> {
  // API-backed (primary path).
  if (adminKey) {
    try {
      const resp = await api.getSavedHunts(adminKey);
      return resp.items || [];
    } catch {
      // Fall through to localStorage fallback.
    }
  }
  return loadHuntsFromLocalStorage();
}

/** localStorage fallback for saved hunts (offline / no-admin-key mode). */
function loadHuntsFromLocalStorage(): SavedHunt[] {
  if (typeof window === "undefined") return [];
  try {
    const raw = window.localStorage.getItem(SAVED_HUNTS_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw) as SavedHunt[];
    if (!Array.isArray(parsed)) return [];
    console.warn("Using localStorage fallback for saved hunts");
    return parsed;
  } catch {
    return [];
  }
}

/**
 * Save a hunt to the server API. On failure, saves to localStorage as
 * a best-effort fallback.
 */
export async function saveHunt(adminKey: string, name: string, query: object): Promise<SavedHunt | null> {
  if (adminKey) {
    try {
      return await api.createSavedHunt(adminKey, name, query);
    } catch {
      // Fall through to localStorage fallback.
    }
  }
  // localStorage fallback
  console.warn("Using localStorage fallback for saved hunts");
  const existing = loadHuntsFromLocalStorage();
  const h: SavedHunt = {
    id: `${Date.now()}`,
    org_id: "",
    name,
    query: query as SavedHunt["query"],
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  };
  const next = [h, ...existing].slice(0, 16);
  if (typeof window !== "undefined") {
    window.localStorage.setItem(SAVED_HUNTS_KEY, JSON.stringify(next));
  }
  return h;
}

/**
 * Delete a saved hunt via the server API. Falls back to localStorage removal.
 */
export async function deleteHunt(adminKey: string, id: string): Promise<void> {
  if (adminKey) {
    try {
      await api.deleteSavedHunt(adminKey, id);
      return;
    } catch {
      // Fall through to localStorage fallback.
    }
  }
  // localStorage fallback
  console.warn("Using localStorage fallback for saved hunts");
  const existing = loadHuntsFromLocalStorage();
  const next = existing.filter((x) => x.id !== id);
  if (typeof window !== "undefined") {
    window.localStorage.setItem(SAVED_HUNTS_KEY, JSON.stringify(next));
  }
}
