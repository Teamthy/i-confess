/** Community feed — published confessions from other listeners. */
import Link from "next/link";
import { api } from "@/lib/api";
export default async function CommunityPage() {
  const feedRes = await api.communityFeed();
  const communityRes = await api.communityConfessions();
  const feed = feedRes.ok ? feedRes.data : [];
  const confessions = communityRes.ok ? communityRes.data.confessions : [];

  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Community</h1>
        <p>Confessions written by people like you. Every one reviewed before publication.</p>
      </div>
      <Link href="/app/community/create" className="ic-btn ic-btn--primary" style={{ width: "fit-content" }}>Write a confession</Link>

      {confessions.length > 0 && (
        <section>
          <h2 className="ia-section-title">Community confessions</h2>
          <div className="ia-card-grid">
            {(confessions as Array<Record<string, unknown>>).slice(0, 12).map((c, i) => (
              <div key={String(c.id ?? i)} className="ic-card" style={{ padding: "var(--ic-spacing-5)" }}>
                <h3 style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-font-size-subheading)" }}>
                  {String(c.title ?? "Untitled")}
                </h3>
                <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", marginTop: "var(--ic-spacing-2)" }}>
                  {String(c.short_text ?? c.description ?? "")}
                </p>
              </div>
            ))}
          </div>
        </section>
      )}

      {confessions.length === 0 && (
        <div className="ic-state">
          <h3>The community is growing</h3>
          <p>Published confessions from the community will appear here.</p>
        </div>
      )}
    </div>
  );
}
