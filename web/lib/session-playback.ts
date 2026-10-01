import type { PlaybackItem } from "./audio-playback";
import type { AppApiResult, AppApiOptions } from "./app-api";

export type ApiSession = {
  id: string;
  title?: string;
  status: string;
  voice_id?: string;
  voice_downgraded?: boolean;
  voice_downgrade_reason?: string;
  target_duration?: number;
  actual_duration?: number;
  duration_seconds?: number;
  items: {
    id: string; position: number; title?: string; category?: string; text?: string;
    confession_id?: string; audio_url?: string; duration_seconds: number;
    locked?: boolean; lock_reason?: string; status?: string;
  }[];
};
export function sessionQueue(session: ApiSession): PlaybackItem[] {
  return [...session.items].sort((a, b) => a.position - b.position).map((item) => ({
    slug: item.confession_id || item.id, itemId: item.id, sessionId: session.id,
    confessionId: item.confession_id, title: item.title || session.title || "Confession",
    category: item.category || "Session", text: item.text || "",
    // Explicitly null is a locked/unavailable recording, not device speech.
    src: item.locked ? null : item.audio_url || null, durationSeconds: item.duration_seconds,
    disclosure: "Published session audio",
  }));
}

type Send = (path: string, options: AppApiOptions) => Promise<AppApiResult<unknown>>;
/** Serial writes preserve item/lifecycle ordering while the next track starts.
 * In particular /complete must not race the last item's COMPLETED progress. */
export class SessionReporter {
  private pending: Promise<void> = Promise.resolve();
  private active = true;
  private finished = false;
  constructor(
    readonly id: string, private readonly token: string,
    private readonly send: Send, private readonly onError: (message: string) => void,
    private readonly deviceId: string,
  ) {}
  private enqueue(path: string, body?: unknown) {
    if (this.finished) return;
    const options: AppApiOptions = { token: this.token, method: "POST", body };
    const requestId = `${this.deviceId}-${Date.now()}-${Math.random().toString(36).slice(2)}`;
    options.idempotencyKey = requestId;
    this.pending = this.pending.then(async () => {
      try {
        const result = await this.send(`/sessions/${encodeURIComponent(this.id)}/${path}`, options);
        if (!result.ok) this.onError(result.message);
      } catch { this.onError("Listening progress could not be synced. Check your connection."); }
    });
  }
  progress(item: PlaybackItem, position: number, status?: "PLAYING" | "COMPLETED" | "SKIPPED") {
    if (item.sessionId !== this.id) return;
    this.enqueue("progress", {
      queue_item_id: item.itemId, position_ms: Math.round(Math.max(0, position) * 1000),
      device_id: this.deviceId, last_updated_at: new Date().toISOString(),
      ...(status ? { item_status: status } : {}),
    });
  }
  pause() { if (this.active) { this.enqueue("pause"); this.active = false; } }
  resume() { if (!this.active) { this.enqueue("resume"); this.active = true; } }
  complete() { this.enqueue("complete"); this.finished = true; }
  interrupt() { if (!this.finished) this.enqueue("interrupt"); this.finished = true; }
  flushed(): Promise<void> { return this.pending; }
}
