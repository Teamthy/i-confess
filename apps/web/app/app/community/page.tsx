/**
 * Community — the approved feed (client-fetched, GET /community/feed) above
 * the published community confessions (server-rendered, anonymous by
 * construction: the API's projection strips author fields, and this page
 * never asks for them).
 */
import Link from "next/link";
import { api } from "@/lib/api";
import { CommunityFeedClient } from "@/components/CommunityFeedClient";

export default async function CommunityPage() {
  const communityRes = await api.communityConfessions();
  const confessions = communityRes.ok ? communityRes.data.confessions : [];

  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Community</h1>
        <p>Confessions written by people like you. Every one reviewed before publication.</p>
      </div>
      <Link href="/app/community/create" className="ic-btn ic-btn--primary" style={{ width: "fit-content" }}>
        Share something
      </Link>

      <section style={{ marginTop: "var(--ic-spacing-6)" }} aria-labelledby="feed-heading">
        <h2 id="feed-heading" className="ia-section-title">The feed</h2>
        <CommunityFeedClient />
      </section>

      {confessions.length > 0 && (
        <section style={{ marginTop: "var(--ic-spacing-6)" }}>
          <h2 className="ia-section-title">Published confessions</h2>
          <div className="ia-card-grid">
            {(confessions as Array<Record<string, unknown>>).slice(0, 12).map((c, i) => (
              <div key={String(c.id ?? i)} className="ic-card" style={{ padding: "var(--ic-spacing-5)" }}>
                <h3 style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-font-size-subheading)" }}>
                  {String(c.title ?? "Untitled")}
                </h3>
                <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", marginTop: "var(--ic-spacing-2)" }}>
                  {String(c.short_text ?? c.text ?? "").slice(0, 220)}
                </p>
              </div>
            ))}
          </div>
        </section>
      )}

      {confessions.length === 0 && (
        <div className="ic-state" style={{ marginTop: "var(--ic-spacing-6)" }}>
          <h3>The community library is growing</h3>
          <p>Published confessions from listeners will appear here as moderation approves them.</p>
        </div>
      )}
    </div>
  );
}
