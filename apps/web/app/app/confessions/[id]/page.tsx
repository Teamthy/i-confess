/** One confession with scriptures and audio variants. */
import Link from "next/link";
import { notFound } from "next/navigation";
import { api } from "@/lib/api";
export default async function ConfessionDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const res = await api.confession(id);
  if (!res.ok) notFound();
  const c = res.data;

  return (
    <div className="ia-page">
      <Link href="/app/explore" className="ic-backlink" style={{ color: "var(--ic-color-neutral-500)" }}>← Back to explore</Link>
      <div className="ia-page__head">
        <p className="ia-section-title">Confession</p>
        <h1>{c.title}</h1>
        {c.author && <p style={{ color: "var(--ic-color-neutral-500)" }}>By {c.author}</p>}
      </div>

      {(c.long_text || c.medium_text || c.short_text) && (
        <blockquote className="ic-scripture" style={{ padding: "var(--ic-spacing-6)", background: "var(--ic-color-neutral-50)", borderRadius: "var(--ic-radius-lg)", borderLeft: "3px solid var(--ic-color-brand-600)" }}>
          {c.long_text || c.medium_text || c.short_text}
        </blockquote>
      )}

      {c.scriptures && c.scriptures.length > 0 && (
        <section>
          <h2 className="ia-section-title">Scripture</h2>
          {c.scriptures.map((s) => (
            <p key={s.id} style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-font-size-body)", lineHeight: "var(--ic-font-lineHeight-relaxed)", color: "var(--ic-color-neutral-700)" }}>
              {s.book}{s.chapter ? ` ${s.chapter}` : ""}{s.verse ? `:${s.verse}` : ""} ({s.translation})
              {s.is_direct_quote && <span style={{ color: "var(--ic-color-brand-600)", marginLeft: "var(--ic-spacing-2)" }}>Direct quote</span>}
            </p>
          ))}
        </section>
      )}

      {c.variants && c.variants.length > 0 && (
        <section>
          <h2 className="ia-section-title">Audio variants</h2>
          <div className="ia-card-grid">
            {c.variants.map((v) => (
              <div key={v.id} className="ic-card" style={{ padding: "var(--ic-spacing-5)" }}>
                <h3 style={{ fontWeight: "var(--ic-font-weight-semibold)" }}>{v.label}</h3>
                <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-500)", marginTop: "var(--ic-spacing-2)" }}>
                  {Math.floor(v.duration_seconds / 60)}:{String(v.duration_seconds % 60).padStart(2, "0")}
                </p>
              </div>
            ))}
          </div>
        </section>
      )}

      {c.tags && c.tags.length > 0 && (
        <div style={{ display: "flex", flexWrap: "wrap", gap: "var(--ic-spacing-2)" }}>
          {c.tags.map((t) => (
            <span key={t} style={{ fontSize: "var(--ic-font-size-caption)", padding: "var(--ic-spacing-1) var(--ic-spacing-3)", borderRadius: "var(--ic-radius-full)", background: "var(--ic-color-neutral-100)", color: "var(--ic-color-neutral-600)" }}>{t}</span>
          ))}
        </div>
      )}
    </div>
  );
}
