"use client";

/**
 * /app/community — the approved feed, real.
 *
 * GET /community/feed is a public endpoint (approved, published posts only;
 * author identity is stripped server-side — nothing here can display an
 * author, because the payload does not contain one). Reactions, however, are
 * an authenticated act, so the buttons render for everyone but the ones that
 * need a session explain that rather than failing with a 401 toast.
 *
 * This replaced the earlier server-side `api.communityFeed()` call, which
 * fetched once at build/revalidate time and rendered a frozen list: the feed
 * is exactly the surface that should not be frozen.
 */

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth-context";
import { appApi, formatDate } from "@/lib/app-api";
import { ErrorBlock, LoadingBlock } from "@/components/app-ui";

type FeedPost = {
  id: string;
  body: string;
  visibility: string;
  status: string;
  created_at: string;
};

type FeedResponse = { posts: FeedPost[] | null };

export function CommunityFeedClient() {
  const { token } = useAuth();
  const router = useRouter();
  const [posts, setPosts] = useState<FeedPost[] | null>(null);
  const [error, setError] = useState("");
  const [busyId, setBusyId] = useState("");
  const [reacted, setReacted] = useState<Record<string, string>>({});
  const [nonce, setNonce] = useState(0);

  useEffect(() => {
    const ac = new AbortController();
    setError("");
    appApi<FeedResponse>("/community/feed", { signal: ac.signal }).then((r) => {
      if (ac.signal.aborted) return;
      if (r.ok) setPosts(r.data.posts ?? []);
      else setError(r.message);
    });
    return () => ac.abort();
  }, [nonce]);

  const react = useCallback(
    async (postId: string, reaction: string) => {
      if (!token) {
        router.push("/login?next=" + encodeURIComponent("/app/community"));
        return;
      }
      setBusyId(postId + reaction);
      setError("");
      const r = await appApi(`/community/posts/${encodeURIComponent(postId)}/react`, {
        token,
        method: "POST",
        body: { reaction },
      });
      setBusyId("");
      if (r.ok) setReacted((m) => ({ ...m, [postId]: reaction }));
      else setError(r.message);
    },
    [token, router],
  );

  if (error && posts === null) return <ErrorBlock message={error} onRetry={() => setNonce((n) => n + 1)} />;
  if (posts === null) return <LoadingBlock rows={3} />;

  return (
    <div className="ic-feed">
      {posts.length === 0 && (
        <div className="ic-state">
          <h3>Nothing has been approved yet</h3>
          <p>
            Only posts a moderator has approved appear here. That&rsquo;s the point: this feed is
            evidence of care, not volume.
          </p>
        </div>
      )}
      {posts.map((p) => (
        <article key={p.id} className="ic-feed__post ic-card">
          <p className="ic-feed__body">{p.body}</p>
          <div className="ic-feed__foot">
            <span className="ic-feed__time">
              {formatDate(p.created_at)}
              {p.visibility !== "public" ? ` · ${p.visibility}` : ""}
            </span>
            <div className="ic-feed__reacts" role="group" aria-label="React to this post">
              {(["amen", "heart", "pray"] as const).map((kind) => (
                <button
                  key={kind}
                  type="button"
                  className="ic-feed__react"
                  disabled={busyId === p.id + kind}
                  aria-pressed={reacted[p.id] === kind}
                  onClick={() => void react(p.id, kind)}
                >
                  {kind === "amen" ? "Amen" : kind === "heart" ? "♥" : "🙏"}
                </button>
              ))}
            </div>
          </div>
        </article>
      ))}
      {error && posts.length > 0 && (
        <p role="alert" style={{ color: "var(--ic-color-semantic-danger-light)", fontSize: "var(--ic-font-size-bodySm)" }}>
          {error}
        </p>
      )}
    </div>
  );
}
