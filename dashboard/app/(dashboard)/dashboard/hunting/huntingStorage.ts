import { api, type SavedHunt } from "@/lib/api";
import { SAVED_HUNTS_KEY } from "./_constants";

/**
 * Saved hunts live on the proxy. Without an admin key there is no proxy to
 * ask, and they are kept in this browser instead. A proxy that refuses is an
 * error the caller reports: a hunt must never look saved when it is not.
 */
export async function loadHunts(adminKey: string): Promise<SavedHunt[]> {
  if (adminKey) {
    const resp = await api.getSavedHunts(adminKey);
    return resp.items || [];
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
    return parsed;
  } catch {
    return [];
  }
}

/** Saves a hunt on the proxy, or in this browser when there is no admin key. */
export async function saveHunt(adminKey: string, name: string, query: object): Promise<SavedHunt | null> {
  if (adminKey) return api.createSavedHunt(adminKey, name, query);
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

/** Deletes a saved hunt from the proxy, or from this browser when there is no admin key. */
export async function deleteHunt(adminKey: string, id: string): Promise<void> {
  if (adminKey) {
    await api.deleteSavedHunt(adminKey, id);
    return;
  }
  const existing = loadHuntsFromLocalStorage();
  const next = existing.filter((x) => x.id !== id);
  if (typeof window !== "undefined") {
    window.localStorage.setItem(SAVED_HUNTS_KEY, JSON.stringify(next));
  }
}
