/** Current plan and billing. */
import Link from "next/link";
export default function SubscriptionPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Subscription</h1>
        <p>Your current plan and benefits.</p>
      </div>
      <div className="ia-settings">
        <div className="ia-settings__group">
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
            <h2>Free plan</h2>
            <span className="ia-badge ia-badge--free">Free</span>
          </div>
          <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)" }}>
            Access to core categories and standard session lengths.
          </p>
          <Link href="/premium" className="ic-btn ic-btn--primary" style={{ width: "fit-content" }}>Explore Premium</Link>
        </div>
      </div>
    </div>
  );
}
