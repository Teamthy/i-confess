"use client";

import { Suspense, useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { AuthCard } from "@/components/AuthCard";
import { authFetch } from "@/lib/auth-client";
import { useAuth } from "@/lib/auth-context";

/**
 * Sign in. Talks to POST /auth/login through the same-origin /api proxy and,
 * on success, hands {token, user} to AuthProvider — which is what every /app
 * page uses to authenticate its own calls. Before this existed the form
 * "signed in" by redirecting home without ever storing the session, and the
 * authenticated app could never see one.
 *
 * MFA accounts get `mfa_required` back; the same form then asks for the code
 * and re-submits. The server accepts {email, password, code} in one round
 * trip, so this is a second step, not a second flow.
 */
function safeNext(raw: string | null): string {
  // Only same-origin paths into the app; never an absolute URL.
  if (raw && /^\/(app|welcome)([/?#].*)?$/.test(raw)) return raw;
  return "/app";
}

function LoginFormInner() {
  const router = useRouter();
  const search = useSearchParams();
  const { setToken } = useAuth();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [showPw, setShowPw] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [mfaStep, setMfaStep] = useState(false);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    if (!email.trim() || !password) {
      setError("Enter your email and password.");
      return;
    }
    if (mfaStep && code.trim().length < 6) {
      setError("Enter the 6-digit code from your authenticator app.");
      return;
    }
    setBusy(true);
    const res = await authFetch("/auth/login", {
      email: email.trim(),
      password,
      ...(mfaStep ? { code: code.trim() } : {}),
    });
    setBusy(false);
    if (res.code === "mfa_required" && !mfaStep) {
      setMfaStep(true);
      setError(res.message ?? "");
      return;
    }
    if (res.ok && res.token && res.user) {
      setToken(res.token, res.user);
      router.push(safeNext(search.get("next")));
      return;
    }
    setError(res.message ?? "That didn't work. Please check the details and try again.");
  }

  return (
    <AuthCard
      eyebrow="Welcome back"
      title="Sign in to iCONFESS"
      lede="Your practice is where you left it."
      footer={
        <>
          New here?{" "}
          <Link href="/register" style={{ color: "var(--ic-color-brand-300)" }}>
            Create an account
          </Link>
        </>
      }
    >
      <form onSubmit={onSubmit} noValidate style={{ display: "grid", gap: "var(--ic-spacing-4)" }}>
        <div className="ic-field">
          <label htmlFor="login-email">Email</label>
          <input
            id="login-email"
            type="email"
            autoComplete="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
          />
        </div>
        <div className="ic-field">
          <label htmlFor="login-password">Password</label>
          <div style={{ position: "relative" }}>
            <input
              id="login-password"
              type={showPw ? "text" : "password"}
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              style={{ paddingRight: "3.5rem" }}
            />
            <button
              type="button"
              onClick={() => setShowPw((s) => !s)}
              style={{ position: "absolute", right: "0.75rem", top: "50%", translate: "0 -50%", fontSize: "var(--ic-font-size-caption)", color: "var(--ic-color-neutral-500)" }}
            >
              {showPw ? "Hide" : "Show"}
            </button>
          </div>
        </div>
        {mfaStep && (
          <div className="ic-field">
            <label htmlFor="login-code">Authentication code</label>
            <input
              id="login-code"
              type="text"
              inputMode="numeric"
              autoComplete="one-time-code"
              pattern="[0-9]{6}"
              maxLength={10}
              value={code}
              onChange={(e) => setCode(e.target.value)}
              required
            />
            <p className="ic-field__hint">Six digits from your authenticator app, or a recovery code.</p>
          </div>
        )}
        {error && (
          <div role="alert" style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-semantic-danger-light)" }}>
            {error}
          </div>
        )}
        <button type="submit" className="ic-btn ic-btn--primary" disabled={busy} style={{ width: "100%" }}>
          {busy ? "Signing in…" : mfaStep ? "Verify & sign in" : "Continue"}
        </button>
        <p style={{ textAlign: "center" }}>
          <Link href="/forgot-password" className="ic-btn--text" style={{ fontSize: "var(--ic-font-size-bodySm)" }}>
            Forgot your password?
          </Link>
        </p>
      </form>
    </AuthCard>
  );
}

/**
 * The page is statically rendered, and reading `?next` is a dynamic operation;
 * Next requires a Suspense boundary around that read. Wrapping here keeps the
 * boundary next to the code that needs it.
 */
export function LoginForm() {
  return (
    <Suspense fallback={null}>
      <LoginFormInner />
    </Suspense>
  );
}
