"use client";

/**
 * /app/settings/* — real preference storage.
 *
 * GET /me/preferences on load; every change goes straight to
 * PATCH /me/preferences and is optimistic with a rollback: if the server
 * refuses (a premium voice without entitlement → 402, an unknown voice id →
 * 422, a bad quality string never reaches here because the select cannot
 * produce one), the control snaps back and the server's own message is shown.
 *
 * The notification channel toggles are a separate surface —
 * GET/PATCH /me/notifications — and live on the notifications page, because
 * they are delivered preferences, not app preferences.
 */

import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { useAuth } from "@/lib/auth-context";
import { appApi, useApiData } from "@/lib/app-api";
import type { Voice } from "@/lib/api";
import { ErrorBlock, LoadingBlock, SavedNote, SignedOut } from "@/components/app-ui";

export type Preferences = {
  id: string;
  user_id: string;
  default_duration: number;
  default_voice_id?: string;
  autoplay: boolean;
  preferred_quality: string;
  download_over_wifi: boolean;
  notifications_enabled: boolean;
  recommendations_enabled: boolean;
  personalization_enabled: boolean;
  language: string;
  theme: string;
  updated_at?: string;
};

export type SettingsSection = "playback" | "privacy" | "notifications" | "accessibility";

export function PreferencesClient({ section }: { section: SettingsSection }) {
  const { token, loading: authLoading } = useAuth();
  const prefs = useApiData<Preferences>(token, "/me/preferences");
  const voices = useApiData<Voice[]>(token, "/voices");
  const [note, setNote] = useState("");
  const [error, setError] = useState("");

  const patch = useCallback(
    async (body: Partial<Preferences>) => {
      if (!token) return;
      setError("");
      setNote("");
      // Optimistic merge into the fetched copy; the PATCH response is the
      // truth and replaces it. On failure, the next render reads the still
      // -stale fetched copy and the control snaps back on its own.
      const res = await appApi<Preferences>("/me/preferences", { token, method: "PATCH", body });
      if (res.ok) {
        prefs.reload();
        flash(setNote, "Saved.");
      } else {
        setError(`${res.message}${res.code === "ENTITLEMENT_REQUIRED" ? " — available on Premium." : ""}`);
      }
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [token],
  );

  if (authLoading) return <LoadingBlock rows={4} />;
  if (!token) return <SignedOut next={`/app/settings/${section}`} />;
  if (prefs.loading && !prefs.data) return <LoadingBlock rows={4} />;
  if (prefs.error && !prefs.data) {
    if (prefs.error.status === 401) return <SignedOut next={`/app/settings/${section}`} />;
    return <ErrorBlock message={prefs.error.message} onRetry={prefs.reload} />;
  }
  const p = prefs.data as Preferences;
  const voiceList = (voices.data ?? []).filter((v) => v.status === "active");

  return (
    <div className="ia-settings" style={{ maxWidth: "40rem" }}>
      {error && (
        <div role="alert" className="ic-field__error" style={{ marginBottom: "var(--ic-spacing-3)" }}>
          {error}
        </div>
      )}
      {note && !error && (
        <div style={{ marginBottom: "var(--ic-spacing-3)" }}>
          <SavedNote>{note}</SavedNote>
        </div>
      )}

      {section === "playback" && (
        <>
          <Group title="Audio">
            <Toggle
              label="Auto-play next"
              hint="Continue to the next confession in a session."
              checked={p.autoplay}
              onChange={(v) => patch({ autoplay: v })}
            />
            <Field label="Preferred quality">
              <select
                aria-label="Preferred audio quality"
                value={p.preferred_quality}
                onChange={(e) => patch({ preferred_quality: e.target.value })}
              >
                <option value="low">Low — saves data</option>
                <option value="standard">Standard</option>
                <option value="high">High</option>
              </select>
            </Field>
            <Field label="Default session length">
              <select
                aria-label="Default session length"
                value={p.default_duration}
                onChange={(e) => patch({ default_duration: Number(e.target.value) })}
              >
                {[300, 600, 900, 1800, 2700, 3600].map((s) => (
                  <option key={s} value={s}>
                    {s / 60} minutes
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Default voice">
              <select
                aria-label="Default voice"
                value={p.default_voice_id ?? ""}
                onChange={(e) => patch({ default_voice_id: e.target.value })}
              >
                <option value="">No preference</option>
                {voiceList.map((v) => (
                  <option key={v.id} value={v.id}>
                    {v.name}
                    {v.premium ? " · Premium" : ""}
                  </option>
                ))}
              </select>
              {voiceList.length === 0 && voices.loading && (
                <p className="ic-field__hint">Loading voices…</p>
              )}
              {voiceList.length === 0 && !voices.loading && (
                <p className="ic-field__hint">No voices are available to choose yet.</p>
              )}
            </Field>
          </Group>
          <Group title="Offline">
            <Toggle
              label="Download over Wi-Fi only"
              hint="Save mobile data by taking confessions offline only on Wi-Fi."
              checked={p.download_over_wifi}
              onChange={(v) => patch({ download_over_wifi: v })}
            />
          </Group>
        </>
      )}

      {section === "privacy" && (
        <>
          <Group title="Personalization">
            <Toggle
              label="Recommendations"
              hint="Use what you listen to to suggest what to return to. Without it, suggestions fall back to broad categories."
              checked={p.recommendations_enabled}
              onChange={(v) => patch({ recommendations_enabled: v })}
            />
            <Toggle
              label="Personalize across surfaces"
              hint="Let your saved interests and listening history shape the order of what you see."
              checked={p.personalization_enabled}
              onChange={(v) => patch({ personalization_enabled: v })}
            />
          </Group>
          <Group title="Your data">
            <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", margin: 0 }}>
              Export, deletion requests and the full list of what is stored are handled through the
              support surface and the privacy policy. Nothing here turns off security mail — those
              notices are always sent.
            </p>
            <div className="ic-btn-row" style={{ marginTop: "var(--ic-spacing-4)" }}>
              <a href="/privacy" className="ic-btn ic-btn--secondary">Privacy policy</a>
              <a href="/app/support" className="ic-btn ic-btn--text">Contact support</a>
            </div>
          </Group>
        </>
      )}

      {section === "notifications" && (
        <>
          <Group title="In-app">
            <Toggle
              label="Notifications from iCONFESS"
              hint="The master switch for everything below."
              checked={p.notifications_enabled}
              onChange={(v) => patch({ notifications_enabled: v })}
            />
          </Group>
          <NotificationChannels token={token} onFlash={() => flash(setNote, "Saved.")} />
        </>
      )}

      {section === "accessibility" && (
        <>
          <Group title="Appearance">
            <Field label="Theme">
              <select
                aria-label="Theme"
                value={p.theme}
                onChange={(e) => patch({ theme: e.target.value })}
              >
                <option value="system">Match my device</option>
                <option value="light">Light</option>
                <option value="dark">Dark</option>
              </select>
            </Field>
          </Group>
          <Group title="Motion on this device">
            <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", margin: "0 0 var(--ic-spacing-3)" }}>
              Your operating system&rsquo;s reduced-motion setting is always respected. This control
              applies the same restraint on this browser alone — it is saved on the device, not to
              your account, because it belongs to the screen you are looking at.
            </p>
            <DeviceMotionToggle />
          </Group>
        </>
      )}
    </div>
  );
}

/** Delivered-channel toggles from GET/PATCH /me/notifications. */
function NotificationChannels({ token, onFlash }: { token: string; onFlash: () => void }) {
  const [channels, setChannels] = useState<Record<string, boolean> | null>(null);
  const [failed, setFailed] = useState("");
  const [busy, setBusy] = useState("");

  useEffect(() => {
    const ac = new AbortController();
    appApi<{ preferences: Record<string, boolean>; note: string }>("/me/notifications", { token, signal: ac.signal }).then((r) => {
      if (ac.signal.aborted) return;
      if (r.ok) setChannels(r.data.preferences ?? {});
      else setFailed(r.message);
    });
    return () => ac.abort();
  }, [token]);

  async function save(key: string, value: boolean) {
    if (!channels) return;
    const prev = channels;
    setChannels({ ...channels, [key]: value });
    setBusy(key);
    // PATCH returns the updated NotificationPreferences flat; GET wraps it in
    // {preferences, note}. Both shapes are consumed explicitly — no guessing.
    const res = await appApi<Record<string, boolean>>("/me/notifications", {
      token,
      method: "PATCH",
      body: { [key]: value },
    });
    setBusy("");
    if (!res.ok) {
      setChannels(prev);
      setFailed(res.message);
      return;
    }
    setChannels((cur) => ({ ...(cur ?? {}), ...(res.data ?? {}) }));
    setFailed("");
    onFlash();
  }

  if (failed && !channels) return <ErrorBlock message={failed} />;
  if (!channels) return <LoadingBlock rows={3} />;

  const rows: { key: string; label: string; hint: string }[] = [
    { key: "scheduled_sessions", label: "Scheduled sessions", hint: "A nudge when a session you scheduled is ready." },
    { key: "new_content", label: "New content", hint: "When a category you follow adds confessions." },
    { key: "recommendations", label: "Recommendations", hint: "Occasional suggestions for what to return to." },
    { key: "product_updates", label: "Product updates", hint: "Changes to iCONFESS itself. Rare." },
  ];

  return (
    <Group title="What we may send">
      {rows.map((row) => (
        <Toggle
          key={row.key}
          label={row.label}
          hint={row.hint}
          checked={Boolean(channels[row.key])}
          disabled={busy === row.key}
          onChange={(v) => void save(row.key, v)}
        />
      ))}
      <p style={{ fontSize: "var(--ic-font-size-caption)", color: "var(--ic-color-neutral-500)", margin: "var(--ic-spacing-3) 0 0" }}>
        Security notifications are always sent and cannot be disabled.
      </p>
      {failed && (
        <p role="alert" className="ic-field__error" style={{ margin: "var(--ic-spacing-2) 0 0" }}>
          {failed}
        </p>
      )}
    </Group>
  );
}

/** Reduced-motion override stored on this device and applied to <html>. */
function DeviceMotionToggle() {
  const [reduced, setReduced] = useState(false);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    try {
      const v = localStorage.getItem("ic-reduce-motion") === "1";
      setReduced(v);
      applyReduceMotion(v);
    } catch {
      // private mode without storage — the OS-level media query still applies
    }
    setLoaded(true);
  }, []);

  return (
    <Toggle
      label="Reduce motion"
      hint="Calm transitions and reveals in this browser."
      checked={reduced}
      disabled={!loaded}
      onChange={(v) => {
        setReduced(v);
        applyReduceMotion(v);
        try {
          if (v) localStorage.setItem("ic-reduce-motion", "1");
          else localStorage.removeItem("ic-reduce-motion");
        } catch {
          // best-effort persistence
        }
      }}
    />
  );
}

function applyReduceMotion(on: boolean) {
  if (typeof document === "undefined") return;
  if (on) document.documentElement.setAttribute("data-ic-reduce-motion", "1");
  else document.documentElement.removeAttribute("data-ic-reduce-motion");
}

function Group({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="ia-settings__group">
      <h2>{title}</h2>
      <div style={{ display: "grid", gap: "var(--ic-spacing-2)" }}>{children}</div>
    </div>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="ic-field" style={{ paddingTop: "var(--ic-spacing-2)" }}>
      <label>{label}</label>
      {children}
    </div>
  );
}

export function Toggle({
  label,
  hint,
  checked,
  onChange,
  disabled,
}: {
  label: string;
  hint?: string;
  checked: boolean;
  onChange: (v: boolean) => void;
  disabled?: boolean;
}) {
  return (
    <div className="ia-toggle">
      <div>
        <p style={{ fontWeight: "var(--ic-font-weight-medium)", margin: 0 }}>{label}</p>
        {hint && (
          <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-500)", margin: "2px 0 0" }}>
            {hint}
          </p>
        )}
      </div>
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        aria-label={label}
        disabled={disabled}
        className="ia-toggle__track"
        onClick={() => onChange(!checked)}
      >
        <span className="ia-toggle__thumb" />
      </button>
    </div>
  );
}

function flash(set: (s: string) => void, msg: string) {
  set(msg);
  setTimeout(() => set(""), 2600);
}
