import type { Metadata } from "next";
import Link from "next/link";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";

export const metadata: Metadata = {
  title: "Download",
  description: "Take iCONFESS with you — the practice in your pocket, with offline listening and scheduled reminders.",
  alternates: { canonical: "/download" },
};

/**
 * Download page.
 *
 * INTEGRATION BOUNDARY (audit §5): the app is in development; store listings
 * do not exist yet. No fabricated store badges, ratings, or review counts —
 * the page registers interest honestly and states exactly what ships with the
 * app. The badge slots render as intentional placeholders until Apple/Google
 * listing URLs are configured in the environment.
 */
const APP_STORE_URL = process.env.NEXT_PUBLIC_APP_STORE_URL || "";
const PLAY_STORE_URL = process.env.NEXT_PUBLIC_PLAY_STORE_URL || "";

export default function DownloadPage() {
  return (
    <div>
      <SiteHeader light />
      <main id="main">
        <section className="ic-page-hero on-ink">
          <div className="ic-container ic-section-head--split ic-section-head" style={{ alignItems: "center" }}>
            <div>
              <p className="ic-eyebrow">Download</p>
              <h1 style={{ marginTop: "var(--ic-spacing-4)" }}>Take iCONFESS with you.</h1>
              <p className="ic-lede" style={{ marginTop: "var(--ic-spacing-5)" }}>
                Offline listening, scheduled reminders, and your whole practice
                in your pocket.
              </p>
              <div className="ic-btn-row" style={{ marginTop: "var(--ic-spacing-6)" }}>
                {APP_STORE_URL ? (
                  <a href={APP_STORE_URL} className="ic-btn ic-btn--on-dark">Download on the App Store</a>
                ) : (
                  <span className="ic-btn ic-btn--secondary on-ink" aria-disabled="true" style={{ opacity: 0.7 }}>App Store — coming soon</span>
                )}
                {PLAY_STORE_URL ? (
                  <a href={PLAY_STORE_URL} className="ic-btn ic-btn--on-dark">Get it on Google Play</a>
                ) : (
                  <span className="ic-btn ic-btn--secondary on-ink" aria-disabled="true" style={{ opacity: 0.7 }}>Google Play — coming soon</span>
                )}
              </div>
            </div>
            <div className="ic-frame" style={{ maxWidth: "20rem", marginLeft: "auto" }}>
              <div className="ic-frame__body" style={{ textAlign: "center", gap: "var(--ic-spacing-4)" }}>
                <span className="ic-frame__kicker">Your practice, offline</span>
                <p style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-font-size-subheading)", lineHeight: 1.4 }}>
                  Morning light, evening quiet.
                </p>
                <div className="ic-player-line" aria-hidden="true"><i /><b /></div>
              </div>
            </div>
          </div>
        </section>

        <section className="ic-section">
          <div className="ic-container">
            <h2 className="ic-title" style={{ marginBottom: "var(--ic-spacing-7)" }}>
              What the app adds.
            </h2>
            <div className="ic-grid ic-grid--3">
              {[
                ["Offline sessions", "Premium sessions download for listening without a signal — flights, mornings, the commute."],
                ["Scheduled reminders", "A gentle nudge at the times you chose. Not more notifications — the right ones."],
                ["Everything syncs", "Favourites, history, and schedules follow you between the app and the web."],
              ].map(([t, b]) => (
                <div key={t} className="ic-card ic-card--hover" style={{ padding: "var(--ic-spacing-6)", display: "grid", gap: "var(--ic-spacing-3)" }}>
                  <h3 style={{ fontSize: "var(--ic-font-size-subheading)", fontWeight: "var(--ic-font-weight-semibold)" }}>{t}</h3>
                  <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", lineHeight: "var(--ic-font-lineHeight-relaxed)" }}>{b}</p>
                </div>
              ))}
            </div>
            <div className="ic-state" style={{ marginTop: "var(--ic-spacing-8)" }}>
              <h3>While the app finishes its final testing</h3>
              <p>The full experience is available on the web today — sign in and begin.</p>
              <Link href="/register" className="ic-btn ic-btn--primary">Begin on the web</Link>
            </div>
          </div>
        </section>
      </main>
      <SiteFooter />
    </div>
  );
}
