"use client";

import { useState } from "react";
import Link from "next/link";
import { AuthCard } from "@/components/AuthCard";
import { authFetch } from "@/lib/auth-client";
import { useAuth } from "@/lib/auth-context";

/**
 * Create an account. Talks to POST /auth/register through the same-origin
 * proxy. The password policy is enforced by the server; the client mirrors
 * its public shape (length) only as a courtesy hint, never as the authority.
 */
export function RegisterForm() {
  const { setToken } = useAuth();
  const [fields, setFields] = useState({ name: "", email: "", password: "", confirm: "" });
  const [issues, setIssues] = useState<Record<string, string>>({});
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  const [signedIn, setSignedIn] = useState(false);

  function validate() {
    const next: Record<string, string> = {};
    if (!fields.name.trim()) next.name = "Please add a name we can greet you with.";
    if (!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(fields.email)) next.email = "Enter a valid email address.";
    if (fields.password.length < 8) next.password = "Passwords are at least 8 characters.";
    if (fields.password !== fields.confirm) next.confirm = "The passwords don't match.";
    setIssues(next);
    return Object.keys(next).length === 0;
  }

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    if (!validate()) return;
    setBusy(true);
    const tz =
      typeof Intl !== "undefined" ? Intl.DateTimeFormat().resolvedOptions().timeZone : "";
    const res = await authFetch("/auth/register", {
      email: fields.email.trim(),
      password: fields.password,
      display_name: fields.name.trim(),
      ...(tz ? { timezone: tz } : {}),
    });
    setBusy(false);
    if (res.ok) {
      // A brand-new account gets a session immediately (the verification mail
      // then upgrades the flag, not the login). An address that already exists
      // gets the identical "check your email" response with no session — the
      // form shows the same state either way, because the server does.
      if (res.token && res.user) {
        setToken(res.token, res.user);
        setSignedIn(true);
      }
      setDone(true);
      return;
    }
    setError(res.message ?? "That didn't work. Please check the details and try again.");
  }

  const set = (k: keyof typeof fields) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setFields((f) => ({ ...f, [k]: e.target.value }));

  return (
    <AuthCard
      eyebrow="Begin"
      title="Create your account"
      lede="One account for the practice — web and mobile."
      footer={
        <>
          Already have an account?{" "}
          <Link href="/login" style={{ color: "var(--ic-color-brand-300)" }}>
            Sign in
          </Link>
        </>
      }
    >
      {done ? (
        <div role="status" className="ic-state" style={{ borderStyle: "solid", borderColor: "var(--ic-color-brand-300)", padding: "var(--ic-spacing-6)" }}>
          <h3>Check your email.</h3>
          <p>
            We sent a verification link to {fields.email}. Verify to finish —
            and welcome.
          </p>
          {signedIn ? (
            <Link href="/welcome" className="ic-btn ic-btn--primary">Go to your experience</Link>
          ) : (
            <Link href="/login" className="ic-btn ic-btn--primary">Sign in to continue</Link>
          )}
        </div>
      ) : (
        <form onSubmit={onSubmit} noValidate style={{ display: "grid", gap: "var(--ic-spacing-4)" }}>
          <div className="ic-field">
            <label htmlFor="reg-name">Name</label>
            <input id="reg-name" autoComplete="name" value={fields.name} onChange={set("name")} aria-invalid={issues.name ? true : undefined} />
            {issues.name && <p className="ic-field__error">{issues.name}</p>}
          </div>
          <div className="ic-field">
            <label htmlFor="reg-email">Email</label>
            <input id="reg-email" type="email" autoComplete="email" value={fields.email} onChange={set("email")} aria-invalid={issues.email ? true : undefined} />
            {issues.email && <p className="ic-field__error">{issues.email}</p>}
          </div>
          <div className="ic-field">
            <label htmlFor="reg-password">Password</label>
            <input id="reg-password" type="password" autoComplete="new-password" value={fields.password} onChange={set("password")} aria-invalid={issues.password ? true : undefined} />
            {issues.password ? (
              <p className="ic-field__error">{issues.password}</p>
            ) : (
              <p className="ic-field__hint">At least 8 characters.</p>
            )}
          </div>
          <div className="ic-field">
            <label htmlFor="reg-confirm">Confirm password</label>
            <input id="reg-confirm" type="password" autoComplete="new-password" value={fields.confirm} onChange={set("confirm")} aria-invalid={issues.confirm ? true : undefined} />
            {issues.confirm && <p className="ic-field__error">{issues.confirm}</p>}
          </div>
          {error && (
            <div role="alert" style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-semantic-danger-light)" }}>
              {error}
            </div>
          )}
          <p style={{ fontSize: "var(--ic-font-size-caption)", color: "var(--ic-color-neutral-500)", lineHeight: "var(--ic-font-lineHeight-relaxed)" }}>
            By creating an account you agree to our{" "}
            <Link href="/terms">terms</Link> and{" "}
            <Link href="/privacy">privacy policy</Link>. Your private confessions
            stay private.
          </p>
          <button type="submit" className="ic-btn ic-btn--primary" disabled={busy} style={{ width: "100%" }}>
            {busy ? "Creating account…" : "Create account"}
          </button>
        </form>
      )}
    </AuthCard>
  );
}
