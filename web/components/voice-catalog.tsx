"use client";
/* iCONFESS Audio + Voice Engine — voice discovery (§97), voice profile (§98),
   in-app voice browser (§12–§15), sound panel (§65–§66), profiles (§25). */
import React, { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { Icon, EmptyState } from "./ui";
import { useApp, mutate } from "@/lib/store";
import { useToast } from "@/lib/ui";
import { VOICE_CATALOG, effectiveVoices, searchVoices, voiceAvailable, SOUND_PROFILES, AMBIENCES, type CatalogVoice } from "@/lib/voices";
import { motifStyle, CONFESSIONS } from "@/lib/data";
import { resolveVoice, logAE, createJob, loadJobs } from "@/lib/audio-engine";

export function VoiceOrb({ v, size = 72 }: { v: CatalogVoice; size?: number }) {
  const hasPhoto = v.slug === "grace";
  if (hasPhoto) return <img src="/assets/voice-grace.jpg" alt={v.displayName} style={{ width: size, height: size, borderRadius: "50%", objectFit: "cover" }} />;
  return (
    <span aria-hidden="true" style={{ width: size, height: size, borderRadius: "50%", display: "inline-flex", alignItems: "center", justifyContent: "center", fontSize: size / 2.6, fontWeight: 700, color: "#fff", ...motifStyle(v.slug) }}>
      {v.displayName.slice(0, 1)}
    </span>
  );
}

/* §13 cached, short preview — rendered on-device through the same provider path as sessions */
export function VoicePreview({ v, label = "Preview" }: { v: CatalogVoice; label?: string }) {
  const [on, setOn] = useState(false);
  const uRef = useRef<SpeechSynthesisUtterance | null>(null);
  useEffect(() => () => { try { window.speechSynthesis?.cancel(); } catch { } }, []);
  const toggle = () => {
    if (on) { try { window.speechSynthesis.cancel(); } catch { } setOn(false); return; }
    try {
      if (typeof window === "undefined" || !window.speechSynthesis) return;
      window.speechSynthesis.cancel();
      const { resolved } = resolveVoice(v.id);
      const u = new SpeechSynthesisUtterance(v.previewText);
      if (resolved?.synthVoice) u.voice = resolved.synthVoice;
      u.rate = resolved?.rateMul ?? 1; u.pitch = resolved?.pitchMul ?? 1;
      u.onend = () => setOn(false); u.onerror = () => setOn(false);
      uRef.current = u; setOn(true); window.speechSynthesis.speak(u);
      logAE("voice_previewed", { voiceId: v.id });
    } catch { setOn(false); }
  };
  return <button type="button" className={"btn btn-sm " + (on ? "btn-primary" : "btn-ghost")} onClick={toggle} aria-label={(on ? "Stop preview of " : "Preview ") + v.displayName}><Icon n={on ? "pause" : "play"} s={12} /> {on ? "Playing" : label}</button>;
}

const RIGHT_TAG: Record<string, string> = { APPROVED: "shared", PENDING: "pending", RESTRICTED: "private", EXPIRED: "private", REVOKED: "private" };

export function VoiceCard({ v }: { v: CatalogVoice }) {
  const st = useApp(); const toast = useToast();
  const avail = voiceAvailable(v);
  const use = () => {
    if (!avail) { toast(v.displayName + " is in " + v.status.toLowerCase().replace("_", " ") + " — not live yet"); return; }
    if (v.premium && st.user?.plan === "free") { toast("Premium voices unlock with a plan"); return; }
    mutate((s) => { s.settings.voiceId = v.slug; });
    logAE("voice_selected", { voiceId: v.id });
    toast("Narration voice: " + v.displayName);
  };
  return (
    <div className="v-card vc-2" data-avail={avail ? "1" : "0"}>
      <div style={{ display: "flex", gap: 14, alignItems: "center" }}>
        <VoiceOrb v={v} size={58} />
        <div style={{ minWidth: 0 }}>
          <h3 style={{ margin: 0 }}>{v.displayName}</h3>
          <span className="v-style">{v.accent} · {v.genderPresentation}</span>
          <div style={{ display: "flex", gap: 6, marginTop: 6, flexWrap: "wrap" }}>
            <span className={"tag " + (RIGHT_TAG[v.rightsStatus] || "private")}>{v.rightsStatus.toLowerCase()}</span>
            {v.premium && <span className="tag private">premium</span>}
            {v.locale === "en-NG-PIDGIN" && <span className="tag shared">pidgin</span>}
          </div>
        </div>
      </div>
      <p className="v-desc">{v.description}</p>
      <div className="chip-row" style={{ marginTop: 4 }}>{v.styles.slice(0, 3).map((s) => <span className="chip" key={s} style={{ pointerEvents: "none" }}>{s}</span>)}</div>
      <div style={{ display: "flex", gap: 8, marginTop: 12, alignItems: "center" }}>
        <VoicePreview v={v} />
        <button type="button" className="btn btn-primary btn-sm" onClick={use}>Use voice</button>
        <Link className="textlink" href={`/voices/${v.slug}`} style={{ marginLeft: "auto" }}>Profile <Icon n="arrow" s={12} /></Link>
      </div>
    </div>
  );
}

/* §97 public discovery — search + filters + curated sections */
const REGIONS = ["", "Nigeria", "West Africa", "Europe", "North America", "Global"];
export function VoicesDiscovery() {
  const [q, setQ] = useState("");
  const [region, setRegion] = useState("");
  const [pidgin, setPidgin] = useState(false);
  const [free, setFree] = useState(false);
  const [voices, setVoices] = useState<CatalogVoice[]>(VOICE_CATALOG);
  useEffect(() => { setVoices(effectiveVoices()); }, []);
  const pool = voices.filter((v) => v.status !== "SUSPENDED" && v.rightsStatus !== "REVOKED");
  const filtered = searchVoices(pool, q, { region: region || undefined, free: free || undefined });
  const shown = pidgin ? filtered.filter((v) => v.locale === "en-NG-PIDGIN") : filtered;
  const searching = !!(q || region || pidgin || free);
  const sections = ([
    ["Featured", shown.filter((v) => v.curated === "featured")],
    ["Nigerian voices", shown.filter((v) => v.region === "Nigeria" && v.curated !== "featured")],
    ["West African voices", shown.filter((v) => v.region === "West Africa")],
    ["European voices", shown.filter((v) => v.region === "Europe")],
    ["North American voices", shown.filter((v) => v.region === "North America")],
    ["Global · accent-neutral", shown.filter((v) => v.region === "Global")],
  ] as [string, CatalogVoice[]][]).filter((s) => s[1].length > 0);
  return (
    <section className="section container" style={{ paddingTop: "calc(var(--header-h) + 56px)" }}>
      <div className="section-head" style={{ textAlign: "center", maxWidth: 720, margin: "0 auto" }}>
        <span className="eyebrow">Voice Library</span>
        <h1 className="h1" style={{ marginTop: 10 }}>Find a voice that fits <span className="accent">the moment.</span></h1>
        <p className="lede" style={{ marginTop: 14 }}>Every voice is original, licensed and rights-tracked. Nigerian English, Nigerian Pidgin, West African, British, American and neutral — filtered honestly, never impersonating anyone.</p>
      </div>
      <div className="vc-toolbar" role="search">
        <input type="search" aria-label="Search voices" placeholder="Search: Nigerian · Pidgin · calm · preacher · British…" value={q} onChange={(e) => setQ(e.target.value)} />
        <div className="chip-row" style={{ justifyContent: "center" }}>
          {REGIONS.map((r) => <button key={r || "all"} className="chip" aria-pressed={region === r} onClick={() => setRegion(r)}>{r || "All regions"}</button>)}
          <button className="chip" aria-pressed={pidgin} onClick={() => setPidgin(!pidgin)}>Pidgin</button>
          <button className="chip" aria-pressed={free} onClick={() => setFree(!free)}>Free</button>
        </div>
      </div>
      {searching ? (
        <div className="app-section" style={{ marginTop: 8 }}>
          <div className="section-head"><h3 className="h3">{shown.length} voice{shown.length === 1 ? "" : "s"}</h3></div>
          {shown.length ? <div className="voice-grid vc-grid">{shown.map((v) => <VoiceCard key={v.id} v={v} />)}</div> : <EmptyState icon="mic" title="No voices match" sub="Try clearing a filter — the catalog grows as rights clear." />}
        </div>
      ) : sections.map(([title, list]) => (
        <div className="app-section" key={title}>
          <div className="section-head"><h3 className="h3">{title}</h3><span className="small">{list.length}</span></div>
          <div className="voice-grid vc-grid">{list.map((v) => <VoiceCard key={v.id} v={v} />)}</div>
        </div>
      ))}
      <div className="form-card" style={{ marginTop: 40, maxWidth: 760, marginLeft: "auto", marginRight: "auto" }}>
        <h4>Rights, plainly.</h4>
        <p className="small" style={{ marginTop: 6 }}>Each voice carries a rights record — owner, licence reference, territories, expiry. Voices in <b>rights review</b> or <b>testing</b> cannot generate production audio; expired or revoked licences are pulled automatically. Nothing here imitates a real, identifiable person.</p>
      </div>
    </section>
  );
}

/* §74 on-demand generation — "Generating your audio..." then Ready (simulated pipeline) */
function GenerateMaster({ v }: { v: CatalogVoice }) {
  const [jobId, setJobId] = useState<string | null>(null);
  const [status, setStatus] = useState<string>("");
  useEffect(() => {
    if (!jobId) return;
    const t = window.setInterval(() => {
      const j = loadJobs().find((x) => x.id === jobId);
      if (j) { setStatus(j.status); if (j.status === "READY" || j.status === "FAILED") window.clearInterval(t); }
    }, 400);
    return () => window.clearInterval(t);
  }, [jobId]);
  const run = () => {
    const c = CONFESSIONS.find((x) => x.category && v.categories.includes(x.category.toLowerCase())) || CONFESSIONS[0];
    const { job, reused } = createJob(c.slug, c.title, v);
    setJobId(job.id); setStatus(reused ? "READY (cached)" : "QUEUED");
  };
  if (!jobId) return <button type="button" className="btn btn-ghost btn-sm" onClick={run}>Generate studio master</button>;
  return <span className="tag pending" style={{ alignSelf: "center" }}>{status === "READY (cached)" ? "Ready — reused cached master" : "Generating your audio… " + status.toLowerCase()}</span>;
}

/* §98 voice profile */
export function VoiceProfile({ slug }: { slug: string }) {
  const [v, setV] = useState<CatalogVoice | null>(() => VOICE_CATALOG.find((x) => x.slug === slug) || null);
  useEffect(() => { setV(effectiveVoices().find((x) => x.slug === slug) || null); }, [slug]);
  if (!v) return null;
  const avail = voiceAvailable(v);
  const featured = v.categories.flatMap((cat) => CONFESSIONS.filter((c) => c.category.toLowerCase() === cat).slice(0, 1)).slice(0, 4);
  return (
    <section className="container detail-hero">
      <Link className="textlink" href="/voices"><Icon n="arrowL" s={12} /> Voices</Link>
      <div style={{ display: "flex", gap: 28, alignItems: "center", marginTop: 28, flexWrap: "wrap" }}>
        <VoiceOrb v={v} size={140} />
        <div>
          <h1 className="h1">{v.displayName}</h1>
          <p className="lede" style={{ marginTop: 8 }}>{v.description}</p>
          <div style={{ display: "flex", gap: 10, marginTop: 14, flexWrap: "wrap" }}>
            <span className={"tag " + (RIGHT_TAG[v.rightsStatus] || "private")}>rights {v.rightsStatus.toLowerCase()}</span>
            <span className="tag private">{v.locale}</span>
            <span className="tag private">{v.accent}</span>
            {v.premium && <span className="tag pending">premium</span>}
          </div>
          <div style={{ marginTop: 18, display: "flex", gap: 10 }}>
            <VoicePreview v={v} label="Play preview" />
            {avail && <Link className="btn btn-primary btn-sm" href="/app">Use this voice</Link>}
            {avail && <GenerateMaster v={v} />}
          </div>
        </div>
      </div>
      <div className="card-row" style={{ marginTop: 40 }}>
        <div className="tile"><span className="t-meta">Style</span><h4>{v.styles.join(" · ")}</h4><p>{v.tone} · pace {v.pace}</p></div>
        <div className="tile"><span className="t-meta">Style profile</span><h4>Warm {v.styleProfile.warm} · Depth {v.styleProfile.depth}</h4><p>Energy {v.styleProfile.energy} · Pace {v.styleProfile.pace} · Presence {v.styleProfile.presence}</p></div>
        <div className="tile"><span className="t-meta">Rights</span><h4>{v.rights.rightsOwner}</h4><p>{v.rights.licenseReference} · {v.rights.territories}{v.rights.expirationDate ? " · expires " + v.rights.expirationDate : ""}</p></div>
        <div className="tile"><span className="t-meta">Curated for</span><h4>{v.categories.length} categories</h4><p>{v.categories.slice(0, 6).join(", ")}{v.categories.length > 6 ? "…" : ""}</p></div>
      </div>
      <div className="app-section" style={{ marginTop: 48 }}>
        <div className="section-head"><h3 className="h3">Preview line</h3></div>
        <p className="lede" style={{ maxWidth: 640 }}>“{v.previewText}”</p>
        {featured.length ? <div className="grid4 snap">{featured.map((c) => <Link className="tile" key={c.slug} href={`/confessions/${c.slug}`}><span className="t-meta">{c.category}</span><h4>{c.title}</h4></Link>)}</div> : <p className="small" style={{ marginTop: 12 }}>Featured content for this voice appears once its masters are published.</p>}
      </div>
    </section>
  );
}

/* §65–§66 sound panel — basic mode first, advanced on request; all client-side, instant */
export function SoundPanel({ compact }: { compact?: boolean }) {
  const st = useApp(); const toast = useToast();
  const [adv, setAdv] = useState(false);
  const s = st.settings;
  const setNum = (k: "bass" | "mid" | "treble" | "warmth" | "clarity" | "presence" | "ambienceLevel") => (e: React.ChangeEvent<HTMLInputElement>) => {
    const val = Number(e.target.value);
    mutate((x) => { (x.settings as unknown as Record<string, number>)[k] = val; x.settings.profileId = "custom"; });
    logAE("eq_changed", { k, val });
  };
  const slider = (label: string, k: "bass" | "mid" | "treble" | "warmth" | "clarity" | "presence", min: number, max: number, fmt: (n: number) => string) => (
    <div className="setting-row" key={k}><div><b>{label}</b><span>{fmt(s[k])}</span></div><input type="range" min={min} max={max} step={1} value={s[k]} onChange={setNum(k)} aria-label={label} style={{ width: 150 }} /></div>
  );
  const applyProfile = (id: string) => {
    const all = SOUND_PROFILES.find((x) => x.id === id);
    const custom = st.audioProfiles.find((x) => "u" + x.id === id);
    if (all) { mutate((x) => { Object.assign(x.settings, all.config); x.settings.profileId = id; }); toast(all.name + " profile"); }
    else if (custom) { mutate((x) => { Object.assign(x.settings, custom.cfg); x.settings.profileId = id; }); toast(custom.name + " profile"); }
    logAE("profile_applied", { id });
  };
  const saveProfile = () => {
    const name = window.prompt("Name this sound profile", "My Evening");
    if (!name) return;
    mutate((x) => { x.audioProfiles = [...x.audioProfiles, { id: Date.now(), name, cfg: { bass: s.bass, mid: s.mid, treble: s.treble, warmth: s.warmth, clarity: s.clarity, presence: s.presence, ambienceLevel: s.ambienceLevel, rate: s.rate } }]; });
    toast("Profile saved: " + name);
  };
  return (
    <div className="form-card sound-panel" style={compact ? { maxWidth: 560 } : undefined}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <h4>Sound</h4>
        <button type="button" className="textlink" onClick={() => setAdv(!adv)} aria-expanded={adv}>{adv ? "Basic" : "Advanced"}</button>
      </div>
      <div className="setting-row"><div><b>Volume</b><span>{Math.round(s.volume * 100)}%</span></div><input type="range" min={0} max={1} step={0.05} value={s.volume} onChange={(e) => mutate((x) => { x.settings.volume = Number(e.target.value); })} aria-label="Volume" style={{ width: 150 }} /></div>
      <div className="setting-row"><div><b>Speed</b><span>{s.rate}×</span></div><input type="range" min={0.75} max={2} step={0.05} value={s.rate} onChange={(e) => { mutate((x) => { x.settings.rate = Number(e.target.value); }); logAE("speed_changed", { rate: e.target.value }); }} aria-label="Playback speed" style={{ width: 150 }} /></div>
      {!adv && slider("Bass", "bass", -12, 12, (n) => (n > 0 ? "+" : "") + n + " dB")}
      {!adv && slider("Treble", "treble", -12, 12, (n) => (n > 0 ? "+" : "") + n + " dB")}
      {adv && <>
        <div className="section-head" style={{ marginTop: 14 }}><h4 style={{ fontSize: 13 }}>Equalizer</h4></div>
        {slider("Bass", "bass", -12, 12, (n) => (n > 0 ? "+" : "") + n + " dB")}
        {slider("Mid", "mid", -12, 12, (n) => (n > 0 ? "+" : "") + n + " dB")}
        {slider("Treble", "treble", -12, 12, (n) => (n > 0 ? "+" : "") + n + " dB")}
        <div className="section-head" style={{ marginTop: 14 }}><h4 style={{ fontSize: 13 }}>Voice character</h4></div>
        {slider("Warmth", "warmth", 0, 100, (n) => n + "")}
        {slider("Clarity", "clarity", 0, 100, (n) => n + "")}
        {slider("Presence", "presence", 0, 100, (n) => n + "")}
        <p className="small">Voice character shapes the narration engine; the equalizer shapes ambience and studio masters. Nothing is destructive — the source audio never changes.</p>
      </>}
      <div className="section-head" style={{ marginTop: 14 }}><h4 style={{ fontSize: 13 }}>Ambience <span className="small">· ducks automatically while the voice speaks</span></h4></div>
      <div className="chip-row">
        <button className="chip" aria-pressed={!s.ambienceId} onClick={() => mutate((x) => { x.settings.ambienceId = ""; })}>Off</button>
        {AMBIENCES.map((a) => <button key={a.id} className="chip" aria-pressed={s.ambienceId === a.id} onClick={() => { mutate((x) => { x.settings.ambienceId = a.id; }); logAE("ambience_changed", { id: a.id }); }}>{a.name}</button>)}
      </div>
      {s.ambienceId && <div className="setting-row"><div><b>Ambience level</b><span>{s.ambienceLevel}%</span></div><input type="range" min={0} max={100} step={5} value={s.ambienceLevel} onChange={setNum("ambienceLevel")} aria-label="Ambience level" style={{ width: 150 }} /></div>}
      <div className="section-head" style={{ marginTop: 14 }}><h4 style={{ fontSize: 13 }}>Profiles</h4></div>
      <div className="chip-row">
        {SOUND_PROFILES.map((p) => <button key={p.id} className="chip" aria-pressed={s.profileId === p.id} onClick={() => applyProfile(p.id)}>{p.name}</button>)}
        {st.audioProfiles.map((p) => <button key={p.id} className="chip" aria-pressed={s.profileId === "u" + p.id} onClick={() => applyProfile("u" + p.id)}>{p.name} ✕</button>)}
        <button className="chip" onClick={saveProfile}>Save current…</button>
      </div>
    </div>
  );
}
