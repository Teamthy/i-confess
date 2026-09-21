/** Offline — no network connection. */

import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Offline — iCONFESS",
  robots: { index: false, follow: false },
};

export default function OfflinePage() {
  return (
    <div style={{ minHeight: "100vh", display: "grid", placeItems: "center", background: "var(--ic-color-neutral-950)", color: "var(--ic-color-neutral-0)", padding: "var(--ic-spacing-6)" }}>
      <div style={{ textAlign: "center", maxWidth: "28rem" }}>
        <h1 style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "clamp(1.5rem, 1rem + 1.5vw, 2rem)", fontWeight: "var(--ic-font-weight-regular)" }}>
          You appear to be offline.
        </h1>
        <p style={{ marginTop: "var(--ic-spacing-4)", color: "var(--ic-color-neutral-400)", lineHeight: "var(--ic-font-lineHeight-relaxed)" }}>
          Check your internet connection and try again. Your saved confessions are still available in the mobile app for offline listening.
        </p>
        <a href="/offline" className="ic-btn ic-btn--on-dark" style={{ marginTop: "var(--ic-spacing-7)" }}>
          Try again
        </a>
      </div>
    </div>
  );
}
