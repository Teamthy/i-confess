/** Billing, renewal, and cancellation. */
import Link from "next/link";
export default function ManageSubscriptionPage() {
  return (
    <div className="ia-page">
      <Link href="/app/subscription" className="ic-backlink" style={{ color: "var(--ic-color-neutral-500)" }}>← Subscription</Link>
      <div className="ia-page__head">
        <h1>Manage subscription</h1>
        <p>Billing, renewal, and plan changes.</p>
      </div>
      <div className="ic-state">
        <h3>No active subscription</h3>
        <p>You are on the Free plan. Upgrade to Premium for the full library, longer sessions, and premium voices.</p>
        <Link href="/premium" className="ic-btn ic-btn--primary">Explore Premium</Link>
      </div>
    </div>
  );
}
