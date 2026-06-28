"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface SessionUser {
  id: string;
  email: string;
  name: string;
  avatar: string;
  role: string;
}

interface SessionState {
  user: SessionUser | null;
  token: string | null;
  loading: boolean;
  error: string | null;
}

interface SessionActions {
  login: () => void; // Redirects to GitHub OAuth
  logout: () => void;
  refreshSession: () => Promise<void>;
}

type SessionContextValue = SessionState & SessionActions;

// ---------------------------------------------------------------------------
// Token storage (sessionStorage survives page refreshes, cleared on tab close)
// ---------------------------------------------------------------------------

const TOKEN_KEY = "tamga_session_token";
const USER_KEY = "tamga_session_user";

function getStoredToken(): string | null {
  if (typeof window === "undefined") return null;
  return sessionStorage.getItem(TOKEN_KEY);
}

function clearStoredToken(): void {
  sessionStorage.removeItem(TOKEN_KEY);
  sessionStorage.removeItem(USER_KEY);
}

function getStoredUser(): SessionUser | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = sessionStorage.getItem(USER_KEY);
    return raw ? (JSON.parse(raw) as SessionUser) : null;
  } catch {
    return null;
  }
}

function setStoredUser(user: SessionUser): void {
  sessionStorage.setItem(USER_KEY, JSON.stringify(user));
}

// ---------------------------------------------------------------------------
// Context
// ---------------------------------------------------------------------------

const SessionContext = createContext<SessionContextValue>({
  user: null,
  token: null,
  loading: true,
  error: null,
  login: () => {},
  logout: () => {},
  refreshSession: async () => {},
});

export function useSession(): SessionContextValue {
  return useContext(SessionContext);
}

/**
 * Returns a Bearer token header object for use with fetchAPI.
 * Falls back to admin key when no SSO session is active.
 */
export function useAuthHeaders(): Record<string, string> {
  const { token } = useSession();
  if (token) {
    return { Authorization: `Bearer ${token}` };
  }
  // Fall back to admin key (non-SSO mode)
  const adminKey = process.env.NEXT_PUBLIC_ADMIN_KEY;
  return adminKey ? { "X-Tamga-Admin-Key": adminKey } : {};
}

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8443";

export function SessionProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<SessionUser | null>(getStoredUser);
  const [token, setToken] = useState<string | null>(getStoredToken);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // On mount, verify any stored token is still valid.
  useEffect(() => {
    const storedToken = getStoredToken();
    if (!storedToken) {
      setLoading(false);
      return;
    }

    fetch(`${API_BASE}/api/v1/auth/session`, {
      headers: { Authorization: `Bearer ${storedToken}` },
    })
      .then((res) => {
        if (!res.ok) throw new Error("Session expired");
        return res.json();
      })
      .then((data: { user: SessionUser }) => {
        setUser(data.user);
        setStoredUser(data.user);
        setToken(storedToken);
      })
      .catch(() => {
        // Token expired or invalid — clear.
        clearStoredToken();
        setUser(null);
        setToken(null);
      })
      .finally(() => setLoading(false));
  }, []);

  const login = useCallback(() => {
    window.location.href = `${API_BASE}/api/v1/auth/github/login`;
  }, []);

  const logout = useCallback(() => {
    clearStoredToken();
    setUser(null);
    setToken(null);
  }, []);

  const refreshSession = useCallback(async () => {
    const storedToken = getStoredToken();
    if (!storedToken) {
      setUser(null);
      setToken(null);
      return;
    }

    try {
      const res = await fetch(`${API_BASE}/api/v1/auth/session`, {
        headers: { Authorization: `Bearer ${storedToken}` },
      });
      if (!res.ok) throw new Error("Session expired");
      const data = (await res.json()) as { user: SessionUser };
      setUser(data.user);
      setStoredUser(data.user);
      setToken(storedToken);
      setError(null);
    } catch (err) {
      clearStoredToken();
      setUser(null);
      setToken(null);
      setError(err instanceof Error ? err.message : "Session check failed");
    }
  }, []);

  const value = useMemo<SessionContextValue>(
    () => ({ user, token, loading, error, login, logout, refreshSession }),
    [user, token, loading, error, login, logout, refreshSession],
  );

  return (
    <SessionContext.Provider value={value}>{children}</SessionContext.Provider>
  );
}
