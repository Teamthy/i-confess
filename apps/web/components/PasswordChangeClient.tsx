"use client";

/**
 * POST /auth/change-password — one real control on the account settings page.
 *
 * The server revokes every OTHER signed-in session when the password changes
 * (S28). That is product behaviour the user must see before they do it, so it
 * is part of the confirm copy, not buried in a docs page. On success the local
 * session stays alive (its own token is not revoked) and the form locks to a
 * one-shot confirmation — you cannot "un-change" a password by resubmitting
 * the same body, and reusing a stale form would only produce 401s.
 */

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth-context";
import { appApi } from "@/lib/app-api";
import { SignedOut } from "@/components/app-ui";

export function PasswordChangeClient() {
  const { token, loading: authLoading, logout } = useAuth();
  const router = useRouter();
  const [current, setCurrent] = useState("");
  const [nextPw, setNextPw] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [done, setDone] = useState(false);

  if (authLoading) return null;
  if (!token) return <SignedOut next="/app/settings/account" />;

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    if (nextPw.length < 8) {
      setError("Passwords are at least 8 characters.");
      return;
    }
    if (nextPw !== confirm) {
      setError("The new passwords don't match.");
      return;
    }
    if (!current) {
      setError("Enter your current password to confirm this.");
      return;
    }
    setBusy(true);
    const res = await appApi("/auth/change-password", {
      token,
      method: "POST",
      body: { current_password: current, new_password: nextPw },
    });
    setBusy(false);
    if (!res.ok) {
      // 401 here means "current password is incorrect" — the local session is
      // still valid, so say the specific thing rather than redirecting.
      setError(res.status === 401 ? "That current password isn't right." : res.message);
      return;
    }
    setCurrent("");
    setNextPw("");
    setConfirm("");
    setDone(true);
    // The server revoked every other device; reflect that here too, and let
    // the user sign back in with the new password rather than trusting a token
    // that another device may have rotated around it.
    setTimeout(() => {
      logout();
      router.push("/login?changed=1");
    }, 1800);
  }

  if (done) {
    return (
      <div className="ia-settings__group" role="status">
        <h2>Password changed</h2>
        <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)" }}>
          Every other device was signed out. You&rsquo;ll be returned to sign in with your new
          password shortly — it&rsquo;s safer than trusting the old session.
        </p>
      </div>
    );
  }

  return (
    <form onSubmit={onSubmit} noValidate style={{ display: "grid", gap: "var(--ic-spacing-4)" }}>
      <div className="ic-field">
        <label htmlFor="pw-current">Current password</label>
        <input
          id="pw-current"
          type="password"
          autoComplete="current-password"
          value={current}
          onChange={(e) => setCurrent(e.target.value)}
          required
        />
      </div>
      <div className="ic-field">
        <label htmlFor="pw-new">New password</label>
        <input
          id="pw-new"
          type="password"
          autoComplete="new-password"
          value={nextPw}
          onChange={(e) => setNextPw(e.target.value)}
          required
          aria-describedby="pw-new-hint"
        />
        <p id="pw-new-hint" className="ic-field__hint">At least 8 characters.</p>
      </div>
      <div className="ic-field">
        <label htmlFor="pw-confirm">Confirm new password</label>
        <input
          id="pw-confirm"
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
      <p style={{ fontSize: "var(--ic-font-size-caption)", color: "var(--ic-color-neutral-500)", margin: 0 }}>
        Changing your password signs out every other device. This browser will ask you to sign in
        again once it&rsquo;s done.
      </p>
      <button type="submit" className="ic-btn ic-btn--primary" disabled={busy} style={{ width: "fit-content" }}>
        {busy ? "Updating…" : "Update password"}
      </button>
    </form>
  );
}
