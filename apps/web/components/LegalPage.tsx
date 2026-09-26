import { SiteHeader } from "./SiteHeader";
import { SiteFooter } from "./SiteFooter";

/**
 * Shared layout for legal pages: ink hero, dated prose body from a typed
 * content model (content/legal.ts), and the standard footer. Legal copy lives
 * in one place so the four pages cannot drift.
 */
export function LegalPage({
  title,
  lede,
  updated,
  sections,
}: {
  title: string;
  lede: string;
  updated: string;
  sections: { heading: string; body: string[] }[];
}) {
  return (
    <div>
      <SiteHeader light />
      <main id="main">
        <section className="ic-page-hero on-ink">
          <div className="ic-container">
            <p className="ic-eyebrow">Legal</p>
            <h1 style={{ marginTop: "var(--ic-spacing-4)" }}>{title}</h1>
            <p className="ic-lede" style={{ marginTop: "var(--ic-spacing-5)" }}>{lede}</p>
            <p style={{ marginTop: "var(--ic-spacing-5)", fontSize: "var(--ic-font-size-caption)", color: "var(--ic-color-neutral-400)" }}>
              Last updated: <time dateTime={updated}>{updated}</time>
            </p>
          </div>
        </section>
        <section className="ic-section">
          <div className="ic-container ic-container--text ic-prose">
            {sections.map((s) => (
              <section key={s.heading}>
                <h2>{s.heading}</h2>
                {s.body.map((p, i) => (
                  <p key={i}>{p}</p>
                ))}
              </section>
            ))}
          </div>
        </section>
      </main>
      <SiteFooter />
    </div>
  );
}
