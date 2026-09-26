"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useAuth } from "@/lib/auth-context";

type Json = Record<string, any>;
const grantFields = [
  ["public_domain", "Public domain classification"],
  ["commercial_use", "Commercial use"],
  ["redistribution_allowed", "Redistribution"],
  ["modification_allowed", "Modification"],
  ["audio_allowed", "Audio use"],
  ["offline_allowed", "Offline use"],
  ["copy_allowed", "Copying"],
  ["share_allowed", "Sharing"],
  ["search_index_allowed", "Search indexing"],
  ["api_exposure_allowed", "API exposure"],
  ["attribution_required", "Attribution required"],
] as const;
const box = { background: "var(--surface, #fff)", border: "1px solid var(--border, #ddd)", borderRadius: 14, padding: 18, margin: "14px 0" } as const;
const button = { border: 0, borderRadius: 9, padding: "9px 13px", background: "#263b32", color: "white", fontWeight: 650, cursor: "pointer" } as const;
const input = { width: "100%", border: "1px solid #ccd4cf", borderRadius: 8, padding: "9px 10px", margin: "5px 0 12px", background: "white", color: "#18221d" } as const;

async function apiRequest(path: string, token: string, method = "GET", body?: unknown) {
  const response = await fetch(`/api${path}`, {
    method,
    headers: { accept: "application/json", authorization: `Bearer ${token}`, ...(body ? { "content-type": "application/json" } : {}) },
    ...(body ? { body: JSON.stringify(body) } : {}),
    cache: "no-store",
  });
  const data = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(typeof data.error === "string" ? data.error : `Request failed (${response.status})`);
  return data as Json;
}

export default function BibleAdminPage() {
  const { token, user, loading } = useAuth();
  const [overview, setOverview] = useState<Json | null>(null);
  const [health, setHealth] = useState<Json | null>(null);
  const [metrics, setMetrics] = useState<Json | null>(null);
  const [catalog, setCatalog] = useState<Json[]>([]);
  const [plans, setPlans] = useState<Json[]>([]);
  const [audio, setAudio] = useState<Json[]>([]);
  const [audioRationales, setAudioRationales] = useState<Record<string, string>>({});
  const [audioDraft, setAudioDraft] = useState({ translation_id: "", book_id: "", chapter: 1, verse_start: 1, verse_end: 1, voice_id: "", audio_source: "human_recording", source_rights_evidence_url: "", storage_key: "", checksum_sha256: "", duration_ms: 1, alignment_json: "[]", attribution_text: "" });
  const [verseDays, setVerseDays] = useState<Json[]>([]);
  const [crossRefs, setCrossRefs] = useState<Json[]>([]);
  const [crossDraft, setCrossDraft] = useState({ source_reference: "", target_reference: "", editor_note: "" });
  const [crossReview, setCrossReview] = useState<Record<number, { evidence_url: string; rationale: string }>>({});
  const [selected, setSelected] = useState<Json | null>(null);
  const [grants, setGrants] = useState<Record<string, boolean>>({});
  const [rights, setRights] = useState({ evidence_url: "", evidence_sha256: "", rationale: "", attribution_text: "", offline_max_days: 0, decision: "approved" });
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [planRationale, setPlanRationale] = useState("Editorial review completed; canonical references checked.");
  const [planDetails, setPlanDetails] = useState<Record<string, Json>>({});
  const [newPlan, setNewPlan] = useState({ slug: "", title: "", description: "", language: "en", duration_days: 1, source_note: "", days_json: "[]" });
  const [dayEdit, setDayEdit] = useState<Record<number, { reference: string; note: string; evidence: string }>>({});

  const load = useCallback(async () => {
    if (!token) return;
    setBusy(true); setMessage("");
    try {
      const [o, h, m, p, a, v, x] = await Promise.all([
        apiRequest("/admin/bible/overview", token), apiRequest("/admin/bible/health", token),
        apiRequest("/admin/bible/metrics", token), apiRequest("/admin/bible/plans", token),
        apiRequest("/admin/bible/audio", token), apiRequest("/admin/bible/verse-of-day", token),
        apiRequest("/admin/bible/cross-references", token),
      ]);
      setOverview(o); setHealth(h); setMetrics(m);
      setPlans(p.plans ?? []); setAudio(a.assets ?? []); setVerseDays(v.entries ?? []); setCrossRefs(x.references ?? []);
      // Keep stored review records visible when upstream discovery is down;
      // approvals still require the API to recheck the live source digest.
      try {
        const c = await apiRequest("/admin/bible/catalog", token);
        setCatalog(c.translations ?? []);
        if (!c.provider_available) setMessage("HelloAO is unavailable. Stored review records are visible, but approval requires an upstream source check.");
      } catch { setCatalog([]); setMessage("Catalog review records could not be loaded. Other editorial queues remain accessible."); }
    } catch (error) { setMessage(error instanceof Error ? error.message : "Admin data could not be loaded."); }
    finally { setBusy(false); }
  }, [token]);
  useEffect(() => { void load(); }, [load]);

  const totals = useMemo(() => [
    ["Translations", overview?.translations ?? "—"], ["Active", overview?.active ?? "—"],
    ["Rights pending", overview?.pending_review ?? "—"], ["Failed imports · 7d", overview?.failed_imports ?? "—"],
    ["Published audio", overview?.published_audio ?? "—"], ["Offline packages", overview?.offline_packages ?? "—"],
  ], [overview]);

  async function run(path: string, method: string, body: unknown, success: string) {
    if (!token) return;
    setBusy(true); setMessage("");
    try { await apiRequest(path, token, method, body); setMessage(success); await load(); }
    catch (error) { setMessage(error instanceof Error ? error.message : "The action could not be completed."); }
    finally { setBusy(false); }
  }

  async function syncCatalog() {
    if (!token) return;
    setBusy(true); setMessage("");
    try { const r = await apiRequest("/admin/bible/catalog/sync", token, "POST", {}); setMessage(`Catalog metadata synchronized: ${r.discovered} records; rights remain unapproved by default.`); await load(); }
    catch (error) { setMessage(error instanceof Error ? error.message : "HelloAO discovery is unavailable."); }
    finally { setBusy(false); }
  }

  async function submitRights(event: React.FormEvent) {
    event.preventDefault(); if (!selected || !token) return;
    const body = { ...rights, ...grants };
    await run(`/admin/bible/translations/${encodeURIComponent(selected.registry_id)}/rights`, "POST", body, "Rights decision recorded with an audit trail.");
    setSelected(null);
  }

  async function openPlan(id: string) {
    if (!token) return;
    try { const data = await apiRequest(`/admin/bible/plans/${encodeURIComponent(id)}`, token); setPlanDetails({ ...planDetails, [id]: data }); }
    catch (error) { setMessage(error instanceof Error ? error.message : "Could not load plan days."); }
  }

  async function createPlan(event: React.FormEvent) {
    event.preventDefault();
    try {
      const days = JSON.parse(newPlan.days_json) as Json[];
      await run("/admin/bible/plans", "POST", { slug: newPlan.slug, title: newPlan.title, description: newPlan.description, language: newPlan.language, duration_days: newPlan.duration_days, source_note: newPlan.source_note, days }, "Draft reading plan created with its complete day list.");
      setNewPlan({ ...newPlan, slug: "", title: "", description: "", source_note: "", days_json: "[]" });
    } catch (error) { setMessage(error instanceof Error ? error.message : "Plan day JSON must be valid."); }
  }

  async function registerAudio(event: React.FormEvent) {
    event.preventDefault();
    try { const alignment = JSON.parse(audioDraft.alignment_json); await run("/admin/bible/audio", "POST", { ...audioDraft, alignment }, "Audio manifest registered for independent rights review."); }
    catch (error) { setMessage(error instanceof Error ? error.message : "Alignment must be valid JSON."); }
  }

  if (loading) return <main style={{ maxWidth: 1100, margin: "40px auto", padding: 20 }}>Loading admin session…</main>;
  if (!token) return <main style={{ maxWidth: 800, margin: "50px auto", padding: 24 }}><h1>Bible platform operations</h1><p>Sign in with an iCONFESS administrator account to access the catalog, rights review, editorial workflows and operational metrics.</p><a href="/login">Sign in</a></main>;

  return <main id="main" style={{ maxWidth: 1180, margin: "28px auto", padding: "0 20px 60px", color: "var(--text, #1a241e)" }}>
    <header style={{ display: "flex", justifyContent: "space-between", alignItems: "start", gap: 20, flexWrap: "wrap" }}>
      <div><p style={{ textTransform: "uppercase", letterSpacing: ".13em", fontSize: 12, opacity: .65 }}>iCONFESS · ADMIN</p><h1 style={{ fontSize: 34, margin: "5px 0" }}>Bible platform operations</h1><p style={{ margin: 0, opacity: .7 }}>Catalog discovery, independent rights decisions, editorial review and sanitized health metrics. Signed in as {user?.email ?? "administrator"}.</p></div>
      <button style={button} disabled={busy} onClick={() => void load()}>{busy ? "Refreshing…" : "Refresh dashboard"}</button>
    </header>
    {message && <div role="status" aria-live="polite" style={{ ...box, borderColor: message.includes("could not") || message.includes("failed") ? "#b64747" : "#5b8b68" }}>{message}</div>}

    <section aria-label="Bible platform summary" style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(145px,1fr))", gap: 10, marginTop: 18 }}>
      {totals.map(([label, value]) => <div key={String(label)} style={{ ...box, margin: 0 }}><div style={{ fontSize: 12, opacity: .65 }}>{label}</div><strong style={{ fontSize: 25 }}>{value}</strong></div>)}
    </section>

    <section style={box}>
      <div style={{ display: "flex", justifyContent: "space-between", gap: 12, alignItems: "center", flexWrap: "wrap" }}><div><h2 style={{ margin: "0 0 4px" }}>HelloAO catalog review</h2><p style={{ margin: 0, opacity: .7 }}>HelloAO is contacted only by the iCONFESS API. Discovery and metadata synchronization never grants content rights.</p></div><button style={button} disabled={busy} onClick={() => void syncCatalog()}>Sync provider catalog</button></div>
      {catalog.length === 0 ? <p>No catalog entries returned. Check provider health or sync the catalog.</p> : <div style={{ overflowX: "auto" }}><table style={{ width: "100%", borderCollapse: "collapse", marginTop: 12, textAlign: "left" }}><thead><tr>{["Translation", "Language", "Status", "Publisher / license", "Review"].map(x => <th key={x} style={{ padding: 9, borderBottom: "1px solid #ccd4cf" }}>{x}</th>)}</tr></thead><tbody>{catalog.map((entry) => { const t = entry.translation ?? {}; return <tr key={entry.registry_id}><td style={{ padding: 9, borderBottom: "1px solid #e5e9e6" }}><strong>{t.name}</strong><br/><small>{t.abbreviation} · provider id {t.provider_translation_id ?? t.id}</small></td><td style={{ padding: 9 }}>{t.language_name ?? t.language} · {t.direction ?? "ltr"}</td><td style={{ padding: 9 }}>{entry.status}{entry.remote_unavailable && <small style={{ display: "block" }}>Upstream edition not verified</small>}</td><td style={{ padding: 9 }}>{t.publisher ?? "—"}<br/><a href={t.license_url ?? t.licenseUrl ?? "https://bible.helloao.org/docs/reference/translations/"} target="_blank" rel="noreferrer">License evidence ↗</a></td><td style={{ padding: 9 }}><button style={button} onClick={() => { setSelected(entry); setGrants(Object.fromEntries(grantFields.map(([key]) => [key, false]))); setRights({ evidence_url: t.license_url ?? "", evidence_sha256: "", rationale: "", attribution_text: t.attribution_text ?? "", offline_max_days: 0, decision: entry.remote_unavailable ? "suspended" : "approved" }); }}>Review rights</button></td></tr>; })}</tbody></table></div>}
    </section>

    <section style={box}><h2 style={{ marginTop: 0 }}>Editorial reading plans</h2><p style={{ opacity: .7 }}>Publishing requires a complete day list and a different administrator from the plan creator. Add canonical references only; chapter wording always comes from the selected reviewed translation.</p>
      <details style={{ border: "1px solid #e5e9e6", borderRadius: 10, padding: 12, marginBottom: 14 }}><summary style={{ cursor: "pointer", fontWeight: 650 }}>Create draft plan</summary><form onSubmit={createPlan} style={{ paddingTop: 12 }}><div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(200px,1fr))", gap: 10 }}><input required placeholder="Slug (lowercase-with-hyphens)" value={newPlan.slug} onChange={(e) => setNewPlan({ ...newPlan, slug: e.target.value })} style={input}/><input required placeholder="Plan title" value={newPlan.title} onChange={(e) => setNewPlan({ ...newPlan, title: e.target.value })} style={input}/><input required placeholder="Language id (for example en)" value={newPlan.language} onChange={(e) => setNewPlan({ ...newPlan, language: e.target.value })} style={input}/><input required type="number" min={1} max={366} placeholder="Duration in days" value={newPlan.duration_days} onChange={(e) => setNewPlan({ ...newPlan, duration_days: Number(e.target.value) })} style={input}/></div><textarea required placeholder="Description" value={newPlan.description} onChange={(e) => setNewPlan({ ...newPlan, description: e.target.value })} style={{ ...input, minHeight: 70 }}/><textarea required minLength={12} placeholder="Editorial source and selection note" value={newPlan.source_note} onChange={(e) => setNewPlan({ ...newPlan, source_note: e.target.value })} style={{ ...input, minHeight: 60 }}/><label>Complete day list JSON. Each entry: {`{"day_number":1,"title":"","references":["JHN.3.16"]}`}<textarea required value={newPlan.days_json} onChange={(e) => setNewPlan({ ...newPlan, days_json: e.target.value })} style={{ ...input, minHeight: 110, fontFamily: "monospace" }}/></label><button disabled={busy} style={button}>Create draft plan</button></form></details>
      <label>Editorial review rationale for status changes<input value={planRationale} onChange={(e) => setPlanRationale(e.target.value)} style={input}/></label>
      {plans.map((plan) => { const detail = planDetails[plan.id]; return <div key={plan.id} style={{ borderTop: "1px solid #e5e9e6", padding: "12px 0" }}><div style={{ display: "grid", gridTemplateColumns: "minmax(180px,1fr) 150px minmax(180px,1fr) auto", alignItems: "center", gap: 10 }}><div><strong>{plan.title}</strong><br/><small>{plan.slug} · {plan.duration_days} days · {plan.status}</small></div><select aria-label={`Status for ${plan.title}`} value={plan.status} onChange={(e) => void run(`/admin/bible/plans/${encodeURIComponent(plan.id)}`, "PATCH", { status: e.target.value, rationale: planRationale }, `Plan status changed to ${e.target.value}.`)} style={input}><option>draft</option><option>review</option><option>published</option><option>archived</option></select><small>Created by {plan.created_by ?? "unknown"} · reviewed by {plan.reviewed_by ?? "—"}</small><button style={{ ...button, background: "#52675a" }} onClick={() => void openPlan(plan.id)}>{detail ? "Reload days" : "Edit days"}</button></div>{detail && <div style={{ marginTop: 10, padding: 12, background: "#f7f9f7", borderRadius: 10 }}>{(detail.days ?? []).map((day: Json) => <div key={day.day_number} style={{ display: "grid", gridTemplateColumns: "100px minmax(160px,1fr) minmax(220px,2fr) auto", gap: 8, alignItems: "start" }}><strong>Day {day.day_number}</strong><input aria-label={`Day ${day.day_number} title`} defaultValue={day.title} id={`plan-${plan.id}-title-${day.day_number}`} style={input}/><textarea aria-label={`Day ${day.day_number} references, one per line`} defaultValue={(day.references ?? []).join("\n")} id={`plan-${plan.id}-refs-${day.day_number}`} style={{ ...input, minHeight: 60 }}/><button style={button} onClick={() => { const title = (document.getElementById(`plan-${plan.id}-title-${day.day_number}`) as HTMLInputElement)?.value ?? ""; const text = (document.getElementById(`plan-${plan.id}-refs-${day.day_number}`) as HTMLTextAreaElement)?.value ?? ""; void run(`/admin/bible/plans/${encodeURIComponent(plan.id)}/days/${day.day_number}`, "PUT", { day_number: day.day_number, title, references: text.split("\n").map((ref: string) => ref.trim()).filter(Boolean) }, `Plan day ${day.day_number} saved.`); }}>Save day</button></div>)}</div>}</div>; })}</section>

    <section style={box}><h2 style={{ marginTop: 0 }}>Verse-of-day review</h2><p style={{ opacity: .7 }}>Use canonical references. Each update records reviewer and evidence in the audit trail.</p><div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(320px,1fr))", gap: 12 }}>{Array.from({ length: 7 }, (_, i) => { const day = i + 1; const saved = verseDays.find((v) => Number(v.day_index) === day); const value = dayEdit[day] ?? { reference: saved ? `${saved.book_id}.${saved.chapter}.${saved.verse}` : "", note: saved?.editor_note ?? "", evidence: "" }; return <div key={day} style={{ border: "1px solid #e5e9e6", borderRadius: 10, padding: 12 }}><strong>Day {day}</strong><small style={{ marginLeft: 8, opacity: .7 }}>Current: {saved ? `${saved.book_id}.${saved.chapter}.${saved.verse}` : "not set"}</small><input aria-label={`Day ${day} canonical reference`} value={value.reference} onChange={(e) => setDayEdit({ ...dayEdit, [day]: { ...value, reference: e.target.value } })} style={input}/><input aria-label={`Day ${day} editorial note`} placeholder="Short editorial note" value={value.note} onChange={(e) => setDayEdit({ ...dayEdit, [day]: { ...value, note: e.target.value } })} style={input}/><input aria-label={`Day ${day} evidence`} placeholder="HTTPS review evidence" value={value.evidence} onChange={(e) => setDayEdit({ ...dayEdit, [day]: { ...value, evidence: e.target.value } })} style={input}/><button style={button} onClick={() => void run(`/admin/bible/verse-of-day/${day}`, "PUT", { reference: value.reference, editor_note: value.note, evidence: value.evidence }, `Day ${day} reviewed.`)}>Save review</button></div>; })}</div></section>

    <section style={box}>
      <h2 style={{ marginTop: 0 }}>Cross-reference review</h2>
      <p style={{ opacity: .7 }}>Seeded suggestions and new pointers stay private until independently reviewed. Withdrawals immediately remove them from public results; Scripture text is never edited.</p>
      <details style={{ border: "1px solid var(--border, #ddd)", borderRadius: 10, padding: 12, marginBottom: 14 }}>
        <summary style={{ cursor: "pointer", fontWeight: 650 }}>Propose a cross-reference</summary>
        <form onSubmit={(event) => { event.preventDefault(); void run("/admin/bible/cross-references", "POST", crossDraft, "Cross-reference proposed for independent review."); }} style={{ paddingTop: 12 }}>
          <label>Source verse or range<input required placeholder="John 3:16" value={crossDraft.source_reference} onChange={(event) => setCrossDraft({ ...crossDraft, source_reference: event.target.value })} style={input}/></label>
          <label>Target passage<input required placeholder="Romans 5:8" value={crossDraft.target_reference} onChange={(event) => setCrossDraft({ ...crossDraft, target_reference: event.target.value })} style={input}/></label>
          <label>Editorial note<input maxLength={300} value={crossDraft.editor_note} onChange={(event) => setCrossDraft({ ...crossDraft, editor_note: event.target.value })} style={input}/></label>
          <button disabled={busy} style={button}>Submit for review</button>
        </form>
      </details>
      {crossRefs.length === 0 ? <p>No cross-references proposed.</p> : crossRefs.map((item) => {
        const review = crossReview[item.id] ?? { evidence_url: "", rationale: "" };
        return <div key={item.id} style={{ borderTop: "1px solid var(--border, #ddd)", padding: "12px 0" }}>
          <strong>{item.source_book_id} {item.source_chapter}:{item.source_verse_start}{item.source_verse_end !== item.source_verse_start ? `–${item.source_verse_end}` : ""} → {item.target_reference}</strong>
          <small style={{ display: "block", opacity: .7 }}>{item.reviewed_at ? "Published" : "Awaiting review"} · proposed by {item.created_by ?? "seeded"} · reviewed by {item.reviewed_by ?? "—"}</small>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(220px,1fr))", gap: 10, marginTop: 8 }}>
            <label>HTTPS review evidence<input type="url" placeholder="https://…" value={review.evidence_url} onChange={(event) => setCrossReview({ ...crossReview, [item.id]: { ...review, evidence_url: event.target.value } })} style={input}/></label>
            <label>Review rationale<input minLength={8} maxLength={2000} value={review.rationale} onChange={(event) => setCrossReview({ ...crossReview, [item.id]: { ...review, rationale: event.target.value } })} style={input}/></label>
          </div>
          <button disabled={busy || review.rationale.trim().length < 8 || !review.evidence_url.startsWith("https://") || item.created_by === user?.id} style={button} onClick={() => void run(`/admin/bible/cross-references/${item.id}`, "PATCH", { ...review, decision: "approve" }, "Cross-reference independently approved.")}>Approve</button>{" "}
          <button disabled={busy || review.rationale.trim().length < 8 || !review.evidence_url.startsWith("https://")} style={{ ...button, background: "#823d3d" }} onClick={() => void run(`/admin/bible/cross-references/${item.id}`, "PATCH", { ...review, decision: "withdraw" }, "Cross-reference withdrawn.")}>Withdraw</button>
        </div>;
      })}
    </section>

    <section style={box}><h2 style={{ marginTop: 0 }}>Rights-cleared audio queue</h2><p style={{ opacity: .7 }}>Approval rechecks active translation, voice license, immutable content hash, storage object and checksum. Registration points to an existing API-controlled object; it does not upload arbitrary client files.</p><details style={{ border: "1px solid #e5e9e6", borderRadius: 10, padding: 12, marginBottom: 14 }}><summary style={{ cursor: "pointer", fontWeight: 650 }}>Register audio manifest for review</summary><form onSubmit={registerAudio} style={{ paddingTop: 12 }}><div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(190px,1fr))", gap: 8 }}>{(["translation_id", "book_id", "voice_id", "source_rights_evidence_url", "storage_key", "checksum_sha256", "attribution_text"] as const).map((key) => <label key={key}>{key.replaceAll("_", " ")}<input required={key !== "attribution_text"} value={audioDraft[key]} onChange={(e) => setAudioDraft({ ...audioDraft, [key]: e.target.value })} style={input}/></label>)}<label>Chapter<input type="number" min={1} value={audioDraft.chapter} onChange={(e) => setAudioDraft({ ...audioDraft, chapter: Number(e.target.value) })} style={input}/></label><label>Verse start<input type="number" min={1} value={audioDraft.verse_start} onChange={(e) => setAudioDraft({ ...audioDraft, verse_start: Number(e.target.value) })} style={input}/></label><label>Verse end<input type="number" min={1} value={audioDraft.verse_end} onChange={(e) => setAudioDraft({ ...audioDraft, verse_end: Number(e.target.value) })} style={input}/></label><label>Duration ms<input type="number" min={1} value={audioDraft.duration_ms} onChange={(e) => setAudioDraft({ ...audioDraft, duration_ms: Number(e.target.value) })} style={input}/></label><label>Audio source<select value={audioDraft.audio_source} onChange={(e) => setAudioDraft({ ...audioDraft, audio_source: e.target.value })} style={input}><option value="human_recording">Human recording</option><option value="licensed_master">Licensed master</option><option value="synthetic">Synthetic</option></select></label></div><label>Word/verse alignment JSON<textarea value={audioDraft.alignment_json} onChange={(e) => setAudioDraft({ ...audioDraft, alignment_json: e.target.value })} style={{ ...input, minHeight: 70, fontFamily: "monospace" }}/></label><button disabled={busy} style={button}>Register for review</button></form></details>{audio.length === 0 ? <p>No registered audio manifests.</p> : audio.map((asset) => <div key={asset.id} style={{ borderTop: "1px solid #e5e9e6", padding: 12, display: "flex", justifyContent: "space-between", flexWrap: "wrap", gap: 10 }}><div><strong>{asset.translation_id} · {asset.book_id} {asset.chapter}:{asset.verse_start}–{asset.verse_end}</strong><br/><small>{asset.audio_source} · voice {asset.voice_id} · {asset.duration_ms} ms · {asset.status}</small></div>{asset.status === "pending_review" && <div style={{ minWidth: 260, flex: "1 1 100%" }}><label>Review rationale<textarea minLength={8} value={audioRationales[asset.id] ?? ""} onChange={(e) => setAudioRationales({ ...audioRationales, [asset.id]: e.target.value })} style={{ ...input, minHeight: 55 }}/></label><button disabled={busy || (audioRationales[asset.id] ?? "").trim().length < 8} style={button} onClick={() => void run(`/admin/bible/audio/${asset.id}/review`, "POST", { decision: "approve", rationale: audioRationales[asset.id] }, "Audio approved and published.")}>Approve</button> <button disabled={busy || (audioRationales[asset.id] ?? "").trim().length < 8} style={{ ...button, background: "#823d3d" }} onClick={() => void run(`/admin/bible/audio/${asset.id}/review`, "POST", { decision: "withdraw", rationale: audioRationales[asset.id] }, "Audio withdrawn.")}>Withdraw</button></div>}</div>)}</section>

    <section style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(300px,1fr))", gap: 14 }}><div style={box}><h2 style={{ marginTop: 0 }}>Service health</h2><pre style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere", fontSize: 12 }}>{JSON.stringify(health, null, 2)}</pre></div><div style={box}><h2 style={{ marginTop: 0 }}>Sanitized operational metrics</h2><p>Process-lifetime aggregates only; private note contents and reading payloads are not recorded.</p><pre style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere", fontSize: 12 }}>{JSON.stringify(metrics, null, 2)}</pre></div></section>

    {selected && <div role="presentation" onMouseDown={(e) => { if (e.target === e.currentTarget) setSelected(null); }} style={{ position: "fixed", inset: 0, background: "#07100dcc", display: "grid", placeItems: "center", padding: 16, zIndex: 30 }}><form onSubmit={submitRights} style={{ ...box, width: "min(760px, 100%)", maxHeight: "92vh", overflowY: "auto", margin: 0 }}><h2 style={{ marginTop: 0 }}>Rights review · {selected.translation?.name}</h2><p>Grant each use independently from the evidence. Unchecked permissions remain denied; approval does not infer any permission.</p>{selected.remote_unavailable && <p role="alert">The upstream edition is not verified. It can be rejected or suspended, but approval requires a fresh provider check.</p>}<label>Decision<select value={rights.decision} onChange={(e) => setRights({ ...rights, decision: e.target.value })} style={input}><option value="approved" disabled={!!selected.remote_unavailable}>Approve with explicit grants</option><option value="rejected">Reject</option><option value="suspended">Suspend</option></select></label><div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(220px,1fr))" }}>{grantFields.map(([key, label]) => <label key={key} style={{ padding: 7, display: "flex", gap: 9, alignItems: "center" }}><input type="checkbox" checked={grants[key] ?? false} onChange={(e) => setGrants({ ...grants, [key]: e.target.checked })}/>{label}</label>)}</div><label>Offline license maximum days<input type="number" min={0} max={3650} value={rights.offline_max_days} onChange={(e) => setRights({ ...rights, offline_max_days: Number(e.target.value) })} style={input}/></label><label>Attribution text<input value={rights.attribution_text} onChange={(e) => setRights({ ...rights, attribution_text: e.target.value })} style={input}/></label><label>HTTPS rights evidence URL<input type="url" required value={rights.evidence_url} onChange={(e) => setRights({ ...rights, evidence_url: e.target.value })} style={input}/></label><label>Evidence SHA-256 (optional)<input value={rights.evidence_sha256} onChange={(e) => setRights({ ...rights, evidence_sha256: e.target.value })} style={input}/></label><label>Review rationale<textarea required minLength={8} maxLength={4000} value={rights.rationale} onChange={(e) => setRights({ ...rights, rationale: e.target.value })} style={{ ...input, minHeight: 80 }}/></label><div style={{ display: "flex", justifyContent: "end", gap: 10 }}><button type="button" onClick={() => setSelected(null)} style={{ ...button, background: "#66736b" }}>Cancel</button><button disabled={busy} style={button}>Record rights decision</button></div></form></div>}
  </main>;
}
