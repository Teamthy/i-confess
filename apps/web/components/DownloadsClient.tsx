"use client";

/**
 * /app/downloads — the offline library, from GET /me/downloads.
 *
 * A download is a time-bounded licence, not a permanent copy, and the server
 * says so on every read: each row carries `expires_at` and a server-computed
 * `expired`, alongside the plan's licence limit and offline window. This page
 * renders exactly that — including "expired" on a lapsed licence, because
 * content that silently stops working is worse than content that says so.
 *
 * Whether downloads are allowed at all is not decided here. The same response
 * carries `limit` (0 on a plan without downloads), so the empty state can be
 * honest about why the list is empty without the client asserting a plan.
 * A 402 — the contract's answer for a plan that cannot hold licences — gets
 * the Premium offer, the same as every other gated surface.
 */

import { useAuth } from "@/lib/auth-context";
import { useApiData, formatDate } from "@/lib/app-api";
import { EmptyState, ErrorBlock, LoadingBlock, PremiumOffer, SignedOut } from "@/components/app-ui";

type Download = {
  id: string;
  audio_asset_id: string;
  confession_id?: string;
  voice_id?: string;
  title?: string;
  duration_seconds: number;
  size_bytes?: number;
  status: string;
  expires_at?: string;
  expired: boolean;
  created_at: string;
};

type DownloadsResponse = {
  downloads: Download[] | null;
  limit: number;
  used: number;
  offline_hours_allowed: number;
};

function formatSize(bytes: number | undefined): string {
  if (!bytes || bytes <= 0) return "";
  return `${(bytes / 1_048_576).toFixed(1)} MB`;
}

export function DownloadsClient() {
  const { token, loading: authLoading } = useAuth();
  const dl = useApiData<DownloadsResponse>(token, "/me/downloads");

  if (authLoading) return <LoadingBlock rows={3} />;
  if (!token) return <SignedOut next="/app/downloads" />;
  if (dl.loading && !dl.data) return <LoadingBlock rows={3} />;
  if (dl.error && !dl.data) {
    if (dl.error.status === 401) return <SignedOut next="/app/downloads" />;
    if (dl.error.status === 402) return <PremiumOffer body={dl.error.message} />;
    return <ErrorBlock message={dl.error.message} onRetry={dl.reload} />;
  }

  const data = dl.data ?? { downloads: [], limit: 0, used: 0, offline_hours_allowed: 0 };
  const list = data.downloads ?? [];
  const canDownload = data.limit > 0;

  if (list.length === 0) {
    return canDownload ? (
      <EmptyState
        title="No downloads yet"
        body="Confessions you take offline will appear here, each with the date its licence expires."
      />
    ) : (
      <EmptyState
        title="Downloads are part of Premium"
        body="Offline listening is a Premium feature: save confessions before you travel and they keep working without a connection, for as long as the licence runs."
        ctaHref="/app/subscription"
        ctaLabel="See Premium"
      />
    );
  }

  return (
    <div style={{ display: "grid", gap: "var(--ic-spacing-5)" }}>
      <p style={{ margin: 0, color: "var(--ic-color-neutral-600)", fontSize: "var(--ic-font-size-bodySm)" }} role="status">
        {data.used} of {data.limit} licences in use
        {data.offline_hours_allowed > 0
          ? ` · licences run ${data.offline_hours_allowed} hours from download`
          : ""}
        . Expired licences stop playing until they are refreshed.
      </p>
      <ul className="ip-queue" style={{ listStyle: "none" }}>
        {list.map((d) => {
          const size = formatSize(d.size_bytes);
          const minutes = d.duration_seconds > 0 ? Math.round(d.duration_seconds / 60) : 0;
          const meta = [
            minutes > 0 ? `${minutes} min` : "",
            size,
            d.expires_at ? `expires ${formatDate(d.expires_at)}` : "",
          ].filter(Boolean);
          return (
            <li key={d.id} className="ip-queue__item">
              <span className="ip-queue__num" aria-hidden="true">
                {d.expired ? "⚠" : "↓"}
              </span>
              <span style={{ flex: 1 }}>
                {d.confession_id ? (
                  <a
                    href={`/app/confessions/${encodeURIComponent(d.confession_id)}`}
                    style={{ textDecoration: "none", color: "inherit" }}
                  >
                    <strong style={{ fontWeight: "var(--ic-font-weight-medium)" }}>
                      {d.title || "Downloaded confession"}
                    </strong>
                  </a>
                ) : (
                  <strong style={{ fontWeight: "var(--ic-font-weight-medium)" }}>
                    {d.title || "Downloaded confession"}
                  </strong>
                )}
                <span style={{ display: "block", color: "var(--ic-color-neutral-500)", fontSize: "var(--ic-font-size-caption)" }}>
                  {meta.join(" · ")}
                </span>
                {d.expired && (
                  <span style={{ display: "block", fontSize: "var(--ic-font-size-caption)", color: "var(--ic-color-neutral-600)", marginTop: "var(--ic-spacing-1)" }}>
                    This licence has expired — the file will not play until it is refreshed.
                  </span>
                )}
              </span>
              {!d.expired && <span className="ia-badge">Offline</span>}
            </li>
          );
        })}
      </ul>
    </div>
  );
}
