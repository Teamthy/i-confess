import type { Metadata } from "next";
import Link from "next/link";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";

export const metadata: Metadata = {
  title: "Pricing",
  description:
    "iCONFESS plans and regional pricing — driven live from the same source the app uses. Free core, Premium for the full practice.",
  alternates: { canonical: "/pricing" },
};

type PlanWire = {
  id: string;
  name: string;
  interval: string;
  trial_days: number;
  prices: Record<string, { display: string; currency: string; minor: number }>;
};

/**
 * Live pricing. The plans table is admin-editable in the backend
 * (GET /subscriptions/plans); nothing here is hardcoded (§16 of the brief).
 */
async function getPlans(): Promise<{
  currencies: string[];
  plans: PlanWire[];
} | null> {
  try {
    const res = await fetch(
      `${process.env.IC_API_URL || "http://127.0.0.1:8080"}/subscriptions/plans`,
      { next: { revalidate: 600 } },
    );
    if (!res.ok) return null;
    return (await res.json()) as { currencies: string[]; plans: PlanWire[] };
  } catch {
    return null;
  }
}

const FREE = [
  "Core confessions across the library",
  "Standard session lengths",
  "Voices in the standard set",
  "Personal confessions, always private",
  "History and favourites",
];

const PREMIUM = [
  "All 39 areas of life, including premium categories",
  "Extended session lengths",
  "Premium voices",
  "Personalised routines and advanced experiences",
  "Offline listening in the mobile app",
];

export default async function PricingPage() {
  const data = await getPlans();

  return (
    <div>
      <SiteHeader light />
      <main id="main">
        <section className="ic-page-hero on-ink">
          <div className="ic-container">
            <p className="ic-eyebrow">Pricing</p>
            <h1 style={{ marginTop: "var(--ic-spacing-4)" }}>Begin free. Go deeper when it earns it.</h1>
            <p className="ic-lede" style={{ marginTop: "var(--ic-spacing-5)" }}>
              The core practice is free, always. Premium exists for the people
              who want the whole library and the longest sessions — and its
              prices are set regionally, not converted at the border.
            </p>
          </div>
        </section>

        <section className="ic-section">
          <div className="ic-container">
            <div className="ic-grid" style={{ alignItems: "stretch" }}>
              {/* Free tier */}
              <div className="ic-card" style={{ padding: "var(--ic-spacing-7)", display: "grid", gap: "var(--ic-spacing-5)", alignContent: "start" }}>
                <p className="ic-frame__kicker" style={{ color: "var(--ic-color-brand-600)" }}>Free</p>
                <h2 style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-web-title)" }}>
                  The practice, whole enough to keep.
                </h2>
                <ul role="list" style={{ display: "grid", gap: "var(--ic-spacing-3)" }}>
                  {FREE.map((f) => (
                    <li key={f} style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-700)", display: "flex", gap: "var(--ic-spacing-3)" }}>
                      <span aria-hidden="true" style={{ color: "var(--ic-color-brand-600)" }}>✓</span> {f}
                    </li>
                  ))}
                </ul>
                <Link href="/register" className="ic-btn ic-btn--secondary" style={{ width: "fit-content" }}>
                  Begin free
                </Link>
              </div>

              {/* Plans from the API */}
              <div className="ic-card" style={{ padding: "var(--ic-spacing-7)", display: "grid", gap: "var(--ic-spacing-5)", alignContent: "start", borderColor: "var(--ic-color-brand-600)", borderWidth: 2 }}>
                <p className="ic-frame__kicker" style={{ color: "var(--ic-color-brand-600)" }}>Premium</p>
                <h2 style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-web-title)" }}>
                  Everything above, and then some.
                </h2>
                {data && data.plans.length > 0 ? (
                  <div style={{ display: "grid", gap: "var(--ic-spacing-4)" }}>
                    {data.plans.map((p) => {
                      const usd = p.prices.USD ?? Object.values(p.prices)[0];
                      return (
                        <div key={p.id} style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline", gap: "var(--ic-spacing-4)", borderBottom: "1px solid var(--ic-color-neutral-200)", paddingBottom: "var(--ic-spacing-4)" }}>
                          <div>
                            <h3 style={{ fontSize: "var(--ic-font-size-body)", fontWeight: "var(--ic-font-weight-semibold)" }}>{p.name}</h3>
                            {p.trial_days > 0 && (
                              <p style={{ fontSize: "var(--ic-font-size-caption)", color: "var(--ic-color-neutral-500)", marginTop: 2 }}>
                                {p.trial_days}-day free trial
                              </p>
                            )}
                          </div>
                          <p style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-font-size-heading)", whiteSpace: "nowrap" }}>
                            {usd?.display}
                            <span style={{ fontFamily: "var(--ic-font-family-sans)", fontSize: "var(--ic-font-size-caption)", color: "var(--ic-color-neutral-500)" }}> / {p.interval}</span>
                          </p>
                        </div>
                      );
                    })}
                    <p style={{ fontSize: "var(--ic-font-size-caption)", color: "var(--ic-color-neutral-500)" }}>
                      Regional pricing in {data.currencies.join(", ")} — shown in the app before you confirm.
                    </p>
                  </div>
                ) : (
                  <div className="ic-state" style={{ padding: "var(--ic-spacing-6)" }}>
                    <p>We couldn't load current pricing. Please check back shortly.</p>
                  </div>
                )}
                <ul role="list" style={{ display: "grid", gap: "var(--ic-spacing-3)" }}>
                  {PREMIUM.map((f) => (
                    <li key={f} style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-700)", display: "flex", gap: "var(--ic-spacing-3)" }}>
                      <span aria-hidden="true" style={{ color: "var(--ic-color-accent-gold)" }}>★</span> {f}
                    </li>
                  ))}
                </ul>
                <Link href="/register" className="ic-btn ic-btn--primary" style={{ width: "fit-content" }}>
                  Start your experience
                </Link>
              </div>
            </div>
          </div>
        </section>
      </main>
      <SiteFooter />
    </div>
  );
}
