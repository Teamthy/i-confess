"use client";

/* ============================================================================
   Minister Voices: the admin console for licensed voice cloning.

   The console follows the platform's workflow in order: rights, then
   recordings, review, datasets, training, models, preview and audit. Every
   control calls an admin endpoint that repeats its own rights and role
   checks. This UI never decides that something is allowed. When the API
   refuses, the console shows the reason and the missing capabilities.
   ========================================================================= */

import { SyntheticPlayer, type RenderedAudio } from "./synthetic-player";
import React, { useCallback, useEffect, useRef, useState } from "react";
import { adminApi, useAdminResource } from "./admin";
import { useAuth } from "@/lib/auth-context";

/* ---------------------------------------------------------------- types */

type RightsStatus = "PENDING" | "UNDER_REVIEW" | "APPROVED" | "RESTRICTED" | "EXPIRED" | "SUSPENDED" | "REVOKED";

type MinisterVoice = {
  id: string;
  name: string;
  displayName: string;
  language: string;
  locale?: string;
  accent?: string;
  status: string;
  rightsStatus: RightsStatus;
  grantVersion: number;
};

type Decision = { allowed: boolean; reason?: string; detail?: string; missing?: string[] };

type GrantView = {
  grant: {
    rightsHolder: string;
    status: RightsStatus;
    capabilities: Record<string, boolean>;
    territories: string[] | null;
    languages: string[] | null;
    expiresAt?: string;
    postTermination: string;
    version: number;
  } | null;
  effectiveStatus: RightsStatus;
  existingAssetPolicy: string;
  allCapabilities: string[];
  evaluation: Record<string, Decision>;
};

type Recording = {
  id: string;
  title: string;
  sourceType: string;
  status: string;
  durationMs?: number;
  trainingAllowed: boolean;
  processingPermission: boolean;
  error?: string;
  report?: Record<string, unknown>;
  createdAt: string;
};

type Segment = {
  id: string;
  recordingId: string;
  startMs: number;
  endMs: number;
  rawTranscript: string;
  verifiedTranscript?: string;
  asrConfidence: number | null;
  quality: number;
  snrDb: number | null;
  speakerConfidence: number | null;
  flags?: string[];
  status: string;
  rejectReason?: string;
  style?: string;
  audioUrl?: string;
};

type Dataset = {
  id: string;
  datasetVersion: string;
  totalSegments: number;
  usableSeconds: number;
  avgQuality: number;
  testSegments: number;
  manifestSha256: string;
  status: string;
  createdAt: string;
};

type TrainingRun = {
  id: string;
  datasetVersion: string;
  engine: string;
  baseModel: string;
  status: string;
  progress: number | null;
  finalLoss: number | null;
  baselineScore: number | null;
  justification: string;
  error?: string;
  modelId?: string;
  createdAt: string;
};

type Model = {
  id: string;
  engine: string;
  engine_version: string;
  model_version: string;
  mode: string;
  status: string;
  icf_voice_score: number;
  license_reviewed: boolean;
  dataset_version?: string;
};

type AuditEntry = { id: string; actor: string; action: string; decision: string; reason?: string; detail?: string; timestamp: string };

const STYLES = ["neutral", "teaching", "preaching", "prayer", "reflection", "scripture", "encouragement"];
const TABS = ["Rights", "Recordings", "Review", "Datasets", "Training", "Models", "Preview", "Audit"] as const;
type Tab = (typeof TABS)[number];

function statusPill(s: string) {
  const green = ["APPROVED", "processed", "approved", "frozen", "COMPLETED", "production"];
  const red = ["REVOKED", "SUSPENDED", "EXPIRED", "rejected", "failed", "FAILED", "REJECTED"];
  return <span className={`adm-pill ${green.includes(s) ? "green" : red.includes(s) ? "red" : "blue"}`}>{s}</span>;
}

const fmtSecs = (ms: number) => `${(ms / 1000).toFixed(1)}s`;
const pct = (v: number | null | undefined) => (v == null ? "—" : `${Math.round(v * 100)}%`);

/* ------------------------------------------------------------ root */

export function VoiceStudio() {
  const voices = useAdminResource<{ voices: MinisterVoice[] }>("/admin/minister-voices");
  const [selected, setSelected] = useState<string>("");
  const [tab, setTab] = useState<Tab>("Rights");
  const list = voices.data?.voices || [];
  const current = list.find((v) => v.id === selected) || null;

  useEffect(() => {
    if (!selected && list.length > 0) setSelected(list[0].id);
  }, [list, selected]);

  return (
    <>
      {voices.error && <div className="adm-note error">{voices.error}</div>}
      <section className="adm-card">
        <div className="adm-card-head">
          <h3>Minister voices</h3>
          <button className="btn btn-ghost btn-sm" onClick={() => voices.reload()}>
            Refresh
          </button>
        </div>
        {voices.loading && <p className="small">Loading voices…</p>}
        {!voices.loading && list.length === 0 && !voices.error && (
          <p className="small">No minister voices yet. Register one below; it authorizes nothing until approved.</p>
        )}
        <div className="adm-chips">
          {list.map((v) => (
            <button key={v.id} className={`adm-chip ${v.id === selected ? "on" : ""}`} onClick={() => setSelected(v.id)}>
              {v.displayName} <em>{v.rightsStatus}</em>
            </button>
          ))}
        </div>
        <RegisterVoice onDone={(id) => { voices.reload(); setSelected(id); setTab("Rights"); }} />
      </section>

      {current && (
        <>
          <div className="adm-toolbar" role="tablist" aria-label="Voice workflow">
            {TABS.map((t) => (
              <button key={t} role="tab" aria-selected={tab === t} className={`adm-chip ${tab === t ? "on" : ""}`} onClick={() => setTab(t)}>
                {t}
              </button>
            ))}
          </div>
          {tab === "Rights" && <RightsPanel voice={current} onChange={() => voices.reload()} />}
          {tab === "Recordings" && <RecordingsPanel voice={current} />}
          {tab === "Review" && <ReviewPanel voice={current} />}
          {tab === "Datasets" && <DatasetsPanel voice={current} />}
          {tab === "Training" && <TrainingPanel voice={current} />}
          {tab === "Models" && <ModelsPanel voice={current} />}
          {tab === "Preview" && <PreviewPanel voice={current} />}
          {tab === "Audit" && <AuditPanel voice={current} />}
        </>
      )}
    </>
  );
}

function RegisterVoice({ onDone }: { onDone: (id: string) => void }) {
  const { token } = useAuth();
  const [name, setName] = useState("");
  const [holder, setHolder] = useState("");
  const [locale, setLocale] = useState("en-NG");
  const [msg, setMsg] = useState("");
  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    const r = await adminApi<{ voice: { id: string } }>("/admin/minister-voices", token, {
      method: "POST",
      body: { name, displayName: name, rightsHolder: holder, language: "en", locale },
    });
    setMsg(r.ok ? "Registered as PENDING. Record its rights terms next." : r.message);
    if (r.ok) {
      setName("");
      setHolder("");
      onDone(r.data.voice.id);
    }
  };
  return (
    <form className="adm-toolbar" onSubmit={submit} aria-label="Register a minister voice">
      <input className="adm-input" placeholder="Minister name" value={name} onChange={(e) => setName(e.target.value)} required />
      <input className="adm-input" placeholder="Rights holder (legal name)" value={holder} onChange={(e) => setHolder(e.target.value)} required />
      <select className="adm-input" value={locale} onChange={(e) => setLocale(e.target.value)} aria-label="Locale">
        <option value="en-NG">English (Nigeria)</option>
        <option value="en-NG-PIDGIN">Nigerian Pidgin</option>
        <option value="en">English</option>
      </select>
      <button className="btn btn-primary btn-sm" type="submit">Register voice</button>
      {msg && <span className="small">{msg}</span>}
    </form>
  );
}

/* ------------------------------------------------------------ rights */

function RightsPanel({ voice, onChange }: { voice: MinisterVoice; onChange: () => void }) {
  const { token } = useAuth();
  const g = useAdminResource<GrantView>(`/admin/voices/${voice.id}/grant`);
  const [caps, setCaps] = useState<Record<string, boolean>>({});
  const [holder, setHolder] = useState("");
  const [expires, setExpires] = useState("");
  const [post, setPost] = useState("unpublish");
  const [attest, setAttest] = useState(false);
  const [reason, setReason] = useState("");
  const [doc, setDoc] = useState({ title: "", storageKey: "", sha256: "", documentType: "voice_license" });
  const [msg, setMsg] = useState("");

  useEffect(() => {
    if (g.data?.grant) {
      setCaps(g.data.grant.capabilities || {});
      setHolder(g.data.grant.rightsHolder || "");
      setExpires(g.data.grant.expiresAt ? g.data.grant.expiresAt.slice(0, 10) : "");
      setPost(g.data.grant.postTermination || "unpublish");
    }
  }, [g.data]);

  const refresh = () => { g.reload(); onChange(); };

  const saveTerms = async () => {
    const r = await adminApi(`/admin/voices/${voice.id}/grant`, token, {
      method: "PUT",
      body: { rightsHolder: holder, capabilities: caps, attestation: attest, postTermination: post,
        expiresAt: expires ? new Date(expires + "T23:59:59Z").toISOString() : null },
    });
    setMsg(r.ok ? "Terms saved. A new grant version was recorded." : r.message);
    if (r.ok) { setAttest(false); refresh(); }
  };

  const transition = async (action: string) => {
    const r = await adminApi(`/admin/voices/${voice.id}/${action}`, token, { method: "POST", body: { reason } });
    setMsg(r.ok ? `Rights status: ${action}.` : r.message);
    if (r.ok) { setReason(""); refresh(); }
  };

  const addDoc = async () => {
    const r = await adminApi(`/admin/voices/${voice.id}/rights-documents`, token, { method: "POST", body: doc });
    setMsg(r.ok ? "Document filed." : r.message);
    if (r.ok) setDoc({ title: "", storageKey: "", sha256: "", documentType: "voice_license" });
  };

  if (g.loading) return <div className="adm-note">Loading rights…</div>;
  if (g.error) return <div className="adm-note error">{g.error}</div>;
  const data = g.data!;
  return (
    <>
      {msg && <div className="adm-note">{msg}</div>}
      <div className="adm-split">
        <section className="adm-card">
          <div className="adm-card-head"><h3>Status</h3>{statusPill(data.effectiveStatus)}</div>
          <ul className="adm-list">
            <li>Grant version <b>{data.grant?.version ?? 0}</b></li>
            <li>After termination <b>{data.existingAssetPolicy}</b></li>
          </ul>
          <input className="adm-input" placeholder="Reason (required except for review/approve)" value={reason}
            onChange={(e) => setReason(e.target.value)} aria-label="Reason for status change" />
          <div className="adm-row-actions">
            <button className="btn btn-ghost btn-sm" onClick={() => transition("review")}>Send to review</button>
            <button className="btn btn-primary btn-sm" onClick={() => transition("approve")}>Approve</button>
            <button className="btn btn-ghost btn-sm" onClick={() => transition("restrict")}>Restrict</button>
            <button className="btn btn-ghost btn-sm" onClick={() => transition("suspend")}>Suspend</button>
            <button className="btn btn-ghost btn-sm" onClick={() => {
              if (window.confirm("Revocation is permanent and stops all generation, streaming, intake and training for this voice. Continue?")) void transition("revoke");
            }}>Revoke</button>
          </div>
          <h4>What these terms allow right now</h4>
          <ul className="adm-list">
            {Object.entries(data.evaluation || {}).map(([k, d]) => (
              <li key={k}>
                {k.replace(":", " · ")}
                <b title={d.missing?.join(", ")}>{d.allowed ? "allowed" : `no: ${d.missing?.join(", ") || d.reason}`}</b>
              </li>
            ))}
          </ul>
        </section>

        <section className="adm-card">
          <div className="adm-card-head"><h3>Rights terms</h3></div>
          <input className="adm-input" placeholder="Rights holder" value={holder} onChange={(e) => setHolder(e.target.value)} aria-label="Rights holder" />
          <div className="adm-toolbar">
            <label className="small">Expires <input className="adm-input" type="date" value={expires} onChange={(e) => setExpires(e.target.value)} /></label>
            <label className="small">On termination{" "}
              <select className="adm-input" value={post} onChange={(e) => setPost(e.target.value)}>
                {["retain", "archive", "unpublish", "delete"].map((p) => <option key={p}>{p}</option>)}
              </select>
            </label>
          </div>
          <div className="adm-perm-grid">
            {(data.allCapabilities || []).map((c) => (
              <label key={c} className="adm-check">
                <input type="checkbox" checked={!!caps[c]} onChange={(e) => setCaps({ ...caps, [c]: e.target.checked })} /> {c.replace(/^can_/, "").replace(/_/g, " ")}
              </label>
            ))}
          </div>
          <label className="adm-check">
            <input type="checkbox" checked={attest} onChange={(e) => setAttest(e.target.checked)} /> I confirm these terms match the signed rights document on file.
          </label>
          <button className="btn btn-primary btn-sm" disabled={!attest} onClick={saveTerms}>Save terms</button>

          <h4>File a rights document</h4>
          <div className="adm-toolbar">
            <input className="adm-input" placeholder="Title" value={doc.title} onChange={(e) => setDoc({ ...doc, title: e.target.value })} />
            <input className="adm-input" placeholder="Private storage key" value={doc.storageKey} onChange={(e) => setDoc({ ...doc, storageKey: e.target.value })} />
            <input className="adm-input" placeholder="SHA-256" value={doc.sha256} onChange={(e) => setDoc({ ...doc, sha256: e.target.value })} />
            <button className="btn btn-ghost btn-sm" onClick={addDoc} disabled={!doc.title}>File document</button>
          </div>
        </section>
      </div>
    </>
  );
}

/* ------------------------------------------------------------ recordings */

function RecordingsPanel({ voice }: { voice: MinisterVoice }) {
  const { token } = useAuth();
  const recs = useAdminResource<{ recordings: Recording[] }>(`/admin/voices/${voice.id}/recordings`);
  const [msg, setMsg] = useState("");
  const [busy, setBusy] = useState(false);
  const form = useRef<HTMLFormElement>(null);

  const upload = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!token) return;
    const fd = new FormData(e.currentTarget);
    for (const k of ["processingPermission", "trainingAllowed", "sourceAttestation"]) fd.set(k, fd.get(k) ? "true" : "false");
    setBusy(true);
    try {
      const res = await fetch(`/api/admin/voices/${encodeURIComponent(voice.id)}/recordings`, {
        method: "POST", body: fd, headers: { authorization: `Bearer ${token}` },
      });
      const data = await res.json().catch(() => ({}));
      setMsg(res.ok ? "Uploaded. Ingestion is queued." : (data as { error?: string }).error || "Upload failed.");
      if (res.ok) { form.current?.reset(); recs.reload(); }
    } catch {
      setMsg("The API is not reachable from this deployment.");
    }
    setBusy(false);
  };

  const reingest = async (id: string) => {
    const r = await adminApi(`/admin/recordings/${id}/ingest`, token, { method: "POST" });
    setMsg(r.ok ? "Re-ingestion queued. Human review decisions are kept." : r.message);
    recs.reload();
  };

  return (
    <>
      {msg && <div className="adm-note">{msg}</div>}
      <section className="adm-card">
        <div className="adm-card-head"><h3>Upload a licensed recording</h3></div>
        <p className="small">
          Only lossless WAV or FLAC supplied under a rights document on file. Never upload audio downloaded from YouTube or
          other platforms; a source URL is recorded for provenance only.
        </p>
        <form ref={form} onSubmit={upload} className="adm-toolbar">
          <input className="adm-input" type="file" name="file" accept=".wav,.flac" required />
          <input className="adm-input" name="title" placeholder="Title" required />
          <input className="adm-input" name="rightsDocumentId" placeholder="Rights document ID" required />
          <select className="adm-input" name="sourceType" aria-label="Source type">
            <option value="studio_session">Studio session</option>
            <option value="upload">Upload from rights holder</option>
            <option value="authorized_archive">Authorized archive</option>
          </select>
          <input className="adm-input" name="recordingDate" type="date" aria-label="Recording date" />
          <input className="adm-input" name="sourceUrl" placeholder="Source URL (provenance only)" />
          <label className="adm-check"><input type="checkbox" name="processingPermission" defaultChecked /> Processing permitted</label>
          <label className="adm-check"><input type="checkbox" name="trainingAllowed" /> Training permitted</label>
          <label className="adm-check"><input type="checkbox" name="sourceAttestation" required /> Supplied under the rights document, not downloaded</label>
          <button className="btn btn-primary btn-sm" disabled={busy}>{busy ? "Uploading…" : "Upload"}</button>
        </form>
      </section>
      <section className="adm-card">
        <div className="adm-card-head"><h3>Recordings</h3><button className="btn btn-ghost btn-sm" onClick={() => recs.reload()}>Refresh</button></div>
        {recs.error && <div className="adm-note error">{recs.error}</div>}
        <div className="adm-table-wrap">
          <table className="adm-table">
            <thead><tr><th>Title</th><th>Status</th><th>Duration</th><th>Permissions</th><th>Pipeline</th><th /></tr></thead>
            <tbody>
              {(recs.data?.recordings || []).map((r) => (
                <tr key={r.id}>
                  <td><b>{r.title}</b><small>{r.sourceType} · {r.createdAt.slice(0, 10)}</small></td>
                  <td>{statusPill(r.status)}{r.error && <small>{r.error}</small>}</td>
                  <td>{r.durationMs ? fmtSecs(r.durationMs) : "—"}</td>
                  <td><small>{r.processingPermission ? "process" : "no processing"} · {r.trainingAllowed ? "train" : "no training"}</small></td>
                  <td><small>{r.report ? `VAD ${String(r.report.vad)} · ASR ${String(r.report.asr)} · speaker ${String(r.report.speaker_check)}` : "—"}</small></td>
                  <td>{r.processingPermission && <button className="btn btn-ghost btn-sm" onClick={() => reingest(r.id)}>Re-ingest</button>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </>
  );
}

/* ------------------------------------------------------------ review */

function ReviewPanel({ voice }: { voice: MinisterVoice }) {
  const { token } = useAuth();
  const [filter, setFilter] = useState("pending");
  const segs = useAdminResource<{ segments: Segment[] }>(`/admin/voices/${voice.id}/segments?status=${filter}`);
  const [draft, setDraft] = useState<Record<string, { text: string; style: string; reason: string }>>({});
  const [msg, setMsg] = useState("");

  const d = (s: Segment) => draft[s.id] || { text: s.verifiedTranscript || s.rawTranscript || "", style: s.style || "", reason: "" };

  const review = async (s: Segment, approve: boolean) => {
    const v = d(s);
    const r = await adminApi(`/admin/segments/${s.id}/review`, token, {
      method: "POST", body: { approve, verifiedTranscript: v.text, style: v.style, reason: v.reason },
    });
    setMsg(r.ok ? (approve ? "Approved with verified transcript." : "Rejected.") : r.message);
    if (r.ok) segs.reload();
  };

  return (
    <>
      {msg && <div className="adm-note">{msg}</div>}
      <section className="adm-card">
        <div className="adm-card-head">
          <h3>Segment review</h3>
          <div className="adm-chips">
            {["pending", "approved", "rejected"].map((f) => (
              <button key={f} className={`adm-chip ${filter === f ? "on" : ""}`} onClick={() => setFilter(f)}>{f}</button>
            ))}
          </div>
        </div>
        <p className="small">
          Listen to every clip. Confirm it is the licensed minister, then type or correct the exact words. The ASR draft is only a
          starting point: nothing enters a dataset without a transcript a human has verified. Speaker scores are a heuristic.
        </p>
        {segs.error && <div className="adm-note error">{segs.error}</div>}
        {!segs.loading && (segs.data?.segments || []).length === 0 && <p className="small">Nothing here.</p>}
        {(segs.data?.segments || []).map((s) => {
          const v = d(s);
          const set = (patch: Partial<typeof v>) => setDraft({ ...draft, [s.id]: { ...v, ...patch } });
          return (
            <div key={s.id} className="adm-card" style={{ marginTop: 12 }}>
              <div className="adm-toolbar">
                {s.audioUrl ? <audio controls preload="none" src={s.audioUrl} /> : <span className="small">no audio URL</span>}
                <span className="small">{fmtSecs(s.endMs - s.startMs)} · quality {pct(s.quality)} · SNR {s.snrDb?.toFixed(1) ?? "—"} dB · speaker {pct(s.speakerConfidence)} · ASR {pct(s.asrConfidence)}</span>
                {(s.flags || []).map((f) => <span key={f} className="adm-pill red">{f}</span>)}
                {statusPill(s.status)}
              </div>
              <textarea className="adm-input" rows={2} style={{ width: "100%" }} value={v.text} placeholder="Verified transcript"
                onChange={(e) => set({ text: e.target.value })} aria-label="Verified transcript" />
              <div className="adm-row-actions">
                <select className="adm-input" value={v.style} onChange={(e) => set({ style: e.target.value })} aria-label="Style">
                  <option value="">style…</option>
                  {STYLES.map((st) => <option key={st}>{st}</option>)}
                </select>
                <button className="btn btn-primary btn-sm" disabled={!v.text.trim()} onClick={() => review(s, true)}>Approve</button>
                <input className="adm-input" placeholder="Reject reason" value={v.reason} onChange={(e) => set({ reason: e.target.value })} />
                <button className="btn btn-ghost btn-sm" disabled={!v.reason.trim()} onClick={() => review(s, false)}>Reject</button>
              </div>
            </div>
          );
        })}
      </section>
    </>
  );
}

/* ------------------------------------------------------------ datasets */

function DatasetsPanel({ voice }: { voice: MinisterVoice }) {
  const { token } = useAuth();
  const ds = useAdminResource<{ datasets: Dataset[] }>(`/admin/voices/${voice.id}/datasets`);
  const [msg, setMsg] = useState("");
  const freeze = async () => {
    const r = await adminApi<{ dataset: Dataset }>(`/admin/voices/${voice.id}/datasets`, token, { method: "POST" });
    setMsg(r.ok ? `Frozen ${r.data.dataset.datasetVersion}: ${r.data.dataset.totalSegments} segments.` : r.message);
    if (r.ok) ds.reload();
  };
  return (
    <>
      {msg && <div className="adm-note">{msg}</div>}
      <section className="adm-card">
        <div className="adm-card-head">
          <h3>Dataset versions</h3>
          <button className="btn btn-primary btn-sm" onClick={freeze}>Freeze approved segments</button>
        </div>
        <p className="small">A frozen dataset is immutable and hashed. Only approved segments from recordings whose licence permits training are included. The split is deterministic, so test clips never leak into training.</p>
        <div className="adm-table-wrap">
          <table className="adm-table">
            <thead><tr><th>Version</th><th>Segments</th><th>Usable audio</th><th>Avg quality</th><th>Held out</th><th>Manifest</th></tr></thead>
            <tbody>
              {(ds.data?.datasets || []).map((d) => (
                <tr key={d.id}>
                  <td><b>{d.datasetVersion}</b><small>{d.createdAt.slice(0, 10)}</small></td>
                  <td>{d.totalSegments}</td>
                  <td>{Math.round(d.usableSeconds / 60)} min</td>
                  <td>{pct(d.avgQuality)}</td>
                  <td>{d.testSegments} test</td>
                  <td><code className="adm-code">{d.manifestSha256.slice(0, 12)}…</code></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </>
  );
}

/* ------------------------------------------------------------ training */

function TrainingPanel({ voice }: { voice: MinisterVoice }) {
  const { token } = useAuth();
  const runs = useAdminResource<{ runs: TrainingRun[] }>(`/admin/voices/${voice.id}/training-runs`);
  const ds = useAdminResource<{ datasets: Dataset[] }>(`/admin/voices/${voice.id}/datasets`);
  const [form, setForm] = useState({ datasetId: "", engine: "gpt_sovits", baseModel: "", justification: "", override: false, hyperparameters: "{}" });
  const [msg, setMsg] = useState("");

  useEffect(() => {
    const t = setInterval(() => {
      if ((runs.data?.runs || []).some((r) => r.status === "RUNNING" || r.status === "QUEUED")) runs.reload();
    }, 10000);
    return () => clearInterval(t);
  }, [runs]);

  const submit = async () => {
    let hp: unknown;
    try { hp = JSON.parse(form.hyperparameters || "{}"); } catch { setMsg("Hyperparameters must be valid JSON."); return; }
    const r = await adminApi(`/admin/voices/${voice.id}/training-runs`, token, { method: "POST", body: { ...form, hyperparameters: hp } });
    setMsg(r.ok ? "Run queued. Its result becomes a candidate model; it is never promoted automatically." : r.message);
    if (r.ok) runs.reload();
  };
  const cancel = async (id: string) => {
    const r = await adminApi(`/admin/training-runs/${id}/cancel`, token, { method: "POST" });
    setMsg(r.ok ? "Run cancelled." : r.message);
    runs.reload();
  };

  return (
    <>
      {msg && <div className="adm-note">{msg}</div>}
      <section className="adm-card">
        <div className="adm-card-head"><h3>Request fine-tuning</h3></div>
        <p className="small">Zero-shot comes first. A run needs an evaluated zero-shot baseline and a written justification. If zero-shot already meets the configured bar, you must explicitly override.</p>
        <div className="adm-toolbar">
          <select className="adm-input" value={form.datasetId} onChange={(e) => setForm({ ...form, datasetId: e.target.value })} aria-label="Dataset">
            <option value="">dataset…</option>
            {(ds.data?.datasets || []).map((d) => <option key={d.id} value={d.id}>{d.datasetVersion} ({Math.round(d.usableSeconds / 60)} min)</option>)}
          </select>
          <select className="adm-input" value={form.engine} onChange={(e) => setForm({ ...form, engine: e.target.value })} aria-label="Engine">
            <option value="gpt_sovits">GPT-SoVITS</option>
            <option value="cosyvoice">CosyVoice</option>
          </select>
          <input className="adm-input" placeholder="Base model" value={form.baseModel} onChange={(e) => setForm({ ...form, baseModel: e.target.value })} />
        </div>
        <textarea className="adm-input" rows={2} style={{ width: "100%" }} placeholder="Justification: what does zero-shot get wrong?"
          value={form.justification} onChange={(e) => setForm({ ...form, justification: e.target.value })} />
        <textarea className="adm-input" rows={2} style={{ width: "100%" }} placeholder='Hyperparameters JSON, e.g. {"epochs": 8}'
          value={form.hyperparameters} onChange={(e) => setForm({ ...form, hyperparameters: e.target.value })} />
        <div className="adm-row-actions">
          <label className="adm-check"><input type="checkbox" checked={form.override} onChange={(e) => setForm({ ...form, override: e.target.checked })} /> Override “zero-shot is sufficient”</label>
          <button className="btn btn-primary btn-sm" disabled={!form.datasetId || !form.baseModel || !form.justification.trim()} onClick={submit}>Queue run</button>
        </div>
      </section>
      <section className="adm-card">
        <div className="adm-card-head"><h3>Runs</h3><button className="btn btn-ghost btn-sm" onClick={() => runs.reload()}>Refresh</button></div>
        <div className="adm-table-wrap">
          <table className="adm-table">
            <thead><tr><th>Run</th><th>Status</th><th>Progress</th><th>Loss</th><th>Baseline</th><th>Candidate</th><th /></tr></thead>
            <tbody>
              {(runs.data?.runs || []).map((r) => (
                <tr key={r.id}>
                  <td><b>{r.engine} · {r.datasetVersion}</b><small>{r.justification}</small></td>
                  <td>{statusPill(r.status)}{r.error && <small>{r.error}</small>}</td>
                  <td>{pct(r.progress)}</td>
                  <td>{r.finalLoss?.toFixed(3) ?? "—"}</td>
                  <td>{r.baselineScore?.toFixed(3) ?? "—"}</td>
                  <td><small>{r.modelId || "—"}</small></td>
                  <td>{(r.status === "QUEUED" || r.status === "RUNNING") && <button className="btn btn-ghost btn-sm" onClick={() => cancel(r.id)}>Cancel</button>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </>
  );
}

/* ------------------------------------------------------------ models */

function ModelsPanel({ voice }: { voice: MinisterVoice }) {
  const { token } = useAuth();
  const models = useAdminResource<{ models: Model[] }>(`/admin/voices/${voice.id}/models`);
  const [msg, setMsg] = useState("");
  const promote = async (m: Model) => {
    if (!window.confirm(`Promote ${m.model_version} to production for ${voice.displayName}? The current production model is retired and stays available for rollback.`)) return;
    const r = await adminApi(`/admin/voices/${voice.id}/models/${m.id}/promote`, token, { method: "POST" });
    setMsg(r.ok ? "Promoted." : r.message);
    models.reload();
  };
  const rollback = async () => {
    const r = await adminApi(`/admin/voices/${voice.id}/rollback`, token, { method: "POST" });
    setMsg(r.ok ? "Rolled back." : r.message);
    models.reload();
  };
  return (
    <>
      {msg && <div className="adm-note">{msg}</div>}
      <section className="adm-card">
        <div className="adm-card-head"><h3>Model versions</h3><button className="btn btn-ghost btn-sm" onClick={rollback}>Roll back production</button></div>
        <p className="small">ICF_VOICE_SCORE is an internal comparative composite, not an industry similarity percentage. Promotion requires a passing gate (golden set, blind evaluation, human approval), a reviewed engine licence, and current rights.</p>
        <div className="adm-table-wrap">
          <table className="adm-table">
            <thead><tr><th>Version</th><th>Engine</th><th>Mode</th><th>Status</th><th>ICF score</th><th>Licence</th><th /></tr></thead>
            <tbody>
              {(models.data?.models || []).map((m) => (
                <tr key={m.id}>
                  <td><b>{m.model_version}</b><small>{m.dataset_version || ""}</small></td>
                  <td>{m.engine} {m.engine_version}</td>
                  <td>{m.mode}</td>
                  <td>{statusPill(m.status)}</td>
                  <td>{m.icf_voice_score ? m.icf_voice_score.toFixed(3) : "—"}</td>
                  <td>{m.license_reviewed ? "reviewed" : <span className="adm-pill red">not reviewed</span>}</td>
                  <td>{(m.status === "approved" || m.status === "retired") && <button className="btn btn-primary btn-sm" onClick={() => promote(m)}>Promote</button>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </>
  );
}

/* ------------------------------------------------------------ preview (streaming) */

/** Plays the /voices/stream WAV body as it arrives, using Web Audio. The
 *  response is a 44-byte WAV header followed by 16-bit mono PCM. Each chunk is
 *  scheduled back to back, so playback starts before synthesis finishes. */
async function playStream(res: Response, onStart: () => void, signal: AbortSignal): Promise<void> {
  const reader = res.body!.getReader();
  const ctx = new AudioContext();
  signal.addEventListener("abort", () => { void ctx.close(); });
  let header = new Uint8Array(0);
  let rate = 0;
  let carry: Uint8Array | null = null;
  let at = ctx.currentTime + 0.15;
  let started = false;
  for (;;) {
    const { value, done } = await reader.read();
    if (done || signal.aborted) break;
    let bytes: Uint8Array = value;
    if (rate === 0) {
      const merged = new Uint8Array(header.length + bytes.length);
      merged.set(header);
      merged.set(bytes, header.length);
      if (merged.length < 44) { header = merged; continue; }
      rate = new DataView(merged.buffer).getUint32(24, true);
      bytes = merged.slice(44);
    }
    if (carry) {
      const m = new Uint8Array(carry.length + bytes.length);
      m.set(carry);
      m.set(bytes, carry.length);
      bytes = m;
      carry = null;
    }
    if (bytes.length % 2) { carry = bytes.slice(-1); bytes = bytes.slice(0, -1); }
    if (bytes.length === 0) continue;
    const pcm = new Int16Array(bytes.buffer, bytes.byteOffset, bytes.length / 2);
    const buf = ctx.createBuffer(1, pcm.length, rate);
    const ch = buf.getChannelData(0);
    for (let i = 0; i < pcm.length; i++) ch[i] = pcm[i] / 32768;
    const src = ctx.createBufferSource();
    src.buffer = buf;
    src.connect(ctx.destination);
    at = Math.max(at, ctx.currentTime);
    src.start(at);
    at += buf.duration;
    if (!started) { started = true; onStart(); }
  }
}

function PreviewPanel({ voice }: { voice: MinisterVoice }) {
  const { token } = useAuth();
  const [text, setText] = useState("Grace and peace to you. Be still, and know that He is God.");
  const [style, setStyle] = useState("reflection");
  const [state, setState] = useState("");
  const [rendered, setRendered] = useState<RenderedAudio | null>(null);
  const abort = useRef<AbortController | null>(null);

  /** Queue a real (cached, mastered, encoded) render and poll until done. */
  const renderMastered = async () => {
    setRendered(null);
    setState("Queued…");
    const headers = { "content-type": "application/json", authorization: `Bearer ${token}` };
    const res = await fetch("/api/v1/voices/generate", { method: "POST", headers,
      body: JSON.stringify({ voiceId: voice.id, text, style, purpose: "reflection" }) });
    const data = (await res.json().catch(() => ({}))) as { error?: string; generation?: { generationId: string; status: string } } & RenderedAudio;
    if (!res.ok || !data.generation) { setState(data.error || `Refused (${res.status}).`); return; }
    const id = data.generation.generationId;
    for (let i = 0; i < 60; i++) {
      const j = (await (await fetch(`/api/v1/audio/jobs/${id}`, { headers })).json()) as typeof data;
      const st = j.generation?.status || "";
      if (st === "COMPLETED") {
        // Encodings are derived just after completion; give them a moment.
        if (!j.variants && i < 58) { await new Promise((r) => setTimeout(r, 700)); continue; }
        setRendered(j); setState(""); return;
      }
      if (st === "FAILED" || st === "CANCELLED") { setState(`Render ${st.toLowerCase()}.`); return; }
      setState(`${st.toLowerCase()}…`);
      await new Promise((r) => setTimeout(r, 1000));
    }
    setState("Still rendering; check the job later.");
  };

  const stop = useCallback(() => { abort.current?.abort(); abort.current = null; setState("Stopped."); }, []);
  useEffect(() => () => abort.current?.abort(), []);

  const play = async () => {
    abort.current?.abort();
    const ac = new AbortController();
    abort.current = ac;
    setState("Requesting…");
    const t0 = performance.now();
    try {
      const res = await fetch("/api/voices/stream", {
        method: "POST", signal: ac.signal,
        headers: { "content-type": "application/json", authorization: `Bearer ${token}` },
        body: JSON.stringify({ voiceId: voice.id, text, style, purpose: "reflection" }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        setState((data as { error?: string }).error || `Refused (${res.status}).`);
        return;
      }
      await playStream(res, () => setState(`Playing. First audio after ${Math.round(performance.now() - t0)} ms.`), ac.signal);
    } catch (e) {
      if (!ac.signal.aborted) setState("The stream could not be played.");
    }
  };

  return (
    <section className="adm-card">
      <div className="adm-card-head"><h3>Live preview</h3><span className="adm-pill blue">AI-generated · synthetic voice</span></div>
      <p className="small">Streams from the GPU worker for a quick listen. Streams are not cached and are only roughly loudness-matched; the queued render is the mastered asset. Requires the can_stream right.</p>
      <textarea className="adm-input" rows={3} style={{ width: "100%" }} value={text} onChange={(e) => setText(e.target.value)} aria-label="Preview text" />
      <div className="adm-row-actions">
        <select className="adm-input" value={style} onChange={(e) => setStyle(e.target.value)} aria-label="Style">
          {STYLES.map((s) => <option key={s}>{s}</option>)}
        </select>
        <button className="btn btn-primary btn-sm" onClick={play}>Stream</button>
        <button className="btn btn-ghost btn-sm" onClick={stop}>Stop</button>
        <button className="btn btn-ghost btn-sm" onClick={renderMastered}>Render mastered</button>
        {state && <span className="small">{state}</span>}
      </div>
      {rendered && <SyntheticPlayer render={rendered} />}
    </section>
  );
}

/* ------------------------------------------------------------ audit */

function AuditPanel({ voice }: { voice: MinisterVoice }) {
  const audit = useAdminResource<{ entries?: AuditEntry[]; audit?: AuditEntry[] }>(`/admin/voices/${voice.id}/audit`);
  const rows = audit.data?.entries || audit.data?.audit || [];
  return (
    <section className="adm-card">
      <div className="adm-card-head"><h3>Rights audit log</h3><button className="btn btn-ghost btn-sm" onClick={() => audit.reload()}>Refresh</button></div>
      {audit.error && <div className="adm-note error">{audit.error}</div>}
      <div className="adm-table-wrap">
        <table className="adm-table">
          <thead><tr><th>When</th><th>Actor</th><th>Action</th><th>Decision</th><th>Detail</th></tr></thead>
          <tbody>
            {rows.map((e) => (
              <tr key={e.id}>
                <td><small>{e.timestamp.slice(0, 19).replace("T", " ")}</small></td>
                <td>{e.actor}</td>
                <td><b>{e.action}</b></td>
                <td>{e.decision}{e.reason && <small>{e.reason}</small>}</td>
                <td><small>{e.detail}</small></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
