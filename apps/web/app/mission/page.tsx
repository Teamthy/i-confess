/** Mission — why iCONFESS exists. */

import type { Metadata } from "next";
import Link from "next/link";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";

export const metadata: Metadata = {
  title: "Our Mission — iCONFESS",
  description: "Why iCONFESS exists: to turn spoken confession into a daily practice.",
};

export default function MissionPage() {
  return (
    <div>
      <SiteHeader />
      <main id="main">
        <section className="ic-page-hero">
          <div className="ic-container">
            <h1>Our mission</h1>
            <p className="ic-lede">To make spoken confession and reflection a daily practice — accessible, intentional, and personal.</p>
          </div>
        </section>
        <section className="ic-section">
          <div className="ic-container ic-container--text">
            <div className="ic-prose">
              <h2>Words are more powerful when you return to them.</h2>
              <p>iCONFESS was built on a simple observation: the words you speak over your life shape how you live. Not because they are magic, but because repetition is how belief settles in.</p>
              <p>Most people have heard words of peace, purpose, and faith. Few have a practice of returning to them. iCONFESS gives you that practice — in a voice, at a pace, in a moment you choose.</p>
              <h2>Not scrolling. Not skimming.</h2>
              <p>The product is deliberately small. A session is a few minutes. A confession is a few sentences. The discipline is in the return — tomorrow, and the day after.</p>
              <h2>Private by default.</h2>
              <p>Your confessions are yours. Nothing is shared without your explicit choice, and nothing is published without human review.</p>
            </div>
            <div style={{ marginTop: "var(--ic-spacing-8)" }}>
              <Link href="/register" className="ic-btn ic-btn--primary">Start your experience</Link>
            </div>
          </div>
        </section>
      </main>
      <SiteFooter />
    </div>
  );
}
