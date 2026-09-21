"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { AuthCard } from "@/components/AuthCard";
import { authFetch } from "@/lib/auth-client";

/**
 * Sign in. Talks to POST /auth/login through the same-origin /api proxy.
 * MFA-capable accounts receive `mfa_required` — the second step comes with
 * the full authenticated shell in PHASE 39; until then the server's message
 * is shown verbatim rather than a fake success.
 */
export function LoginForm() {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [showPw, setShowPw] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    if (!email.trim() || !password) {
      setError("Enter your email and password.");
      return;
    }
    setBusy(true);
    const res = await authFetch("/auth/login", { email: email.trim(), password });
    setBusy(false);
    if (res.ok) {
      // Session establishment for the /app shell lands in PHASE 39; direct
      // sign-ins there once it exists. Until then, confirm and return home.
      router.push("/?signed-in=1");
      return;
    }
    setError(res.message);
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
        {error && (
          <div role="alert" style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-semantic-danger-light)" }}>
            {error}
          </div>
        )}
        <button type="submit" className="ic-btn ic-btn--primary" disabled={busy} style={{ width: "100%" }}>
          {busy ? "Signing in…" : "Continue"}
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
