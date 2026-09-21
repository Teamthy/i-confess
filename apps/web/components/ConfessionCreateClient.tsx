"use client";

/**
 * /app/community/create — write a personal confession (POST /me/confessions).
 *
 * Visibility is intent, not outcome, and the UI says so: `private` saves to
 * the author's shelf; `shared`/`public` additionally call
 * POST /me/confessions/{id}/submit, which moves the row into the moderation
 * queue. Only a moderator publishes it (policy §10 — the platform never
 * auto-publishes user content), so the confirmation names what comes next
 * rather than claiming the words are "live".
 *
 * An unverified address is refused (403) at submission, not at saving —
 * writing privately never needs verification. The refusal renders as the
 * guidance it is.
 */

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth-context";
import { appApi, useApiData } from "@/lib/app-api";
import type { Category } from "@/lib/api";
import { ErrorBlock, LoadingBlock, SignedOut } from "@/components/app-ui";

type UserConfession = {
  id: string;
  title: string;
  text: string;
  status: string;
  visibility: string;
};

export function ConfessionCreateClient() {
  const router = useRouter();
  const { token, loading: authLoading } = useAuth();
  const cats = useApiData<Category[]>(token, "/categories");

  const [title, setTitle] = useState("");
  const [text, setText] = useState("");
  const [categoryId, setCategoryId] = useState("");
  const [visibility, setVisibility] = useState<"private" | "shared" | "public">("private");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [done, setDone] = useState<{ id: string; submitted: boolean } | null>(null);

  useEffect(() => {
    if (!authLoading && !token) {
      router.replace("/login?next=" + encodeURIComponent("/app/community/create"));
    }
  }, [authLoading, token, router]);

  const categories = useMemo(() => (cats.data ?? []).slice(), [cats.data]);

  if (authLoading) return <LoadingBlock rows={3} />;
  if (!token) return <SignedOut next="/app/community/create" />;

  if (done) {
    return (
      <div className="ic-state" role="status">
        <h3>{done.submitted ? "Sent for review." : "Saved — private to you."}</h3>
        <p>
          {done.submitted
            ? "A moderator reads every submission before it reaches anyone else. You'll find its status on your confessions."
            : "Nobody else can see this. When you want to offer it to the community, submit it from your confessions."}
        </p>
        <div className="ic-btn-row">
          <a href="/app/confessions" className="ic-btn ic-btn--secondary">Your confessions</a>
          <a href="/app/community" className="ic-btn ic-btn--text">Back to community</a>
        </div>
      </div>
    );
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    const t = title.trim();
    const b = text.trim();
    if (!t || !b) {
      setError("A title and the words themselves are both needed.");
      return;
    }
    setBusy(true);
    const created = await appApi<UserConfession>("/me/confessions", {
      token,
      method: "POST",
      body: {
        title: t,
        text: b,
        ...(categoryId ? { category_id: categoryId } : {}),
        visibility,
      },
    });
    if (!created.ok) {
      setBusy(false);
      setError(created.message);
      return;
    }

    if (visibility === "private") {
      setBusy(false);
      setDone({ id: created.data.id, submitted: false });
      return;
    }

    const submitted = await appApi<UserConfession>(
      `/me/confessions/${encodeURIComponent(created.data.id)}/submit`,
      { token, method: "POST" },
    );
    setBusy(false);
    if (!submitted.ok) {
      // The draft exists even though the submit failed — say exactly that,
      // and where to retry, rather than pretending nothing happened.
      setDone({ id: created.data.id, submitted: false });
      setError(
        submitted.status === 403 && /verif/i.test(submitted.message)
          ? "Saved privately. Confirm your email address first, then submit it for review from your confessions."
          : `Saved privately. Submission didn't go through (${submitted.message}) — retry from your confessions.`,
      );
      return;
    }
    setDone({ id: created.data.id, submitted: true });
  }

  return (
    <form onSubmit={submit} noValidate style={{ display: "grid", gap: "var(--ic-spacing-4)", maxWidth: "38rem" }}>
      <div className="ic-field">
        <label htmlFor="cc-title">Title</label>
        <input id="cc-title" value={title} onChange={(e) => setTitle(e.target.value)} maxLength={140} />
      </div>
      <div className="ic-field">
        <label htmlFor="cc-text">Your confession</label>
        <textarea
          id="cc-text"
          rows={10}
          value={text}
          onChange={(e) => setText(e.target.value)}
          maxLength={8000}
          placeholder="In your own words. This is read aloud nowhere unless it is approved and recorded — what you write stays text until you decide otherwise."
        />
        <p className="ic-field__hint">{text.trim().length} characters</p>
      </div>
      <div className="ic-field">
        <label htmlFor="cc-cat">Area of life (optional)</label>
        <select id="cc-cat" value={categoryId} onChange={(e) => setCategoryId(e.target.value)}>
          <option value="">None</option>
          {categories.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
        </select>
      </div>
      <fieldset style={{ border: "none", padding: 0, margin: 0 }}>
        <legend style={{ fontSize: "var(--ic-font-size-bodySm)", fontWeight: "var(--ic-font-weight-medium)" }}>
          Who can see it
        </legend>
        {(
          [
            { v: "private", label: "Private", hint: "Only you, always." },
            { v: "shared", label: "Shared", hint: "A link you hand out, after review." },
            { v: "public", label: "Community", hint: "Offered to moderation for the community library." },
          ] as const
        ).map((o) => (
          <label key={o.v} style={{ display: "flex", gap: "var(--ic-spacing-2)", alignItems: "baseline", padding: "var(--ic-spacing-1) 0" }}>
            <input type="radio" name="visibility" value={o.v} checked={visibility === o.v} onChange={() => setVisibility(o.v)} />
            <span>
              <strong style={{ fontWeight: "var(--ic-font-weight-medium)" }}>{o.label}</strong>{" "}
              <span style={{ color: "var(--ic-color-neutral-600)", fontSize: "var(--ic-font-size-bodySm)" }}>{o.hint}</span>
            </span>
          </label>
        ))}
      </fieldset>
      {error && (
        <p role="alert" style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-semantic-danger-light)", margin: 0 }}>
          {error}
        </p>
      )}
      <div className="ic-btn-row">
        <button type="submit" className="ic-btn ic-btn--primary" disabled={busy} style={{ width: "fit-content" }}>
          {busy ? "Saving…" : visibility === "private" ? "Save privately" : "Save & submit for review"}
        </button>
        <a href="/app/community" className="ic-btn ic-btn--text">Cancel</a>
      </div>
    </form>
  );
}
