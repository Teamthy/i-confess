/** Verify phone — phone number verification (auth screen). */

import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = {
  title: "Verify phone — iCONFESS",
  robots: { index: false, follow: false },
};

export default function VerifyPhonePage() {
  return (
    <div style={{ minHeight: "100vh", display: "grid", placeItems: "center", background: "var(--ic-color-neutral-950)", color: "var(--ic-color-neutral-0)", padding: "var(--ic-spacing-6)" }}>
      <div style={{ maxWidth: "24rem", width: "100%" }}>
        <p style={{ textAlign: "center", marginBottom: "var(--ic-spacing-6)" }}>
          <Link href="/" style={{ fontFamily: "var(--ic-font-family-sans)", fontWeight: "var(--ic-font-weight-semibold)", fontSize: "var(--ic-font-size-bodySm)", letterSpacing: "0.14em", textTransform: "uppercase", color: "var(--ic-color-neutral-0)", textDecoration: "none" }}>iCONFESS</Link>
        </p>
        <div style={{ background: "var(--ic-color-neutral-0)", color: "var(--ic-color-neutral-950)", borderRadius: "var(--ic-radius-lg)", padding: "var(--ic-spacing-7)" }}>
          <h1 style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-font-size-heading)", fontWeight: "var(--ic-font-weight-regular)", textAlign: "center" }}>Verify your phone</h1>
          <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", textAlign: "center", marginTop: "var(--ic-spacing-3)" }}>
            Enter the code sent to your phone number.
          </p>
          <div style={{ marginTop: "var(--ic-spacing-6)" }}>
            <div className="ic-field">
              <label htmlFor="phone-code">Verification code</label>
              <input id="phone-code" type="text" inputMode="numeric" maxLength={6} autoComplete="one-time-code" placeholder="000000" style={{ textAlign: "center", fontSize: "var(--ic-font-size-heading)", letterSpacing: "0.3em" }} />
            </div>
            <button type="button" className="ic-btn ic-btn--primary" style={{ width: "100%", marginTop: "var(--ic-spacing-4)" }}>Verify</button>
          </div>
        </div>
      </div>
    </div>
  );
}
