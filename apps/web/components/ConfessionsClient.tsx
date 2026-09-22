"use client";

/**
 * /app/confessions — the listener's personal shelf.
 *
 * GET /me/confessions lists what the account has written; there is no
 * per-item detail endpoint (design/routes.json), so each confession expands
 * inline instead of linking to a detail page that cannot exist. Drafts can
 * be offered for review (POST /me/confessions/{id}/submit); review outcomes
 * (review_notes / rejection_reason) are shown verbatim when present.
 *
 * This page is also the "Create" destination of the mobile bottom tab and
 * the confirmation target of /app/community/create — both 404'd before it
 * existed.
 */

import { useState } from "react";
import Link from "next/link";
import { useAuth } from "@/lib/auth-context";
import { appApi, formatDate, useApiData } from "@/lib/app-api";
import { EmptyState, ErrorBlock, LoadingBlock, SignedOut } from "@/components/app-ui";

type UserConfession = {
  id: string;
  title: string;
  text: string;
  category_id?: string;
  is_private: boolean;
  status: string;
  visibility: string;
  review_notes?: string;
  rejection_reason?: string;
  created_at: string;
  updated_at: string;
};

const STATUS_COPY: Record<string, { label: string; body: string }> = {
  draft: { label: "Draft", body: "Only you can see this." },
  private: { label: "Private", body: "Only you can see this." },
  submitted: { label: "In review", body: "A moderator reads every submission before it reaches anyone else." },
  pending: { label: "In review", body: "A moderator reads every submission before it reaches anyone else." },
  approved: { label: "Approved", body: "Accepted for the community." },
  published: { label: "Published", body: "Visible in the community." },
  rejected: { label: "Not published", body: "This one didn't go through — the note below says why." },
};

function statusOf(c: UserConfession): { label: string; body: string } {
  const hit = STATUS_COPY[c.status.toLowerCase()];
  if (hit) return hit;
  return { label: c.status || "Saved", body: c.is_private ? "Only you can see this." : "" };
}

export function ConfessionsClient() {
  const { token, loading: authLoading } = useAuth();
  const shelf = useApiData<UserConfession[]>(token, "/me/confessions");
  const [openId, setOpenId] = useState<string | null>(null);
  const [submittingId, setSubmittingId] = useState<string | null>(null);
  const [submitError, setSubmitError] = useState("");

  if (authLoading || shelf.loading) return <LoadingBlock rows={4} />;
  if (!token) return <SignedOut next="/app/confessions" />;
  if (shelf.error) return <ErrorBlock message={shelf.error.message} onRetry={shelf.reload} />;

  const items = shelf.data ?? [];
  if (items.length === 0) {
    return (
      <EmptyState
        title="Nothing written yet"
        body="Your confessions live here — private by default, shared only when you choose."
        ctaHref="/app/community/create"
        ctaLabel="Write your first confession"
      />
    );
  }

  async function submitForReview(id: string) {
    if (!token) return;
    setSubmittingId(id);
    setSubmitError("");
    const r = await appApi<UserConfession>(`/me/confessions/${id}/submit`, { token, method: "POST" });
    setSubmittingId(null);
    if (!r.ok) {
      setSubmitError(r.message);
      return;
    }
    shelf.reload();
  }

  return (
    <div style={{ display: "grid", gap: "var(--ic-spacing-3)" }}>
      {submitError && (
        <p role="alert" style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-semantic-danger-light)" }}>
          {submitError}
        </p>
      )}
      {items.map((c) => {
        const st = statusOf(c);
        const open = openId === c.id;
        const canSubmit = ["draft", "private"].includes(c.status.toLowerCase());
        return (
          <article key={c.id} className="ic-card" style={{ padding: "var(--ic-spacing-5)" }}>
            <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", gap: "var(--ic-spacing-4)" }}>
              <h3 style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-font-size-subheading)" }}>{c.title}</h3>
              <span
                className="ia-badge"
                style={{ flexShrink: 0 }}
              >
                {st.label}
              </span>
            </div>
            <p style={{ fontSize: "var(--ic-font-size-caption)", color: "var(--ic-color-neutral-500)", marginTop: "var(--ic-spacing-1)" }}>
              {st.body} {c.created_at ? `· Written ${formatDate(c.created_at)}` : ""}
            </p>
            {open && (
              <div style={{ marginTop: "var(--ic-spacing-4)" }}>
                <p style={{ fontFamily: "var(--ic-font-family-serif)", lineHeight: "var(--ic-font-lineHeight-relaxed)", whiteSpace: "pre-wrap" }}>
                  {c.text}
                </p>
                {(c.review_notes || c.rejection_reason) && (
                  <p style={{ marginTop: "var(--ic-spacing-3)", fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)" }}>
                    Reviewer note: {c.rejection_reason || c.review_notes}
                  </p>
                )}
              </div>
            )}
            <div className="ic-btn-row" style={{ marginTop: "var(--ic-spacing-4)" }}>
              <button
                type="button"
                className="ic-btn ic-btn--text"
                aria-expanded={open}
                onClick={() => setOpenId(open ? null : c.id)}
              >
                {open ? "Hide" : "Read"}
              </button>
              {canSubmit && (
                <button
                  type="button"
                  className="ic-btn ic-btn--secondary ic-btn--small"
                  disabled={submittingId === c.id}
                  onClick={() => submitForReview(c.id)}
                >
                  {submittingId === c.id ? "Sending…" : "Offer for review"}
                </button>
              )}
            </div>
          </article>
        );
      })}
      <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-500)" }}>
        Want to write another? <Link href="/app/community/create">Start a new confession</Link>.
      </p>
    </div>
  );
}
