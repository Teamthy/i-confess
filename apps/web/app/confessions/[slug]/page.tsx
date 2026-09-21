/** Public confession detail — resolves by UUID (the API has no slug routes). */

import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import { api } from "@/lib/api";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";

export async function generateMetadata({ params }: { params: Promise<{ slug: string }> }): Promise<Metadata> {
  const { slug } = await params;
  const res = await api.confession(slug);
  if (!res.ok) return { title: "Confession not found — iCONFESS" };
  return {
    title: `${res.data.title} — iCONFESS`,
    description: res.data.description || res.data.short_text?.slice(0, 160),
  };
}

export default async function ConfessionPublicPage({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params;
  const res = await api.confession(slug);
  if (!res.ok) notFound();
  const c = res.data;

  return (
    <div>
      <SiteHeader />
      <main id="main">
        <section className="ic-page-hero">
          <div className="ic-container">
            <Link href="/confessions" className="ic-backlink">← Confessions</Link>
            <h1>{c.title}</h1>
            {c.author && <p className="ic-lede">By {c.author}</p>}
          </div>
        </section>

        <section className="ic-section">
          <div className="ic-container ic-container--text">
            {(c.long_text || c.medium_text || c.short_text) && (
              <blockquote className="ic-scripture" style={{ padding: "var(--ic-spacing-6)", background: "var(--ic-color-neutral-50)", borderRadius: "var(--ic-radius-lg)", borderLeft: "3px solid var(--ic-color-brand-600)" }}>
                {c.long_text || c.medium_text || c.short_text}
              </blockquote>
            )}

            {c.scriptures && c.scriptures.length > 0 && (
              <div style={{ marginTop: "var(--ic-spacing-7)" }}>
                <h2 style={{ fontSize: "var(--ic-font-size-subheading)", fontWeight: "var(--ic-font-weight-semibold)", marginBottom: "var(--ic-spacing-4)" }}>Scripture</h2>
                {c.scriptures.map((s) => (
                  <p key={s.id} style={{ fontFamily: "var(--ic-font-family-serif)", lineHeight: "var(--ic-font-lineHeight-relaxed)", color: "var(--ic-color-neutral-700)" }}>
                    {s.book}{s.chapter ? ` ${s.chapter}` : ""}{s.verse ? `:${s.verse}` : ""} ({s.translation})
                  </p>
                ))}
              </div>
            )}

            {c.variants && c.variants.length > 0 && (
              <div style={{ marginTop: "var(--ic-spacing-7)" }}>
                <h2 style={{ fontSize: "var(--ic-font-size-subheading)", fontWeight: "var(--ic-font-weight-semibold)", marginBottom: "var(--ic-spacing-4)" }}>Audio</h2>
                <p style={{ color: "var(--ic-color-neutral-600)" }}>
                  {c.variants.length} variant{c.variants.length > 1 ? "s" : ""} available.
                  {" "}<Link href="/register">Create an account</Link> to listen.
                </p>
              </div>
            )}

            <div style={{ marginTop: "var(--ic-spacing-8)" }}>
              <Link href="/register" className="ic-btn ic-btn--primary">Start your experience</Link>
            </div>
          </div>
        </section>
      </main>
      <SiteFooter />
    </div>
  );
}
