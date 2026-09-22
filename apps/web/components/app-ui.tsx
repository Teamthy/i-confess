"use client";

/**
 * Small shared building blocks for the wired /app surfaces.
 *
 * These are deliberately minimal and reuse the classes in styles/site.css:
 * loading (skeleton), error (message + retry), empty (the product has nothing
 * for you yet), and signed-out (the honest pre-auth state — not an error).
 * Every data-driven page composes the four; none of them renders a spinner as
 * a final state (§52).
 */

import type { ReactNode } from "react";
import { useRouter } from "next/navigation";

export function LoadingBlock({ rows = 3 }: { rows?: number }) {
  return (
    <div aria-busy="true" aria-live="polite" aria-label="Loading">
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} className="ic-skeleton" style={{ marginBottom: "var(--ic-spacing-3)" }} />
      ))}
    </div>
  );
}

export function ErrorBlock({
  message,
  onRetry,
}: {
  message: string;
  onRetry?: () => void;
}) {
  return (
    <div className="ic-state" role="alert">
      <h3>That didn't load</h3>
      <p>{message}</p>
      {onRetry && (
        <button type="button" className="ic-btn ic-btn--secondary" onClick={onRetry}>
          Try again
        </button>
      )}
    </div>
  );
}

export function EmptyState({
  title,
  body,
  ctaHref,
  ctaLabel,
}: {
  title: string;
  body: string;
  ctaHref?: string;
  ctaLabel?: string;
}) {
  const router = useRouter();
  return (
    <div className="ic-state">
      <h3>{title}</h3>
      <p>{body}</p>
      {ctaHref && ctaLabel && (
        <button type="button" className="ic-btn ic-btn--secondary" onClick={() => router.push(ctaHref)}>
          {ctaLabel}
        </button>
      )}
    </div>
  );
}

export function SignedOut({ next }: { next?: string }) {
  const router = useRouter();
  const target = encodeURIComponent(next ?? "/app");
  return (
    <div className="ic-state">
      <h3>Sign in to use this</h3>
      <p>Your profile, preferences, sessions and saves live in your account. Nothing is lost by waiting — this page will be here.</p>
      <div className="ic-btn-row">
        <button type="button" className="ic-btn ic-btn--primary" onClick={() => router.push(`/login?next=${target}`)}>
          Sign in
        </button>
        <button type="button" className="ic-btn ic-btn--text" onClick={() => router.push(`/register?next=${target}`)}>
          Create an account
        </button>
      </div>
    </div>
  );
}

/** Inline note shown under a field group after a successful save. */
export function SavedNote({ children }: { children: ReactNode }) {
  return (
    <p
      role="status"
      style={{
        fontSize: "var(--ic-font-size-bodySm)",
        color: "var(--ic-color-brand-700, #081D61)",
        margin: 0,
      }}
    >
      {children}
    </p>
  );
}
