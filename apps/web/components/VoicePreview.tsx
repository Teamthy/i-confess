"use client";

/**
 * Voice sample preview for marketing surfaces.
 *
 * Plays the voice's own `sample_url` from GET /voices through a plain
 * HTMLAudio element — no wavesurfer, no fake waveform. When the API serves
 * no sample the control renders as an honest disabled state with a reason,
 * never a dead play button. Progress is timeupdate-driven; only one preview
 * plays at a time per page (each instance pauses its siblings via a shared
 * CustomEvent — cheap, no context needed).
 */

import { useEffect, useRef, useState } from "react";
import { formatClock } from "@/lib/app-api";

const PAUSE_OTHERS = "iconfess:voice-preview-pause";

export function VoicePreview({ sampleUrl, voiceName }: { sampleUrl?: string; voiceName: string }) {
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const [playing, setPlaying] = useState(false);
  const [progress, setProgress] = useState(0);
  const [elapsed, setElapsed] = useState(0);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    const audio = audioRef.current;
    if (!audio) return;
    const onTime = () => {
      const dur = audio.duration;
      setElapsed(audio.currentTime);
      setProgress(Number.isFinite(dur) && dur > 0 ? Math.min(1, audio.currentTime / dur) : 0);
    };
    const onEnd = () => {
      setPlaying(false);
      setProgress(0);
      setElapsed(0);
    };
    const onPauseOthers = () => {
      if (!audio.paused) {
        audio.pause();
        setPlaying(false);
      }
    };
    audio.addEventListener("timeupdate", onTime);
    audio.addEventListener("ended", onEnd);
    window.addEventListener(PAUSE_OTHERS, onPauseOthers);
    return () => {
      audio.removeEventListener("timeupdate", onTime);
      audio.removeEventListener("ended", onEnd);
      window.removeEventListener(PAUSE_OTHERS, onPauseOthers);
    };
  }, []);

  if (!sampleUrl || failed) {
    return (
      <div className="ic-voice__player" role="status">
        <span className="ic-voice__btn" aria-hidden="true">
          <svg width="14" height="14" viewBox="0 0 18 18"><path d="M5 3l10 6-10 6V3z" fill="currentColor" /></svg>
        </span>
        <span style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-500)" }}>
          {failed ? "This sample wouldn't play — try the voice page." : "Hear this voice inside the app."}
        </span>
      </div>
    );
  }

  async function toggle() {
    const audio = audioRef.current;
    if (!audio) return;
    if (audio.paused) {
      window.dispatchEvent(new CustomEvent(PAUSE_OTHERS));
      try {
        await audio.play();
        setPlaying(true);
      } catch {
        setFailed(true);
      }
    } else {
      audio.pause();
      setPlaying(false);
    }
  }

  return (
    <div className="ic-voice__player">
      <audio ref={audioRef} src={sampleUrl} preload="none" onError={() => setFailed(true)} aria-hidden="true" />
      <button
        type="button"
        className="ic-voice__btn"
        onClick={toggle}
        aria-label={playing ? `Pause ${voiceName} sample` : `Play ${voiceName} sample`}
        aria-pressed={playing}
      >
        {playing ? (
          <svg width="14" height="14" viewBox="0 0 18 18" aria-hidden="true"><path d="M4 3h4v12H4zM10 3h4v12h-4z" fill="currentColor" /></svg>
        ) : (
          <svg width="14" height="14" viewBox="0 0 18 18" aria-hidden="true"><path d="M5 3l10 6-10 6V3z" fill="currentColor" /></svg>
        )}
      </button>
      <div className="ic-voice__track" aria-hidden="true">
        <div className="ic-voice__fill" style={{ width: `${Math.round(progress * 100)}%` }} />
      </div>
      <span className="ic-voice__time">{formatClock(elapsed)}</span>
    </div>
  );
}
