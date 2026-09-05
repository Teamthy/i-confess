// Admin Dashboard §96 — users, sessions, plays, completion, subs, revenue, retention, popular, moderation queue, system health
// Fetches from /v1/admin/* (admin role required). No PII exfil beyond need.

async function fetchAdmin(path: string) {
  const api = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";
  // In dev, admin token is injected via NEXT_PUBLIC_ADMIN_TOKEN; prod uses httpOnly cookie
  const token = process.env.NEXT_PUBLIC_ADMIN_TOKEN || "";
  const headers: Record<string, string> = { Accept: "application/json" };
  if (token) headers["Authorization"] = `Bearer ${token}`;
  const r = await fetch(`${api}${path}`, { headers, cache: "no-store" });
  if (!r.ok) return null;
  return r.json();
}

export const metadata = { title: "Admin — Dashboard · I CONFESS" };

export default async function DashboardPage() {
  const [stats, metrics, queue] = await Promise.all([
    fetchAdmin("/v1/admin/stats"),
    fetchAdmin("/v1/admin/metrics"),
    fetchAdmin("/v1/admin/moderation/queue"),
  ]);

  const s = stats || { users: "—", sessions: "—", plays: "—", completion_rate: "—", subs: "—", revenue: "—" };
  const m = metrics as any;
  const q: any[] | null = Array.isArray(queue) ? queue : (queue as any)?.items ?? null;

  const cards = [
    { k: "Users", v: s.users ?? s.total_users ?? "—" },
    { k: "Sessions", v: s.sessions ?? s.total_sessions ?? "—" },
    { k: "Plays", v: s.plays ?? s.total_plays ?? "—" },
    { k: "Completion", v: s.completion_rate ?? s.completion ?? "—" },
    { k: "Subs", v: s.subs ?? s.subscriptions ?? "—" },
    { k: "Revenue", v: s.revenue ?? "—" },
  ];

  return (
    <main className="min-h-screen bg-[#0f1220] text-[#e8eaf6] p-6">
      <div className="max-w-6xl mx-auto">
        <p className="text-[11px] tracking-[0.14em] uppercase text-[#9aa1c0]">Admin · §96</p>
        <h1 className="font-serif text-3xl mt-2">Dashboard</h1>
        <p className="text-sm text-[#9aa1c0] mt-1">Live from <code className="bg-[#1c2138] px-1 py-0.5 rounded">GET /v1/admin/stats</code> · <code>/v1/admin/metrics</code> · <code>/v1/admin/moderation/queue</code></p>

        <div className="grid sm:grid-cols-3 lg:grid-cols-6 gap-3 mt-6">
          {cards.map((c) => (
            <div key={c.k} className="bg-[#1c2138] border border-[#2d3350] rounded-xl p-4">
              <div className="text-[11px] uppercase tracking-wide text-[#9aa1c0]">{c.k}</div>
              <div className="text-xl font-bold mt-1">{String(c.v)}</div>
            </div>
          ))}
        </div>

        <div className="grid md:grid-cols-2 gap-4 mt-6">
          <div className="bg-[#1c2138] border border-[#2d3350] rounded-xl p-4">
            <h2 className="font-semibold text-sm">System health</h2>
            <pre className="text-xs text-[#9aa1c0] mt-2 whitespace-pre-wrap break-all">{m ? JSON.stringify(m, null, 2) : "No metrics (admin only)"}</pre>
            <a href="/admin/preview" className="inline-block mt-3 text-xs bg-[#7c8cf8] text-[#0b0e1c] px-3 py-1.5 rounded-lg font-semibold">Preview simulator §97 →</a>
          </div>
          <div className="bg-[#1c2138] border border-[#2d3350] rounded-xl p-4">
            <h2 className="font-semibold text-sm">Moderation queue</h2>
            {!q ? (
              <p className="text-xs text-[#9aa1c0] mt-2">Queue requires admin auth. Sign in via <code>/admin/</code> then reload.</p>
            ) : q.length === 0 ? (
              <p className="text-xs text-[#9aa1c0] mt-2">Empty — no UGC pending.</p>
            ) : (
              <ul className="mt-2 space-y-2">
                {q.slice(0, 6).map((it: any, i: number) => (
                  <li key={i} className="text-xs border border-[#2d3350] rounded-lg p-2">
                    <span className="text-[#e8c67a]">{it.status || it.state || "open"}</span> · <span className="text-[#9aa1c0]">{it.entity_type || it.type}:{String(it.entity_id || it.id).slice(0, 8)}</span>
                  </li>
                ))}
              </ul>
            )}
            <p className="text-[11px] text-[#6b7280] mt-3">UGC never auto-published as system content (§10). Reject needs reason, audit logged.</p>
          </div>
        </div>

        <div className="mt-6 bg-[#121632] border border-[#2d3350] rounded-xl p-4 text-xs text-[#9aa1c0] leading-relaxed">
          Popular content/voices, retention, scheduled usage, trial→paid — from <code className="bg-[#1c2138] px-1 py-0.5 rounded">/v1/admin/stats</code> (to be enriched with analytics batch retention). Cache hit-rate & trace ID via <code>/metrics</code> + <code>X-Trace-Id</code>.
        </div>
      </div>
    </main>
  );
}
