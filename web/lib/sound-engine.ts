/** Listener-only DSP. Stored recordings are never changed. Device speech has
 * no PCM output exposed by browsers, so EQ/compression apply to files only. */
import { bounded } from "./audio-playback";

type Band = { type: BiquadFilterType; freq: number; gain: number; q?: number };
export const EQ_PRESETS: Record<string, { label: string; bands: Band[] }> = {
  flat: { label: "As mastered", bands: [] },
  warm: { label: "Warm", bands: [{ type: "lowshelf", freq: 200, gain: 3 }, { type: "highshelf", freq: 6000, gain: -2 }] },
  clear: { label: "Clear speech", bands: [{ type: "highpass", freq: 120, gain: 0, q: 0.7 }, { type: "peaking", freq: 3000, gain: 4, q: 1 }] },
  night: { label: "Soft night", bands: [{ type: "lowshelf", freq: 150, gain: -4 }, { type: "highshelf", freq: 5000, gain: -5 }] },
  small: { label: "Phone speaker", bands: [{ type: "highpass", freq: 180, gain: 0, q: 0.7 }, { type: "peaking", freq: 2500, gain: 3, q: 1.2 }] },
};
export const SOUNDS = {
  rain: { label: "Soft rain", description: "A light, even wash of filtered noise." },
  ocean: { label: "Ocean hush", description: "Slow waves of low, soft noise." },
  brown: { label: "Deep calm", description: "A warm, steady brown-noise bed." },
};
export type SoundName = keyof typeof SOUNDS;
export type SoundSettings = {
  ambient: boolean;
  ambientSound: SoundName;
  ambientVolume: number;
  volume: number;
  eq: string;
  normalize: boolean;
  chime: boolean;
};
type Ambient = { source: AudioBufferSourceNode; filter: BiquadFilterNode; gain: GainNode; lfo?: OscillatorNode; lfoGain?: GainNode; kind: SoundName };

export class SoundEngine {
  private ctx: AudioContext | null = null;
  private input: MediaElementAudioSourceNode | null = null;
  private filters: AudioNode[] = [];
  private ambient: Ambient | null = null;
  private playing = false;
  private unlocked = false;
  private disposed = false;
  private idleTimer: ReturnType<typeof setTimeout> | null = null;
  private graphKey = "";
  private settings: SoundSettings = {
    ambient: false, ambientSound: "rain", ambientVolume: 0.2, volume: 1,
    eq: "flat", normalize: true, chime: false,
  };

  constructor(private readonly makeContext: () => AudioContext, private readonly onError: (message: string) => void = () => {}) {}

  private context(): AudioContext | null {
    if (this.disposed) return null;
    if (!this.ctx) {
      try { this.ctx = this.makeContext(); }
      catch { this.onError("Sound effects are not supported in this browser. Narration can still play."); }
    }
    return this.ctx;
  }
  /** Called only by play/resume/toggle gestures, not on mount. */
  unlock(media?: HTMLAudioElement) {
    this.unlocked = true;
    if (this.idleTimer) { clearTimeout(this.idleTimer); this.idleTimer = null; }
    if (!media && !this.settings.ambient && !this.settings.chime) return;
    const ctx = this.context();
    if (!ctx) return;
    if (ctx.state === "suspended") void ctx.resume().catch(() => this.onError("Tap play again to enable sound effects."));
    if (media && !this.input) {
      try { this.input = ctx.createMediaElementSource(media); this.rebuild(); }
      catch { this.onError("EQ is unavailable for this audio. Check the audio server's CORS configuration."); }
    }
    this.syncAmbient();
  }
  configure(settings: SoundSettings) {
    this.settings = {
      ...settings,
      ambientSound: settings.ambientSound in SOUNDS ? settings.ambientSound : "rain",
      ambientVolume: bounded(settings.ambientVolume, 0, 0.6, 0.2),
      volume: bounded(settings.volume, 0, 1, 1),
      eq: settings.eq in EQ_PRESETS ? settings.eq : "flat",
    };
    if (this.input) this.rebuild();
    this.syncAmbient();
  }
  setPlaying(playing: boolean) {
    this.playing = playing;
    if (playing && this.ctx?.state === "suspended" && this.unlocked) void this.ctx.resume().catch(() => {});
    this.syncAmbient();
    if (!playing && this.ctx) {
      if (this.idleTimer) clearTimeout(this.idleTimer);
      this.idleTimer = setTimeout(() => {
        this.idleTimer = null;
        if (!this.playing && this.ctx?.state === "running") void this.ctx.suspend().catch(() => {});
      }, 1600); // allows the optional completion chime and fades to settle
    }
  }
  private rebuild() {
    if (!this.ctx || !this.input) return;
    const key = `${this.settings.eq}:${this.settings.normalize}`;
    if (key === this.graphKey) return;
    this.graphKey = key;
    this.input.disconnect();
    for (const node of this.filters) node.disconnect();
    const bands = EQ_PRESETS[this.settings.eq]?.bands || [];
    this.filters = bands.map((b) => {
      const n = this.ctx!.createBiquadFilter();
      n.type = b.type; n.frequency.value = b.freq; n.gain.value = b.gain;
      if (b.q) n.Q.value = b.q;
      return n;
    });
    // Headroom before any boost prevents EQ from clipping. Gentle compression
    // is a listening option, not a claim to measure integrated LUFS.
    const headroom = this.ctx.createGain();
    headroom.gain.value = Math.pow(10, -Math.max(0, ...bands.map((b) => b.gain)) / 20);
    this.filters.unshift(headroom);
    if (this.settings.normalize) {
      const c = this.ctx.createDynamicsCompressor();
      c.threshold.value = -24; c.knee.value = 15; c.ratio.value = 2;
      c.attack.value = 0.02; c.release.value = 0.25;
      this.filters.push(c);
    }
    let last: AudioNode = this.input;
    for (const n of this.filters) { last.connect(n); last = n; }
    last.connect(this.ctx.destination);
  }
  private syncAmbient() {
    if (!this.playing || !this.settings.ambient || this.settings.volume === 0 || this.settings.ambientVolume === 0) {
      this.stopAmbient(); return;
    }
    if (!this.unlocked) return;
    const ctx = this.context();
    if (!ctx) return;
    if (this.ambient?.kind !== this.settings.ambientSound) this.stopAmbient();
    if (!this.ambient) {
      const kind = this.settings.ambientSound;
      const buffer = ctx.createBuffer(1, ctx.sampleRate * 4, ctx.sampleRate);
      const samples = buffer.getChannelData(0);
      let brown = 0;
      for (let i = 0; i < samples.length; i++) {
        const noise = Math.random() * 2 - 1;
        brown = (brown + noise * 0.02) / 1.02;
        samples[i] = kind === "rain" ? noise * 0.35 : brown * 3;
      }
      // Smooth the loop boundary to avoid a click every four seconds.
      const fade = Math.min(256, samples.length / 2);
      for (let i = 0; i < fade; i++) {
        samples[i] *= i / fade;
        samples[samples.length - 1 - i] *= i / fade;
      }
      const source = ctx.createBufferSource(); source.buffer = buffer; source.loop = true;
      const filter = ctx.createBiquadFilter(); filter.type = "lowpass";
      filter.frequency.value = kind === "rain" ? 1600 : kind === "ocean" ? 650 : 400;
      const gain = ctx.createGain(); gain.gain.value = 0;
      source.connect(filter); filter.connect(gain); gain.connect(ctx.destination);
      this.ambient = { source, filter, gain, kind };
      if (kind === "ocean") {
        const lfo = ctx.createOscillator(); lfo.frequency.value = 0.12;
        const lfoGain = ctx.createGain(); lfoGain.gain.value = 0;
        lfo.connect(lfoGain); lfoGain.connect(gain.gain); lfo.start();
        this.ambient.lfo = lfo; this.ambient.lfoGain = lfoGain;
      }
      source.start();
    }
    const target = this.settings.ambientVolume * this.settings.volume * 0.35;
    this.ambient.gain.gain.cancelScheduledValues(ctx.currentTime);
    this.ambient.gain.gain.setTargetAtTime(target, ctx.currentTime, 0.08);
    if (this.ambient.lfoGain) this.ambient.lfoGain.gain.setTargetAtTime(target * 0.35, ctx.currentTime, 0.08);
  }
  private stopAmbient() {
    const a = this.ambient;
    if (!a || !this.ctx) return;
    this.ambient = null;
    const at = this.ctx.currentTime;
    a.gain.gain.cancelScheduledValues(at);
    a.gain.gain.setTargetAtTime(0, at, 0.04);
    if (a.lfoGain) a.lfoGain.gain.setTargetAtTime(0, at, 0.04);
    a.source.onended = () => { a.source.disconnect(); a.filter.disconnect(); a.gain.disconnect(); a.lfo?.disconnect(); a.lfoGain?.disconnect(); };
    a.source.stop(at + 0.2);
    a.lfo?.stop(at + 0.2);
  }
  playChime() {
    if (!this.settings.chime || !this.unlocked || this.settings.volume === 0) return;
    const ctx = this.context();
    if (!ctx) return;
    if (ctx.state === "suspended") void ctx.resume().catch(() => {});
    const oscillator = ctx.createOscillator(); oscillator.frequency.value = 528;
    const gain = ctx.createGain(); const at = ctx.currentTime;
    gain.gain.setValueAtTime(0.06 * this.settings.volume, at);
    gain.gain.exponentialRampToValueAtTime(0.0001, at + 1.2);
    oscillator.connect(gain); gain.connect(ctx.destination);
    oscillator.onended = () => { oscillator.disconnect(); gain.disconnect(); };
    oscillator.start(); oscillator.stop(at + 1.3);
  }
  dispose() {
    this.playing = false; this.disposed = true;
    if (this.idleTimer) clearTimeout(this.idleTimer);
    this.stopAmbient();
    this.input?.disconnect();
    for (const n of this.filters) n.disconnect();
    if (this.ctx) void this.ctx.close().catch(() => {});
    this.ctx = null;
  }
}
