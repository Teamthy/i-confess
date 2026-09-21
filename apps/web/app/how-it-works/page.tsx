import type { Metadata } from "next";
import Link from "next/link";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";
import { SectionHead } from "@/components/Sections";
import { Reveal } from "@/components/Reveal";

export const metadata: Metadata = {
  title: "How it works",
  description:
    "Choose your moment, choose your area of life, listen, repeat. How the iCONFESS practice becomes a ritual.",
  alternates: { canonical: "/how-it-works" },
};

const STEPS = [
  {
    n: "01",
    t: "Choose your moment",
    b: "Morning before the noise. Midday between everything. Night when the day needs releasing. The practice is designed for the minutes you actually have — five of them count.",
  },
  {
    n: "02",
    t: "Choose your area of life",
    b: "Thirty-nine areas, from peace and healing to purpose and provision. You always know where to turn, because the library is ordered, not endless.",
  },
  {
    n: "03",
    t: "Listen",
    b: "A session assembles confessions with the Scripture they stand on, spoken in a curated voice. Every word is reviewed before it is ever published.",
  },
  {
    n: "04",
    t: "Repeat",
    b: "Say the words with the voice. Return to them tomorrow. Favourites keep what matters close; history shows how far the practice has come.",
  },
  {
    n: "05",
    t: "Build your ritual",
    b: "Schedule sessions for the times you want to keep. The streak is not the point — the return is.",
  },
];

export default function HowItWorksPage() {
  return (
    <div>
      <SiteHeader light />
      <main id="main">
        <section className="ic-page-hero on-ink">
          <div className="ic-container">
            <p className="ic-eyebrow">How it works</p>
            <h1 style={{ marginTop: "var(--ic-spacing-4)" }}>
              A small practice, kept daily.
            </h1>
            <p className="ic-lede" style={{ marginTop: "var(--ic-spacing-5)" }}>
              iCONFESS is not a feed to refresh. It is five quiet steps that
              turn spoken words into a ritual you keep.
            </p>
          </div>
        </section>

        <section className="ic-section">
          <div className="ic-container">
            {STEPS.map((s, i) => (
              <Reveal key={s.n} delay={60}>
                <div className="ic-step" style={i === 0 ? { borderTop: "none" } : undefined}>
                  <span className="ic-step__num" aria-hidden="true">{s.n}</span>
                  <div>
                    <h3 style={{ fontSize: "var(--ic-font-size-heading)", fontFamily: "var(--ic-font-family-serif)", fontWeight: "var(--ic-font-weight-regular)" }}>{s.t}</h3>
                    <p style={{ maxWidth: "56ch", marginTop: "var(--ic-spacing-2)" }}>{s.b}</p>
                  </div>
                </div>
              </Reveal>
            ))}
          </div>
        </section>

        <section className="ic-section ic-section--ink on-ink" style={{ textAlign: "center" }}>
          <div className="ic-container ic-container--text">
            <SectionHead
              eyebrow="Begin"
              title="The first session is the hardest. It is also five minutes."
            />
            <div className="ic-btn-row" style={{ justifyContent: "center" }}>
              <Link href="/register" className="ic-btn ic-btn--on-dark">Begin your experience</Link>
              <Link href="/categories" className="ic-btn ic-btn--secondary on-ink">Explore categories</Link>
            </div>
          </div>
        </section>
      </main>
      <SiteFooter />
    </div>
  );
}
