import type { Metadata } from "next";
import Link from "next/link";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";
import { SectionHead } from "@/components/Sections";
import { api } from "@/lib/api";

export const metadata: Metadata = {
  title: "Community",
  description:
    "Write confessions of your own — private by default, reviewed before public. The iCONFESS community model.",
  alternates: { canonical: "/community" },
};

export default async function CommunityPage() {
  const res = await api.communityConfessions();
  const published = res.ok ? res.data.confessions : [];
  const recent = published.slice(0, 6) as {
    id: string;
    title: string;
    text?: string;
    published_at?: string;
  }[];

  return (
    <div>
      <SiteHeader />
      <main id="main">
        <section className="ic-page-hero on-ink">
          <div className="ic-container">
            <p className="ic-eyebrow">Community</p>
            <h1 style={{ marginTop: "var(--ic-spacing-4)" }}>Your words matter too.</h1>
            <p className="ic-lede" style={{ marginTop: "var(--ic-spacing-5)" }}>
              Every account can write confessions in its own words — keep them
              private, share them deliberately, or offer them to the community.
              Reviewed before publication, always.
            </p>
            <div className="ic-btn-row" style={{ marginTop: "var(--ic-spacing-6)" }}>
              <Link href="/register" className="ic-btn ic-btn--on-dark">Create your confession</Link>
            </div>
          </div>
        </section>

        <section className="ic-section">
          <div className="ic-container">
            <SectionHead
              eyebrow="How sharing works"
              title="Privacy is the default, not the upgrade."
              lede="Three states, and you choose. Nothing becomes public without a person reading it first."
            />
            <div className="ic-grid ic-grid--3">
              {[
                ["Private", "Only you", "A confession you write is private the moment it exists. No feed, no exposure, no exceptions."],
                ["Shared", "People you choose", "Shared content is visible only to the people you explicitly include. You can change your mind later."],
                ["Community", "Reviewed, then public", "Offer a confession for review. A person reads it before anything is published — and you always know its state."],
              ].map(([t, k, b]) => (
                <div key={t} className="ic-card ic-card--hover" style={{ padding: "var(--ic-spacing-6)", display: "grid", gap: "var(--ic-spacing-3)" }}>
                  <span className="ic-frame__kicker" style={{ color: "var(--ic-color-brand-600)" }}>{k}</span>
                  <h3 style={{ fontSize: "var(--ic-font-size-subheading)", fontWeight: "var(--ic-font-weight-semibold)" }}>{t}</h3>
                  <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", lineHeight: "var(--ic-font-lineHeight-relaxed)" }}>{b}</p>
                </div>
              ))}
            </div>
          </div>
        </section>

        <section className="ic-section ic-section--mist" aria-labelledby="stories-title">
          <div className="ic-container">
            <SectionHead
              id="stories-title"
              eyebrow="From the community"
              title="Testimonies, offered deliberately."
              lede="Published community confessions — each one reviewed by a person before it appeared here."
            />
            {res.ok && recent.length > 0 ? (
              <div className="ic-grid ic-grid--3">
                {recent.map((t) => (
                  <article key={t.id} className="ic-card ic-confession-card">
                    <span className="ic-confession-card__kicker">Community</span>
                    <h3>{t.title}</h3>
                    <p>{t.text}</p>
                  </article>
                ))}
              </div>
            ) : (
              <div className="ic-state">
                <h3>No published testimonies yet</h3>
                <p>The community is young. Yours could be among the first — written, reviewed, and published with your name on the choice, not ours.</p>
                <Link href="/register" className="ic-btn ic-btn--secondary">Create your confession</Link>
              </div>
            )}
          </div>
        </section>

        <section className="ic-section">
          <div className="ic-container">
            <SectionHead
              eyebrow="Safety"
              title="Moderation is a duty, not a feature."
              lede="Reports are read. Decisions are recorded. Rejected content is explained, and appeals exist. Read the guidelines to see exactly where the lines are."
            />
            <Link href="/community-guidelines" className="ic-btn ic-btn--secondary">
              Read the community guidelines
            </Link>
          </div>
        </section>
      </main>
      <SiteFooter />
    </div>
  );
}
