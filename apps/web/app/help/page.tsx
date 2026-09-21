/** Help centre — getting started guides. */

import type { Metadata } from "next";
import Link from "next/link";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";

export const metadata: Metadata = {
  title: "Help — iCONFESS",
  description: "Guides and how-tos for using iCONFESS.",
};

export default function HelpPage() {
  return (
    <div>
      <SiteHeader />
      <main id="main">
        <section className="ic-page-hero">
          <div className="ic-container">
            <h1>Help centre</h1>
            <p className="ic-lede">Guides to help you get the most from iCONFESS.</p>
          </div>
        </section>
        <section className="ic-section">
          <div className="ic-container">
            <div className="ic-grid ic-grid--3">
              {[
                { title: "Getting started", desc: "Create an account, explore categories, and start your first session." },
                { title: "Listening", desc: "How audio works, choosing voices, and adjusting playback settings." },
                { title: "Creating confessions", desc: "Write, save, and share your own confessions." },
                { title: "Premium", desc: "What Premium includes, how to subscribe, and how to cancel." },
                { title: "Account", desc: "Profile, password, privacy settings, and data export." },
                { title: "Community", desc: "How community confessions work and how moderation keeps things safe." },
              ].map((g) => (
                <article key={g.title} className="ic-card" style={{ padding: "var(--ic-spacing-6)" }}>
                  <h3 style={{ fontSize: "var(--ic-font-size-subheading)", fontWeight: "var(--ic-font-weight-semibold)" }}>{g.title}</h3>
                  <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", marginTop: "var(--ic-spacing-2)" }}>{g.desc}</p>
                </article>
              ))}
            </div>
            <div style={{ marginTop: "var(--ic-spacing-8)", textAlign: "center" }}>
              <p style={{ color: "var(--ic-color-neutral-500)" }}>Still need help?</p>
              <Link href="/contact" className="ic-btn ic-btn--secondary" style={{ marginTop: "var(--ic-spacing-3)" }}>Contact us</Link>
            </div>
          </div>
        </section>
      </main>
      <SiteFooter />
    </div>
  );
}
