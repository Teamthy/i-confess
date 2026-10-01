/** The shared file + device-speech queue. No React or browser globals: the
 * transports are injected, so cancellation, repeats and queue edits are tested.
 * `src: undefined` explicitly means device speech; `src: null` means unavailable
 * server audio and NEVER silently substitutes another voice. */
export type PlaybackItem = {
  slug: string;
  title: string;
  category: string;
  text: string;
  itemId?: string;
  sessionId?: string;
  confessionId?: string;
  src?: string | null;
  durationSeconds?: number;
  preview?: boolean;
  disclosure?: string;
};
export type PlaybackStatus = "idle" | "loading" | "playing" | "paused" | "completed" | "error";
export type PlaybackState = {
  queue: PlaybackItem[];
  idx: number;
  status: PlaybackStatus;
  progress: number;
  position: number;
  duration: number;
  source: "speech" | "file" | null;
  error: string | null;
  endedToken: number;
};
export const EMPTY_PLAYBACK: PlaybackState = {
  queue: [], idx: 0, status: "idle", progress: 0, position: 0, duration: 0,
  source: null, error: null, endedToken: 0,
};
export type SpeechSettings = { rate: number; pitch: number; volume: number; repeat: number; deviceVoice?: string };
export function bounded(value: number, min: number, max: number, fallback: number): number {
  return Number.isFinite(value) ? Math.min(max, Math.max(min, value)) : fallback;
}
export function selectDeviceVoice(voices: SpeechSynthesisVoice[], uri?: string, language = "en"): SpeechSynthesisVoice | null {
  const chosen = voices.find((v) => v.voiceURI === uri);
  if (chosen) return chosen;
  return voices.find((v) => v.default && v.lang.toLowerCase().startsWith(language))
    || voices.find((v) => v.lang.toLowerCase().startsWith(language))
    || voices.find((v) => v.default) || voices[0] || null;
}

type PlaybackEvents = {
  change: (state: PlaybackState) => void;
  beforePlay?: (source: "speech" | "file") => void;
  itemCompleted?: (item: PlaybackItem) => void;
  itemSkipped?: (item: PlaybackItem) => void;
  completed?: (items: PlaybackItem[]) => void;
};

export class PlaybackController {
  private state: PlaybackState = { ...EMPTY_PLAYBACK, queue: [] };
  private generation = 0;
  private queueSerial = 0;
  private repeatsLeft = 0;
  private charOffset = 0;
  private utterance: SpeechSynthesisUtterance | null = null;
  private suppressMediaEvents = false;
  private disposed = false;
  private lastSettings: SpeechSettings | null = null;
  private readonly listeners: [string, EventListener][];

  constructor(
    private readonly media: HTMLAudioElement,
    private readonly synth: SpeechSynthesis | null,
    private readonly makeUtterance: (text: string) => SpeechSynthesisUtterance,
    private readonly settings: () => SpeechSettings,
    private readonly events: PlaybackEvents,
  ) {
    const on = (fn: () => void): EventListener => () => {
      if (!this.disposed && !this.suppressMediaEvents && this.state.source === "file") fn();
    };
    this.listeners = [
      ["timeupdate", on(() => this.readMediaTime())],
      ["loadedmetadata", on(() => this.readMediaTime())],
      ["durationchange", on(() => this.readMediaTime())],
      ["playing", on(() => this.patch({ status: "playing", error: null }))],
      ["pause", on(() => { if (this.state.status === "playing") this.patch({ status: "paused" }); })],
      ["waiting", on(() => { if (!this.media.paused) this.patch({ status: "loading" }); })],
      ["ended", on(() => this.itemEnded())],
      ["error", on(() => this.fail("This audio could not be loaded. Retry to refresh its link, or choose another item."))],
    ];
    for (const [name, fn] of this.listeners) media.addEventListener(name, fn);
  }

  get snapshot(): PlaybackState { return this.state; }
  current(): PlaybackItem | null { return this.state.queue[this.state.idx] || null; }
  private patch(p: Partial<PlaybackState>) {
    if (this.disposed) return;
    this.state = { ...this.state, ...p };
    this.events.change(this.state);
  }
  private prefs(): SpeechSettings {
    const s = this.settings();
    return {
      rate: bounded(s.rate, 0.5, 2, 1), pitch: bounded(s.pitch, 0.5, 1.5, 1),
      volume: bounded(s.volume, 0, 1, 1), repeat: Math.round(bounded(s.repeat, 1, 7, 1)),
      deviceVoice: s.deviceVoice,
    };
  }
  private invalidate() {
    ++this.generation; // cancel() can synchronously report interrupted/canceled
    this.utterance = null;
    this.synth?.cancel();
    this.suppressMediaEvents = true;
    this.media.pause();
    this.suppressMediaEvents = false;
  }
  private fail(message: string) {
    this.invalidate();
    this.patch({ status: "error", error: message });
  }

  /** A new queue resets repeats. Each entry gets an identity even when the same
   * confession occurs several times in a built session. */
  playQueue(queue: PlaybackItem[], start = 0, autoplay = true) {
    this.invalidate();
    if (!queue.length) { this.stop(); return; }
    const serial = ++this.queueSerial;
    const q = queue.map((item, i) => ({ ...item, itemId: item.itemId || `local-${serial}-${i}` }));
    const idx = Math.floor(bounded(start, 0, q.length - 1, 0));
    this.patch({ queue: q, idx, status: "paused", progress: 0, position: 0, error: null });
    this.startItem(autoplay, true);
  }

  /** Editing the up-next list must not rewind or resume the current item. */
  updateQueue(queue: PlaybackItem[], idx: number) {
    if (!queue.length) { this.stop(); return; }
    const nextIdx = Math.floor(bounded(idx, 0, queue.length - 1, 0));
    const current = this.current();
    const next = queue[nextIdx];
    const unchanged = current?.itemId === next.itemId && current?.src === next.src;
    this.patch({ queue: [...queue], idx: nextIdx });
    if (!unchanged) {
      if (current) this.events.itemSkipped?.(current);
      this.startItem(true, true);
    }
  }

  private startItem(autoplay: boolean, resetRepeats: boolean) {
    this.invalidate();
    const item = this.current();
    if (!item) { this.stop(); return; }
    const prefs = this.prefs();
    if (resetRepeats) this.repeatsLeft = item.preview ? 0 : prefs.repeat - 1;
    this.charOffset = 0;
    const source = item.src === undefined ? "speech" : "file";
    const duration = source === "file" ? item.durationSeconds || 0 : Math.max(1, item.text.trim().split(/\s+/).length / (2.5 * prefs.rate));
    this.patch({ source, status: "paused", progress: 0, position: 0, duration, error: null });
    if (source === "file") {
      this.media.removeAttribute("src");
      this.media.load();
      if (!item.src) { this.fail("Audio is unavailable or locked for this item. No other voice has been substituted."); return; }
      this.media.src = item.src;
      this.media.playbackRate = prefs.rate;
      this.media.volume = prefs.volume;
      this.media.load();
      if (autoplay) void this.resume();
    } else {
      if (!this.synth) { this.fail("Device speech is not available in this browser. Use published audio or try another browser."); return; }
      if (!item.text.trim()) { this.fail("There are no words to read in this item."); return; }
      if (autoplay) this.speakFrom(0);
    }
  }

  private speakFrom(offset: number) {
    const item = this.current();
    if (!item || !this.synth) return;
    this.invalidate();
    const token = this.generation;
    const prefs = this.prefs();
    this.lastSettings = prefs;
    const u = this.makeUtterance(item.text.slice(offset));
    this.utterance = u; // keep alive until completion (WebKit/Chrome GC)
    const voice = selectDeviceVoice(this.synth.getVoices(), prefs.deviceVoice);
    if (voice) { u.voice = voice; u.lang = voice.lang; } else u.lang = "en";
    u.rate = prefs.rate; u.pitch = prefs.pitch; u.volume = prefs.volume;
    const valid = () => !this.disposed && token === this.generation;
    u.onstart = () => { if (valid()) this.patch({ status: "playing", error: null }); };
    u.onboundary = (e) => {
      if (!valid()) return;
      this.charOffset = Math.min(item.text.length, offset + (e.charIndex || 0));
      const progress = this.charOffset / Math.max(1, item.text.length);
      this.patch({ progress, position: progress * this.state.duration });
    };
    u.onend = () => { if (valid()) this.itemEnded(); };
    u.onerror = (e) => {
      if (!valid() || e.error === "canceled" || e.error === "interrupted") return;
      this.fail(e.error === "not-allowed" ? "Your browser blocked speech. Press play to try again." : "Device speech stopped unexpectedly. Press retry to continue.");
    };
    this.events.beforePlay?.("speech");
    this.patch({ status: "loading", error: null });
    try { this.synth.resume(); this.synth.speak(u); }
    catch { this.fail("Device speech could not start. Press retry or choose another device voice."); }
  }

  private readMediaTime() {
    const d = Number.isFinite(this.media.duration) && this.media.duration > 0 ? this.media.duration : this.state.duration;
    const position = bounded(this.media.currentTime, 0, d || Number.MAX_SAFE_INTEGER, 0);
    this.patch({ position, duration: d, progress: d ? Math.min(1, position / d) : 0 });
  }
  private itemEnded() {
    if (this.state.status !== "playing" && this.state.status !== "loading") return;
    const item = this.current();
    if (!item) return;
    if (this.repeatsLeft > 0) {
      --this.repeatsLeft;
      this.startItem(true, false);
      return;
    }
    this.events.itemCompleted?.(item);
    this.patch({ progress: 1, position: this.state.duration, endedToken: this.state.endedToken + 1 });
    if (this.state.idx + 1 < this.state.queue.length) {
      this.patch({ idx: this.state.idx + 1 });
      this.startItem(true, true);
    } else {
      this.invalidate();
      this.patch({ status: "completed" });
      this.events.completed?.(this.state.queue);
    }
  }

  pause() {
    if (this.state.status !== "playing" && this.state.status !== "loading") return;
    if (this.state.source === "speech") this.synth?.pause();
    else this.media.pause();
    this.patch({ status: "paused" });
  }
  async resume(): Promise<boolean> {
    if (!this.current()) return false;
    if (this.state.status === "completed") { this.playQueue(this.state.queue); return true; }
    if (this.state.source === "speech") {
      if (!this.synth) return false;
      if (this.utterance && this.synth.paused) {
        this.events.beforePlay?.("speech");
        this.synth.resume();
        this.patch({ status: "playing", error: null });
      } else this.speakFrom(this.charOffset);
      return this.state.status !== "error";
    }
    if (!this.current()?.src) return false;
    const token = this.generation;
    this.events.beforePlay?.("file");
    this.patch({ status: "loading", error: null });
    try {
      await this.media.play();
      if (token !== this.generation || this.disposed) return false;
      this.patch({ status: "playing", error: null });
      return true;
    } catch (e) {
      if (token !== this.generation || this.disposed) return false;
      if ((e as Error)?.name === "NotAllowedError") {
        this.patch({ status: "paused", error: "Your browser needs a tap to start audio. Press play." });
      } else this.fail("This audio could not be played. Retry to refresh its link.");
      return false;
    }
  }
  next() {
    if (this.state.idx >= this.state.queue.length - 1) return;
    const item = this.current();
    if (item) this.events.itemSkipped?.(item);
    this.patch({ idx: this.state.idx + 1 });
    this.startItem(true, true);
  }
  prev() {
    if (!this.state.queue.length) return;
    if (this.state.idx > 0) this.patch({ idx: this.state.idx - 1 });
    this.startItem(true, true);
  }
  seek(seconds: number) {
    const item = this.current();
    if (!item || !Number.isFinite(seconds)) return;
    const position = bounded(seconds, 0, this.state.duration, 0);
    if (this.state.source === "file") {
      try { this.media.currentTime = position; this.readMediaTime(); } catch { /* metadata not loaded */ }
    } else {
      const active = this.state.status === "playing" || this.state.status === "loading";
      let offset = Math.floor((position / Math.max(1, this.state.duration)) * item.text.length);
      while (offset > 0 && !/\s/.test(item.text[offset - 1])) --offset;
      this.invalidate();
      this.charOffset = offset;
      this.patch({ position, progress: offset / Math.max(1, item.text.length), status: "paused" });
      if (active) this.speakFrom(offset);
    }
  }
  configure() {
    const prefs = this.prefs();
    this.media.volume = prefs.volume;
    this.media.playbackRate = prefs.rate;
    // SpeechSynthesis does not reliably change an utterance's controls while
    // speaking. Continue from the last word boundary, never rewind the queue.
    if (this.state.source === "speech" && this.lastSettings && this.utterance) {
      const changed = ["rate", "pitch", "volume", "deviceVoice"].some((k) => prefs[k as keyof SpeechSettings] !== this.lastSettings?.[k as keyof SpeechSettings]);
      if (changed) {
        const active = this.state.status === "playing" || this.state.status === "loading";
        const offset = this.charOffset;
        this.invalidate();
        this.patch({ status: "paused" });
        this.lastSettings = prefs;
        if (active) this.speakFrom(offset);
      }
    }
  }
  retry() { this.startItem(true, true); }
  stop() {
    this.invalidate();
    this.media.removeAttribute("src");
    this.media.load();
    this.patch({ ...EMPTY_PLAYBACK, queue: [], endedToken: this.state.endedToken });
  }
  dispose() {
    this.stop();
    this.disposed = true;
    for (const [name, fn] of this.listeners) this.media.removeEventListener(name, fn);
  }
}
