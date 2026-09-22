/* iCONFESS Audio + Voice Engine — client DSP layer (§26, §103).
   CLIENT-side, non-destructive: ambience rendering + ducking, EQ chains for real
   <audio> elements, and tone mapping for the on-device speech engine.
   Server mastering (§28–§32: -16 LUFS, -1 dBTP, de-ess) applies to studio masters
   in the cloud pipeline — see AUDIO_ENGINE.md. */
"use client";

type Nodes = { ctx: AudioContext; src: AudioBufferSourceNode | null; gain: GainNode; };
let cur: Nodes | null = null;
let duckTarget = 1;
let level = 0.45;
let crackleTimer: number | null = null;

const ctxOf = (): AudioContext => {
  const w = window as unknown as { webkitAudioContext?: typeof AudioContext };
  return cur?.ctx || new (window.AudioContext || w.webkitAudioContext!)();
};

/* seamless procedural noise — generated once, looped via AudioBufferSourceNode.loop (no clicks, §60) */
function noiseBuffer(ctx: AudioContext, kind: "white" | "brown"): AudioBuffer {
  const len = ctx.sampleRate * 4;
  const buf = ctx.createBuffer(1, len, ctx.sampleRate);
  const d = buf.getChannelData(0);
  let last = 0;
  for (let i = 0; i < len; i++) {
    const w = Math.random() * 2 - 1;
    if (kind === "white") d[i] = w * 0.5;
    else { last = (last + 0.02 * w) / 1.02; d[i] = last * 3.2; }
  }
  return buf;
}

export const ambienceRunning = () => !!cur;

export function startAmbience(id: string, levelPct: number) {
  stopAmbience();
  level = levelPct / 100 * 0.5;
  const ctx = ctxOf();
  if (ctx.state === "suspended") ctx.resume().catch(() => { });
  const gain = ctx.createGain();
  gain.gain.value = 0;
  gain.connect(ctx.destination);
  const src = ctx.createBufferSource();
  src.buffer = noiseBuffer(ctx, id === "ocean" || id === "fireplace" ? "brown" : "white");
  src.loop = true;

  let node: AudioNode = src;
  const add = (n: AudioNode) => { node.connect(n); node = n; };
  if (id === "rain") { const hp = ctx.createBiquadFilter(); hp.type = "highpass"; hp.frequency.value = 400; add(hp); const bp = ctx.createBiquadFilter(); bp.type = "bandpass"; bp.frequency.value = 1400; bp.Q.value = 0.4; add(bp); }
  if (id === "ocean") { const lp = ctx.createBiquadFilter(); lp.type = "lowpass"; lp.frequency.value = 480; add(lp); const lfo = ctx.createOscillator(); lfo.frequency.value = 0.06; const lg = ctx.createGain(); lg.gain.value = 0.5; lfo.connect(lg); lg.connect(gain.gain); lfo.start(); }
  if (id === "night") { const lp = ctx.createBiquadFilter(); lp.type = "lowpass"; lp.frequency.value = 700; add(lp); gain.gain.value = 0; }
  if (id === "fireplace") { const lp = ctx.createBiquadFilter(); lp.type = "lowpass"; lp.frequency.value = 900; add(lp); }
  if (id === "room") { const lp = ctx.createBiquadFilter(); lp.type = "lowpass"; lp.frequency.value = 260; add(lp); }
  node.connect(gain);
  src.start();
  cur = { ctx, src, gain };
  /* smooth fade-in — no abrupt jumps (§22) */
  gain.gain.setTargetAtTime(level * duckTarget, ctx.currentTime, 0.8);
  if (id === "fireplace") crackle(ctx, gain);
}
function crackle(ctx: AudioContext, gain: GainNode) {
  const pop = () => {
    if (!cur) return;
    const g = ctx.createGain(); g.gain.value = 0; g.connect(ctx.destination);
    const o = ctx.createBufferSource(); o.buffer = noiseBuffer(ctx, "white");
    const bp = ctx.createBiquadFilter(); bp.type = "bandpass"; bp.frequency.value = 2500 + Math.random() * 2000;
    o.connect(bp); bp.connect(g); o.start(); o.stop(ctx.currentTime + 0.06);
    g.gain.setValueAtTime(0, ctx.currentTime);
    g.gain.linearRampToValueAtTime(0.05 + Math.random() * 0.05, ctx.currentTime + 0.01);
    g.gain.linearRampToValueAtTime(0, ctx.currentTime + 0.06);
    crackleTimer = window.setTimeout(pop, 300 + Math.random() * 1800);
  };
  crackleTimer = window.setTimeout(pop, 600);
}
export function setAmbienceLevel(levelPct: number) {
  level = levelPct / 100 * 0.5;
  if (cur) cur.gain.gain.setTargetAtTime(level * duckTarget, cur.ctx.currentTime, 0.3);
}
/* §22/§62 smart ducking: attack ~0.6s down to 22%, release ~1.4s back */
export function setDucked(ducked: boolean) {
  duckTarget = ducked ? 0.22 : 1;
  if (cur) cur.gain.gain.setTargetAtTime(level * duckTarget, cur.ctx.currentTime, ducked ? 0.6 : 1.4);
}
export function stopAmbience() {
  if (crackleTimer) { clearTimeout(crackleTimer); crackleTimer = null; }
  if (cur) {
    const c = cur; cur = null;
    c.gain.gain.setTargetAtTime(0, c.ctx.currentTime, 0.25);
    window.setTimeout(() => { try { c.src?.stop(); } catch { } }, 700);
  }
}

/* §26 EQ chain for real audio elements (journal voice notes, future studio masters):
   MediaElementSource → lowshelf → peaking → highshelf → compressor → gain → destination */
export type EQSettings = { bass: number; mid: number; treble: number };
export function attachEQ(el: HTMLAudioElement, s: EQSettings) {
  const ctx = ctxOf();
  if (ctx.state === "suspended") ctx.resume().catch(() => { });
  const source = ctx.createMediaElementSource(el);
  const bass = ctx.createBiquadFilter(); bass.type = "lowshelf"; bass.frequency.value = 180; bass.gain.value = s.bass;
  const mid = ctx.createBiquadFilter(); mid.type = "peaking"; mid.frequency.value = 1200; mid.Q.value = 0.9; mid.gain.value = s.mid;
  const treble = ctx.createBiquadFilter(); treble.type = "highshelf"; treble.frequency.value = 4800; treble.gain.value = s.treble;
  const comp = ctx.createDynamicsCompressor(); comp.threshold.value = -24; comp.ratio.value = 2.5;
  const out = ctx.createGain(); out.gain.value = 1;
  source.connect(bass); bass.connect(mid); mid.connect(treble); treble.connect(comp); comp.connect(out); out.connect(ctx.destination);
  return {
    update(n: EQSettings) { bass.gain.value = n.bass; mid.gain.value = n.mid; treble.gain.value = n.treble; },
    dispose() { try { source.disconnect(); } catch { } },
  };
}

/* Voice tone mapping for the device speech engine until studio masters ship.
   Real biquad EQ cannot capture speechSynthesis output — warmth/clarity/presence
   map to synthesis parameters; the same values drive the biquad chain on real audio. */
export function toneToSpeech(t: { warmth: number; clarity: number; presence: number }): { rateMul: number; pitchMul: number; volMul: number } {
  return {
    rateMul: 1 + ((t.clarity ?? 55) - 55) / 100 * 0.10,          /* clarity nudges pace +articulation */
    pitchMul: 1 - ((t.warmth ?? 60) - 60) / 100 * 0.10,          /* warmth lowers pitch slightly */
    volMul: 0.85 + (t.presence ?? 60) / 100 * 0.25,              /* presence = forward level */
  };
}
