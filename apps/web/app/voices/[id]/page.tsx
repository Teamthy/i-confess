import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";
import { api } from "@/lib/api";
import { SITE_URL } from "@/lib/site";

/**
 * Voice profile. There is no GET /voices/{id} yet (audit §5), so the profile
 * resolves the voice from the public list — same data, one extra step, and
 * the page exists the day the detail endpoint lands by switching one call.
 */
export async function generateMetadata({
  params,
}: {
  params: Promise<{ id: string }>;
}): Promise<Metadata> {
  const { id } = await params;
  const res = await api.voices();
  const voice = res.ok ? res.data.find((v) => v.id === id) : undefined;
  if (!voice) notFound(); // 404 status, not a 200 shell
  return {
    title: `${voice.name} — Voice`,
    description: voice.description || `Meet ${voice.name}, an iCONFESS narration voice.`,
    alternates: { canonical: `/voices/${voice.id}` },
    openGraph: {
      title: `${voice.name} — iCONFESS Voices`,
      description: voice.description || "",
      url: `${SITE_URL}/voices/${voice.id}`,
    },
  };
}

export default async function VoiceProfilePage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const res = await api.voices();
  const voice = res.ok ? res.data.find((v) => v.id === id) : undefined;
  if (!voice) notFound();

  return (
    <div>
      <SiteHeader />
      <main id="main">
        <section className="ic-page-hero on-ink">
          <div className="ic-container">
            <Link href="/voices" className="ic-backlink">← All voices</Link>
            <div style={{ display: "flex", alignItems: "center", gap: "var(--ic-spacing-5)", flexWrap: "wrap" }}>
              <span className="ic-voice-avatar" style={{ width: 72, height: 72, fontSize: "2rem" }} aria-hidden="true">
                {voice.name.charAt(0)}
              </span>
              <div>
                <p className="ic-eyebrow">{voice.premium ? "Premium voice" : "Voice"}</p>
                <h1 style={{ marginTop: "var(--ic-spacing-2)" }}>{voice.name}</h1>
              </div>
            </div>
            {voice.description && (
              <p className="ic-lede" style={{ marginTop: "var(--ic-spacing-5)" }}>{voice.description}</p>
            )}
          </div>
        </section>

        <section className="ic-section">
          <div className="ic-container">
            <div className="ic-grid ic-grid--3">
              <div className="ic-card" style={{ padding: "var(--ic-spacing-6)" }}>
                <p className="ic-frame__kicker" style={{ color: "var(--ic-color-brand-600)" }}>Character</p>
                <p style={{ marginTop: "var(--ic-spacing-3)", fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", lineHeight: "var(--ic-font-lineHeight-relaxed)" }}>
                  {voice.description || "A curated iCONFESS narration voice."}
                </p>
              </div>
              <div className="ic-card" style={{ padding: "var(--ic-spacing-6)" }}>
                <p className="ic-frame__kicker" style={{ color: "var(--ic-color-brand-600)" }}>Available in</p>
                <p style={{ marginTop: "var(--ic-spacing-3)", fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)" }}>
                  English sessions
                  {voice.premium ? " — Premium sessions" : " — free and Premium sessions"}
                </p>
              </div>
              <div className="ic-card ic-card--hover" style={{ padding: "var(--ic-spacing-6)", display: "grid", alignContent: "start", gap: "var(--ic-spacing-3)" }}>
                <p className="ic-frame__kicker" style={{ color: "var(--ic-color-brand-600)" }}>Hear {voice.name}</p>
                <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)" }}>
                  {voice.name} speaks throughout guided sessions. Start one to hear the voice carry the words.
                </p>
                <Link href="/register" className="ic-btn ic-btn--primary ic-btn--small" style={{ width: "fit-content" }}>
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
