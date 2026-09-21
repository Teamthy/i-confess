/** Maintenance — scheduled downtime. */

import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Under maintenance — iCONFESS",
  robots: { index: false, follow: false },
};

export default function MaintenancePage() {
  return (
    <div style={{ minHeight: "100vh", display: "grid", placeItems: "center", background: "var(--ic-color-neutral-950)", color: "var(--ic-color-neutral-0)", padding: "var(--ic-spacing-6)" }}>
      <div style={{ textAlign: "center", maxWidth: "28rem" }}>
        <p style={{ fontFamily: "var(--ic-font-family-sans)", fontWeight: "var(--ic-font-weight-semibold)", fontSize: "var(--ic-font-size-bodySm)", letterSpacing: "0.14em", textTransform: "uppercase", color: "var(--ic-color-brand-300)" }}>iCONFESS</p>
        <h1 style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "clamp(1.5rem, 1rem + 1.5vw, 2rem)", marginTop: "var(--ic-spacing-5)", fontWeight: "var(--ic-font-weight-regular)" }}>
          We are performing scheduled maintenance.
        </h1>
        <p style={{ marginTop: "var(--ic-spacing-4)", color: "var(--ic-color-neutral-400)", lineHeight: "var(--ic-font-lineHeight-relaxed)" }}>
          iCONFESS will be back shortly. Your confessions and history are safe.
        </p>
      </div>
    </div>
  );
}
