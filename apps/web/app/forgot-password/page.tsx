"use client";

import { useState } from "react";
import Link from "next/link";
import { AuthCard } from "@/components/AuthCard";
import { authFetch } from "@/lib/auth-client";

/**
 * Request a password reset. POST /auth/request-password-reset through the
 * same-origin proxy. The response is deliberately identical whether or not
 * the address exists — the page says the same thing either way, so the form
 * cannot be used to enumerate accounts.
 */
export default function ForgotPasswordPage() {
  const [email, setEmail] = useState("");
  const [busy, setBusy] = useState(false);
  const [sent, setSent] = useState(false);
  const [error, setError] = useState("");

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    if (!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(email)) {
      setError("Enter a valid email address.");
      return;
    }
    setBusy(true);
    const res = await authFetch("/auth/request-password-reset", { email: email.trim() });
    setBusy(false);
    // Both "sent" and "accepted" shapes end here; anything else shows the
    // human error. Account-existence is never revealed.
    if (res.ok || res.status === 202 || res.status === 200) setSent(true);
    else setError(res.message);
  }

  return (
    <AuthCard
      eyebrow="Account recovery"
      title="Reset your password"
      lede="We'll email you a link to choose a new one."
      footer={
        <>
          Remembered it?{" "}
          <Link href="/login" style={{ color: "var(--ic-color-brand-300)" }}>
            Sign in
          </Link>
        </>
      }
    >
      {sent ? (
        <div role="status" className="ic-state" style={{ borderStyle: "solid", borderColor: "var(--ic-color-brand-300)", padding: "var(--ic-spacing-6)" }}>
          <h3>If that address has an account, the link is on its way.</h3>
          <p>Check your inbox — and the folder messages go to quietly.</p>
        </div>
      ) : (
        <form onSubmit={onSubmit} noValidate style={{ display: "grid", gap: "var(--ic-spacing-4)" }}>
          <div className="ic-field">
            <label htmlFor="forgot-email">Email</label>
            <input
              id="forgot-email"
              type="email"
              autoComplete="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              required
            />
          </div>
          {error && (
            <div role="alert" style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-semantic-danger-light)" }}>
              {error}
            </div>
          )}
          <button type="submit" className="ic-btn ic-btn--primary" disabled={busy} style={{ width: "100%" }}>
            {busy ? "Sending…" : "Send reset link"}
          </button>
        </form>
      )}
    </AuthCard>
  );
}
