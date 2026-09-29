"use client";

/* ============================================================================
   SyntheticPlayer: plays a mastered platform render (spec sections 26, 29, 54).

   - Picks the best delivery encoding the browser can play (Opus, then AAC,
     then MP3), falling back to the WAV master.
   - Listener EQ runs in the browser with Web Audio biquad filters. It never
     changes the stored asset and is not a "voice changer": presets only
     shape tone for the room or device (warmth, clarity, quiet night use).
   - The transparency label is always visible and cannot be hidden.
   ========================================================================= */

import { useEffect, useMemo, useRef, useState } from "react";

export type AudioVariant = { url: string; contentType: string; bytes?: number };

export type RenderedAudio = {
  audioUrl?: string;
  variants?: Record<string, AudioVariant>;
  disclosure?: string;
  available?: boolean;
  unavailableReason?: string;
};

type Band = { type: BiquadFilterType; freq: number; gain: number; q?: number };

/** Gentle presets. Gains stay within +/-6 dB so no preset can clip badly or
 *  alter the speaker's identity. */
export const EQ_PRESETS: Record<string, { label: string; bands: Band[] }> = {
  flat: { label: "Flat (as mastered)", bands: [] },
  warm: { label: "Warm", bands: [{ type: "lowshelf", freq: 200, gain: 3 }, { type: "highshelf", freq: 6000, gain: -2 }] },
  clear: { label: "Clear speech", bands: [{ type: "highpass", freq: 120, gain: 0, q: 0.7 }, { type: "peaking", freq: 3000, gain: 4, q: 1 }] },
  night: { label: "Night (soft)", bands: [{ type: "lowshelf", freq: 150, gain: -4 }, { type: "highshelf", freq: 5000, gain: -5 }] },
  small: { label: "Phone speaker", bands: [{ type: "highpass", freq: 180, gain: 0, q: 0.7 }, { type: "peaking", freq: 2500, gain: 3, q: 1.2 }] },
};

const ORDER = ["opus", "aac", "mp3"];

/** Choose the first variant this browser reports it can play. */
export function pickSource(r: RenderedAudio, canPlay: (type: string) => string): { url: string; format: string } | null {
  for (const f of ORDER) {
    const v = r.variants?.[f];
    if (v && canPlay(v.contentType) !== "") return { url: v.url, format: f };
  }
  return r.audioUrl ? { url: r.audioUrl, format: "wav" } : null;
}

export function SyntheticPlayer({ render, title }: { render: RenderedAudio; title?: string }) {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const graph = useRef<{ ctx: AudioContext; src: MediaElementAudioSourceNode; nodes: BiquadFilterNode[] } | null>(null);
  const [preset, setPreset] = useState("flat");
  const [rate, setRate] = useState(1);

  const source = useMemo(() => {
    if (typeof document === "undefined") return null;
    const probe = document.createElement("audio");
    return pickSource(render, (t) => probe.canPlayType(t));
  }, [render]);

  // Build (or rebuild) the filter chain. The AudioContext is created lazily
  // on first play, because browsers block audio before a user gesture.
  const connect = () => {
    const el = audioRef.current;
    if (!el) return;
    if (!graph.current) {
      const ctx = new AudioContext();
      graph.current = { ctx, src: ctx.createMediaElementSource(el), nodes: [] };
    }
    const g = graph.current;
    g.src.disconnect();
    g.nodes.forEach((n) => n.disconnect());
    g.nodes = EQ_PRESETS[preset].bands.map((b) => {
      const n = g.ctx.createBiquadFilter();
      n.type = b.type;
      n.frequency.value = b.freq;
      n.gain.value = b.gain;
      if (b.q) n.Q.value = b.q;
      return n;
    });
    let last: AudioNode = g.src;
    for (const n of g.nodes) { last.connect(n); last = n; }
    last.connect(g.ctx.destination);
    if (g.ctx.state === "suspended") void g.ctx.resume();
  };

  useEffect(() => { if (graph.current) connect(); /* eslint-disable-next-line react-hooks/exhaustive-deps */ }, [preset]);
  useEffect(() => { if (audioRef.current) audioRef.current.playbackRate = rate; }, [rate]);
  useEffect(() => () => { void graph.current?.ctx.close(); }, []);

  const label = render.disclosure || "AI-generated using an authorized synthetic voice.";

  if (render.available === false) {
    return (
      <div className="adm-note">
        This audio is no longer available{render.unavailableReason === "voice_rights_ended" ? " because the voice licence has ended." : "."}
      </div>
    );
  }
  if (!source) return <div className="adm-note">No playable audio yet.</div>;

  return (
    <div className="adm-card" style={{ marginTop: 12 }}>
      <div className="adm-card-head">
        <h3>{title || "Mastered render"}</h3>
        <span className="adm-pill blue" role="note">{label}</span>
      </div>
      {/* crossOrigin lets Web Audio read signed CDN media; the CDN must send CORS headers. */}
      <audio ref={audioRef} controls preload="metadata" crossOrigin="anonymous" src={source.url} onPlay={connect} style={{ width: "100%" }} />
      <div className="adm-row-actions">
        <label className="small">EQ{" "}
          <select className="adm-input" value={preset} onChange={(e) => setPreset(e.target.value)} aria-label="Listener EQ preset">
            {Object.entries(EQ_PRESETS).map(([k, p]) => <option key={k} value={k}>{p.label}</option>)}
          </select>
        </label>
        <label className="small">Speed{" "}
          <select className="adm-input" value={rate} onChange={(e) => setRate(Number(e.target.value))} aria-label="Playback speed">
            {[0.8, 0.9, 1, 1.1, 1.25].map((r) => <option key={r} value={r}>{r}×</option>)}
          </select>
        </label>
        <span className="small">Format: {source.format.toUpperCase()}{source.format === "wav" ? " (master; encodings not ready yet)" : ""}</span>
      </div>
    </div>
  );
}
