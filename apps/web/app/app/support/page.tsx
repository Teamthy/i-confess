/** Get help with iCONFESS. */
import Link from "next/link";
export default function SupportPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Support</h1>
        <p>Need help? Start here.</p>
      </div>
      <div style={{ display: "grid", gap: "var(--ic-spacing-3)" }}>
        {[
          { href: "/faq", label: "FAQ", desc: "Frequently asked questions about iCONFESS" },
          { href: "/help", label: "Help centre", desc: "Guides and how-tos" },
          { href: "/contact", label: "Contact", desc: "Reach the iCONFESS team" },
          { href: "/app/feedback", label: "Feedback", desc: "Share your thoughts" },
        ].map((s) => (
          <Link key={s.href} href={s.href} className="ic-card ic-card--hover" style={{ padding: "var(--ic-spacing-5)", textDecoration: "none", display: "flex", justifyContent: "space-between", alignItems: "center" }}>
            <div>
              <h3 style={{ fontWeight: "var(--ic-font-weight-semibold)" }}>{s.label}</h3>
              <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-500)", marginTop: "var(--ic-spacing-1)" }}>{s.desc}</p>
            </div>
            <span style={{ color: "var(--ic-color-neutral-400)" }}>→</span>
          </Link>
        ))}
      </div>
    </div>
  );
}
