"use client";

/**
 * The single audio element for the authenticated app (§31).
 *
 * The element is mounted once, in the /app layout, so playback survives
 * client-side navigation: leaving /app/player for /app/history must not
 * stop the words. Both the full player and the mini-player drive and
 * reflect this one element — there is never a second source of truth for
 * position, rate, or whether sound is coming out.
 *
 * Server narration (progress sync, lifecycle, completion) stays in
 * PlayerClient, which owns the session. This context owns only transport:
 * what is loaded, where it is, and whether it plays.
 */

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

export type PlayerTrack = {
  /** Queue-item id — the identity load() dedupes on. */
  itemId: string;
  sessionId: string;
  title: string;
  subtitle: string;
  /** Signed URL. Null clears the element (locked items). */
  src: string | null;
  /** Fallback length when metadata has not arrived yet. */
  durationHint?: number;
};

type PlayerContextValue = {
  track: PlayerTrack | null;
  playing: boolean;
  position: number;
  duration: number;
  rate: number;
  volume: number;
  /** Set when the element reports an error for a loaded src. Cleared by load(). */
  failed: boolean;
  /** Increments on every natural `ended`. Consumers advance the queue. */
  endedToken: number;
  load: (track: PlayerTrack, autoplay: boolean) => void;
  play: () => Promise<boolean>;
  pause: () => void;
  seek: (seconds: number) => void;
  setRate: (rate: number) => void;
  setVolume: (volume: number) => void;
  /** Stop and forget — the mini-player hides. */
  clear: () => void;
};

const PlayerCtx = createContext<PlayerContextValue | null>(null);

export function usePlayer(): PlayerContextValue {
  const ctx = useContext(PlayerCtx);
  if (!ctx) throw new Error("usePlayer must be used inside the /app PlayerProvider");
  return ctx;
}

/** Same channel VoicePreview listens on: session audio ducks samples. */
const PAUSE_SAMPLES = "iconfess:voice-preview-pause";

export function PlayerProvider({ children }: { children: ReactNode }) {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const [track, setTrack] = useState<PlayerTrack | null>(null);
  const [playing, setPlaying] = useState(false);
  const [position, setPosition] = useState(0);
  const [duration, setDuration] = useState(0);
  const [rate, setRateState] = useState(1);
  const [volume, setVolumeState] = useState(1);
  const [failed, setFailed] = useState(false);
  const [endedToken, setEndedToken] = useState(0);
  const trackRef = useRef<PlayerTrack | null>(null);
  trackRef.current = track;

  // Element event wiring, once.
  useEffect(() => {
    const a = audioRef.current;
    if (!a) return;
    const onTime = () => {
      setPosition(a.currentTime);
      const hint = trackRef.current?.durationHint ?? 0;
      setDuration(Number.isFinite(a.duration) && a.duration > 0 ? a.duration : hint);
    };
    const onEnded = () => {
      setPlaying(false);
      setEndedToken((t) => t + 1);
    };
    const onPlay = () => setPlaying(true);
    const onPause = () => setPlaying(false);
    const onError = () => {
      if (trackRef.current?.src) setFailed(true);
      setPlaying(false);
    };
    a.addEventListener("timeupdate", onTime);
    a.addEventListener("loadedmetadata", onTime);
    a.addEventListener("ended", onEnded);
    a.addEventListener("play", onPlay);
    a.addEventListener("pause", onPause);
    a.addEventListener("error", onError);
    return () => {
      a.removeEventListener("timeupdate", onTime);
      a.removeEventListener("loadedmetadata", onTime);
      a.removeEventListener("ended", onEnded);
      a.removeEventListener("play", onPlay);
      a.removeEventListener("pause", onPause);
      a.removeEventListener("error", onError);
    };
  }, []);

  // Rate/volume are applied to the element as state changes.
  useEffect(() => {
    if (audioRef.current) audioRef.current.playbackRate = rate;
  }, [rate]);
  useEffect(() => {
    if (audioRef.current) audioRef.current.volume = volume;
  }, [volume]);

  const load = useCallback((next: PlayerTrack, autoplay: boolean) => {
    const a = audioRef.current;
    const cur = trackRef.current;
    // Same item, same bytes: nothing to do (prevents seek resets on rerender).
    if (a && cur && cur.itemId === next.itemId && cur.src === next.src) {
      if (autoplay && a.paused && next.src) void a.play().catch(() => {});
      return;
    }
    setTrack(next);
    setFailed(false);
    setPosition(0);
    setDuration(next.durationHint ?? 0);
    if (!a) return;
    if (next.src) {
      a.src = next.src;
      a.load();
      if (autoplay) {
        window.dispatchEvent(new CustomEvent(PAUSE_SAMPLES));
        void a.play().catch(() => {});
      }
    } else {
      a.removeAttribute("src");
      a.load();
      setPlaying(false);
    }
  }, []);

  const play = useCallback(async () => {
    const a = audioRef.current;
    if (!a || !trackRef.current?.src) return false;
    window.dispatchEvent(new CustomEvent(PAUSE_SAMPLES));
    try {
      await a.play();
      return true;
    } catch {
      return false;
    }
  }, []);

  const pause = useCallback(() => {
    audioRef.current?.pause();
  }, []);

  const seek = useCallback((seconds: number) => {
    const a = audioRef.current;
    if (!a) return;
    a.currentTime = Math.max(0, seconds);
    setPosition(a.currentTime);
  }, []);

  const setRate = useCallback((r: number) => setRateState(r), []);
  const setVolume = useCallback((v: number) => setVolumeState(Math.min(1, Math.max(0, v))), []);

  const clear = useCallback(() => {
    const a = audioRef.current;
    if (a) {
      a.pause();
      a.removeAttribute("src");
      a.load();
    }
    setTrack(null);
    setPlaying(false);
    setPosition(0);
    setDuration(0);
    setFailed(false);
  }, []);

  const value = useMemo<PlayerContextValue>(
    () => ({ track, playing, position, duration, rate, volume, failed, endedToken, load, play, pause, seek, setRate, setVolume, clear }),
    [track, playing, position, duration, rate, volume, failed, endedToken, load, play, pause, seek, setRate, setVolume, clear],
  );

  return (
    <PlayerCtx.Provider value={value}>
      {/* The one element. Hidden but not display:none (some engines throttle
          inaudible-but-playing media less aggressively when rendered). */}
      <audio
        ref={audioRef}
        preload="none"
        aria-hidden="true"
        tabIndex={-1}
        style={{ position: "absolute", width: 1, height: 1, opacity: 0, pointerEvents: "none" }}
      />
      {children}
    </PlayerCtx.Provider>
  );
}
