"use client";
import { useCallback, useEffect, useState } from "react";

type Post = { id: string; body: string; created_at: string };
type Confession = { id: string; title: string; text: string; published_at?: string; created_at: string };
const api = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

export default function CommunityPage() {
  const [posts, setPosts] = useState<Post[]>([]);
  const [confessions, setConfessions] = useState<Confession[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const load = useCallback(async () => {
    setLoading(true); setError("");
    try {
      const [feedRes, confRes] = await Promise.all([
        fetch(`${api}/v1/community/feed`, { cache: "no-store" }),
        fetch(`${api}/v1/community/confessions`, { cache: "no-store" }),
      ]);
      if (!feedRes.ok) throw new Error(`feed HTTP ${feedRes.status}`);
      if (!confRes.ok) throw new Error(`confessions HTTP ${confRes.status}`);
      const feedPayload = await feedRes.json();
      const confPayload = await confRes.json();
      setPosts(Array.isArray(feedPayload.posts) ? feedPayload.posts : []);
      setConfessions(Array.isArray(confPayload.confessions) ? confPayload.confessions : []);
    } catch (_) { setError("Community stories could not be loaded."); }
    finally { setLoading(false); }
  }, []);
  useEffect(() => { void load(); }, [load]);

  return <main className="min-h-screen bg-[#0f1220] text-[#e8eaf6] px-6 py-12">
    <div className="max-w-3xl mx-auto">
      <p className="text-xs uppercase tracking-widest text-[#9aa1c0]">Moderated · Anonymous by design</p>
      <h1 className="font-serif text-4xl mt-2 text-[#e8c67a]">Community stories</h1>
      <p className="text-[#9aa1c0] mt-3">Only posts approved for public sharing appear here. Author account identifiers are never returned by this API. Published user confessions (G-40) are now readable anonymously.</p>
      {loading && <div role="status" className="mt-10">Loading community stories…</div>}
      {error && <div role="alert" className="mt-10 text-[#ffb4b4]">{error} <button className="underline" onClick={() => void load()}>Try again</button></div>}
      {!loading && !error && posts.length === 0 && confessions.length === 0 && <p className="mt-10">No approved stories yet.</p>}

      {!loading && !error && confessions.length > 0 && (
        <section className="mt-10">
          <h2 className="text-xl font-semibold text-[#e8c67a]">Testimonies</h2>
          <p className="text-sm text-[#9aa1c0] mt-1">Published confessions shared publicly by the community — anonymous, moderated, newest first.</p>
          <div className="mt-4 grid gap-4">{confessions.map(c => <article key={c.id} className="rounded-xl border border-[#2d3350] bg-[#1c2138] p-5">
            {c.title && <h3 className="font-semibold text-[#e8eaf6]">{c.title}</h3>}
            <p className="leading-relaxed whitespace-pre-wrap mt-2">{c.text}</p>
            <time className="block mt-4 text-xs text-[#9aa1c0]" dateTime={c.published_at || c.created_at}>{new Date(c.published_at || c.created_at).toLocaleDateString()}</time>
          </article>)}</div>
        </section>
      )}

      {!loading && !error && posts.length > 0 && (
        <section className="mt-10">
          <h2 className="text-xl font-semibold text-[#e8c67a]">Stories</h2>
          <div className="mt-4 grid gap-4">{posts.map(post => <article key={post.id} className="rounded-xl border border-[#2d3350] bg-[#1c2138] p-5">
            <p className="leading-relaxed whitespace-pre-wrap">{post.body}</p>
            <time className="block mt-4 text-xs text-[#9aa1c0]" dateTime={post.created_at}>{new Date(post.created_at).toLocaleDateString()}</time>
          </article>)}</div>
        </section>
      )}
    </div>
  </main>;
}
