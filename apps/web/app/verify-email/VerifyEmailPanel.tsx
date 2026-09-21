"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { AuthCard } from "@/components/AuthCard";
import { authFetch } from "@/lib/auth-client";

/**
 * Email verification panel (/verify-email?token=...). Verifies once on load
 * when a token is present; otherwise explains what to do. States: verifying,
 * verified, already-verified/invalid, and error - each human-readable.
 */
export function VerifyEmailPanel() {
  const params = useSearchParams();
  const token = params.get("token") || "";
  const [state, setState] = useState<"verifying" | "verified" | "failed" | "no-token">(
    token ? "verifying" : "no-token",
  );
  const [message, setMessage] = useState("");

  useEffect(() => {
    if (!token) return;
    let cancelled = false;
    (async () => {
      const res = await authFetch("/auth/verify-email", { token });
      if (cancelled) return;
      if (res.ok) setState("verified");
      else {
        setState("failed");
        setMessage(
          res.status === 409 || res.status === 404
            ? "This link has already been used or has expired. You can request a fresh one after signing in."
            : res.message,
        );
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [token]);

  return (
    <AuthCard
      eyebrow="Welcome"
      title="Verify your email"
      footer={
        <Link href="/" style={{ color: "var(--ic-color-brand-300)" }}>
          Back to iCONFESS
        </Link>
      }
    >
      {state === "verifying" && (
        <div role="status" style={{ display: "grid", gap: "var(--ic-spacing-4)", justifyItems: "center" }}>
          <div className="ic-skeleton" style={{ width: "60%", height: 16 }} />
          <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-500)" }}>
            Checking your link...
          </p>
        </div>
      )}
      {state === "verified" && (
        <div role="status" className="ic-state" style={{ borderStyle: "solid", borderColor: "var(--ic-color-brand-300)", padding: "var(--ic-spacing-6)" }}>
          <h3>Email verified.</h3>
          <p>Your account is complete. The practice is ready when you are.</p>
          <Link href="/login" className="ic-btn ic-btn--primary">Sign in</Link>
        </div>
      )}
      {(state === "failed" || state === "no-token") && (
        <div role="alert" className="ic-state" style={{ padding: "var(--ic-spacing-6)" }}>
          <h3>{state === "no-token" ? "This page needs a link from your email." : "We couldn\u2019t verify that link."}</h3>
          <p>{state === "no-token" ? "Open the verification email and follow the button inside." : message}</p>
          <Link href="/login" className="ic-btn ic-btn--secondary">Sign in</Link>
        </div>
      )}
    </AuthCard>
  );
}
