import type { Metadata } from "next";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";
import { ContactForm } from "./ContactForm";
import { SUPPORT_EMAIL } from "@/lib/site";

export const metadata: Metadata = {
  title: "Contact",
  description: "Questions about the practice, your account, or content — a human reads every message.",
  alternates: { canonical: "/contact" },
};

export default function ContactPage() {
  return (
    <div>
      <SiteHeader light />
      <main id="main">
        <section className="ic-page-hero on-ink">
          <div className="ic-container">
            <p className="ic-eyebrow">Contact</p>
            <h1 style={{ marginTop: "var(--ic-spacing-4)" }}>Talk to a person.</h1>
            <p className="ic-lede" style={{ marginTop: "var(--ic-spacing-5)" }}>
              Questions about the practice, your account, or content — a human
              reads every message.
            </p>
          </div>
        </section>

        <section className="ic-section">
          <div className="ic-container ic-container--text">
            <h2 className="ic-visually-hidden">Send a message</h2>
            <ContactForm />
            <p className="ic-field__hint" style={{ marginTop: "var(--ic-spacing-4)" }}>
              Prefer email? Write to <a href={`mailto:${SUPPORT_EMAIL}`} style={{ color: "var(--ic-color-brand-700)" }}>{SUPPORT_EMAIL}</a>.
            </p>
          </div>
        </section>
      </main>
      <SiteFooter />
    </div>
  );
}
