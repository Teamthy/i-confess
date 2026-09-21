"use client";

/*
 * Route error boundary. Renders a human-readable recovery surface; the
 * technical detail goes to the console for diagnostics, never to the page
 * (§50 — no stack traces, no internal IDs).
 */

import { useEffect } from "react";
import Link from "next/link";

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    console.error("route error:", error.message);
  }, [error]);

  return (
    <main
      id="main"
      style={{
        minHeight: "80vh",
        display: "grid",
        placeItems: "center",
        background: "var(--ic-color-neutral-950)",
        color: "var(--ic-color-neutral-100)",
        padding: "var(--ic-spacing-8)",
      }}
      className="on-ink"
    >
      <div style={{ textAlign: "center", maxWidth: "36rem" }}>
        <p className="ic-eyebrow" style={{ justifyContent: "center" }}>Something interrupted us</p>
        <h1 className="ic-display" style={{ margin: "var(--ic-spacing-4) auto 0", color: "var(--ic-color-neutral-0)" }}>
          A quiet moment, briefly disturbed.
        </h1>
        <p className="ic-lede" style={{ margin: "var(--ic-spacing-5) auto 0" }}>
          Something went wrong while preparing this experience. It has been
          noted. Please try again.
        </p>
        <div className="ic-btn-row" style={{ justifyContent: "center", marginTop: "var(--ic-spacing-7)" }}>
          <button type="button" className="ic-btn ic-btn--on-dark" onClick={reset}>
            Try again
          </button>
          <Link href="/" className="ic-btn ic-btn--secondary on-ink">
            Return home
          </Link>
        </div>
      </div>
    </main>
  );
}
