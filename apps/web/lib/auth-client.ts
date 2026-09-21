"use client";

/**
 * Browser-side API access for the auth screens.
 *
 * The Go API is reached same-origin via the /api proxy (next.config.mjs), so
 * no API origin is ever embedded in client code and no cross-site request is
 * made. Tokens are held in memory + localStorage by the caller (the
 * authenticated shell in PHASE 39 will centralise this); never in URLs.
 */

export type AuthUser = {
  id: string;
  email: string;
  display_name: string;
  timezone?: string;
  status: string;
  email_verified: boolean;
};

export type AuthResult =
  | { ok: true; token: string; user: AuthUser }
  | { ok: false; status: number; code?: string; message: string };

function messageFor(status: number): string {
  if (status === 401) return "That email and password don't match an account.";
  if (status === 409) return "An account with this email already exists.";
  if (status === 429) return "Too many attempts — please wait a moment and try again.";
  if (status >= 500) return "We couldn't reach the service. Please try again shortly.";
  return "That didn't work. Please check the details and try again.";
}

export async function authFetch(
  path: string,
  body: unknown,
  method = "POST",
): Promise<AuthResult> {
  try {
    const res = await fetch(`/api${path}`, {
      method,
      headers: { "content-type": "application/json" },
      body: JSON.stringify(body),
    });
    const data = (await res.json().catch(() => ({}))) as Record<string, unknown>;
    if (!res.ok) {
      return {
        ok: false,
        status: res.status,
        code: typeof data.code === "string" ? data.code : undefined,
        message:
          typeof data.error === "string" && data.error.length < 200 && !/sql|stack|internal id/i.test(data.error)
            ? data.error
            : messageFor(res.status),
      };
    }
    return { ok: true, token: String(data.token ?? ""), user: data.user as AuthUser };
  } catch {
    return { ok: false, status: 0, message: "We couldn't reach the service. Please try again shortly." };
  }
}
