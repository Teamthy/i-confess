/** Voice library — choose the voice that helps the words land. */
import Link from "next/link";
import { api } from "@/lib/api";
export default async function VoicesPage() {
  const res = await api.voices();
  const voices = res.ok ? res.data : [];

  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Voices</h1>
        <p>{voices.length} curated narration voices. Every confession can be heard as well as read.</p>
      </div>
      <div className="ia-card-grid">
        {voices.map((v) => (
          <Link key={v.id} href={`/app/voices/${v.id}`} className="ia-voice">
            <span className="ic-voice-avatar" aria-hidden="true">{v.name.charAt(0)}</span>
            <h3>{v.name}</h3>
            <p>{v.description}</p>
            <div style={{ display: "flex", gap: "var(--ic-spacing-3)", alignItems: "center", fontSize: "var(--ic-font-size-caption)", color: "var(--ic-color-neutral-500)" }}>
              <span>{v.type}</span>
              {v.gender && <span>· {v.gender}</span>}
              {v.premium && <span className="ia-badge ia-badge--premium">Premium</span>}
            </div>
          </Link>
        ))}
      </div>
      {voices.length === 0 && (
        <div className="ic-state"><h3>No voices available</h3><p>The voice library is being updated.</p></div>
      )}
    </div>
  );
}
