import type { Metadata } from "next";
import Link from "next/link";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";
import { SectionHead } from "@/components/Sections";

export const metadata: Metadata = {
  title: "Premium",
  description:
    "Go deeper with iCONFESS Premium — the full library, longer sessions, premium voices, and personalised routines.",
  alternates: { canonical: "/premium" },
};

/**
 * Premium page. All pricing is driven by GET /subscriptions/plans — the same
 * source the mobile paywall uses. If the API is unreachable, the page renders
 * its capability comparison without inventing a price (§86: never fabricate).
 */
export default async function PremiumPage() {
  const res = await fetch(
    `${process.env.IC_API_URL || "http://127.0.0.1:8080"}/subscriptions/plans`,
    { next: { revalidate: 600 } },
  ).catch(() => null);
  const plans = res?.ok
    ? ((await res.json()) as {
        currencies: string[];
        plans: {
          id: string;
          name: string;
          interval: string;
          trial_days: number;
          prices: Record<string, { display: string; currency: string }>;
        }[];
      })
    : null;

  return (
    <div>
      <SiteHeader />
      <main id="main">
        <section className="ic-page-hero on-ink">
          <div className="ic-container">
            <p className="ic-eyebrow">Premium</p>
            <h1 style={{ marginTop: "var(--ic-spacing-4)" }}>Go deeper with iCONFESS.</h1>
            <p className="ic-lede" style={{ marginTop: "var(--ic-spacing-5)" }}>
              The complete library, longer sessions, premium voices, and a
              routine that adjusts to your season.
            </p>
          </div>
        </section>

        <section className="ic-section">
          <div className="ic-container">
            <SectionHead
              eyebrow="What Premium adds"
              title="Everything the practice can be."
            />
            <div className="ic-grid ic-grid--3">
              {[
                ["Expanded library", "All 39 areas of life, including the premium categories."],
                ["Premium voices", "Voices curated for depth — added to the standard set."],
                ["Advanced experiences", "Longer, structured sessions for an hour or a season."],
                ["Personalised routines", "Sessions shaped by your interests and your moments."],
                ["Exclusive content", "Confessions and collections published only for Premium."],
                ["Offline listening", "Saved sessions travel with you in the mobile app."],
              ].map(([t, b]) => (
                <div key={t} className="ic-card ic-card--hover" style={{ padding: "var(--ic-spacing-6)", display: "grid", gap: "var(--ic-spacing-3)" }}>
                  <span className="ic-chip">Premium</span>
                  <h3 style={{ fontSize: "var(--ic-font-size-subheading)", fontWeight: "var(--ic-font-weight-semibold)" }}>{t}</h3>
                  <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", lineHeight: "var(--ic-font-lineHeight-relaxed)" }}>{b}</p>
                </div>
              ))}
            </div>
          </div>
        </section>

        <section className="ic-section ic-section--mist" aria-labelledby="plans-title">
          <div className="ic-container">
            <SectionHead
              id="plans-title"
              eyebrow="Plans"
              title="Simple, transparent pricing."
              lede="Every plan begins with a trial. Prices are set by region and managed centrally — what you see here is what the app charges."
            />
            {plans && plans.plans.length > 0 ? (
              <div className="ic-grid ic-grid--3">
                {plans.plans.map((p) => {
                  const usd = p.prices.USD ?? Object.values(p.prices)[0];
                  return (
                    <div key={p.id} className="ic-card" style={{ padding: "var(--ic-spacing-7)", display: "grid", gap: "var(--ic-spacing-4)" }}>
                      <h3 style={{ fontSize: "var(--ic-font-size-subheading)", fontWeight: "var(--ic-font-weight-semibold)" }}>{p.name}</h3>
                      <p style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-font-size-metric)" }}>
                        {usd?.display}
                        <span style={{ fontSize: "var(--ic-font-size-caption)", color: "var(--ic-color-neutral-500)", fontFamily: "var(--ic-font-family-sans)" }}>
                          {" "}per {p.interval}
                        </span>
                      </p>
                      {p.trial_days > 0 && (
                        <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)" }}>
                          {p.trial_days}-day free trial included.
                        </p>
                      )}
                      <Link href="/register" className="ic-btn ic-btn--primary">Start your experience</Link>
                    </div>
                  );
                })}
                <p style={{ fontSize: "var(--ic-font-size-caption)", color: "var(--ic-color-neutral-500)", gridColumn: "1 / -1" }}>
                  Also priced in {plans.currencies.filter((c) => c !== "USD").join(", ")} — shown in the app at checkout.
                </p>
              </div>
            ) : (
              <div className="ic-state">
                <h3>Plans are being refreshed</h3>
                <p>We couldn't load current pricing. It will be back shortly — the practice, free or Premium, is unchanged.</p>
                <Link href="/register" className="ic-btn ic-btn--secondary">Begin free</Link>
              </div>
            )}
          </div>
        </section>
      </main>
      <SiteFooter />
    </div>
  );
}
