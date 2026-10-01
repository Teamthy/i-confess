"use client";

/** One transport for device speech, voice samples, Bible audio and API session
 * queues. Mounted in the root layout: navigation cannot reset a queue or leave
 * a second sound source running. Audio effects and sleep timers live here too. */
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { PlaybackController, EMPTY_PLAYBACK, bounded, type PlaybackItem, type PlaybackState } from "./audio-playback";
import { SoundEngine } from "./sound-engine";
import { getAppState, mutate, track as trackEvent, useApp } from "./store";
import { appApi, type AppApiResult } from "./app-api";
import { SessionReporter, sessionQueue, type ApiSession } from "./session-playback";
import { useAuth } from "./auth-context";
import { CONFESSIONS } from "./data";

export type PlayerTrack = {
  itemId: string; sessionId: string; title: string; subtitle: string;
  src: string | null; durationHint?: number;
};
type PlayerContextValue = PlaybackState & {
  track: PlayerTrack | null;
  playing: boolean;
  rate: number;
  volume: number;
  failed: boolean;
  deviceVoices: SpeechSynthesisVoice[];
  speechAvailable: boolean;
  soundError: string | null;
  syncError: string | null;
  sleepUntil: number | null;
  load: (track: PlayerTrack, autoplay: boolean) => void;
  playQueue: (queue: PlaybackItem[], start?: number) => void;
  playSession: (session: ApiSession, token: string) => Promise<AppApiResult<ApiSession>>;
  play: () => Promise<boolean>;
  pause: () => void;
  seek: (seconds: number) => void;
  next: () => void;
  prev: () => void;
  updateQueue: (queue: PlaybackItem[], index: number) => void;
  retry: () => void;
  setRate: (rate: number) => void;
  setVolume: (volume: number) => void;
  setSleepTimer: (minutes: number) => void;
  unlockSound: () => void;
  clear: () => void;
};
const PlayerCtx = createContext<PlayerContextValue | null>(null);
export function usePlayer(): PlayerContextValue {
  const ctx = useContext(PlayerCtx);
  if (!ctx) throw new Error("usePlayer requires PlayerProvider");
  return ctx;
}
export const AUDIO_FOCUS = "iconfess:audio-focus";
export function claimAudioFocus(owner: string) {
  window.dispatchEvent(new CustomEvent(AUDIO_FOCUS, { detail: { owner } }));
  window.dispatchEvent(new CustomEvent("iconfess:voice-preview-pause"));
}

export function PlayerProvider({ children }: { children: ReactNode }) {
  const media = useRef<HTMLAudioElement | null>(null);
  const controller = useRef<PlaybackController | null>(null);
  const effects = useRef<SoundEngine | null>(null);
  const reporter = useRef<SessionReporter | null>(null);
  const [state, setState] = useState<PlaybackState>(EMPTY_PLAYBACK);
  const [deviceVoices, setDeviceVoices] = useState<SpeechSynthesisVoice[]>([]);
  const [speechAvailable, setSpeechAvailable] = useState(false);
  const [soundError, setSoundError] = useState<string | null>(null);
  const [syncError, setSyncError] = useState<string | null>(null);
  const [sleepUntil, setSleepUntil] = useState<number | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const launchGeneration = useRef(0);
  const lastItem = useRef<string | null>(null);
  const auth = useAuth();
  const authRef = useRef(auth); authRef.current = auth;
  const app = useApp();
  const settings = app.settings;

  const pause = useCallback(() => {
    const c = controller.current;
    if (!c || !["playing", "loading"].includes(c.snapshot.status)) return;
    const item = c.current();
    if (item) reporter.current?.progress(item, c.snapshot.position);
    c.pause(); reporter.current?.pause();
  }, []);
  const play = useCallback(async () => {
    reporter.current?.resume();
    return await controller.current?.resume() || false;
  }, []);
  const clear = useCallback(() => {
    ++launchGeneration.current;
    reporter.current?.interrupt(); reporter.current = null;
    controller.current?.stop(); effects.current?.setPlaying(false);
    if (timer.current) clearTimeout(timer.current);
    timer.current = null; setSleepUntil(null); setSyncError(null);
  }, []);

  useEffect(() => {
    const el = media.current;
    if (!el) return;
    let deviceId = "web";
    try {
      deviceId = localStorage.getItem("ic-audio-device") || `web-${crypto.randomUUID()}`;
      localStorage.setItem("ic-audio-device", deviceId);
    } catch { /* private browsing: progress still works */ }
    el.dataset.deviceId = deviceId;
    const synth = window.speechSynthesis || null;
    setSpeechAvailable(Boolean(synth));
    const voicesChanged = () => setDeviceVoices(synth?.getVoices() || []);
    voicesChanged(); synth?.addEventListener("voiceschanged", voicesChanged);
    const sound = new SoundEngine(() => new window.AudioContext(), setSoundError);
    sound.configure(getAppState().settings);
    effects.current = sound;
    const c = new PlaybackController(el, synth, (text) => new SpeechSynthesisUtterance(text), () => getAppState().settings, {
      change: (next) => {
        setState(next);
        sound.setPlaying(next.status === "playing");
        const item = next.queue[next.idx];
        if (next.status === "playing" && item?.itemId !== lastItem.current) {
          lastItem.current = item?.itemId || null;
          if (item) reporter.current?.progress(item, next.position, "PLAYING");
        }
      },
      beforePlay: (source) => { claimAudioFocus("main"); sound.unlock(source === "file" ? el : undefined); },
      itemCompleted: (item) => {
        reporter.current?.progress(item, c.snapshot.duration, "COMPLETED");
        if (item.preview || item.slug.startsWith("voice-preview-")) return;
        if (!getAppState().settings.privacy.pauseHistory) mutate((s) => {
          const local = CONFESSIONS.find((conf) => conf.title === item.title);
          const slug = local?.slug || item.slug;
          s.history = [{ slug, title: item.title, category: item.category, at: Date.now() }, ...s.history].slice(0, 60);
          const today = new Date().toDateString();
          if (s.streak.last !== today) {
            const yesterday = new Date(); yesterday.setDate(yesterday.getDate() - 1);
            s.streak = { count: s.streak.last === yesterday.toDateString() ? s.streak.count + 1 : 1, last: today };
          }
        });
        trackEvent("confession_played", { slug: item.slug });
      },
      itemSkipped: (item) => reporter.current?.progress(item, 0, "SKIPPED"),
      completed: (items) => {
        reporter.current?.complete();
        if (items.every((item) => !item.preview && !item.slug.startsWith("voice-preview-"))) {
          trackEvent("session_completed", { items: items.length }); sound.playChime();
        }
      },
    });
    controller.current = c;
    const onFocus = (event: Event) => { if ((event as CustomEvent).detail?.owner !== "main") pause(); };
    window.addEventListener(AUDIO_FOCUS, onFocus);
    // Opportunistic progress. No text, voice samples or unsigned storage keys
    // are uploaded, and a timer is not proof of completion.
    const progressTimer = setInterval(() => {
      if (c.snapshot.status === "playing" && c.current()) reporter.current?.progress(c.current()!, c.snapshot.position);
    }, 10000);
    return () => {
      ++launchGeneration.current;
      clearInterval(progressTimer);
      synth?.removeEventListener("voiceschanged", voicesChanged);
      window.removeEventListener(AUDIO_FOCUS, onFocus);
      reporter.current?.interrupt(); reporter.current = null;
      c.dispose(); sound.dispose(); controller.current = null; effects.current = null;
      if (timer.current) clearTimeout(timer.current);
    };
  }, [pause]);

  useEffect(() => { controller.current?.configure(); effects.current?.configure(settings); }, [settings]);
  // Audio and pending launches must not leak across account changes/sign-out.
  const account = auth.user?.id || app.user?.email || null;
  const lastAccount = useRef(account);
  useEffect(() => {
    if (lastAccount.current !== account) clear();
    lastAccount.current = account;
  }, [account, clear]);

  const playQueue = useCallback((queue: PlaybackItem[], start = 0) => {
    ++launchGeneration.current;
    reporter.current?.interrupt(); reporter.current = null; setSyncError(null);
    lastItem.current = null;
    controller.current?.playQueue(queue, start);
  }, []);
  const load = useCallback((next: PlayerTrack, autoplay: boolean) => {
    const current = controller.current?.current();
    if (current?.itemId === next.itemId && current.src === next.src) { if (autoplay) void play(); return; }
    ++launchGeneration.current;
    reporter.current?.interrupt(); reporter.current = null; lastItem.current = null;
    controller.current?.playQueue([{
      slug: next.itemId, itemId: next.itemId, sessionId: next.sessionId,
      title: next.title, category: next.subtitle, text: "", src: next.src,
      durationSeconds: next.durationHint, preview: next.sessionId.startsWith("bible"),
    }], 0, autoplay);
  }, [play]);

  const playSession = useCallback(async (session: ApiSession, token: string): Promise<AppApiResult<ApiSession>> => {
    if (controller.current?.current()?.sessionId === session.id && reporter.current?.id === session.id) {
      await play(); return { ok: true, data: session };
    }
    const key = ++launchGeneration.current;
    const owner = authRef.current.user?.id;
    if (!session.items?.some((item) => item.audio_url && !item.locked)) {
      return { ok: false, status: 422, code: "AUDIO_UNAVAILABLE", message: "No playable published audio is available for this session. No device voice has been substituted." };
    }
    const status = session.status.toUpperCase();
    const op = ["PAUSED", "INTERRUPTED"].includes(status) ? "resume" : status === "ACTIVE" ? null : "start";
    const result = op ? await appApi<ApiSession>(`/sessions/${encodeURIComponent(session.id)}/${op}`, { token, method: "POST", idempotencyKey: crypto.randomUUID() }) : { ok: true as const, data: session };
    if (key !== launchGeneration.current || owner !== authRef.current.user?.id || token !== authRef.current.token) {
      return { ok: false, status: 0, code: "ABORTED", message: "Playback was cancelled." };
    }
    if (!result.ok) return result;
    reporter.current?.interrupt(); lastItem.current = null;
    setSyncError(null);
    reporter.current = new SessionReporter(session.id, token, appApi, setSyncError, media.current?.dataset.deviceId || "web");
    controller.current?.playQueue(sessionQueue(result.data));
    if (result.data.voice_downgraded) setSyncError("Your plan does not include the requested voice. The server selected an available free voice.");
    trackEvent("session_started", { session_id: session.id });
    return result;
  }, [play]);
  const retry = useCallback(() => {
    const item = controller.current?.current();
    if (item?.sessionId && reporter.current?.id === item.sessionId && authRef.current.token) {
      const id = item.sessionId, currentIndex = controller.current?.snapshot.idx || 0;
      const key = ++launchGeneration.current;
      const token = authRef.current.token;
      void appApi<ApiSession>(`/sessions/${encodeURIComponent(id)}`, { token }).then((result) => {
        if (key !== launchGeneration.current || authRef.current.token !== token) return;
        if (result.ok) controller.current?.playQueue(sessionQueue(result.data), currentIndex);
        else setSyncError(result.message);
      });
    } else controller.current?.retry();
  }, []);
  const setRate = useCallback((rate: number) => mutate((s) => { s.settings.rate = bounded(rate, 0.5, 2, 1); }), []);
  const setVolume = useCallback((volume: number) => mutate((s) => { s.settings.volume = bounded(volume, 0, 1, 1); }), []);
  const setSleepTimer = useCallback((minutes: number) => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = null;
    if (!Number.isFinite(minutes) || minutes <= 0) { setSleepUntil(null); return; }
    const ms = Math.min(180, minutes) * 60000;
    setSleepUntil(Date.now() + ms);
    timer.current = setTimeout(() => { pause(); setSleepUntil(null); timer.current = null; }, ms);
  }, [pause]);

  const item = state.queue[state.idx];
  const playerTrack = useMemo<PlayerTrack | null>(() => item ? {
    itemId: item.itemId || item.slug, sessionId: item.sessionId || "", title: item.title,
    subtitle: item.category, src: item.src || null, durationHint: item.durationSeconds,
  } : null, [item]);
  useEffect(() => {
    if (!("mediaSession" in navigator)) return;
    const session = navigator.mediaSession;
    if (playerTrack && typeof MediaMetadata !== "undefined") session.metadata = new MediaMetadata({ title: playerTrack.title, artist: "iCONFESS", album: playerTrack.subtitle });
    else session.metadata = null;
    session.playbackState = state.status === "playing" ? "playing" : state.status === "idle" ? "none" : "paused";
    const handlers: Partial<Record<MediaSessionAction, MediaSessionActionHandler>> = {
      play: () => { void play(); }, pause, stop: clear,
      nexttrack: () => controller.current?.next(), previoustrack: () => controller.current?.prev(),
      seekto: (event) => { if (event.seekTime !== undefined) controller.current?.seek(event.seekTime); },
      seekbackward: (event) => controller.current?.seek(state.position - (event.seekOffset || 10)),
      seekforward: (event) => controller.current?.seek(state.position + (event.seekOffset || 10)),
    };
    for (const [action, fn] of Object.entries(handlers)) { try { session.setActionHandler(action as MediaSessionAction, fn!); } catch { /* unsupported OS action */ } }
    if (state.source === "file" && state.duration > 0) {
      try { session.setPositionState({ duration: state.duration, position: Math.min(state.position, state.duration), playbackRate: settings.rate }); } catch { /* no OS position support */ }
    } else { try { session.setPositionState(); } catch { } }
    return () => { for (const action of Object.keys(handlers)) { try { session.setActionHandler(action as MediaSessionAction, null); } catch { } } };
  }, [playerTrack, state.status, state.source, state.position, state.duration, settings.rate, pause, play, clear]);

  const value: PlayerContextValue = {
    ...state, track: playerTrack, playing: state.status === "playing", rate: settings.rate,
    volume: settings.volume, failed: state.status === "error", deviceVoices, speechAvailable,
    soundError, syncError, sleepUntil, load, playQueue, playSession, play, pause, retry,
    seek: (seconds) => controller.current?.seek(seconds),
    next: () => controller.current?.next(), prev: () => controller.current?.prev(),
    updateQueue: (queue, index) => controller.current?.updateQueue(queue, index),
    setRate, setVolume, setSleepTimer, clear,
    unlockSound: () => { setSoundError(null); effects.current?.configure(getAppState().settings); effects.current?.unlock(state.source === "file" ? media.current || undefined : undefined); },
  };
  return (
    <PlayerCtx.Provider value={value}>
      <audio ref={media} preload="metadata" crossOrigin="anonymous" aria-hidden="true" tabIndex={-1}
        style={{ position: "absolute", width: 1, height: 1, opacity: 0, pointerEvents: "none" }} />
      {children}
    </PlayerCtx.Provider>
  );
}
