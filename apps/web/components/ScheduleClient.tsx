"use client";

/**
 * /app/schedule — recurring session reminders.
 *
 * Speaks the real schedule endpoints (design/routes.json): GET/POST
 * /schedules, PATCH/DELETE /schedules/{id}, POST /schedules/{id}/start.
 * Shapes follow models.Schedule exactly: label + HH:MM time are required,
 * days_of_week is a list of 0–6 (Sunday-first), duration is clamped by the
 * server to the account's plan (402 renders as the plan message it is).
 *
 * "Start now" builds a session from the schedule immediately and routes to
 * the player; the schedule itself is unchanged.
 */

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth-context";
import { appApi, useApiData } from "@/lib/app-api";
import { ANALYTICS_EVENTS, track } from "@/lib/analytics";
import type { Category, Voice } from "@/lib/api";
import { EmptyState, ErrorBlock, LoadingBlock, PremiumOffer, SignedOut } from "@/components/app-ui";

type Schedule = {
  id: string;
  label: string;
  time: string;
  days_of_week: number[];
  timezone: string;
  duration_seconds: number;
  voice_id?: string;
  category_ids?: string[];
  enabled: boolean;
  created_at: string;
  updated_at: string;
};

type BuiltSession = { id: string };

const DAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

const DURATIONS = [
  { seconds: 300, label: "5 minutes" },
  { seconds: 600, label: "10 minutes" },
  { seconds: 900, label: "15 minutes" },
  { seconds: 1800, label: "30 minutes" },
  { seconds: 3600, label: "1 hour" },
];

function daysLabel(days: number[] | undefined): string {
  if (!days || days.length === 0) return "One day a week — pick days when saving";
  if (days.length === 7) return "Every day";
  const sorted = [...days].sort((a, b) => a - b);
  return sorted.map((d) => DAYS[d] ?? "").join(" · ");
}

export function ScheduleClient() {
  const { token, loading: authLoading } = useAuth();
  const router = useRouter();
  const list = useApiData<Schedule[]>(token, "/schedules");
  const cats = useApiData<Category[]>(token, "/categories");
  const voices = useApiData<Voice[]>(token, "/voices");

  const [formOpen, setFormOpen] = useState(false);
  const [label, setLabel] = useState("");
  const [time, setTime] = useState("06:30");
  const [days, setDays] = useState<number[]>([1, 2, 3, 4, 5]);
  const [duration, setDuration] = useState(900);
  const [voiceId, setVoiceId] = useState("");
  const [categoryIds, setCategoryIds] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [planBlocked, setPlanBlocked] = useState(false);
  const [startingId, setStartingId] = useState<string | null>(null);

  if (authLoading || list.loading) return <LoadingBlock rows={4} />;
  if (!token) return <SignedOut next="/app/schedule" />;
  if (list.error) return <ErrorBlock message={list.error.message} onRetry={list.reload} />;

  const schedules = list.data ?? [];
  const categories = cats.data ?? [];
  const voiceList = (voices.data ?? []).filter((v) => v.status === "active");

  function toggleDay(d: number) {
    setDays((prev) => (prev.includes(d) ? prev.filter((x) => x !== d) : [...prev, d].sort((a, b) => a - b)));
  }

  function toggleCategory(id: string) {
    setCategoryIds((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id].slice(0, 3)));
  }

  async function create(e: React.FormEvent) {
    e.preventDefault();
    if (!token || busy) return;
    setError("");
    setPlanBlocked(false);
    if (!label.trim()) {
      setError("Give this schedule a name — “Morning” is enough.");
      return;
    }
    if (!/^([01][0-9]|2[0-3]):[0-5][0-9]$/.test(time)) {
      setError("Time must be HH:MM, like 06:30.");
      return;
    }
    setBusy(true);
    const tz = (() => {
      try {
        return Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
      } catch {
        return "UTC";
      }
    })();
    const r = await appApi<Schedule>("/schedules", {
      token,
      method: "POST",
      body: {
        label: label.trim(),
        time,
        days_of_week: days,
        timezone: tz,
        duration_seconds: duration,
        voice_id: voiceId || undefined,
        category_ids: categoryIds.length > 0 ? categoryIds : undefined,
      },
    });
    setBusy(false);
    if (!r.ok) {
      if (r.status === 402) {
        setPlanBlocked(true);
        return;
      }
      setError(r.message);
      return;
    }
    track(token, ANALYTICS_EVENTS.scheduleCreated, { schedule_id: r.data.id });
    setFormOpen(false);
    setLabel("");
    setCategoryIds([]);
    list.reload();
  }

  async function setEnabled(sc: Schedule, enabled: boolean) {
    if (!token) return;
    const r = await appApi<Schedule>(`/schedules/${sc.id}`, { token, method: "PATCH", body: { enabled } });
    if (r.ok) list.reload();
    else setError(r.message);
  }

  async function remove(sc: Schedule) {
    if (!token) return;
    if (!window.confirm(`Delete “${sc.label}”? This cannot be undone.`)) return;
    const r = await appApi<{ ok: boolean }>(`/schedules/${sc.id}`, { token, method: "DELETE" });
    if (r.ok) list.reload();
    else setError(r.message);
  }

  async function startNow(sc: Schedule) {
    if (!token || startingId) return;
    setStartingId(sc.id);
    setError("");
    const r = await appApi<BuiltSession>(`/schedules/${sc.id}/start`, { token, method: "POST" });
    setStartingId(null);
    if (!r.ok) {
      if (r.status === 402) setPlanBlocked(true);
      else setError(r.message);
      return;
    }
    router.push(`/app/player?session=${encodeURIComponent(r.data.id)}`);
  }

  return (
    <div style={{ display: "grid", gap: "var(--ic-spacing-5)" }}>
      {error && (
        <p role="alert" style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-semantic-danger-light)", margin: 0 }}>
          {error}
        </p>
      )}
      {planBlocked && (
        <PremiumOffer body="That length needs a plan this account isn't on. Shorter sessions still work — or see what Premium adds." />
      )}

      {!formOpen ? (
        <div>
          <button type="button" className="ic-btn ic-btn--primary" onClick={() => setFormOpen(true)}>
            New scheduled session
          </button>
        </div>
      ) : (
        <form onSubmit={create} className="ic-card" style={{ padding: "var(--ic-spacing-6)", display: "grid", gap: "var(--ic-spacing-5)" }}>
          <div>
            <label htmlFor="sch-label" style={{ display: "block", fontSize: "var(--ic-font-size-bodySm)", fontWeight: "var(--ic-font-weight-semibold)" }}>
              Name
            </label>
            <input
              id="sch-label"
              className="ic-input"
              value={label}
              onChange={(e) => setLabel(e.target.value)}
              placeholder="Morning"
              maxLength={80}
              style={{ marginTop: "var(--ic-spacing-2)", width: "100%" }}
            />
          </div>
          <div className="ic-form-grid-2">
            <div>
              <label htmlFor="sch-time" style={{ display: "block", fontSize: "var(--ic-font-size-bodySm)", fontWeight: "var(--ic-font-weight-semibold)" }}>
                Time
              </label>
              <input
                id="sch-time"
                type="time"
                className="ic-input"
                value={time}
                onChange={(e) => setTime(e.target.value)}
                required
                style={{ marginTop: "var(--ic-spacing-2)", width: "100%" }}
              />
            </div>
            <div>
              <label htmlFor="sch-duration" style={{ display: "block", fontSize: "var(--ic-font-size-bodySm)", fontWeight: "var(--ic-font-weight-semibold)" }}>
                Length
              </label>
              <select
                id="sch-duration"
                className="ic-input"
                value={duration}
                onChange={(e) => setDuration(Number(e.target.value))}
                style={{ marginTop: "var(--ic-spacing-2)", width: "100%" }}
              >
                {DURATIONS.map((d) => (
                  <option key={d.seconds} value={d.seconds}>{d.label}</option>
                ))}
              </select>
            </div>
          </div>
          <fieldset style={{ border: 0, padding: 0, margin: 0 }}>
            <legend style={{ fontSize: "var(--ic-font-size-bodySm)", fontWeight: "var(--ic-font-weight-semibold)", padding: 0 }}>
              Days
            </legend>
            <div style={{ display: "flex", flexWrap: "wrap", gap: "var(--ic-spacing-2)", marginTop: "var(--ic-spacing-2)" }} role="group" aria-label="Days of week">
              {DAYS.map((name, i) => (
                <button
                  key={name}
                  type="button"
                  aria-pressed={days.includes(i)}
                  onClick={() => toggleDay(i)}
                  className="ic-btn ic-btn--small"
                  style={{
                    borderRadius: "var(--ic-radius-full)",
                    background: days.includes(i) ? "var(--ic-color-brand-600)" : "transparent",
                    color: days.includes(i) ? "#fff" : "inherit",
                    border: "1px solid var(--ic-color-neutral-200)",
                  }}
                >
                  {name}
                </button>
              ))}
            </div>
          </fieldset>
          {categories.length > 0 && (
            <fieldset style={{ border: 0, padding: 0, margin: 0 }}>
              <legend style={{ fontSize: "var(--ic-font-size-bodySm)", fontWeight: "var(--ic-font-weight-semibold)", padding: 0 }}>
                Areas of life <span style={{ fontWeight: "var(--ic-font-weight-regular)", color: "var(--ic-color-neutral-500)" }}>(up to 3 — optional)</span>
              </legend>
              <div style={{ display: "flex", flexWrap: "wrap", gap: "var(--ic-spacing-2)", marginTop: "var(--ic-spacing-2)" }} role="group" aria-label="Categories">
                {categories.slice(0, 12).map((c) => (
                  <button
                    key={c.id}
                    type="button"
                    aria-pressed={categoryIds.includes(c.id)}
                    onClick={() => toggleCategory(c.id)}
                    className="ic-btn ic-btn--small"
                    style={{
                      borderRadius: "var(--ic-radius-full)",
                      background: categoryIds.includes(c.id) ? "var(--ic-color-brand-600)" : "transparent",
                      color: categoryIds.includes(c.id) ? "#fff" : "inherit",
                      border: "1px solid var(--ic-color-neutral-200)",
                    }}
                  >
                    {c.name}
                  </button>
                ))}
              </div>
            </fieldset>
          )}
          {voiceList.length > 0 && (
            <div>
              <label htmlFor="sch-voice" style={{ display: "block", fontSize: "var(--ic-font-size-bodySm)", fontWeight: "var(--ic-font-weight-semibold)" }}>
                Voice <span style={{ fontWeight: "var(--ic-font-weight-regular)", color: "var(--ic-color-neutral-500)" }}>(optional)</span>
              </label>
              <select
                id="sch-voice"
                className="ic-input"
                value={voiceId}
                onChange={(e) => setVoiceId(e.target.value)}
                style={{ marginTop: "var(--ic-spacing-2)", width: "100%" }}
              >
                <option value="">Your default voice</option>
                {voiceList.map((v) => (
                  <option key={v.id} value={v.id}>{v.name}{v.premium ? " · Premium" : ""}</option>
                ))}
              </select>
            </div>
          )}
          <div className="ic-btn-row">
            <button type="submit" className="ic-btn ic-btn--primary" disabled={busy}>
              {busy ? "Saving…" : "Save schedule"}
            </button>
            <button type="button" className="ic-btn ic-btn--text" onClick={() => setFormOpen(false)}>
              Cancel
            </button>
          </div>
        </form>
      )}

      {schedules.length === 0 && !formOpen ? (
        <EmptyState
          title="No scheduled sessions"
          body="Pick a time and iCONFESS builds the session for you — daily, weekdays, or whenever you choose."
          ctaHref="/app/routines"
          ctaLabel="See routines"
        />
      ) : (
        <div style={{ display: "grid", gap: "var(--ic-spacing-3)" }}>
          {schedules.map((sc) => (
            <article key={sc.id} className="ic-card" style={{ padding: "var(--ic-spacing-5)", opacity: sc.enabled ? 1 : 0.72 }}>
              <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", gap: "var(--ic-spacing-4)" }}>
                <h3 style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-font-size-subheading)" }}>{sc.label}</h3>
                <span style={{ fontSize: "var(--ic-font-size-body)", fontVariantNumeric: "tabular-nums", fontWeight: "var(--ic-font-weight-semibold)" }}>
                  {sc.time}
                </span>
              </div>
              <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-500)", marginTop: "var(--ic-spacing-1)" }}>
                {daysLabel(sc.days_of_week)} · {Math.round(sc.duration_seconds / 60)} min{sc.timezone ? ` · ${sc.timezone}` : ""}
                {!sc.enabled && " · Paused"}
              </p>
              <div className="ic-btn-row" style={{ marginTop: "var(--ic-spacing-4)" }}>
                <button
                  type="button"
                  className="ic-btn ic-btn--secondary ic-btn--small"
                  disabled={startingId === sc.id}
                  onClick={() => startNow(sc)}
                >
                  {startingId === sc.id ? "Building…" : "Start now"}
                </button>
                <button
                  type="button"
                  className="ic-btn ic-btn--text ic-btn--small"
                  aria-pressed={sc.enabled}
                  onClick={() => setEnabled(sc, !sc.enabled)}
                >
                  {sc.enabled ? "Pause" : "Resume"}
                </button>
                <button
                  type="button"
                  className="ic-btn ic-btn--text ic-btn--small"
                  onClick={() => remove(sc)}
                >
                  Delete
                </button>
              </div>
            </article>
          ))}
        </div>
      )}
    </div>
  );
}
