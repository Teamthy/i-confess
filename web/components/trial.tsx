"use client";
/* ============================================================
   I CONFESS web — the seven-day trial, from the real API.

   GET  /subscriptions/trial        state, today's journey day, completed days
   POST /subscriptions/trial        start the one-time trial
   POST /subscriptions/trial/today  build today's session(s) — guided: the
                                    server builds only when the listener taps

   Nothing here decides entitlement or invents a plan: the server does, and
   the card re-reads the trial after every action.
   ============================================================ */
import React, { useState } from "react";
import { useRouter } from "next/navigation";
import { Icon } from "./ui";
import { useAuth } from "@/lib/auth-context";
import { appApi, newIdempotencyKey, useApiData } from "@/lib/app-api";
import { mutate, track } from "@/lib/store";
import { useToast } from "@/lib/ui";

type TrialDay = { day: number; intent: string; title: string; description: string; cta: string; duration_seconds?: number };
type TrialState = "ELIGIBLE" | "ACTIVE" | "EXPIRING" | "EXPIRED" | "CONVERTED";
type TrialResponse = { state: TrialState; current_day: number; today: TrialDay | null; completed_days: number[] };
type TodayResponse = {
  day: number;
  action: "play" | "review" | "open_builder";
  sessions: { slot: number; label: string; session: { id: string; title?: string } }[];
  builder?: { category_slugs?: string[]; duration_seconds?: number };
  premium_voice_available?: boolean;
};

const running = (s?: TrialState) => s === "ACTIVE" || s === "EXPIRING";

/** The trial on Home and Subscription. `compact` hides the eligible pitch on Home. */
export function TrialCard({ compact }: { compact?: boolean }) {
  const auth = useAuth();
  const trial = useApiData<TrialResponse>(auth.token, "/subscriptions/trial");
  const router = useRouter();
  const toast = useToast();
  const [busy, setBusy] = useState(false);
  const [sessions, setSessions] = useState<TodayResponse["sessions"]>([]);

  if (!auth.token || trial.loading || !trial.data) {
    if (trial.error && !compact) return <div className="feature-notice" role="alert"><p>{trial.error.message}</p><button className="btn btn-ghost btn-sm" onClick={trial.reload}>Retry</button></div>;
    return null;
  }
  const { state, today, current_day: day, completed_days: done } = trial.data;

  const start = async () => {
    setBusy(true);
    const r = await appApi<{ state: TrialState }>("/subscriptions/trial", { token: auth.token, method: "POST", idempotencyKey: newIdempotencyKey() });
    setBusy(false);
    if (!r.ok) { toast(r.message); return; }
    mutate((s) => { if (s.user) s.user.plan = "trial"; });
    track("trial_started");
    toast("Your seven days start today");
    trial.reload();
  };

  const startToday = async () => {
    setBusy(true);
    const r = await appApi<TodayResponse>("/subscriptions/trial/today", { token: auth.token, method: "POST", idempotencyKey: newIdempotencyKey() });
    setBusy(false);
    if (!r.ok) {
      toast(r.code === "TRIAL_DAY_PREPARING" ? "Today's session is still being prepared — try again in a moment." : r.message);
      return;
    }
    const t = r.data;
    track("trial_day_started", { day: t.day, action: t.action });
    if (t.action === "open_builder") {
      const cat = t.builder?.category_slugs?.[0];
      const min = Math.round((t.builder?.duration_seconds || 900) / 60);
      router.push(`/app/session-builder?${new URLSearchParams({ ...(cat ? { cat } : {}), min: String(min) })}`);
      return;
    }
    if (t.premium_voice_available === false) toast("No Premium voice is ready yet — today uses the standard voice.");
    if (t.sessions.length === 1) { router.push(`/app/player?session=${encodeURIComponent(t.sessions[0].session.id)}`); return; }
    setSessions(t.sessions);
  };

  if (state === "ELIGIBLE") {
    if (compact) return null;
    return (
      <div className="trial-card rv in">
        <div>
          <span className="eyebrow on-dark">Premium trial</span>
          <h3 className="h3">Seven days, one practice a day.</h3>
          <p>Each day opens one thing: your first confession, a morning and evening pair, a session made from your areas, a longer sit, a Premium voice, your own build, and a look back at the week.</p>
        </div>
        <button className="btn btn-light" onClick={start} disabled={busy}>{busy ? "Starting…" : "Start 7-day trial"}</button>
      </div>
    );
  }
  if (!running(state) || !today) return null;

  return (
    <div className="trial-card rv in" aria-live="polite">
      <div>
        <span className="eyebrow on-dark">Trial · Day {day} of 7</span>
        <h3 className="h3">{today.title}</h3>
        <p>{today.description}</p>
        <ol className="trial-days" aria-label="Your seven days">
          {[1, 2, 3, 4, 5, 6, 7].map((d) => (
            <li key={d} className={(done.includes(d) ? "done" : "") + (d === day ? " today" : "")} aria-label={`Day ${d}${done.includes(d) ? ", completed" : d === day ? ", today" : ""}`}>{done.includes(d) ? <Icon n="check" s={12} /> : d}</li>
          ))}
        </ol>
      </div>
      {sessions.length > 1 ? (
        <div className="trial-sessions">
          {sessions.map((s) => (
            <button key={s.slot} className="btn btn-light" onClick={() => router.push(`/app/player?session=${encodeURIComponent(s.session.id)}`)}>
              <Icon n="play" s={13} /> {s.label || s.session.title}
            </button>
          ))}
        </div>
      ) : (
        <button className="btn btn-light" onClick={startToday} disabled={busy}>{busy ? "Preparing…" : today.cta}</button>
      )}
    </div>
  );
}
