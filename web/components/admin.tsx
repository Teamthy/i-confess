"use client";
/* iCONFESS Audio + Voice Engine — internal studio (§53, §80–§85, §113–§114).
   Demo-grade admin: catalog + rights actions persist via local overrides;
   production admin is server-rendered against PostgreSQL (see AUDIO_ENGINE.md). */
import React, { useEffect, useMemo, useState } from "react";
import { Icon } from "./ui";
import { useToast } from "@/lib/ui";
import { VOICE_CATALOG, effectiveVoices, loadOverrides, saveOverrides, AMBIENCES, type VoiceStatus, type RightsStatus } from "@/lib/voices";
import { loadJobs, readAE, type AudioJob } from "@/lib/audio-engine";

type Tab = "overview" | "voices" | "rights" | "queue" | "quality" | "cost" | "events";
const TABS: [Tab, string][] = [["overview", "Overview"], ["voices", "Voice catalog"], ["rights", "Rights"], ["queue", "Generation queue"], ["quality", "Quality"], ["cost", "Cost"], ["events", "Events"]];

const statusClass = (s: string) => (s === "PRODUCTION" || s === "APPROVED" || s === "READY" ? "tag shared" : s === "TESTING" || s === "QUALITY_REVIEW" || s === "PENDING" || s === "QUEUED" || s === "PROCESSING" || s === "MASTERING" || s === "ENCODING" ? "tag pending" : "tag private");

export function AdminStudio() {
  const [tab, setTab] = useState<Tab>("overview");
  const [tick, setTick] = useState(0);
  const [voices, setVoices] = useState<ReturnType<typeof effectiveVoices>>(VOICE_CATALOG);
  useEffect(() => { setVoices(effectiveVoices()); }, [tick]);
  useEffect(() => { const t = window.setInterval(() => setTick((x) => x + 1), 3000); return () => clearInterval(t); }, []);
  const jobs = useMemo(() => loadJobs(), [tick]);
  const events = useMemo(() => readAE(), [tick]);
  const toast = useToast();

  const setStatus = (id: string, patch: { status?: VoiceStatus; rightsStatus?: RightsStatus }) => {
    const o = loadOverrides();
    o[id] = { ...o[id], ...patch };
    saveOverrides(o); setTick((x) => x + 1);
    toast(patch.rightsStatus ? "Rights → " + patch.rightsStatus : "Status → " + patch.status);
  };

  const live = voices.filter((v) => v.status === "PRODUCTION" && v.rightsStatus === "APPROVED");
  const counts = (n: string) => events.filter((e) => e.name === n).length;

  return (
    <div style={{ padding: "calc(var(--header-h) + 32px) 24px 120px", maxWidth: 1080, margin: "0 auto" }}>
      <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
        <h1 className="h2" style={{ margin: 0 }}>Audio Studio</h1>
        <span className="tag pending">internal · demo data on this device</span>
      </div>
      <p className="small" style={{ marginTop: 6 }}>Voice catalog, rights registry, generation queue, quality and cost — the operational console for the Audio + Voice Engine.</p>
      <div className="chip-row" style={{ marginTop: 16 }}>{TABS.map(([t, l]) => <button key={t} className="chip" aria-pressed={tab === t} onClick={() => setTab(t)}>{l}</button>)}</div>

      {tab === "overview" && (
        <div className="card-row four" style={{ marginTop: 20 }}>
          <div className="tile"><span className="t-meta">Voices</span><h4>{live.length} in production</h4><p>{voices.filter((v) => v.status !== "PRODUCTION").length} in pipeline · {voices.filter((v) => v.rightsStatus !== "APPROVED").length} rights-pending</p></div>
          <div className="tile"><span className="t-meta">Generation</span><h4>{jobs.filter((j) => j.status === "READY").length} ready</h4><p>{jobs.filter((j) => ["QUEUED", "PROCESSING", "MASTERING", "ENCODING"].includes(j.status)).length} in flight · {jobs.filter((j) => j.status === "FAILED").length} failed</p></div>
          <div className="tile"><span className="t-meta">Playback</span><h4>{counts("audio_started")} starts</h4><p>{counts("audio_completed")} completed · {counts("audio_error")} errors</p></div>
          <div className="tile"><span className="t-meta">Voices used</span><h4>{new Set(events.filter((e) => e.name === "voice_selected").map((e) => String(e.props.voiceId))).size} distinct</h4><p>{counts("voice_previewed")} previews played</p></div>
        </div>
      )}

      {(tab === "voices" || tab === "rights") && (
        <div className="form-card" style={{ marginTop: 20, overflowX: "auto" }}>
          <table className="adm-table">
            <thead><tr><th>Voice</th><th>Locale</th><th>Provider</th><th>Status</th><th>Rights</th><th>Categories</th><th>Actions</th></tr></thead>
            <tbody>
              {voices.map((v) => (
                <tr key={v.id}>
                  <td><b>{v.displayName}</b><br /><span className="small">{v.id} · v{v.version}</span></td>
                  <td>{v.locale}</td>
                  <td>{v.provider}<br /><span className="small">{v.providerVoiceId}</span></td>
                  <td><span className={statusClass(v.status)}>{v.status.replace("_", " ").toLowerCase()}</span></td>
                  <td><span className={statusClass(v.rightsStatus)}>{v.rightsStatus.toLowerCase()}</span>{v.rights.expirationDate && <><br /><span className="small">exp {v.rights.expirationDate}</span></>}</td>
                  <td>{v.categories.length}</td>
                  <td>
                    <div className="chip-row" style={{ gap: 4 }}>
                      {tab === "voices" ? <>
                        {v.status !== "PRODUCTION" && <button className="chip" onClick={() => setStatus(v.id, { status: "PRODUCTION" })}>Approve</button>}
                        {v.status === "PRODUCTION" && <button className="chip" onClick={() => setStatus(v.id, { status: "SUSPENDED" })}>Suspend</button>}
                        {v.status === "DRAFT" && <button className="chip" onClick={() => setStatus(v.id, { status: "TESTING" })}>To testing</button>}
                      </> : <>
                        {v.rightsStatus !== "APPROVED" && <button className="chip" onClick={() => setStatus(v.id, { rightsStatus: "APPROVED" })}>Clear rights</button>}
                        {v.rightsStatus === "APPROVED" && <button className="chip" onClick={() => setStatus(v.id, { rightsStatus: "REVOKED" })}>Revoke</button>}
                        <button className="chip" onClick={() => setStatus(v.id, { rightsStatus: "EXPIRED" })}>Expire</button>
                      </>}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          <p className="small" style={{ marginTop: 10 }}>Rights enforcement is live: revoked/expired voices stop generating immediately; non-production voices are hidden from the catalog. In production this table is PostgreSQL-backed with an audit log per action.</p>
        </div>
      )}

      {tab === "queue" && (
        <div className="form-card" style={{ marginTop: 20, overflowX: "auto" }}>
          <table className="adm-table">
            <thead><tr><th>Job</th><th>Content</th><th>Voice</th><th>Status</th><th>Cache key</th><th>Correlation</th></tr></thead>
            <tbody>
              {jobs.length === 0 && <tr><td colSpan={6} className="small">No jobs yet — generate studio masters from a voice profile page, then watch them flow QUEUED → PROCESSING → MASTERING → ENCODING → READY.</td></tr>}
              {jobs.map((j: AudioJob) => (
                <tr key={j.id}><td className="small">{j.id}</td><td>{j.contentTitle}<br /><span className="small">{j.locale} · {j.outputFormat}</span></td><td>{j.voiceName}</td><td><span className={statusClass(j.status)}>{j.status.toLowerCase()}</span></td><td className="small">{j.cacheKey}</td><td className="small">{j.correlationId}</td></tr>
              ))}
            </tbody>
          </table>
          <p className="small" style={{ marginTop: 10 }}>Identical requests reuse cached assets (idempotent, §88). Production: Redis queue + workers, exponential bounded retries, provider fallback.</p>
        </div>
      )}

      {tab === "quality" && (
        <div className="card-row four" style={{ marginTop: 20 }}>
          <div className="tile"><span className="t-meta">Mastering target</span><h4>-16 LUFS</h4><p>True peak -1 dBTP · configured at the mastering layer</p></div>
          <div className="tile"><span className="t-meta">Generation health</span><h4>{jobs.length ? Math.round((jobs.filter((j) => j.status === "READY").length / jobs.length) * 100) : 100}% ready</h4><p>{jobs.filter((j) => j.status === "FAILED").length} failures to investigate</p></div>
          <div className="tile"><span className="t-meta">Client events</span><h4>{counts("audio_error")} audio errors</h4><p>{counts("audio_completed")} completions · {counts("audio_seeked")} seeks</p></div>
          <div className="tile"><span className="t-meta">Ambiences</span><h4>{AMBIENCES.length} ready</h4><p>All loopable · seamless · iCONFESS Studio rights</p></div>
        </div>
      )}

      {tab === "cost" && (
        <div className="card-row four" style={{ marginTop: 20 }}>
          <div className="tile"><span className="t-meta">TTS characters</span><h4>{jobs.reduce((a, j) => a + j.contentTitle.length * 42, 0).toLocaleString()} est.</h4><p>Estimated from job content</p></div>
          <div className="tile"><span className="t-meta">Cache hits</span><h4>{counts("audio_cache_hit")}</h4><p>Deduplication avoids repeat generation</p></div>
          <div className="tile"><span className="t-meta">Provider</span><h4>internal</h4><p>On-device today · zero per-character cost; cloud providers bill via provider_usage table</p></div>
          <div className="tile"><span className="t-meta">Storage</span><h4>0 B</h4><p>No studio masters stored client-side; R2 + CDN in production</p></div>
        </div>
      )}

      {tab === "events" && (
        <div className="form-card" style={{ marginTop: 20, maxHeight: 480, overflowY: "auto" }}>
          {events.slice().reverse().slice(0, 60).map((e, i) => (
            <div className="list-row" key={i}><div className="lr-main"><h4>{e.name}</h4><p>{JSON.stringify(e.props)} · {new Date(e.at).toLocaleTimeString()}</p></div></div>
          ))}
          {!events.length && <p className="small">No events yet — play a confession, preview voices, tweak EQ, and analytics appear here. Private content text is never recorded (§69).</p>}
        </div>
      )}

      <div style={{ marginTop: 28, display: "flex", gap: 8, alignItems: "center" }}>
        <Icon n="lock" s={14} />
        <p className="small">Demo notice: actions here write to this browser only. The production studio authenticates staff, logs every rights action, and enforces approval workflow server-side.</p>
      </div>
    </div>
  );
}
