"use client";
// Session preview simulator §97 — 10 / 30 / 60 min previews without persistence
import { useState } from "react";

type Preview = { total_items?: number; duration_seconds?: number; items?: any[]; queue?: any[] } | null;

export default function PreviewPage() {
  const [preset, setPreset] = useState<"10m" | "30m" | "60m">("30m");
  const [cats, setCats] = useState("healing,peace");
  const [voice, setVoice] = useState("");
  const [loading, setLoading] = useState(false);
  const [data, setData] = useState<Preview>(null);
  const [error, setError] = useState("");

  const api = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";
  const token = process.env.NEXT_PUBLIC_ADMIN_TOKEN || ""; // dev only

  async function run() {
    setLoading(true);
    setError("");
    setData(null);
    try {
      const res = await fetch(`${api}/v1/sessions/preview`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
        },
        body: JSON.stringify({
          category_ids: cats.split(",").map((s) => s.trim()).filter(Boolean),
          duration_preset: preset,
          voice_id: voice || undefined,
        }),
      });
      const j = await res.json();
      if (!res.ok) throw new Error(j?.msg || j?.error || `HTTP ${res.status}`);
      setData(j);
    } catch (e: any) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  }

  const items: any[] = (data as any)?.items || (data as any)?.queue || [];
  const total = (data as any)?.total_items ?? (data as any)?.duration_seconds ?? items.length;

  return (
    <main className="min-h-screen bg-[#0f1220] text-[#e8eaf6] p-6">
      <div className="max-w-3xl mx-auto">
        <p className="text-[11px] tracking-[0.14em] uppercase text-[#9aa1c0]">Admin · §97</p>
        <h1 className="font-serif text-2xl mt-2">Session preview simulator</h1>
        <p className="text-sm text-[#9aa1c0] mt-1">Dry-run <code className="bg-[#1c2138] px-1 py-0.5 rounded">POST /v1/sessions/preview</code> — no persistence, entitlement-checked, 10/30/60 min.</p>

        <div className="bg-[#1c2138] border border-[#2d3350] rounded-xl p-4 mt-4 space-y-3">
          <div className="flex gap-2">
            {(["10m", "30m", "60m"] as const).map((p) => (
              <button key={p} onClick={() => setPreset(p)} className={`px-3 py-1.5 rounded-full text-xs font-semibold border ${preset === p ? "bg-[#e8c67a] text-[#241b05] border-[#e8c67a]" : "bg-[#0f1220] border-[#2d3350] text-[#9aa1c0]"}`}>
                {p}
              </button>
            ))}
          </div>
          <label className="block text-xs text-[#9aa1c0]">category_ids (comma-separated, 39 backend-controlled)</label>
          <input value={cats} onChange={(e) => setCats(e.target.value)} className="w-full bg-[#0f1220] border border-[#2d3350] rounded-lg px-3 py-2 text-sm" placeholder="healing,peace" />
          <label className="block text-xs text-[#9aa1c0]">voice_id (optional)</label>
          <input value={voice} onChange={(e) => setVoice(e.target.value)} className="w-full bg-[#0f1220] border border-[#2d3350] rounded-lg px-3 py-2 text-sm" placeholder="voice_..." />
          <button onClick={run} disabled={loading} className="w-full bg-[#7c8cf8] text-[#0b0e1c] font-semibold rounded-lg px-4 py-2.5 text-sm disabled:opacity-50">
            {loading ? "Running…" : `Preview ${preset}`}
          </button>
          <p className="text-[11px] text-[#6b7280]">In admin, this is authed; standalone it needs <code className="bg-[#0f1220] px-1 py-0.5 rounded">NEXT_PUBLIC_ADMIN_TOKEN</code> for dev.</p>
        </div>

        {error && <div className="mt-4 bg-[#2a1a1a] border border-[#553333] rounded-xl p-3 text-sm text-[#ffb4b4]">{error}</div>}
        {data && !error && (
          <div className="mt-4 bg-[#1c2138] border border-[#2d3350] rounded-xl p-4">
            <div className="text-sm font-semibold">Result · {total} {typeof total === "number" && total > 60 ? "seconds / items" : "items"}</div>
            <pre className="text-xs text-[#9aa1c0] mt-2 whitespace-pre-wrap break-all max-h-96 overflow-auto">{JSON.stringify(data, null, 2)}</pre>
            {items.length > 0 && <p className="text-[11px] text-[#6b7280] mt-2">{items.length} queue items (confession + voice resolution, entitlement-filtered).</p>}
          </div>
        )}
      </div>
    </main>
  );
}
