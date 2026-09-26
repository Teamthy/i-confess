"use client";

/**
 * Client-side auth state for the whole site.
 *
 * The token lives in memory and is mirrored to localStorage so a refresh
 * survives; it is never placed in a URL and never read by a server component
 * (server pages pass only through /api with a bearer header or render their
 * signed-out state). Mounted in the root layout so /login and /register can
 * establish the session that every /app page then consumes.
 *
 * Expiry is reactive, not speculative: when the API answers 401, callers
 * render their re-sign-in state. There is no client-side "premium flag"
 * anywhere in this provider, because the server decides entitlement and a
 * client copy of that decision could only ever be wrong or misleading.
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
import { authFetch, type AuthUser } from "./auth-client";

export type { AuthUser };

type AuthState = {
  user: AuthUser | null;
  token: string | null;
  loading: boolean;
  login: (email: string, password: string, code?: string) => Promise<{ ok: boolean; error?: string; mfaRequired?: boolean }>;
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
    // Ask the server to revoke this session first, with the bearer token it
    // requires; clearing local state then stands even if that request fails.
    try {
      const t = localStorage.getItem(TOKEN_KEY);
      fetch(`/api/auth/logout`, {
        method: "POST",
        headers: t ? { authorization: `Bearer ${t}` } : {},
      }).catch(() => {});
    } catch {
      // ignore — nothing local is left behind either way
    }
    setTokenState(null);
    setUser(null);
    try {
      localStorage.removeItem(TOKEN_KEY);
      localStorage.removeItem(USER_KEY);
    } catch {
      // ignore
    }
  }, []);

  const login = useCallback(
    async (email: string, password: string, code?: string) => {
      const res = await authFetch("/auth/login", { email, password, ...(code ? { code } : {}) });
      if (res.code === "mfa_required") return { ok: false, mfaRequired: true, error: res.message };
      if (!res.ok) return { ok: false, error: res.message ?? "That didn't work." };
      if (res.token && res.user) setToken(res.token, res.user);
      return { ok: true };
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
