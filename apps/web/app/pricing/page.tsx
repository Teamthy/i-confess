"use client";
// Pricing — never hard-codes amounts. Every display comes from GET /v1/subscriptions/plans.
// Regional currencies: NGN / USD / GBP / EUR / PHP (§6.1)

import { useEffect, useState } from "react";

type Amount = { currency: string; minor: number; display: string };
type Plan = {
  id: string;
  name: string;
  description: string;
  interval: string;
  trial_days: number;
  prices: Record<string, Amount>;
  features: string[];
};

const FALLBACK_CURRENCY = "NGN";

export default function PricingPage() {
  const [plans, setPlans] = useState<Plan[]>([]);
  const [currencies, setCurrencies] = useState<string[]>(["NGN", "USD", "GBP", "EUR", "PHP"]);
  const [currency, setCurrency] = useState(FALLBACK_CURRENCY);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const api = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        setLoading(true);
        setError("");
        const res = await fetch(`${api}/v1/subscriptions/plans`, { cache: "no-store" });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const data = await res.json();
        if (cancelled) return;
        if (Array.isArray(data.plans)) setPlans(data.plans);
        if (Array.isArray(data.currencies) && data.currencies.length) {
          setCurrencies(data.currencies);
          if (!data.currencies.includes(currency)) setCurrency(data.currencies[0]);
        }
      } catch (e: any) {
        if (!cancelled) setError(e?.message || "Unable to load pricing");
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => { cancelled = true; };
  }, [api]);

  const formatInterval = (iv: string) => (iv === "year" ? "year" : iv === "week" ? "week" : "month");

  return (
    <main className="min-h-screen bg-[#0f1220] text-[#e8eaf6]">
      <div className="max-w-5xl mx-auto px-6 py-14">
        <p className="text-[11px] tracking-[0.14em] uppercase text-[#9aa1c0]">Pricing · Regional · No hard-coded amounts</p>
        <h1 className="font-serif text-3xl md:text-4xl mt-3">Choose your <span className="text-[#e8c67a]">confession rhythm</span></h1>
        <p className="text-[#9aa1c0] mt-3 max-w-2xl text-sm leading-relaxed">
          Every price is fetched live from the server. No amount is hard-coded in the app — the DB is the single source for NGN, USD, GBP, EUR and PHP.
          Trial: 7 days · Cancel anytime · Offline licenced · Premium voices.
        </p>

        <div className="mt-6 flex items-center gap-3">
          <label className="text-xs text-[#9aa1c0]">Currency</label>
          <div className="flex gap-1.5 flex-wrap">
            {currencies.map((c) => (
              <button
                key={c}
                onClick={() => setCurrency(c)}
                className={`px-3 py-1.5 rounded-full text-xs font-semibold border ${currency === c ? "bg-[#e8c67a] text-[#241b05] border-[#e8c67a]" : "bg-[#1c2138] border-[#2d3350] text-[#9aa1c0]"}`}
              >
                {c}
              </button>
            ))}
          </div>
          <span className="text-[11px] text-[#6b7280] ml-2">Live from <code className="bg-[#1c2138] px-1.5 py-0.5 rounded">GET /v1/subscriptions/plans</code></span>
        </div>

        {error && (
          <div className="mt-6 bg-[#2a1a1a] border border-[#553333] rounded-xl p-4 text-sm text-[#ffb4b4]">{error}</div>
        )}

        {loading ? (
          <div className="mt-8 grid md:grid-cols-2 gap-4">
            {[0, 1].map((i) => (
              <div key={i} className="bg-[#1c2138] border border-[#2d3350] rounded-2xl p-6 animate-pulse h-64" />
            ))}
          </div>
        ) : (
          <div className="mt-8 grid md:grid-cols-2 gap-4">
            {plans.map((p) => {
              const amt = p.prices?.[currency] ?? Object.values(p.prices ?? {})[0];
              const isAnnual = p.id === "annual" || p.interval === "year";
              return (
                <div
                  key={p.id}
                  className={`rounded-2xl border p-6 flex flex-col ${isAnnual ? "bg-[#1f2340] border-[#e8c67a]/40 shadow-[0_0_0_1px_rgba(232,198,122,0.15)]" : "bg-[#1c2138] border-[#2d3350]"}`}
                >
                  {isAnnual && <span className="text-[11px] tracking-wide uppercase bg-[#e8c67a] text-[#241b05] px-2 py-1 rounded-full self-start font-semibold">Best value · Save ~33%</span>}
                  <h3 className="font-serif text-xl mt-3">{p.name}</h3>
                  {p.description && <p className="text-xs text-[#9aa1c0] mt-1">{p.description}</p>}
                  <div className="mt-4 flex items-baseline gap-2">
                    <span className="text-3xl font-bold">{amt ? amt.display : "—"}</span>
                    <span className="text-sm text-[#9aa1c0]">/ {formatInterval(p.interval)}</span>
                  </div>
                  <p className="text-[11px] text-[#6b7280] mt-1">{amt ? `${amt.currency} · ${amt.minor} minor` : ""} · {p.trial_days} day trial</p>

                  <ul className="mt-4 space-y-1.5 text-sm">
                    {(p.features || []).map((f) => (
                      <li key={f} className="flex gap-2 text-[#c7ccdf]"><span className="text-[#7c8cf8]">✓</span>{f.replace(/_/g, " ")}</li>
                    ))}
                    <li className="flex gap-2 text-[#c7ccdf]"><span className="text-[#7c8cf8]">✓</span>39 categories · schedule & offline licence</li>
                  </ul>

                  <div className="mt-6 flex gap-2">
                    <a
                      href={`/checkout?plan=${encodeURIComponent(p.id)}&currency=${currency}`}
                      className={`flex-1 text-center rounded-lg px-4 py-3 text-sm font-semibold ${isAnnual ? "bg-[#e8c67a] text-[#241b05]" : "bg-[#7c8cf8] text-[#0b0e1c]"}`}
                    >
                      Start 7-day trial
                    </a>
                    <a href="/t/healing" className="border border-[#2d3350] rounded-lg px-4 py-3 text-sm text-center">
                      Preview
                    </a>
                  </div>
                  <p className="text-[11px] text-[#6b7280] mt-3">Price shown for {currency}. Change currency above — amounts come from the server, not the build.</p>
                </div>
              );
            })}
          </div>
        )}

        <div className="mt-10 bg-[#121632] border border-[#2d3350] rounded-xl p-4 text-xs text-[#9aa1c0] leading-relaxed">
          <b className="text-[#e8eaf6]">How we keep pricing honest:</b> The web and mobile never embed an amount literal. They call <code className="bg-[#1c2138] px-1 py-0.5 rounded">GET /v1/subscriptions/plans</code> and render <code className="bg-[#1c2138] px-1 py-0.5 rounded">prices[currency].display</code>. Admin updates via <code className="bg-[#1c2138] px-1 py-0.5 rounded">PUT /v1/admin/plans</code> (DB <code className="bg-[#1c2138] px-1 py-0.5 rounded">subscription_plans</code>), no redeploy. Receipt verification is server-side (<code className="bg-[#1c2138] px-1 py-0.5 rounded">POST /v1/subscriptions/verify</code> with <code className="bg-[#1c2138] px-1 py-0.5 rounded">BILLING_VERIFIER=apple|google|chained</code>).
        </div>

        <footer className="border-t border-[#2d3350] mt-10 pt-6 text-xs text-[#6b7294] flex gap-4">
          <a href="/">Home</a><a href="/privacy">Privacy</a><a href="/terms">Terms</a><span className="ml-auto">NGN · USD · GBP · EUR · PHP — server is the source</span>
        </footer>
      </div>
    </main>
  );
}
