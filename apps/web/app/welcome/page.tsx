/** Welcome — post-registration onboarding. */

import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = {
  title: "Welcome — iCONFESS",
  robots: { index: false, follow: false },
};

export default function WelcomePage() {
  return (
    <div style={{ minHeight: "100vh", display: "grid", placeItems: "center", background: "var(--ic-color-neutral-950)", color: "var(--ic-color-neutral-0)", padding: "var(--ic-spacing-6)" }}>
      <div style={{ textAlign: "center", maxWidth: "32rem" }}>
        <p className="ic-eyebrow" style={{ justifyContent: "center", color: "var(--ic-color-brand-300)" }}>Welcome</p>
        <h1 style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "clamp(1.75rem, 1rem + 2vw, 2.5rem)", marginTop: "var(--ic-spacing-4)", fontWeight: "var(--ic-font-weight-regular)", lineHeight: "var(--ic-font-lineHeight-tight)" }}>
          Your practice begins now.
        </h1>
        <p style={{ marginTop: "var(--ic-spacing-5)", color: "var(--ic-color-neutral-300)", lineHeight: "var(--ic-font-lineHeight-relaxed)" }}>
          You have joined iCONFESS. Start by exploring categories and hearing your first confession.
        </p>
        <div style={{ display: "flex", gap: "var(--ic-spacing-3)", justifyContent: "center", marginTop: "var(--ic-spacing-7)", flexWrap: "wrap" }}>
          <Link href="/app" className="ic-btn ic-btn--on-dark">Go to your experience</Link>
          <Link href="/app/explore" className="ic-btn ic-btn--secondary on-ink">Explore first</Link>
        </div>
      </div>
    </div>
  );
}
