import type { Metadata } from "next";
import Link from "next/link";
import { Icon } from "@/components/ui";

/* /pricing — the live plan table.
 *
 * Carried over from the app being retired into this one, because the prices
 * are admin-editable in the backend and must never be hardcoded in a client.
 * This is a server component: it reads GET /subscriptions/plans over the
 * loopback, so the API origin never reaches the browser. When the API cannot
 * be reached the page says so and points at the plan description rather than
 * inventing a price. */

export const metadata: Metadata = {
  title: "Pricing",
  description:
    "iCONFESS plans and regional pricing, served live from the same source the apps use. Free to start; Premium for the full practice.",
  alternates: { canonical: "/pricing" },
};

type PlanWire = {
  id: string;
  name: string;
  interval: string;
  trial_days: number;
  prices: Record<string, { display: string; currency: string; minor: number }>;
};

async function getPlans(): Promise<{ currencies: string[]; plans: PlanWire[] } | null> {
  try {
    const res = await fetch(`${process.env.IC_API_URL || "http://127.0.0.1:8080"}/subscriptions/plans`, {
      next: { revalidate: 600 },
    });
    if (!res.ok) return null;
    return (await res.json()) as { currencies: string[]; plans: PlanWire[] };
  } catch {
    return null;
  }
}

export default async function PricingPage() {
  const data = await getPlans();
  const plans = data?.plans || [];
  const currencies = data?.currencies || [];

  return (
    <section className="container bible-page">
      <header className="bible-page-head">
        <span className="scripture-eyebrow">Pricing</span>
        <h1 className="h1">Free to start. Premium when it becomes daily.</h1>
        <p className="lede">
          Prices come from the platform's own plan table, not from this page — the same source the apps read, in
          every currency it is published in.
        </p>
      </header>

      {!data && (
        <div className="bible-note">
          The plan table is served by the iCONFESS API and is not reachable from this deployment, so no price is
          shown here. Nothing is substituted for it — see <Link className="textlink" href="/premium">what Premium
          includes</Link>.
        </div>
      )}

      {plans.length > 0 && (
        <>
          <div className="bible-plans">
            {plans.map((plan) => {
              const price = plan.prices?.USD || Object.values(plan.prices || {})[0];
              return (
                <article className="bplan" key={plan.id}>
                  <h3>{plan.name}</h3>
                  <div className="bplan-meta">
                    {price ? (
                      <>
                        <b style={{ fontSize: 26 }}>{price.display}</b> / {plan.interval}
                      </>
                    ) : (
                      "Price not published"
                    )}
                  </div>
                  {plan.trial_days > 0 && <p className="small">{plan.trial_days}-day trial, cancel any time.</p>}
                  <div className="btrans-actions">
                    <Link className="btn btn-primary btn-sm" href="/register">
                      Get started <Icon n="arrow" s={12} />
                    </Link>
                  </div>
                </article>
              );
            })}
          </div>
          {currencies.length > 0 && (
            <p className="small" style={{ marginTop: 18 }}>
              Published in {currencies.join(", ")}. Your region's currency is chosen at checkout.
            </p>
          )}
        </>
      )}
    </section>
  );
}
