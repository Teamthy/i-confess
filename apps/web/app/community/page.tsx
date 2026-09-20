"use client";
import { useCallback, useEffect, useState } from "react";

type Post = { id: string; body: string; created_at: string };
const api = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

export default function CommunityPage() {
  const [posts, setPosts] = useState<Post[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const load = useCallback(async () => {
    setLoading(true); setError("");
    try {
      const response = await fetch(`${api}/v1/community/feed`, { cache: "no-store" });
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const payload = await response.json();
      setPosts(Array.isArray(payload.posts) ? payload.posts : []);
    } catch (_) { setError("Community stories could not be loaded."); }
    finally { setLoading(false); }
  }, []);
  useEffect(() => { void load(); }, [load]);

  return <main className="min-h-screen bg-[#0f1220] text-[#e8eaf6] px-6 py-12">
    <div className="max-w-3xl mx-auto">
      <p className="text-xs uppercase tracking-widest text-[#9aa1c0]">Moderated · Anonymous by design</p>
      <h1 className="font-serif text-4xl mt-2 text-[#e8c67a]">Community stories</h1>
      <p className="text-[#9aa1c0] mt-3">Only posts approved for public sharing appear here. Author account identifiers are never returned by this API.</p>
      {loading && <div role="status" className="mt-10">Loading community stories…</div>}
      {error && <div role="alert" className="mt-10 text-[#ffb4b4]">{error} <button className="underline" onClick={() => void load()}>Try again</button></div>}
      {!loading && !error && posts.length === 0 && <p className="mt-10">No approved stories yet.</p>}
      <div className="mt-8 grid gap-4">{posts.map(post => <article key={post.id} className="rounded-xl border border-[#2d3350] bg-[#1c2138] p-5">
        <p className="leading-relaxed whitespace-pre-wrap">{post.body}</p>
        <time className="block mt-4 text-xs text-[#9aa1c0]" dateTime={post.created_at}>{new Date(post.created_at).toLocaleDateString()}</time>
      </article>)}</div>
    </div>
  </main>;
}
