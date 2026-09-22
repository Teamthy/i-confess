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
 * States §31: idle (no session chosen), loading, playing, paused, completed,
 * error (network or an unplayable asset — audio errors say so and offer skip,
 * they never fail silently).
 */

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth-context";
import { appApi, formatClock, newIdempotencyKey } from "@/lib/app-api";
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

  const [session, setSession] = useState<Session | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [index, setIndex] = useState(0);
  const [playing, setPlaying] = useState(false);
  const [posSec, setPosSec] = useState(0);
  const [durSec, setDurSec] = useState(0);
  const [rate, setRate] = useState<number>(1);
  const [volume, setVolume] = useState(1);
  const [statusNote, setStatusNote] = useState("");
  const [audioBroken, setAudioBroken] = useState(false);
  const [showTranscript, setShowTranscript] = useState(false);
  const [saved, setSaved] = useState(false);

  const audioRef = useRef<HTMLAudioElement | null>(null);
  const lastSync = useRef(0);
  const itemRef = useRef<SessionItem | null>(null);
  const sessionRef = useRef<Session | null>(null);
  sessionRef.current = session;
  const started = useRef(false);

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
      if (r.ok) setStatusNote("");
      else if (r.status !== 409) setStatusNote(r.message);
    });
  }, [token, session]);

  const syncProgress = useCallback(
    async (itemStatus?: "COMPLETED" | "SKIPPED") => {
      const s = sessionRef.current;
      const it = itemRef.current;
      const a = audioRef.current;
      if (!token || !s || !it) return;
      await appApi(`/sessions/${s.id}/progress`, {
        token,
        method: "POST",
        idempotencyKey: newIdempotencyKey(),
        body: {
          queue_item_id: it.id,
          position_ms: Math.round((a?.currentTime ?? 0) * 1000),
          ...(itemStatus ? { item_status: itemStatus } : {}),
        },
      });
    },
    [token],
  );

  // Wire the audio element to the current item.
  useEffect(() => {
    const a = audioRef.current;
    if (!a || !current) return;
    setPosSec(0);
    setDurSec(current.duration_seconds || 0);
    setAudioBroken(false);
    if (current.audio_url) {
      a.src = current.audio_url;
      a.load();
      // Autoplay policy: the play() that follows `load` is user-initiated only
      // through the play button; starting the element here would be blocked
      // anyway, so the element is left armed and [playing] decides whether to
      // call play().
      if (playing) void a.play().catch(() => setPlaying(false));
    } else {
      a.removeAttribute("src");
      setPlaying(false);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [current?.id, current?.audio_url]);

  useEffect(() => {
    const a = audioRef.current;
    if (a) {
      a.playbackRate = rate;
      a.volume = volume;
    }
  }, [rate, volume, current?.id]);

  // Pause/resume mirror the server when the user presses the big button; the
  // audio element remains the local authority on position.
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
              setPlaying(false);
              setStatusNote("Session complete.");
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
    const a = audioRef.current;
    if (!a || !current?.audio_url) return;
    if (playing) {
      a.pause();
      setPlaying(false);
      void syncProgress();
      void callLifecycle("pause");
    } else {
      void a
        .play()
        .then(() => {
          setPlaying(true);
          void callLifecycle("resume");
        })
        .catch(() => {
          setAudioBroken(true);
          setStatusNote("The browser blocked playback — press play once more if a prompt appears.");
        });
    }
  }, [playing, current, syncProgress, callLifecycle]);

  const seek = (toSec: number) => {
    const a = audioRef.current;
    if (!a) return;
    a.currentTime = toSec;
    setPosSec(toSec);
  };

  const canSeekBack = posSec > 3 || index > 0;
  const onEnded = useCallback(() => {
    advance("COMPLETED", index);
  }, [advance, index]);

  const prev = () => {
    const a = audioRef.current;
    if (a && a.currentTime > 3) {
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
      <audio
        ref={audioRef}
        onTimeUpdate={(e) => {
          const el = e.currentTarget;
          setPosSec(el.currentTime);
          setDurSec(Number.isFinite(el.duration) && el.duration > 0 ? el.duration : current?.duration_seconds || 0);
          const now = Date.now();
          if (playing && now - lastSync.current > PROGRESS_EVERY_MS) {
            lastSync.current = now;
            void syncProgress();
          }
        }}
        onEnded={onEnded}
        onError={() => {
          if (!current?.audio_url) return; // no source assigned yet / intentionally cleared
          setAudioBroken(true);
          setPlaying(false);
        }}
        preload="metadata"
      />

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
                setAudioBroken(false);
                const a = audioRef.current;
                if (a && current?.audio_url) {
                  a.src = current.audio_url;
                  a.load();
                  void a.play().then(() => setPlaying(true)).catch(() => setAudioBroken(true));
                }
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
        <select id="ip-rate" value={rate} onChange={(e) => setRate(Number(e.target.value))}>
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
          value={volume}
          onChange={(e) => setVolume(Number(e.target.value))}
          style={{ width: "7rem", accentColor: "var(--ic-color-brand-600)" }}
          aria-valuetext={`${Math.round(volume * 100)} percent`}
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
