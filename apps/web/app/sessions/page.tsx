import Link from "next/link";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";
import { SectionHead } from "@/components/Sections";

export const metadata = {
  title: "Session experience",
  description:
    "An iCONFESS session assembles confessions and Scripture around the time you have — guided, paced, and repeatable.",
  alternates: { canonical: "/sessions" },
};

const STEPS = [
  {
    n: "Before",
    t: "A session is prepared, not found",
    b: "Choose an area of life and a length — five minutes or an hour. iCONFESS assembles confessions and the Scripture they stand on, and assigns a voice. Nothing is improvised; the words are reviewed before they ever reach a session.",
  },
  {
    n: "During",
    t: "You are guided, not timed",
    b: "Each item is spoken, then rests. Progress is visible, pauses are gentle, and you can skip, replay, or end at any moment — the session adapts without judgement.",
  },
  {
    n: "After",
    t: "Completion is the ritual",
    b: "Finishing a session is recorded in your history, feeds your routine, and quietly shapes what is suggested next. The point is not a streak; it is a return.",
  },
];

export default function SessionsPage() {
  return (
    <div>
      <SiteHeader light />
      <main id="main">
        <section className="ic-page-hero on-ink">
          <div className="ic-container">
            <p className="ic-eyebrow">Sessions</p>
            <h1 style={{ marginTop: "var(--ic-spacing-4)" }}>
              One session, built around your day.
            </h1>
            <p className="ic-lede" style={{ marginTop: "var(--ic-spacing-5)" }}>
              A guided sequence of spoken confessions and Scripture, paced to
              the time you actually have.
            </p>
            <div className="ic-btn-row" style={{ marginTop: "var(--ic-spacing-6)" }}>
              <Link href="/register" className="ic-btn ic-btn--on-dark">
                Build your first session
              </Link>
            </div>
          </div>
        </section>

        <section className="ic-section">
          <div className="ic-container">
            <SectionHead
              eyebrow="The shape of a session"
              title="Prepared, guided, completed."
              lede="Sessions are the heart of the practice. This is what one is like."
            />
            <div>
              {STEPS.map((s) => (
                <div className="ic-step" key={s.n}>
                  <span className="ic-step__num" aria-hidden="true">{s.n}</span>
                  <div>
                    <h3>{s.t}</h3>
                    <p>{s.b}</p>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </section>

        <section className="ic-section ic-section--ink on-ink" style={{ textAlign: "center" }}>
          <div className="ic-container ic-container--text">
            <h2 className="ic-display" style={{ marginInline: "auto" }}>
              Five minutes is enough to begin.
            </h2>
            <div className="ic-btn-row" style={{ justifyContent: "center", marginTop: "var(--ic-spacing-7)" }}>
              <Link href="/register" className="ic-btn ic-btn--on-dark">Begin your experience</Link>
            </div>
          </div>
        </section>
      </main>
      <SiteFooter />
    </div>
  );
}
