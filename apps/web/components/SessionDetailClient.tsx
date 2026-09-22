"use client";

/**
 * /app/sessions/[id] — one session, read from GET /sessions/{id}.
 *
 * GET re-signs the audio on every read: each item's audio_url is a fresh,
 * short-lived URL minted for this response, and an item the account cannot
 * play arrives locked with the server's reason instead of a URL. This page is
 * deliberately read-only, so it never consumes those URLs — it shows the
 * queue, its locks, and its state, and hands playback to /app/player, which
 * re-GETs the session and signs again. Nothing here caches a signed URL,
 * because nothing here is entitled to one beyond the response it came in.
 *
 * The lifecycle stays server-owned: status changes, skips and completion
 * happen in the player through the state-machine endpoints. This page only
 * reports what the server says the session is.
 */

import { useAuth } from "@/lib/auth-context";
import { useApiData, formatClock, formatDate } from "@/lib/app-api";
import { ErrorBlock, LoadingBlock, PremiumOffer, SignedOut } from "@/components/app-ui";
import type { Session, SessionItem } from "@/components/PlayerClient";

/** Human words for the server's stable lock reasons (entitlements/audio.go). */
function lockReasonText(reason: string | undefined): string {
  switch (reason) {
    case "premium_voice_requires_subscription":
      return "That voice is part of Premium.";
    case "premium_content_requires_subscription":
      return "That confession is part of Premium.";
    case "session_duration_exceeds_plan_limit":
      return "Longer than your plan allows.";
    case "audio_not_published":
      return "This audio isn't published.";
    default:
      return "Unavailable on your plan right now.";
  }
}

function openPlayerLabel(s: Session): string {
  if (s.status === "COMPLETED") return "Listen again";
  if (s.started_at) return "Resume in player";
  return "Play session";
}

function QueueRow({ item }: { item: SessionItem }) {
  const done = item.status === "COMPLETED" || item.status === "SKIPPED";
  return (
    <li className="ip-queue__item">
      <span className="ip-queue__num" aria-hidden="true">
        {item.status === "SKIPPED" ? "»" : item.status === "COMPLETED" ? "✓" : item.position}
      </span>
      <span style={{ flex: 1 }}>
        <a
          href={`/app/confessions/${encodeURIComponent(item.confession_id)}`}
          style={{
            textDecoration: "none",
            color: "inherit",
            ...(done ? { color: "var(--ic-color-neutral-500)" } : null),
          }}
        >
          <strong className="ip-queue__title" style={{ fontWeight: "var(--ic-font-weight-medium)" }}>
            {item.title || "Confession"}
          </strong>
        </a>
        <span style={{ display: "block", color: "var(--ic-color-neutral-500)", fontSize: "var(--ic-font-size-caption)" }}>
          {[item.category, formatClock(item.duration_seconds), item.status.toLowerCase()].filter(Boolean).join(" · ")}
        </span>
        {item.locked && (
          <span style={{ display: "block", fontSize: "var(--ic-font-size-caption)", color: "var(--ic-color-neutral-600)", marginTop: "var(--ic-spacing-1)" }}>
            Locked — {lockReasonText(item.lock_reason)}
          </span>
        )}
      </span>
    </li>
  );
}

export function SessionDetailClient({ sessionId }: { sessionId: string }) {
  const { token, loading: authLoading } = useAuth();
  const next = `/app/sessions/${encodeURIComponent(sessionId)}`;
  const sess = useApiData<Session>(token, `/sessions/${encodeURIComponent(sessionId)}`);

  if (authLoading) return <LoadingBlock rows={4} />;
  if (!token) return <SignedOut next={next} />;
  if (sess.loading && !sess.data) return <LoadingBlock rows={4} />;
  if (sess.error && !sess.data) {
    if (sess.error.status === 401) return <SignedOut next={next} />;
    if (sess.error.status === 402) return <PremiumOffer body={sess.error.message} />;
    if (sess.error.status === 404) {
      return (
        <div className="ic-state" role="alert">
          <h3>We can&rsquo;t find that session</h3>
          <p>It may have been removed from history, or the link is wrong.</p>
          <a href="/app/sessions" className="ic-btn ic-btn--secondary">Back to sessions</a>
        </div>
      );
    }
    return <ErrorBlock message={sess.error.message} onRetry={sess.reload} />;
  }

  const s = sess.data ?? null;
  if (!s) return <LoadingBlock rows={4} />;
  const items = [...(s.items ?? [])].sort((a, b) => a.position - b.position);
  const title = s.title || `${Math.round((s.actual_duration || s.duration_seconds || 0) / 60)}-minute session`;
  const meta = [
    `Built ${formatDate(s.created_at)}`,
    s.started_at ? `started ${formatDate(s.started_at)}` : "",
    s.completed_at ? `completed ${formatDate(s.completed_at)}` : "",
  ].filter(Boolean);

  return (
    <div style={{ display: "grid", gap: "var(--ic-spacing-6)" }}>
      <header>
        <p className="ia-section-title" style={{ margin: 0 }}>
          Session <span className="ia-badge" style={{ marginLeft: "var(--ic-spacing-2)" }}>{s.status.toLowerCase()}</span>
        </p>
        <h1 style={{ margin: "var(--ic-spacing-2) 0 0", fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-font-size-heading)", fontWeight: "var(--ic-font-weight-regular)", lineHeight: "var(--ic-font-lineHeight-snug)" }}>
          {title}
        </h1>
        {s.description && (
          <p style={{ color: "var(--ic-color-neutral-600)", marginTop: "var(--ic-spacing-2)" }}>{s.description}</p>
        )}
        <p style={{ color: "var(--ic-color-neutral-500)", fontSize: "var(--ic-font-size-caption)", marginTop: "var(--ic-spacing-2)" }}>
          {meta.join(" · ")}
        </p>
        {(s.target_duration > 0 || s.actual_duration > 0) && (
          <p style={{ color: "var(--ic-color-neutral-600)", fontSize: "var(--ic-font-size-bodySm)", marginTop: "var(--ic-spacing-1)" }}>
            {s.target_duration > 0 && <>asked for {formatClock(s.target_duration)}</>}
            {s.target_duration > 0 && s.actual_duration > 0 && " · "}
            {s.actual_duration > 0 && <>assembled to {formatClock(s.actual_duration)}</>}
            {s.strategy ? ` · ${s.strategy.toLowerCase()} strategy` : ""}
            {s.target_duration > 0 && s.actual_duration > 0 && s.actual_duration !== s.target_duration
              ? " — complete confessions are never cut short, so the run may land near the request rather than on it."
              : "."}
          </p>
        )}
        {s.voice_downgraded && (
          <p style={{ color: "var(--ic-color-neutral-600)", fontSize: "var(--ic-font-size-bodySm)", marginTop: "var(--ic-spacing-1)" }}>
            A different voice narrates this session{s.voice_downgrade_reason ? ` (${s.voice_downgrade_reason.replace(/_/g, " ")})` : ""}.
          </p>
        )}
        <div className="ic-btn-row" style={{ marginTop: "var(--ic-spacing-4)" }}>
          <a href={`/app/player?session=${encodeURIComponent(s.id)}`} className="ic-btn ic-btn--primary">
            {openPlayerLabel(s)}
          </a>
        </div>
      </header>

      <section aria-labelledby="queue-heading">
        <h2 id="queue-heading" className="ia-section-title">
          Queue{items.length > 0 ? ` (${items.length})` : ""}
        </h2>
        {items.length === 0 ? (
          <div className="ic-state">
            <h3>This session has no items</h3>
            <p>The queue is empty, so there is nothing here to play.</p>
          </div>
        ) : (
          <ul className="ip-queue" style={{ listStyle: "none" }}>
            {items.map((it) => (
              <QueueRow key={it.id} item={it} />
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
