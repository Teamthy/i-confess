import type { Metadata } from "next";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";
import { SectionHead } from "@/components/Sections";
import { api } from "@/lib/api";

export const metadata: Metadata = {
  title: "Voices",
  description:
    "The voices of iCONFESS — curated, licensed narration that helps the words land.",
  alternates: { canonical: "/voices" },
};

export default async function VoicesPage() {
  const res = await api.voices();
  const voices = res.ok ? res.data : [];

  return (
    <div>
      <SiteHeader />
      <main id="main">
        <section className="ic-page-hero on-ink">
          <div className="ic-container">
            <p className="ic-eyebrow">Voices</p>
            <h1 style={{ marginTop: "var(--ic-spacing-4)" }}>The voices of iCONFESS.</h1>
            <p className="ic-lede" style={{ marginTop: "var(--ic-spacing-5)" }}>
              The same words land differently in different voices. Every voice
              here is curated and cleared for use — hear them, and choose the
              one that helps the words land.
            </p>
          </div>
        </section>

        <section className="ic-section">
          <div className="ic-container">
            {res.ok && voices.length > 0 ? (
              <div className="ic-grid ic-grid--3">
                {voices.map((v) => (
                  <a key={v.id} href={`/voices/${v.id}`} className="ic-card ic-card--hover ic-voice-card">
                    <span className="ic-voice-avatar" aria-hidden="true">{v.name.charAt(0)}</span>
                    <div>
                      <h3>
                        {v.name}
                        {v.premium && <> <span className="ic-chip">Premium</span></>}
                      </h3>
                    </div>
                    <p>{v.description}</p>
                    <footer>
                      <span className="ic-cap">{v.gender || v.type}</span>
                      <span aria-hidden="true">·</span>
                      <span>Meet {v.name}</span>
                    </footer>
                  </a>
                ))}
              </div>
            ) : res.ok ? (
              <div className="ic-state">
                <h3>The voice library is being refreshed</h3>
                <p>No voices are available right now. Please check back soon.</p>
              </div>
            ) : (
              <div className="ic-state" role="alert">
                <h3>We couldn't reach the library</h3>
                <p>Something went wrong while loading the voices. Please try again soon.</p>
              </div>
            )}
          </div>
        </section>

        <section className="ic-section ic-section--mist">
          <div className="ic-container">
            <SectionHead
              eyebrow="Why voices matter"
              title="Written to be spoken. Heard to be kept."
              lede="A confession is meant to be said out loud. When the voice is right, the words stop being text on a screen and start being something you carry."
            />
          </div>
        </section>
      </main>
      <SiteFooter />
    </div>
  );
}
