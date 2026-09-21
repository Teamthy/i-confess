"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { AuthCard } from "@/components/AuthCard";
import { authFetch } from "@/lib/auth-client";

/**
 * Choose a new password from an emailed token (/reset-password?token=…).
 * POST /auth/reset-password through the same-origin proxy. Tokens arrive by
 * query parameter from the email link and are used exactly once, then
 * dropped from the URL by the router on success.
 */
export function ResetForm() {
  const params = useSearchParams();
  const router = useRouter();
  const token = params.get("token") || "";
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    if (!token) {
      setError("This link is missing its token. Request a fresh one.");
      return;
    }
    if (password.length < 8) {
      setError("Passwords are at least 8 characters.");
      return;
    }
    if (password !== confirm) {
      setError("The passwords don't match.");
      return;
    }
    setBusy(true);
    const res = await authFetch("/auth/reset-password", { token, password });
    setBusy(false);
    if (res.ok) {
      setDone(true);
      router.replace("/reset-password");
      return;
    }
    setError(res.message);
  }

  return (
    <AuthCard
      eyebrow="Account recovery"
      title="Choose a new password"
      footer={
        <Link href="/login" style={{ color: "var(--ic-color-brand-300)" }}>
          Back to sign in
        </Link>
      }
    >
      {done ? (
        <div role="status" className="ic-state" style={{ borderStyle: "solid", borderColor: "var(--ic-color-brand-300)", padding: "var(--ic-spacing-6)" }}>
          <h3>Password updated.</h3>
          <p>Your other sessions were signed out. Sign in with the new one.</p>
          <Link href="/login" className="ic-btn ic-btn--primary">Sign in</Link>
        </div>
      ) : (
        <form onSubmit={onSubmit} noValidate style={{ display: "grid", gap: "var(--ic-spacing-4)" }}>
          <div className="ic-field">
            <label htmlFor="reset-password">New password</label>
            <input
              id="reset-password"
              type="password"
              autoComplete="new-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
            />
          </div>
          <div className="ic-field">
            <label htmlFor="reset-confirm">Confirm new password</label>
            <input
              id="reset-confirm"
              type="password"
              autoComplete="new-password"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
              required
            />
          </div>
          {error && (
            <div role="alert" style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-semantic-danger-light)" }}>
              {error}
            </div>
          )}
          <button type="submit" className="ic-btn ic-btn--primary" disabled={busy} style={{ width: "100%" }}>
            {busy ? "Saving…" : "Set new password"}
          </button>
        </form>
      )}
    </AuthCard>
  );
}
