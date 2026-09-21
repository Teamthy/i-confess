import type { Metadata } from "next";
import Link from "next/link";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";

export const metadata: Metadata = {
  title: "About",
  description:
    "Why iCONFESS exists: words worth repeating, a practice worth keeping, and a library you can trust.",
  alternates: { canonical: "/about" },
};

export default function AboutPage() {
  return (
    <div>
      <SiteHeader light />
      <main id="main">
        <section className="ic-page-hero on-ink">
          <div className="ic-container">
            <p className="ic-eyebrow">About</p>
            <h1 style={{ marginTop: "var(--ic-spacing-4)" }}>
              Words are not decoration. They are direction.
            </h1>
            <p className="ic-lede" style={{ marginTop: "var(--ic-spacing-5)" }}>
              iCONFESS exists because most of what we read disappears. Spoken,
              repeated, and returned to, words begin to shape how we live.
            </p>
          </div>
        </section>

        <section className="ic-section">
          <div className="ic-container ic-container--text ic-prose">
            <h2>Mission</h2>
            <p>
              To make spoken confession an intentional daily experience — a
              quiet, modern practice for people who want the words they say to
              be words they live.
            </p>

            <h2>What we believe</h2>
            <p>
              Words are not decoration. They are direction. What you repeat,
              you remember; what you remember, you become. That belief asks
              something of us: the words we publish must deserve repetition.
            </p>

            <h2>Why the product exists</h2>
            <p>
              Reading is fleeting. Feeds are endless. The iCONFESS library is
              ordered — 39 areas of life, each with confessions that stand on
              Scripture, each reviewed before publication. The product is small
              on purpose: choose, listen, repeat. The restraint is the feature.
            </p>

            <h2>Product philosophy</h2>
            <ul>
              <li><strong>Reviewed, never improvised.</strong> Every confession passes editorial and theological review before it is public.</li>
              <li><strong>Private by default.</strong> What you write is yours. Publication only happens through explicit review and consent.</li>
              <li><strong>Calm by design.</strong> No streak anxiety, no noise. The practice adapts to your day, not the reverse.</li>
              <li><strong>Honest by construction.</strong> Entitlements, reviews, and moderation live on the server — the app shows you the truth or says nothing.</li>
            </ul>

            <h2>The library</h2>
            <p>
              The canonical library is written and reviewed by the iCONFESS
              editorial team, with every confession tied to Scripture references
              it genuinely stands on — quotations where the words quote, allusion
              where they allude. Nothing is attributed to a person or church that
              did not write it.
            </p>
          </div>
        </section>

        <section className="ic-section ic-section--ink on-ink" style={{ textAlign: "center" }}>
          <div className="ic-container ic-container--text">
            <h2 className="ic-display" style={{ marginInline: "auto" }}>Begin with one confession.</h2>
            <div className="ic-btn-row" style={{ justifyContent: "center", marginTop: "var(--ic-spacing-7)" }}>
              <Link href="/register" className="ic-btn ic-btn--on-dark">Begin your experience</Link>
              <Link href="/contact" className="ic-btn ic-btn--secondary on-ink">Contact us</Link>
            </div>
          </div>
        </section>
      </main>
      <SiteFooter />
    </div>
  );
}
