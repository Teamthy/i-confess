"use client";

/**
 * /app/session-builder — composes a real session via POST /sessions.
 *
 * The builder assembles choices; the server decides everything else. Length
 * is capped by the plan (GET /entitlements max_session_seconds) and shown as
 * such, but that read is presentational: a POST past the cap still answers
 * 402, and the form renders that answer with a route to /app/subscription
 * rather than pretending the client could be trusted to enforce it.
 *
 * Engine failures come back as 422 with machine codes (CONTENT_UNAVAILABLE,
 * VOICE_UNAVAILABLE, EXACT_DURATION_UNAVAILABLE, DURATION_OUT_OF_BOUNDS) and
 * each gets the specific retry the code implies — a different length, or the
 * BALANCED strategy — not a generic "try again".
 */

import { useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth-context";
import { appApi, useApiData } from "@/lib/app-api";
import { ANALYTICS_EVENTS, track } from "@/lib/analytics";
import type { Category, Voice } from "@/lib/api";
import { ErrorBlock, LoadingBlock, SignedOut } from "@/components/app-ui";

type Entitlements = {
  plan: string;
  max_session_seconds: number;
  premium_voices: boolean;
};

type CreatedSession = {
  id: string;
  status: string;
  actual_duration: number;
  target_duration: number;
};

const PRESETS: { label: string; seconds: number; preset: string }[] = [
  { label: "10 min", seconds: 600, preset: "10m" },
  { label: "15 min", seconds: 900, preset: "15m" },
  { label: "30 min", seconds: 1800, preset: "30m" },
  { label: "45 min", seconds: 2700, preset: "45m" },
  { label: "60 min", seconds: 3600, preset: "60m" },
  { label: "90 min", seconds: 5400, preset: "90m" },
];

const STRATEGIES = [
  { value: "BALANCED", label: "Balanced — best fit near the length" },
  { value: "CLOSEST", label: "Closest to the requested length" },
  { value: "UNDER", label: "Stay under the length" },
  { value: "OVER", label: "Go over rather than shorten" },
  { value: "EXACT", label: "Exact — refuse rather than adjust" },
] as const;

export function SessionBuilderClient({ initialCategoryIds = [] }: { initialCategoryIds?: string[] }) {
  const router = useRouter();
  const { token, loading: authLoading } = useAuth();
  const cats = useApiData<Category[]>(token, "/categories");
  const voices = useApiData<Voice[]>(token, "/voices");
  const ent = useApiData<Entitlements>(token, "/entitlements");

  const [selected, setSelected] = useState<string[]>(initialCategoryIds);
  const [seconds, setSeconds] = useState(900); // 15 min: the free plan cap; longer asks exercise the 402 path
  const [strategy, setStrategy] = useState<string>("BALANCED");
  const [voiceId, setVoiceId] = useState("");
  const [title, setTitle] = useState("");
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<{ message: string; code: string } | null>(null);

  const maxSeconds = ent.data?.max_session_seconds ?? 3600;
  const activeVoices = useMemo(
    () => (voices.data ?? []).filter((v) => v.status === "active"),
    [voices.data],
  );

  const toggle = (id: string) =>
    setSelected((s) => (s.includes(id) ? s.filter((x) => x !== id) : [...s, id]));

  async function build(
    override?: { seconds?: number; strategy?: string },
  ) {
    if (!token || selected.length === 0) return;
    const secs = override?.seconds ?? seconds;
    const strat = override?.strategy ?? strategy;
    setBusy(true);
    setFailure(null);
    const body: Record<string, unknown> = {
      category_ids: selected,
      duration_seconds: secs,
      strategy: strat,
    };
    if (voiceId) body.voice_id = voiceId;
    if (title.trim()) body.title = title.trim();

    const res = await appApi<CreatedSession>("/sessions", { token, method: "POST", body });
    setBusy(false);
    if (!res.ok) {
      setFailure({ message: res.message, code: res.code });
      return;
    }
    router.push(`/app/player?session=${encodeURIComponent(res.data.id)}`);
  }

  if (authLoading) return <LoadingBlock rows={3} />;
  if (!token) return <SignedOut next="/app/session-builder" />;
  if (cats.error && !cats.data) return <ErrorBlock message={cats.error.message} onRetry={cats.reload} />;

  const categories = cats.data ?? [];
  const byId = new Map(categories.map((c) => [c.id, c]));
  const chosen = selected.map((id) => byId.get(id)).filter(Boolean) as Category[];

  return (
    <div className="ia-settings" style={{ maxWidth: "40rem" }}>
      <div className="ia-settings__group">
        <h2>Areas of life</h2>
        <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", margin: "0 0 var(--ic-spacing-3)" }}>
          Choose one or more. The session draws from all of them.
        </p>
        {cats.loading && <LoadingBlock rows={2} />}
        <div className="ic-chipgrid" role="group" aria-label="Categories for this session">
          {categories.map((c) => (
            <button
              key={c.id}
              type="button"
              className="ic-chipgrid__chip"
              aria-pressed={selected.includes(c.id)}
              onClick={() => toggle(c.id)}
            >
              {c.name}
            </button>
          ))}
        </div>
      </div>

      <div className="ia-settings__group">
        <h2>Length</h2>
        <div className="ic-chipgrid" role="group" aria-label="Session length">
          {PRESETS.map((p) => {
            const overPlan = p.seconds > maxSeconds;
            return (
              <button
                key={p.preset}
                type="button"
                className="ic-chipgrid__chip"
                aria-pressed={seconds === p.seconds}
                onClick={() => setSeconds(p.seconds)}
                title={overPlan ? "Above your plan's limit — the server will confirm" : undefined}
              >
                {p.label}
                {overPlan ? " · Premium" : ""}
              </button>
            );
          })}
        </div>
        {ent.data && (
          <p style={{ fontSize: "var(--ic-font-size-caption)", color: "var(--ic-color-neutral-500)", margin: "var(--ic-spacing-3) 0 0" }}>
            Your plan allows sessions up to {Math.floor(maxSeconds / 60)} minutes. Audio is never cut
            mid-confession — if complete confessions cannot land exactly on the length, the strategy
            below decides what happens instead.
          </p>
        )}
      </div>

      <div className="ia-settings__group">
        <h2>Voice &amp; assembly</h2>
        <div className="ic-field">
          <label htmlFor="sb-voice">Voice</label>
          <select id="sb-voice" value={voiceId} onChange={(e) => setVoiceId(e.target.value)}>
            <option value="">No preference (server chooses)</option>
            {activeVoices.map((v) => (
              <option key={v.id} value={v.id}>
                {v.name}
                {v.premium ? " · Premium" : ""}
              </option>
            ))}
          </select>
        </div>
        <div className="ic-field">
          <label htmlFor="sb-strategy">Length strategy</label>
          <select id="sb-strategy" value={strategy} onChange={(e) => setStrategy(e.target.value)}>
            {STRATEGIES.map((s) => (
              <option key={s.value} value={s.value}>
                {s.label}
              </option>
            ))}
          </select>
        </div>
        <div className="ic-field">
          <label htmlFor="sb-title">Name this session (optional)</label>
          <input
            id="sb-title"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="Morning healing, Lent evening…"
            maxLength={120}
          />
        </div>
      </div>

      {failure && (
        <div className="ic-state" role="alert" style={{ borderColor: "var(--ic-color-semantic-danger-light)" }}>
          <h3>The session couldn&rsquo;t be built</h3>
          <p>{failure.message}</p>
          <div className="ic-btn-row">
            {(failure.code === "EXACT_DURATION_UNAVAILABLE" || failure.code === "DURATION_OUT_OF_BOUNDS") && (
              <button
                type="button"
                className="ic-btn ic-btn--secondary"
                onClick={() => {
                  setStrategy("BALANCED");
                  setSeconds(1800);
                  void build({ strategy: "BALANCED", seconds: 900 });
                }}
              >
                Retry, balanced at 15 min
              </button>
            )}
            {failure.code === "ENTITLEMENT_REQUIRED" || failure.code === "PLAN_LIMIT" || failure.code === "HTTP_402" ? (
              <a href="/app/subscription" className="ic-btn ic-btn--primary">See Premium</a>
            ) : (
              <button type="button" className="ic-btn ic-btn--secondary" onClick={() => void build()}>
                Try again
              </button>
            )}
          </div>
        </div>
      )}

      <div style={{ display: "flex", alignItems: "center", gap: "var(--ic-spacing-4)", flexWrap: "wrap" }}>
        <button
          type="button"
          className="ic-btn ic-btn--primary"
          disabled={busy || selected.length === 0}
          onClick={() => void build()}
          style={{ width: "fit-content" }}
        >
          {busy ? "Assembling…" : "Build session"}
        </button>
        <span style={{ fontSize: "var(--ic-font-size-caption)", color: "var(--ic-color-neutral-500)" }}>
          {chosen.length === 0
            ? "Choose at least one area of life."
            : `${chosen.length} ${chosen.length === 1 ? "area" : "areas"} · ${Math.round(seconds / 60)} min`}
        </span>
      </div>
    </div>
  );
}
