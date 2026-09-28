"use client";

/**
 * Browser API access for the authenticated app pages.
 *
 * Same rules as lib/auth-client.ts, generalised for the whole /app surface:
 *   - everything goes through the same-origin /api proxy (next.config.mjs), so
 *     the API origin never appears in client code;
 *   - the bearer token comes from AuthProvider, never from a URL;
 *   - results are typed unions, not thrown exceptions — every page renders
 *     loading / error / empty / success (§52) rather than a crash boundary;
 *   - error responses carry both a human message and the server's machine code
 *     (CONTENT_UNAVAILABLE, USERNAME_TAKEN, EXACT_DURATION_UNAVAILABLE, …) so a
 *     page can react to the failure it was designed for instead of parsing
 *     prose.
 *
 * Nothing here decides permission. The server answers 401/403/402 and the
 * client displays that fact; no surface is gated by a client-side plan check
 * that the API does not repeat.
 */

import { useCallback, useEffect, useRef, useState } from "react";

export type AppApiOk<T> = { ok: true; data: T };
export type AppApiFail = {
  ok: false;
  status: number;
  /** Server machine code when the API supplied one; otherwise HTTP_*. */
  code: string;
  message: string;
};
export type AppApiResult<T> = AppApiOk<T> | AppApiFail;

export type AppApiOptions = {
  token?: string | null;
  method?: "GET" | "POST" | "PATCH" | "PUT" | "DELETE";
  body?: unknown;
  signal?: AbortSignal;
  /** Required by the API's idempotency middleware on retried POSTs. */
  idempotencyKey?: string;
};

function fallbackMessage(status: number): string {
  if (status === 401) return "Your session has expired. Sign in again to continue.";
  if (status === 403) return "You don't have access to this.";
  if (status === 404) return "We couldn't find that.";
  if (status === 409) return "That conflicts with something that's already there.";
  if (status === 429) return "Too many attempts — wait a moment and try again.";
  if (status >= 500) return "Something went wrong on our side. Please try again shortly.";
  return "Something went wrong. Please check and try again.";
}

/** One JSON request to the API, with auth, abort and typed failure. */
export async function appApi<T>(path: string, opts: AppApiOptions = {}): Promise<AppApiResult<T>> {
  const headers: Record<string, string> = { accept: "application/json" };
  if (opts.token) headers.authorization = `Bearer ${opts.token}`;
  if (opts.body !== undefined) headers["content-type"] = "application/json";
  if (opts.idempotencyKey) headers["idempotency-key"] = opts.idempotencyKey;

  let res: Response;
  try {
    res = await fetch(`/api${path}`, {
      method: opts.method ?? "GET",
      headers,
      body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
      signal: opts.signal,
    });
  } catch (err) {
    if ((err as Error)?.name === "AbortError") {
      return { ok: false, status: 0, code: "ABORTED", message: "Cancelled." };
    }
    return {
      ok: false,
      status: 0,
      code: "NETWORK",
      message: "We couldn't reach the service. Check your connection and try again.",
    };
  }

  const text = await res.text().catch(() => "");
  let parsed: Record<string, unknown> = {};
  try {
    parsed = text ? JSON.parse(text) : {};
  } catch {
    // Non-JSON body (proxy error page); fall through to the status message.
  }

  if (!res.ok) {
    const raw = typeof parsed.error === "string" ? parsed.error : "";
    return {
      ok: false,
      status: res.status,
      code: typeof parsed.code === "string" ? parsed.code : `HTTP_${res.status}`,
      // Server messages are short and written for users (§49); cap the length
      // so an upstream HTML error page can never be echoed into the UI.
      message: raw.length > 0 && raw.length < 300 ? raw : fallbackMessage(res.status),
    };
  }
  let data: T;
  try {
    data = (text ? JSON.parse(text) : null) as T;
  } catch {
    return { ok: false, status: res.status, code: "BAD_JSON", message: fallbackMessage(500) };
  }
  return { ok: true, data };
}

export type ApiData<T> = {
  loading: boolean;
  data: T | null;
  error: AppApiFail | null;
  reload: () => void;
};

/**
 * Fetch on mount and on (token, path) change, with abort-on-cleanup.
 *
 * `null` token short-circuits to an idle state: callers render their own
 * signed-out block, because "we are not signed in" is a product state, not an
 * error.
 */
export function useApiData<T>(token: string | null | undefined, path: string): ApiData<T> {
  const [state, setState] = useState<{ loading: boolean; data: T | null; error: AppApiFail | null }>({
    loading: Boolean(token),
    data: null,
    error: null,
  });
  const [nonce, setNonce] = useState(0);
  const ctrl = useRef<AbortController | null>(null);

  useEffect(() => {
    if (!token) {
      setState({ loading: false, data: null, error: null });
      return;
    }
    ctrl.current?.abort();
    const ac = new AbortController();
    ctrl.current = ac;
    setState((s) => ({ ...s, loading: true, error: null }));
    appApi<T>(path, { token, signal: ac.signal }).then((r) => {
      if (ac.signal.aborted) return;
      if (r.ok) setState({ loading: false, data: r.data, error: null });
      else setState({ loading: false, data: null, error: r });
    });
    return () => ac.abort();
  }, [token, path, nonce]);

  const reload = useCallback(() => setNonce((n) => n + 1), []);
  return { ...state, reload };
}

/** Format an ISO date for a human; no locale theatre, one shape everywhere. */
export function formatDate(iso: string | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleDateString(undefined, { year: "numeric", month: "long", day: "numeric" });
}

/** m:ss clock for the player. */
export function formatClock(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) seconds = 0;
  const s = Math.floor(seconds % 60);
  const m = Math.floor(seconds / 60);
  return `${m}:${s.toString().padStart(2, "0")}`;
}

/** A fresh key for the API's idempotent POSTs (start/complete/progress). */
export function newIdempotencyKey(): string {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) return crypto.randomUUID();
  return `web-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}
