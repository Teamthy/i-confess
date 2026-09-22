"use client";

/**
 * /app/player — the immersive audio surface, driven by a real session.
 *
 * Flow: GET /sessions/{id} returns the queue with freshly signed, expiring
 * audio URLs (minted per read, server-side entitlement already applied — an
 * item whose URL is empty is not entitled and is skipped, its lock_reason
 * shown, never played "partially"). The session is started with
 * POST /sessions/{id}/start (idempotency-keyed: a flaky retry must not stamp
 * started_at twice) unless it already carries an open status.
 *
 * Playback state lives in the browser; the server record is an append-only
 * narration of it: POST /sessions/{id}/progress every 10 seconds and on every
 * transition, item_status COMPLETED/SKIPPED riding along with the same call.
 * When the last item ends, the session is completed — never PATCHed by a
 * client "status" guess; the server's state machine owns the transition.
 *
 * Transport (the audio element itself) lives in PlayerProvider, mounted in
 * the /app layout, so playback survives navigation and the mini-player
 * reflects this same session. This component owns the session, the queue,
 * and all server narration — never an <audio> of its own.
 *
 * States §31: idle (no session chosen), loading, playing, paused, completed,
 * error (network or an unplayable asset — audio errors say so and offer skip,
 * they never fail silently).
 */

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth-context";
import { usePlayer, type PlayerTrack } from "@/lib/player-context";
import { appApi, formatClock, newIdempotencyKey } from "@/lib/app-api";
import { ANALYTICS_EVENTS, track } from "@/lib/analytics";
import { ErrorBlock, LoadingBlock, SignedOut } from "@/components/app-ui";
import { railColor } from "@/lib/categoryColor";

export type SessionItem = {
  id: string;
  session_id: string;
  confession_id: string;
  variant_id?: string;
  voice_id?: string;
  position: number;
  duration_seconds: number;
  status: string; // QUEUED | PLAYING | COMPLETED | SKIPPED | FAILED
  title?: string;
  category?: string;
  audio_url?: string;
  text?: string;
  locked?: boolean;
  lock_reason?: string;
};

export type Session = {
  id: string;
  type: string;
  duration_seconds: number;
  target_duration: number;
  actual_duration: number;
  strategy?: string;
  title?: string;
  description?: string;
  voice_id?: string;
  voice_downgraded?: boolean;
  voice_downgrade_reason?: string;
  status: string; // DRAFT | READY | ACTIVE | PAUSED | INTERRUPTED | COMPLETED …
  created_at: string;
  started_at?: string;
  completed_at?: string;
  items?: SessionItem[];
};

const OPEN_STATUSES = new Set(["DRAFT", "READY", "STARTING", "ACTIVE", "PAUSED", "INTERRUPTED"]);
const DONE_ITEM = new Set(["COMPLETED", "SKIPPED"]);
const SPEEDS = [0.75, 1, 1.25, 1.5, 2] as const;
const PROGRESS_EVERY_MS = 10_000;

export function PlayerClient({ sessionId }: { sessionId: string | null }) {
  const router = useRouter();
  const { token, loading: authLoading } = useAuth();
  const player = usePlayer();
  const playerRef = useRef(player);
  playerRef.current = player;

  const [session, setSession] = useState<Session | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [index, setIndex] = useState(0);
  const [statusNote, setStatusNote] = useState("");
  const [playBlocked, setPlayBlocked] = useState(false);
  const [showTranscript, setShowTranscript] = useState(false);
  const [saved, setSaved] = useState(false);

  const lastSync = useRef(0);
  const itemRef = useRef<SessionItem | null>(null);
  const sessionRef = useRef<Session | null>(null);
  sessionRef.current = session;
  const indexRef = useRef(0);
  indexRef.current = index;
  const started = useRef(false);
  const lastEndedToken = useRef(player.endedToken);

  const { playing, position: posSec, duration: durSec } = player;
  const audioBroken = player.failed || playBlocked;

  const items = useMemo(() => {
    const list = [...(session?.items ?? [])];
    list.sort((a, b) => a.position - b.position);
    return list;
  }, [session]);

  const current: SessionItem | null = items[index] ?? null;
  itemRef.current = current;

  const load = useCallback(async () => {
    if (!token || !sessionId) return;
    setLoading(true);
    setLoadError(null);
    const res = await appApi<Session>(`/sessions/${encodeURIComponent(sessionId)}`, { token });
    setLoading(false);
    if (!res.ok) {
      if (res.status === 401) setLoadError("SIGNED_OUT");
      else if (res.status === 404) setLoadError("NOT_FOUND");
      else setLoadError(res.message);
      return;
    }
    setSession(res.data);
    // Resume at the first item the server has not marked finished.
    const sorted = [...(res.data.items ?? [])].sort((a, b) => a.position - b.position);
    const firstOpen = sorted.findIndex((it) => !DONE_ITEM.has(it.status.toUpperCase()));
    setIndex(firstOpen >= 0 ? firstOpen : Math.max(sorted.length - 1, 0));
  }, [token, sessionId]);

  useEffect(() => {
    void load();
  }, [load]);

  // Start once, when the composed session is still a draft on the server.
  useEffect(() => {
    if (!token || !session || started.current) return;
    if (!OPEN_STATUSES.has(session.status)) return;
    if (session.status === "ACTIVE" || session.status === "PAUSED" || session.status === "INTERRUPTED") {
      started.current = true;
      return;
    }
    started.current = true;
    appApi(`/sessions/${session.id}/start`, {
      token,
      method: "POST",
      idempotencyKey: newIdempotencyKey(),
    }).then((r) => {
      if (r.ok) {
        setStatusNote("");
        track(token, ANALYTICS_EVENTS.sessionStarted, { session_id: session.id });
      } else if (r.status !== 409) setStatusNote(r.message);
    });
  }, [token, session]);

  const syncProgress = useCallback(
    async (itemStatus?: "COMPLETED" | "SKIPPED") => {
      const s = sessionRef.current;
      const it = itemRef.current;
      if (!token || !s || !it) return;
      await appApi(`/sessions/${s.id}/progress`, {
        token,
        method: "POST",
        idempotencyKey: newIdempotencyKey(),
        body: {
          queue_item_id: it.id,
          position_ms: Math.round(playerRef.current.position * 1000),
          ...(itemStatus ? { item_status: itemStatus } : {}),
        },
      });
    },
    [token],
  );

  function trackFor(item: SessionItem, sessionId: string): PlayerTrack {
    return {
      itemId: item.id,
      sessionId,
      title: item.title || "Untitled confession",
      subtitle: `${item.category || "Session"}`,
      src: item.audio_url ?? null,
      durationHint: item.duration_seconds || 0,
    };
  }

  // Bind the shared element to the current item. The provider dedupes by
  // item, so remounts (navigation away and back) keep playing undisturbed.
  useEffect(() => {
    if (!current || !session) return;
    setPlayBlocked(false);
    playerRef.current.load(trackFor(current, session.id), playerRef.current.playing);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [current?.id, current?.audio_url, session?.id]);

  // Natural end of the loaded bytes → advance, but only when the ended
  // track is this page's current item (another session may own the element).
  useEffect(() => {
    if (player.endedToken === lastEndedToken.current) return;
    lastEndedToken.current = player.endedToken;
    const t = playerRef.current.track;
    if (t && current && t.itemId === current.id && t.sessionId === session?.id) {
      advance("COMPLETED", indexRef.current);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [player.endedToken]);

  // 10-second progress narration while playing.
  useEffect(() => {
    const now = Date.now();
    if (playing && now - lastSync.current > PROGRESS_EVERY_MS) {
      lastSync.current = now;
      void syncProgress();
    }
  }, [playing, posSec, syncProgress]);

  // Pause/resume mirror the server when the user presses the big button; the
  // shared element remains the local authority on position.
  const callLifecycle = useCallback(
    async (verb: "pause" | "resume") => {
      const s = sessionRef.current;
      if (!token || !s) return;
      const r = await appApi(`/sessions/${s.id}/${verb}`, { token, method: "POST" });
      if (!r.ok && r.status !== 409) setStatusNote(r.message);
      else setStatusNote("");
    },
    [token],
  );

  const advance = useCallback(
    (itemStatus: "COMPLETED" | "SKIPPED", fromIndex: number) => {
      // Record the item's final state first — including when it is the last
      // one — so the session can never be COMPLETED with an item still marked
      // PLAYING in the queue.
      void syncProgress(itemStatus);
      const nextIdx = items.findIndex((_, i) => i > fromIndex && !DONE_ITEM.has(items[i].status.toUpperCase()));
      if (nextIdx === -1) {
        // End of queue: finish the session server-side. COMPLETED only when
        // the queue was genuinely finished; a skip off the end interrupts.
        const s = sessionRef.current;
        if (s && token && itemStatus === "COMPLETED") {
          void appApi(`/sessions/${s.id}/complete`, {
            token,
            method: "POST",
            idempotencyKey: newIdempotencyKey(),
          }).then((r) => {
            if (r.ok) {
              setSession((cur) => (cur ? { ...cur, status: "COMPLETED" } : cur));
              playerRef.current.pause();
              setStatusNote("Session complete.");
              track(token, ANALYTICS_EVENTS.sessionCompleted, { session_id: s.id });
            } else setStatusNote(r.message);
          });
        }
        return false;
      }
      setItemsLocalStatus(fromIndex, itemStatus);
      setIndex(nextIdx);
      return true;
      // eslint-disable-next-line react-hooks/exhaustive-deps
    },
    [token, items],
  );

  // Local mirror of server item statuses — drives the queue list without a
  // refetch storm. A reload() reconciles with the server as the authority.
  const setItemsLocalStatus = (itemIdx: number, status: "COMPLETED" | "SKIPPED") => {
    setSession((s) => {
      if (!s || !s.items) return s;
      const items2 = s.items.map((it, i) => (i === itemIdx ? { ...it, status } : it));
      return { ...s, items: items2 };
    });
  };

  const togglePlay = useCallback(() => {
    if (!current?.audio_url) return;
    if (playerRef.current.playing) {
      playerRef.current.pause();
      void syncProgress();
      void callLifecycle("pause");
    } else {
      setPlayBlocked(false);
      void playerRef.current.play().then((ok) => {
        if (ok) void callLifecycle("resume");
        else {
          setPlayBlocked(true);
          setStatusNote("The browser blocked playback — press play once more if a prompt appears.");
        }
      });
    }
  }, [current, syncProgress, callLifecycle]);

  const seek = (toSec: number) => {
    playerRef.current.seek(toSec);
  };

  const canSeekBack = posSec > 3 || index > 0;
  const prev = () => {
    if (playerRef.current.position > 3) {
      seek(0);
      return;
    }
    if (index > 0) setIndex(index - 1);
  };
  const next = () => {
    if (!advance("SKIPPED", index)) setStatusNote("This was the last item in the session.");
  };

  const saveFavorite = async () => {
    if (!token || !current) return;
    const r = await appApi("/me/favorites", {
      token,
      method: "POST",
      body: { entity_type: "confession", entity_id: current.confession_id },
    });
    if (r.ok) setSaved(true);
    else if (r.status === 409) setSaved(true); // already saved — idempotent outcome
    else setStatusNote(r.message);
  };

  const shareLink = async () => {
    if (!sessionId) return;
    try {
      await navigator.clipboard.writeText(`${window.location.origin}/app/player?session=${encodeURIComponent(sessionId)}`);
      setStatusNote("Link copied — it opens in this browser for signed-in devices.");
    } catch {
      setStatusNote("Copy the address bar to share this link.");
    }
  };

  // ---------------- render ----------------
  if (authLoading) return <LoadingBlock rows={3} />;
  if (!token) return <SignedOut next={sessionId ? `/app/player?session=${sessionId}` : "/app/player"} />;
  if (!sessionId) {
    return (
      <div className="ic-state">
        <h3>Nothing playing</h3>
        <p>Choose a confession, or build a session — the player opens around it.</p>
        <div className="ic-btn-row">
          <a href="/app/session-builder" className="ic-btn ic-btn--primary">Build a session</a>
          <a href="/app/explore" className="ic-btn ic-btn--secondary">Explore first</a>
        </div>
      </div>
    );
  }
  if (loading && !session) return <LoadingBlock rows={3} />;
  if (loadError === "SIGNED_OUT") return <SignedOut next="/app/player" />;
  if (loadError === "NOT_FOUND") {
    return (
      <div className="ic-state">
        <h3>That session isn&rsquo;t there</h3>
        <p>It may have been removed from history, or the link is out of date.</p>
        <a href="/app/sessions" className="ic-btn ic-btn--secondary">See your sessions</a>
      </div>
    );
  }
  if (loadError) return <ErrorBlock message={loadError} onRetry={() => void load()} />;
  if (!session) return <LoadingBlock rows={3} />;
  if (items.length === 0) {
    return (
      <div className="ic-state">
        <h3>This session has no playable items</h3>
        <p>It was built before content changed, or every item needs a plan you don&rsquo;t have.</p>
        <a href="/app/session-builder" className="ic-btn ic-btn--primary">Build a new session</a>
      </div>
    );
  }

  const done = session.status === "COMPLETED";
  const locked = Boolean(current?.locked || !current?.audio_url);

  return (
    <div className="ip-shell">
      <div
        className="ip-art"
        aria-hidden="true"
        style={{ background: railColor((current?.category ?? "i").toLowerCase()) }}
      >
        {(current?.title ?? "iCONFESS").slice(0, 1).toUpperCase()}
      </div>

      <div className="ip-now">
        <p className="ia-section-title" style={{ margin: "0 0 var(--ic-spacing-2)" }}>
          {session.title || `${current?.category || "A session"} · session`}
        </p>
        <h1 className="ip-now__title">{current?.title || "Untitled confession"}</h1>
        <p className="ip-now__meta">
          {current?.category} · item {index + 1} of {items.length}
          {session.actual_duration
            ? ` · session runs ${Math.round(session.actual_duration / 60)} min`
            : ""}
        </p>
        {session.voice_downgraded && (
          <p className="ip-now__meta" role="status">
            Voice adjusted to one available on your plan.
          </p>
        )}
      </div>

      {locked ? (
        <div className="ic-state" style={{ width: "100%" }}>
          <h3>Not on your plan</h3>
          <p>
            {current?.lock_reason
              ? `This item is held back (${current.lock_reason.replace(/_/g, " ")}).`
              : "Audio for this item isn't available to your plan."}
          </p>
          <div className="ic-btn-row">
            <a href="/app/subscription" className="ic-btn ic-btn--primary">See Premium</a>
            <button type="button" className="ic-btn ic-btn--secondary" onClick={next}>
              Skip to the next item
            </button>
          </div>
        </div>
      ) : audioBroken ? (
        <div className="ic-state" style={{ width: "100%" }} role="alert">
          <h3>Audio didn&rsquo;t load</h3>
          <p>The signed link may have expired, or the network dropped. Retry, or skip this item.</p>
          <div className="ic-btn-row">
            <button
              type="button"
              className="ic-btn ic-btn--primary"
              onClick={() => {
                if (!current || !session) return;
                setPlayBlocked(false);
                playerRef.current.load(trackFor(current, session.id), true);
              }}
            >
              Retry this item
            </button>
            <button type="button" className="ic-btn ic-btn--secondary" onClick={next}>
              Skip
            </button>
          </div>
        </div>
      ) : (
        <div className="ip-seek">
          <div className="ip-controls">
            <button type="button" className="ip-btn" onClick={prev} disabled={done || !canSeekBack} aria-label="Previous">
              ⟨
            </button>
            <button
              type="button"
              className="ip-btn ip-btn--play"
              onClick={togglePlay}
              disabled={done}
              aria-label={playing ? "Pause" : "Play"}
            >
              {playing ? "❚❚" : "▶"}
            </button>
            <button
              type="button"
              className="ip-btn"
              onClick={next}
              disabled={done}
              aria-label="Skip this item"
            >
              ⟩
            </button>
          </div>
          <label htmlFor="ip-slider" className="ic-visually-hidden" style={{ position: "absolute", width: 1, height: 1, overflow: "hidden", clip: "rect(0 0 0 0)" }}>
            Seek within this item
          </label>
          <input
            id="ip-slider"
            type="range"
            min={0}
            max={Math.max(durSec, 1)}
            step={1}
            value={Math.min(posSec, durSec)}
            onChange={(e) => seek(Number(e.target.value))}
            disabled={done}
            aria-valuetext={`${formatClock(posSec)} of ${formatClock(durSec)}`}
          />
          <div className="ip-seek__times" aria-hidden="true">
            <span>{formatClock(posSec)}</span>
            <span>{durSec ? formatClock(durSec) : "—:——"}</span>
          </div>
        </div>
      )}

      <div className="ip-row">
        <label htmlFor="ip-rate">Speed</label>
        <select id="ip-rate" value={player.rate} onChange={(e) => player.setRate(Number(e.target.value))}>
          {SPEEDS.map((s) => (
            <option key={s} value={s}>
              {s}×
            </option>
          ))}
        </select>
        <label htmlFor="ip-vol" style={{ marginLeft: "var(--ic-spacing-3)" }}>Volume</label>
        <input
          id="ip-vol"
          type="range"
          min={0}
          max={1}
          step={0.05}
          value={player.volume}
          onChange={(e) => player.setVolume(Number(e.target.value))}
          style={{ width: "7rem", accentColor: "var(--ic-color-brand-600)" }}
          aria-valuetext={`${Math.round(player.volume * 100)} percent`}
        />
        <button type="button" className="ic-btn ic-btn--text" onClick={() => void saveFavorite()} disabled={!current || saved}>
          {saved ? "Saved ✓" : "Save"}
        </button>
        <button type="button" className="ic-btn ic-btn--text" onClick={() => void shareLink()}>
          Copy link
        </button>
        {current?.text && (
          <button type="button" className="ic-btn ic-btn--text" aria-expanded={showTranscript} onClick={() => setShowTranscript((s) => !s)}>
            Transcript
          </button>
        )}
      </div>

      <p className="ip-status" role="status" aria-live="polite">
        {done
          ? "This session is complete."
          : statusNote || (locked || audioBroken ? "" : playing ? "Playing" : posSec > 0 ? "Paused" : "Ready")}
      </p>

      {showTranscript && current?.text && (
        <div className="ip-transcript" tabIndex={0}>
          {current.text}
        </div>
      )}

      <ol className="ip-queue" aria-label="Session queue">
        {items.map((it, i) => (
          <li
            key={it.id}
            className="ip-queue__item"
            data-current={i === index ? "1" : "0"}
            data-state={it.status.toUpperCase()}
          >
            <span className="ip-queue__num">{i + 1}</span>
            <button
              type="button"
              className="ic-btn ic-btn--text"
              style={{ textAlign: "left", flex: 1, padding: 0 }}
              onClick={() => {
                if (i === index) return;
                setIndex(i);
              }}
            >
              <strong style={{ fontWeight: "var(--ic-font-weight-medium)" }}>{it.title || "Untitled"}</strong>
              <span style={{ display: "block", color: "var(--ic-color-neutral-500)", fontSize: "var(--ic-font-size-caption)" }}>
                {it.category} · {formatClock(it.duration_seconds)}
                {it.status.toUpperCase() === "COMPLETED" ? " · done" : it.status.toUpperCase() === "SKIPPED" ? " · skipped" : ""}
                {it.locked ? " · Premium" : ""}
              </span>
            </button>
          </li>
        ))}
      </ol>

      <p style={{ fontSize: "var(--ic-font-size-caption)", color: "var(--ic-color-neutral-500)", textAlign: "center" }}>
        <a href="/app/sessions" style={{ color: "inherit" }}>All sessions</a>
        {" · "}
        <button type="button" className="ic-btn--text" style={{ font: "inherit", border: "none", background: "none", padding: 0, cursor: "pointer", color: "inherit", textDecoration: "underline" }} onClick={() => router.push("/app/routines")}>
          Save this shape as a routine
        </button>
      </p>
    </div>
  );
}
