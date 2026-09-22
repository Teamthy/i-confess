"use client";

/**
 * Browser-side API access for the auth screens.
 *
 * The Go API is reached same-origin via the /api proxy (next.config.mjs), so
 * no API origin is ever embedded in client code and no cross-site request is
 * made. Tokens are held in memory + localStorage by AuthProvider
 * (lib/auth-context.tsx); never in URLs, never in a readable cookie.
 */

export type AuthUser = {
  id: string;
  email: string;
  display_name: string;
  timezone?: string;
  status: string;
  email_verified: boolean;
  mfa_enabled?: boolean;
  avatar_url?: string;
};

export type AuthResult =
  | {
      ok: true;
      status: number;
      /** Absent for flows that intentionally return no session (a pending email). */
      token?: string;
      user?: AuthUser;
      /** Server machine code (pending, …) so forms branch on facts, not prose. */
      code?: string;
      message?: string;
    }
  | { ok: false; status: number; code?: string; message: string };

function messageFor(status: number): string {
  if (status === 401) return "That email and password don't match an account.";
  if (status === 403) return "This account is not available.";
  if (status === 409) return "An account with this email already exists.";
  if (status === 429) return "Too many attempts — please wait a moment and try again.";
  if (status >= 500) return "We couldn't reach the service. Please try again shortly.";
  return "That didn't work. Please check the details and try again.";
}

/**
 * POST a JSON body to one auth endpoint. Errors carry the server's `code`
 * when it supplied one: login uses `mfa_required` to show the second-factor
 * prompt, and registration uses the absence of a token to mean "check your
 * email" (the response for an existing address is deliberately identical to a
 * fresh signup, so the endpoint cannot enumerate accounts).
 */
export async function authFetch(
  path: string,
  body: unknown,
  method = "POST",
): Promise<AuthResult> {
  let res: Response;
  try {
    res = await fetch(`/api${path}`, {
      method,
      headers: { "content-type": "application/json", accept: "application/json" },
      body: JSON.stringify(body),
    });
  } catch {
    return { ok: false, status: 0, message: "We couldn't reach the service. Please try again shortly." };
  }

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

  // Login with MFA enrolled: 200 + mfa_required. Credentials were correct,
  // the sign-in is incomplete — the form switches to the code step.
  if (data.mfa_required === true) {
    return {
      ok: false,
      status: res.status,
      code: "mfa_required",
      message: typeof data.message === "string" ? data.message : "Enter the code from your authenticator app.",
    };
  }

  const token = typeof data.token === "string" ? data.token : "";
  return {
    ok: true,
    status: res.status,
    token,
    user: data.user as AuthUser | undefined,
    code: data.pending === true ? "pending" : undefined,
    message: typeof data.message === "string" ? data.message : undefined,
  };
}
