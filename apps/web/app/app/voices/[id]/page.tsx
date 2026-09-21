/** One voice with sample and confessions. */
import Link from "next/link";
import { notFound } from "next/navigation";
import { api } from "@/lib/api";
export default async function VoiceProfilePage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const res = await api.voices();
  if (!res.ok) notFound();
  const voice = res.data.find((v) => v.id === id);
  if (!voice) notFound();

  return (
    <div className="ia-page">
      <Link href="/app/voices" className="ic-backlink" style={{ color: "var(--ic-color-neutral-500)" }}>← Voices</Link>
      <div className="ia-page__head">
        <div style={{ display: "flex", alignItems: "center", gap: "var(--ic-spacing-4)" }}>
          <span className="ic-voice-avatar" aria-hidden="true">{voice.name.charAt(0)}</span>
          <div>
            <h1>{voice.name}</h1>
            <p style={{ color: "var(--ic-color-neutral-500)" }}>{voice.type}{voice.gender ? ` · ${voice.gender}` : ""} · {voice.language}</p>
          </div>
        </div>
      </div>
      {voice.description && (
        <p style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-font-size-body)", lineHeight: "var(--ic-font-lineHeight-relaxed)", color: "var(--ic-color-neutral-700)" }}>{voice.description}</p>
      )}
      {voice.premium && <span className="ia-badge ia-badge--premium">Premium voice</span>}
      {voice.sample_url && (
        <section>
          <h2 className="ia-section-title">Sample</h2>
          <audio controls src={voice.sample_url} style={{ width: "100%" }}>
            Your browser does not support audio playback.
          </audio>
        </section>
      )}
    </div>
  );
}
