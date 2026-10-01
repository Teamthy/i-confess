"use client";
import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { Icon, EmptyState } from "./ui";
import { PlatformVoicePicker } from "./voice-preferences";
import { useSchedules, type ScheduleDraft } from "@/lib/schedule-context";
import { ALL_DAYS, DAYS_PRESETS, WEEKDAYS, daysLabel, deviceTimezone, nextOccurrence, validateSchedule, type ScheduleRecord } from "@/lib/schedules";
import { preferredVoiceId, useCategoryCatalogue, useVoiceCatalogue } from "@/lib/catalogue";
import { CATEGORIES, catBySlug } from "@/lib/data";
import { mutate, track, useApp } from "@/lib/store";
import { useToast } from "@/lib/ui";
import { useAuth } from "@/lib/auth-context";
import { useApiData } from "@/lib/app-api";

function freshDraft(local: boolean, voice = ""): ScheduleDraft {
  return { label: "Morning practice", time: "07:00", days_of_week: [...ALL_DAYS], timezone: "UTC", duration_seconds: 300, voice_id: voice, category_ids: local ? ["peace"] : [], enabled: true };
}
function nextLabel(sc: ScheduleRecord, now: Date): string {
  const next = nextOccurrence(sc, now);
  if (!sc.enabled) return "Reminders paused";
  if (!next) return "No valid next occurrence — review this routine";
  return `Next: ${next.toLocaleString(undefined, { timeZone: sc.timezone, weekday: "short", month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" })}`;
}

export function ScheduleReminderSettings() {
  const schedules = useSchedules();
  const toast = useToast();
  const [error, setError] = useState<string | null>(null);
  return <div className="schedule-reminder-settings">
    <div className="setting-row"><div><b>{schedules.mode === "account" ? "Schedule reminders" : "Browser-only reminders"}</b><span>{schedules.mode === "account" ? "Synced with your account's scheduled-session notification preference." : "In-app reminders while iCONFESS is open on this browser."}</span></div><button type="button" className="switch" role="switch" aria-label="Schedule reminders" aria-checked={schedules.reminders} disabled={schedules.remindersLoading || !!schedules.preferencesError} onClick={async () => {
      setError(null); const result = await schedules.setReminders(!schedules.reminders);
      if (!result.ok) setError(result.message); else toast(schedules.reminders ? "Schedule reminders off" : "Schedule reminders on");
    }} /></div>
    <div className="setting-row"><div><b>Browser alerts</b><span>{schedules.permission === "granted" ? "Allowed on this browser" : schedules.permission === "denied" ? "Blocked — change your browser's site permissions" : schedules.permission === "unsupported" ? "Not supported — in-app notices are still available" : "Permission is only requested when you choose to enable alerts"}</span></div><button className="btn btn-ghost btn-sm" type="button" disabled={schedules.permission === "unsupported" || schedules.permission === "denied" || schedules.remindersLoading} onClick={async () => { const message = await schedules.enableAlerts(); setError(message); if (!message) toast("Browser alerts enabled"); }}>{schedules.permission === "granted" ? "Enable reminders" : "Enable alerts"}</button></div>
    <p className="small">Browser reminders need this app open, never start audio automatically, and respect your browser's <Link href="/app/settings/notifications">quiet hours</Link>. Closed-app mobile push needs a registered device and configured APNs/FCM delivery.</p>
    {schedules.preferencesError && <div className="feature-notice" role="alert"><p>{schedules.preferencesError.message}</p><button className="btn btn-ghost btn-sm" onClick={schedules.reload}>Retry reminder preferences</button></div>}
    {error && <p className="feature-error" role="alert">{error}</p>}
  </div>;
}

export function SchedulePage() {
  const schedules = useSchedules();
  const catalogue = useCategoryCatalogue();
  const voices = useVoiceCatalogue();
  const auth = useAuth();
  const app = useApp();
  const entitlements = useApiData<{ max_session_seconds: number }>(auth.token, "/entitlements");
  const toast = useToast();
  const local = schedules.mode === "local";
  const [draft, setDraft] = useState<ScheduleDraft>(() => freshDraft(local, app.settings.voice));
  const [editing, setEditing] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [busyRow, setBusyRow] = useState<string | null>(null);
  const [deleting, setDeleting] = useState<string | null>(null);
  const [now, setNow] = useState(() => new Date());
  const [zones, setZones] = useState<string[]>([]);
  const formRef = useRef<HTMLFormElement | null>(null);
  const submitting = useRef(false);
  const categories = local ? CATEGORIES.map((c) => ({ id: c.slug, name: c.name, slug: c.slug })) : (catalogue.data || []);
  const voiceList = voices.data || [];
  const browserRoutines = app.schedule.filter((s) => s.localOwner === app.user?.email);
  useEffect(() => {
    setDraft(freshDraft(local, app.settings.voice)); setEditing(null); setError(null);
    setDraft((s) => ({ ...s, timezone: deviceTimezone() }));
  }, [local, auth.user?.id]);
  useEffect(() => {
    if (!editing && !draft.category_ids?.length && categories.length) {
      const first = categories.find((c) => c.slug === "peace") || categories[0];
      setDraft((s) => ({ ...s, category_ids: [first.id] }));
    }
  }, [catalogue.data, local]);
  useEffect(() => {
    try { setZones([...new Set(["UTC", deviceTimezone(), ...Intl.supportedValuesOf("timeZone")])]); }
    catch { setZones(["UTC", deviceTimezone(), "Africa/Lagos", "Europe/London", "America/New_York", "Asia/Kolkata", "Asia/Manila"]); }
    const timer = setInterval(() => setNow(new Date()), 30000);
    return () => clearInterval(timer);
  }, []);
  const reset = () => { setEditing(null); setError(null); setDraft({ ...freshDraft(local, app.settings.voice), timezone: deviceTimezone(), category_ids: [categories.find((c) => c.slug === "peace")?.id || categories[0]?.id || ""].filter(Boolean) }); };
  const edit = (sc: ScheduleRecord) => {
    setEditing(sc.id); setError(null);
    setDraft({ label: sc.label, time: sc.time, days_of_week: [...sc.days_of_week], timezone: sc.timezone, duration_seconds: sc.duration_seconds, voice_id: sc.voice_id || "", category_ids: [...(sc.category_ids || [])], enabled: sc.enabled });
    formRef.current?.scrollIntoView({ behavior: "smooth", block: "start" });
  };
  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (submitting.current) return;
    const validation = validateSchedule(draft);
    if (validation || !draft.category_ids?.length) { setError(validation || "Choose at least one category."); return; }
    submitting.current = true; setSaving(true); setError(null);
    const result = await schedules.save({ ...draft, label: draft.label.trim(), voice_id: local ? "" : preferredVoiceId(voiceList, draft.voice_id || "") }, editing || undefined);
    submitting.current = false; setSaving(false);
    if (!result.ok) { setError(result.message); return; }
    if (!editing) track("schedule_created", { source: schedules.mode });
    toast(local ? "Routine saved on this browser" : editing ? "Schedule updated" : "Schedule saved to your account");
    reset();
  };
  const runAction = async (sc: ScheduleRecord, action: "start" | "toggle" | "delete") => {
    if (busyRow) return;
    setBusyRow(sc.id); setError(null);
    const result = action === "start" ? await schedules.start(sc) : action === "toggle" ? await schedules.setEnabled(sc, !sc.enabled) : await schedules.remove(sc.id);
    setBusyRow(null);
    if (!result.ok) { setError(result.message); return; }
    if (action === "delete") { setDeleting(null); if (editing === sc.id) reset(); toast("Schedule deleted"); }
    if (action === "toggle") toast(sc.enabled ? "Routine reminders paused" : "Routine reminders enabled");
  };
  const importLocal = async (sc: ScheduleRecord) => {
    if (busyRow) return;
    const ids = (sc.category_ids || []).map((slug) => categories.find((c) => c.slug === slug)?.id).filter((id): id is string => !!id);
    if (!ids.length) { setError("The categories for this browser routine are unavailable in the account catalogue."); return; }
    setBusyRow(sc.id); setError(null);
    const result = await schedules.save({ label: sc.label, time: sc.time, days_of_week: sc.days_of_week, timezone: sc.timezone, duration_seconds: sc.duration_seconds, enabled: sc.enabled, category_ids: ids, voice_id: preferredVoiceId(voiceList, app.settings.voice) });
    setBusyRow(null);
    if (!result.ok) { setError(result.message); return; }
    mutate((s) => { s.schedule = s.schedule.filter((record) => record.id !== sc.id || record.localOwner !== app.user?.email); });
    toast("Browser routine moved to your account");
  };

  return <section className="schedule-page">
    <div className="app-top"><div><h2 className="h2">Make room for your practice.</h2><p className="small">A time, a few words, a rhythm you can keep.</p></div><span className="tag private"><Icon n={local ? "cal" : "check"} s={13} /> {local ? "This browser only" : "Account schedules"}</span></div>
    {local && <div className="feature-notice"><b>Browser-only mode</b><p>These routines are stored on this device, not on a server. They use your selected device speech voice. Sign in to the API to save account schedules and use published narration.</p></div>}
    <div className="schedule-layout">
      <form className="form-card schedule-editor" ref={formRef} onSubmit={submit}>
        <div className="schedule-editor-heading"><div><span className="eyebrow">{editing ? "Adjust your rhythm" : "Create a rhythm"}</span><h3 className="h3">{editing ? "Edit routine" : "New routine"}</h3></div>{editing && <button type="button" className="btn btn-ghost btn-sm" onClick={reset}>Cancel edit</button>}</div>
        <fieldset disabled={saving} className="schedule-fields">
          <div className="field"><label htmlFor="routine-label">Routine name</label><input id="routine-label" required maxLength={120} value={draft.label} onChange={(e) => setDraft((s) => ({ ...s, label: e.target.value }))} /></div>
          <div className="schedule-form-grid"><div className="field"><label htmlFor="routine-time">Time</label><input id="routine-time" type="time" required value={draft.time} onChange={(e) => setDraft((s) => ({ ...s, time: e.target.value }))} /></div><div className="field"><label htmlFor="routine-duration">Session length</label><select id="routine-duration" value={draft.duration_seconds} onChange={(e) => setDraft((s) => ({ ...s, duration_seconds: Number(e.target.value) }))}>{[1, 5, 10, 15, 30, 45, 60, 90, 120, 180].map((m) => <option key={m} value={m * 60} disabled={!local && !!entitlements.data && m * 60 > entitlements.data.max_session_seconds}>{m} {m === 1 ? "minute" : "minutes"}{!local && entitlements.data && m * 60 > entitlements.data.max_session_seconds ? " · requires a longer-session plan" : ""}</option>)}</select></div></div>
          <div className="field"><label htmlFor="routine-timezone">Timezone</label><input id="routine-timezone" list="schedule-timezones" value={draft.timezone} required onChange={(e) => setDraft((s) => ({ ...s, timezone: e.target.value }))} /><datalist id="schedule-timezones">{zones.map((zone) => <option key={zone} value={zone} />)}</datalist><p className="small" style={{ marginTop: 6 }}>The chosen local time stays fixed through daylight saving. Missing spring-forward times are skipped; repeated fall-back times fire once.</p></div>
          <fieldset className="schedule-days"><legend>Repeat on</legend><div className="chip-row">{Object.entries(DAYS_PRESETS).map(([name, days]) => <button className="chip" key={name} type="button" aria-pressed={days.join() === [...draft.days_of_week].sort((a, b) => a - b).join()} onClick={() => setDraft((s) => ({ ...s, days_of_week: [...days] }))}>{name}</button>)}</div><div className="weekday-row">{WEEKDAYS.map((day, i) => <button className="weekday-button" key={day} type="button" aria-label={`Repeat on ${["Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"][i]}`} aria-pressed={draft.days_of_week.includes(i + 1)} onClick={() => setDraft((s) => ({ ...s, days_of_week: s.days_of_week.includes(i + 1) ? s.days_of_week.filter((d) => d !== i + 1) : [...s.days_of_week, i + 1].sort((a, b) => a - b) }))}>{day}</button>)}</div></fieldset>
          <fieldset className="schedule-categories"><legend>Categories <span className="small">Choose one or more</span></legend>{!local && catalogue.loading ? <p className="small">Loading categories…</p> : <div className="chip-row">{categories.map((c) => <button type="button" className="chip" key={c.id} aria-pressed={draft.category_ids?.includes(c.id)} onClick={() => setDraft((s) => ({ ...s, category_ids: s.category_ids?.includes(c.id) ? s.category_ids.filter((id) => id !== c.id) : [...(s.category_ids || []), c.id] }))}>{c.name}</button>)}</div>}{!local && catalogue.error && <div className="feature-error" role="alert">{catalogue.error.message} <button type="button" className="textlink" onClick={catalogue.reload}>Retry categories</button></div>}</fieldset>
          {!local && <><PlatformVoicePicker id="routine-voice" voices={voiceList} value={draft.voice_id || ""} onChange={(value) => setDraft((s) => ({ ...s, voice_id: value }))} disabled={voices.loading} />{voices.error && <p className="feature-error">{voices.error.message} Automatic available narration can still be selected.</p>}</>}
          {local && <p className="small">Device speech uses your <Link href="/app/settings/playback">voice & sound preferences</Link>. Session length is an estimate based on spoken wording.</p>}
          <div className="schedule-next-preview"><Icon n="cal" s={18} /><div><b>{daysLabel(draft.days_of_week)} · {draft.time || "Choose a time"}</b><span>{nextLabel({ ...draft, id: "preview" }, now)} · {draft.timezone}</span></div></div>
          {error && <p className="feature-error" role="alert">{error}</p>}
          <button className="btn btn-primary" type="submit" disabled={saving || (!local && catalogue.loading)}><Icon n={editing ? "check" : "plus"} s={14} /> {saving ? "Saving…" : editing ? "Save changes" : "Save routine"}</button>
        </fieldset>
      </form>
      <div className="schedule-list-panel">
        <div className="section-head"><h3>Your routines</h3><span className="small">{schedules.schedules.length} saved</span></div>
        {schedules.loading && <div className="form-card" role="status">Loading schedules…</div>}
        {schedules.error && <div className="feature-notice" role="alert"><p>{schedules.error.message} No browser-only copy has replaced your account schedules.</p><button className="btn btn-ghost btn-sm" onClick={schedules.reload}>Retry schedules</button></div>}
        {!schedules.loading && !schedules.error && !schedules.schedules.length && <EmptyState icon="cal" title="A little room in your day" sub="Save your first routine. You choose when to start; reminders never autoplay." />}
        {!schedules.error && schedules.schedules.map((sc) => <article className={`schedule-card${sc.enabled ? "" : " paused"}`} key={sc.id}>
          <div className="schedule-card-heading"><span className="schedule-clock">{sc.time}</span><button type="button" role="switch" className="switch" aria-label={`Reminders for ${sc.label}`} aria-checked={sc.enabled} disabled={!!busyRow} onClick={() => { void runAction(sc, "toggle"); }} /></div>
          <h4>{sc.label}</h4><p>{daysLabel(sc.days_of_week)} · {Math.round(sc.duration_seconds / 60)} min{local ? " estimate" : ""}</p><p className="small">{(sc.category_ids || []).map((id) => categories.find((c) => c.id === id)?.name || catBySlug(id)?.name || "Unavailable category").join(" · ") || "All available categories"}</p><div className="schedule-next"><Icon n="cal" s={13} /><span>{nextLabel(sc, now)}<br />{sc.timezone}</span></div>
          <div className="schedule-card-actions"><button type="button" className="btn btn-primary btn-sm" disabled={!!busyRow} onClick={() => { void runAction(sc, "start"); }}><Icon n="play" s={13} /> {busyRow === sc.id ? "Working…" : "Start now"}</button><button type="button" className="btn btn-ghost btn-sm" disabled={!!busyRow} onClick={() => edit(sc)}>Edit</button><button type="button" className="icon-btn" aria-label={`Delete ${sc.label}`} disabled={!!busyRow} onClick={() => setDeleting(sc.id)}><Icon n="x" s={14} /></button></div>
          {deleting === sc.id && <div className="schedule-delete-confirm" role="group" aria-label={`Confirm deleting ${sc.label}`}><p className="small">Delete this routine? This cannot be undone.</p><div><button className="btn btn-ghost btn-sm" type="button" disabled={!!busyRow} onClick={() => setDeleting(null)}>Keep it</button><button className="btn btn-primary btn-sm" type="button" disabled={!!busyRow} onClick={() => { void runAction(sc, "delete"); }}>Delete routine</button></div></div>}
        </article>)}
        <div className="form-card"><h3 className="h4">A nudge, not a demand</h3><ScheduleReminderSettings /></div>
      </div>
    </div>
    {!local && browserRoutines.length > 0 && <div className="form-card local-routine-import"><h3 className="h3">Your browser-only routines</h3><p className="small">These older routines have not been uploaded. Move them to your account explicitly; a successful import removes only that browser copy.</p>{browserRoutines.map((sc) => <div className="list-row" key={sc.id}><div className="lr-main"><h4>{sc.label} · {sc.time}</h4><p>{daysLabel(sc.days_of_week)} · {sc.timezone}</p></div><button className="btn btn-ghost btn-sm" type="button" disabled={!!busyRow || catalogue.loading} onClick={() => { void importLocal(sc); }}>Move to account</button></div>)}</div>}
  </section>;
}

export function ScheduleReminders() {
  const schedules = useSchedules();
  const toast = useToast();
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const notice = schedules.notices[0];
  if (!notice) return null;
  return <aside className="schedule-reminder-banner" role="status" aria-live="polite"><span className="schedule-reminder-icon"><Icon n="cal" s={20} /></span><div><b>A little room for your practice.</b><p>{notice.schedule.label} · {notice.schedule.time}</p>{error && <p className="feature-error">{error}</p>}</div><button className="btn btn-primary btn-sm" disabled={!!busy} onClick={async () => { setBusy(notice.key); setError(null); const result = await schedules.start(notice.schedule); setBusy(null); if (result.ok) { schedules.dismiss(notice.key); toast("Your practice is ready"); } else setError(result.message); }}>{busy ? "Preparing…" : "Start now"}</button><button className="icon-btn" type="button" aria-label="Dismiss reminder" onClick={() => { schedules.dismiss(notice.key); setError(null); }}><Icon n="x" s={16} /></button></aside>;
}
