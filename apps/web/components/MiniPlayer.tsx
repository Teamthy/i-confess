"use client";

/**
 * Persistent mini-player (§31).
 *
 * Mounted once in the app shell, it reflects the single PlayerProvider
 * element: the same transport the full player drives. Hidden until
 * something has been loaded; closing stops playback and forgets the track.
 * Desktop: bottom bar. Mobile: pinned above the bottom tab bar.
 */

import Link from "next/link";
import { usePlayer } from "@/lib/player-context";
import { formatClock } from "@/lib/app-api";

export function MiniPlayer() {
  const player = usePlayer();
  const { track, playing, position, duration } = player;
  if (!track) return null;

  const pct = duration > 0 ? Math.min(100, Math.max(0, (position / duration) * 100)) : 0;

  return (
    <div className="ia-mini" role="region" aria-label="Now playing">
      <div className="ia-mini__fill" aria-hidden="true" style={{ width: `${pct}%` }} />
      <Link
        href={`/app/player?session=${encodeURIComponent(track.sessionId)}`}
        className="ia-mini__art"
        aria-label={`Open ${track.title} in the player`}
      >
        {track.title.slice(0, 1).toUpperCase()}
      </Link>
      <Link href={`/app/player?session=${encodeURIComponent(track.sessionId)}`} className="ia-mini__text">
        <strong>{track.title}</strong>
        <span>
          {track.subtitle}
          {duration > 0 ? ` · ${formatClock(position)} / ${formatClock(duration)}` : ""}
        </span>
      </Link>
      <button
        type="button"
        className="ia-mini__btn"
        onClick={() => (playing ? player.pause() : void player.play())}
        aria-label={playing ? "Pause" : "Play"}
      >
        {playing ? "❚❚" : "▶"}
      </button>
      <button type="button" className="ia-mini__btn ia-mini__btn--ghost" onClick={player.clear} aria-label="Stop and dismiss">
        ✕
      </button>
    </div>
  );
}
