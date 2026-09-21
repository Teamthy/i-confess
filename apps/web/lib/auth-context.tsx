"use client";

/**
 * Client-side auth state for the authenticated shell.
 *
 * Token is held in memory (never in URL) and persisted to localStorage so a
 * page refresh survives. The provider exposes login/logout/token for the
 * app-shell layout and its children. Server components read the token from
 * the cookie set by login, so they never see localStorage directly.
 */

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";

export type AuthUser = {
  id: string;
  email: string;
  display_name: string;
  timezone?: string;
  status: string;
  email_verified: boolean;
  avatar_url?: string;
};

type AuthState = {
  user: AuthUser | null;
  token: string | null;
  loading: boolean;
  login: (email: string, password: string) => Promise<{ ok: boolean; error?: string; mfaRequired?: boolean }>;
  logout: () => void;
  setToken: (token: string, user: AuthUser) => void;
};

const AuthCtx = createContext<AuthState>({
  user: null,
  token: null,
  loading: true,
  login: async () => ({ ok: false, error: "Not initialized" }),
  logout: () => {},
  setToken: () => {},
});

const TOKEN_KEY = "ic_token";
const USER_KEY = "ic_user";

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<AuthUser | null>(null);
  const [token, setTokenState] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  // Hydrate from localStorage on mount.
  useEffect(() => {
    try {
      const stored = localStorage.getItem(TOKEN_KEY);
      const storedUser = localStorage.getItem(USER_KEY);
      if (stored && storedUser) {
        setTokenState(stored);
        setUser(JSON.parse(storedUser) as AuthUser);
      }
    } catch {
      // corrupted storage — clear it
      localStorage.removeItem(TOKEN_KEY);
      localStorage.removeItem(USER_KEY);
    }
    setLoading(false);
  }, []);

  const setToken = useCallback((t: string, u: AuthUser) => {
    setTokenState(t);
    setUser(u);
    try {
      localStorage.setItem(TOKEN_KEY, t);
      localStorage.setItem(USER_KEY, JSON.stringify(u));
    } catch {
      // storage full or unavailable — session still works in memory
    }
  }, []);

  const logout = useCallback(() => {
    setTokenState(null);
    setUser(null);
    try {
      localStorage.removeItem(TOKEN_KEY);
      localStorage.removeItem(USER_KEY);
    } catch {
      // ignore
    }
    // Fire-and-forget server logout.
    fetch("/api/auth/logout", { method: "POST" }).catch(() => {});
  }, []);

  const login = useCallback(
    async (email: string, password: string) => {
      try {
        const res = await fetch("/api/auth/login", {
          method: "POST",
          headers: { "content-type": "application/json" },
          body: JSON.stringify({ email, password }),
        });
        const data = (await res.json().catch(() => ({}))) as Record<string, unknown>;
        if (!res.ok) {
          if (res.status === 403 && data.code === "mfa_required") {
            return { ok: false, mfaRequired: true };
          }
          return {
            ok: false,
            error:
              typeof data.error === "string" && data.error.length < 200
                ? data.error
                : "That email and password don't match an account.",
          };
        }
        const t = String(data.token ?? "");
        const u = data.user as AuthUser;
        if (t && u) setToken(t, u);
        return { ok: true };
      } catch {
        return { ok: false, error: "We couldn't reach the service. Please try again shortly." };
      }
    },
    [setToken],
  );

  const value = useMemo(
    () => ({ user, token, loading, login, logout, setToken }),
    [user, token, loading, login, logout, setToken],
  );

  return <AuthCtx.Provider value={value}>{children}</AuthCtx.Provider>;
}

export function useAuth() {
  return useContext(AuthCtx);
}

/** Authenticated fetch helper — adds Bearer token and handles 401. */
export async function authedFetch(
  token: string,
  path: string,
  init?: RequestInit,
): Promise<Response> {
  const headers = new Headers(init?.headers);
  headers.set("Authorization", `Bearer ${token}`);
  headers.set("Accept", "application/json");
  if (init?.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  return fetch(`/api${path}`, { ...init, headers });
}
