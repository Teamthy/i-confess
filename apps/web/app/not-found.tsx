import Link from "next/link";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";

export const metadata = { title: "Page not found", robots: { index: false } };

export default function NotFound() {
  return (
    <div className="on-dark">
      <SiteHeader />
      <main id="main" className="ic-section--ink" style={{ minHeight: "70vh", display: "grid", alignItems: "center", paddingTop: "4.25rem" }}>
        <div className="ic-container" style={{ textAlign: "center", paddingBlock: "var(--ic-spacing-10)" }}>
          <p className="ic-eyebrow" style={{ justifyContent: "center" }}>404</p>
          <h1 className="ic-display" style={{ margin: "var(--ic-spacing-4) auto 0", maxWidth: "16ch" }}>
            This page isn't part of the experience.
          </h1>
          <p className="ic-lede" style={{ margin: "var(--ic-spacing-5) auto 0" }}>
            The link may be old or the address mistyped. Everything worth finding
            is one step away.
          </p>
          <div className="ic-btn-row" style={{ justifyContent: "center", marginTop: "var(--ic-spacing-7)" }}>
            <Link href="/" className="ic-btn ic-btn--on-dark">Return home</Link>
            <Link href="/explore" className="ic-btn ic-btn--secondary on-ink">Explore confessions</Link>
          </div>
        </div>
      </main>
      <SiteFooter />
    </div>
  );
}
