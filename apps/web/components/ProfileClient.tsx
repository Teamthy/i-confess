"use client";

/**
 * /app/profile — real account data.
 *
 * Reads GET /me (identity + plan) and GET /me/profile; saving goes through
 * PATCH /me/profile with only the fields that actually changed. The server
 * treats every field as a pointer — absent means untouched — so submitting a
 * diff never overwrites what another device changed while this form sat open.
 *
 * Validation mirrors the public shape of the server rules (lengths, https
 * prefixes) as a courtesy only. The authoritative verdict is the 400/409 with
 * a machine code, and that message is surfaced under the field it names:
 * USERNAME_TAKEN, INVALID_TIMEZONE, INVALID_LOCALE.
 */

import { useCallback, useEffect, useMemo, useState } from "react";
import { useAuth } from "@/lib/auth-context";
import { appApi, useApiData, type AppApiFail } from "@/lib/app-api";
import { ErrorBlock, LoadingBlock, SavedNote, SignedOut } from "@/components/app-ui";

type Profile = {
  id: string;
  user_id: string;
  display_name: string;
  username?: string;
  bio?: string;
  avatar_url?: string;
  timezone: string;
  locale: string;
  country_code?: string;
  language: string;
};

type MeResponse = {
  user: {
    id: string;
    email: string;
    display_name?: string;
    timezone?: string;
    status: string;
    email_verified: boolean;
  };
  plan: string;
};

export function ProfileClient() {
  const { token, loading: authLoading } = useAuth();
  const me = useApiData<MeResponse>(token, "/me");
  const profile = useApiData<Profile>(token, "/me/profile");
  const [savedNote, setSavedNote] = useState("");

  const onSaved = useCallback(() => {
    profile.reload();
    setSavedNote("Saved.");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  if (authLoading) return <LoadingBlock rows={4} />;
  if (!token) return <SignedOut next="/app/profile" />;
  if (profile.loading && !profile.data) return <LoadingBlock rows={4} />;
  if (profile.error && !profile.data) {
    if (profile.error.status === 401) return <SignedOut next="/app/profile" />;
    return <ErrorBlock message={profile.error.message} onRetry={profile.reload} />;
  }

  const email = me.data?.user?.email ?? "";
  const verified = me.data?.user?.email_verified;
  const plan = me.data?.plan ?? "";

  return (
    <div className="ia-settings" style={{ maxWidth: "40rem" }}>
      {me.error ? (
        <ErrorBlock message={me.error.message} onRetry={me.reload} />
      ) : (
        <div className="ia-settings__group">
          <h2>Account</h2>
          <dl style={{ display: "grid", gap: "var(--ic-spacing-2)", margin: 0 }}>
            <div style={{ display: "flex", justifyContent: "space-between", gap: "var(--ic-spacing-4)", alignItems: "center" }}>
              <dt style={{ color: "var(--ic-color-neutral-600)", fontSize: "var(--ic-font-size-bodySm)" }}>Email</dt>
              <dd style={{ margin: 0, fontSize: "var(--ic-font-size-bodySm)", fontWeight: "var(--ic-font-weight-medium)" }}>
                {email || "—"}
                {verified === false && (
                  <span className="ia-badge ia-badge--free" style={{ marginLeft: "var(--ic-spacing-2)" }}>not verified</span>
                )}
              </dd>
            </div>
            <div style={{ display: "flex", justifyContent: "space-between", gap: "var(--ic-spacing-4)", alignItems: "center" }}>
              <dt style={{ color: "var(--ic-color-neutral-600)", fontSize: "var(--ic-font-size-bodySm)" }}>Plan</dt>
              <dd style={{ margin: 0, fontSize: "var(--ic-font-size-bodySm)", fontWeight: "var(--ic-font-weight-medium)" }}>
                {plan ? <span style={{ textTransform: "capitalize" }}>{plan}</span> : "—"} ·{" "}
                <a href="/app/subscription" style={{ color: "inherit" }}>manage</a>
              </dd>
            </div>
            {verified === false && (
              <p style={{ fontSize: "var(--ic-font-size-caption)", color: "var(--ic-color-neutral-600)", margin: 0 }}>
                Confirm your address from the verification email we sent — sharing beyond your own
                practice requires it.
              </p>
            )}
          </dl>
        </div>
      )}

      <ProfileForm token={token} profile={profile.data} onSaved={onSaved} savedNote={savedNote} />
    </div>
  );
}

/**
 * Editable identity fields, separate from the read-only summary so "save →
 * refetch summary" can never clobber text mid-edit.
 */
function ProfileForm({
  token,
  profile,
  onSaved,
  savedNote,
}: {
  token: string;
  profile: Profile | null;
  onSaved: () => void;
  savedNote: string;
}) {
  // One state object holding both the draft and the last server-confirmed
  // baseline: "dirty" is the difference, and a refetch may only overwrite the
  // draft when the two still agree (i.e. the user has typed nothing).
  const [doc, setDoc] = useState(() => ({ form: seed(profile), base: seed(profile) }));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<AppApiFail | null>(null);
  const [fieldError, setFieldError] = useState<Record<string, string>>({});

  useEffect(() => {
    const fresh = seed(profile);
    setDoc((d) => ({ form: shallowEqual(d.form, d.base) ? fresh : d.form, base: fresh }));
  }, [profile]);

  const { form, base } = doc;
  const dirty = useMemo(() => {
    const keys = Object.keys(form) as (keyof typeof form)[];
    return keys.filter((k) => form[k] !== base[k]);
  }, [form, base]);

  const set =
    (k: keyof typeof form) =>
    (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
      setDoc((d) => ({ ...d, form: { ...d.form, [k]: e.target.value } }));

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    setFieldError({});

    const issues: Record<string, string> = {};
    if (form.display_name.length > 60) issues.display_name = "Display names are 60 characters or fewer.";
    if (form.bio.length > 500) issues.bio = "Bios are 500 characters or fewer.";
    if (form.avatar_url && !form.avatar_url.startsWith("https://"))
      issues.avatar_url = "Must start with https:// — or leave it empty to remove the photo.";
    if (form.country_code && form.country_code.length !== 2)
      issues.country_code = "Two letters, like NG or US.";
    if (Object.keys(issues).length > 0) {
      setFieldError(issues);
      return;
    }

    const body: Record<string, string> = {};
    for (const k of dirty) body[k] = form[k];
    if (Object.keys(body).length === 0) return;

    setSaving(true);
    const res = await appApi<Profile>("/me/profile", { token, method: "PATCH", body });
    setSaving(false);
    if (!res.ok) {
      setError(res);
      if (res.code === "USERNAME_TAKEN") setFieldError({ username: res.message });
      else if (res.code === "INVALID_TIMEZONE") setFieldError({ timezone: res.message });
      else if (res.code === "INVALID_LOCALE") setFieldError({ locale: res.message, language: res.message });
      return;
    }
    const fresh = seed(res.data);
    setDoc({ form: fresh, base: fresh });
    onSaved();
  }

  return (
    <form onSubmit={onSubmit} noValidate className="ia-settings__group">
      <h2>How you appear</h2>
      <div className="ic-field">
        <label htmlFor="pf-name">Display name</label>
        <input id="pf-name" value={form.display_name} onChange={set("display_name")} maxLength={80} />
        {fieldError.display_name && <p className="ic-field__error">{fieldError.display_name}</p>}
      </div>
      <div className="ic-field">
        <label htmlFor="pf-username">Username</label>
        <input
          id="pf-username"
          value={form.username}
          onChange={set("username")}
          maxLength={30}
          aria-describedby="pf-username-hint"
        />
        <p id="pf-username-hint" className="ic-field__hint">
          Lowercase letters, numbers, dot and underscore; 3–30 characters.
        </p>
        {fieldError.username && <p className="ic-field__error">{fieldError.username}</p>}
      </div>
      <div className="ic-field">
        <label htmlFor="pf-bio">About you</label>
        <textarea id="pf-bio" rows={4} value={form.bio} onChange={set("bio")} maxLength={600} />
        {fieldError.bio && <p className="ic-field__error">{fieldError.bio}</p>}
      </div>
      <div className="ic-field">
        <label htmlFor="pf-avatar">Profile photo URL</label>
        <input id="pf-avatar" type="url" inputMode="url" value={form.avatar_url} onChange={set("avatar_url")} placeholder="https://…" />
        <p className="ic-field__hint">A hosted https image.</p>
        {fieldError.avatar_url && <p className="ic-field__error">{fieldError.avatar_url}</p>}
      </div>

      <h2 style={{ marginTop: "var(--ic-spacing-6)" }}>Place and language</h2>
      <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", margin: 0 }}>
        The time zone decides when a scheduled session fires. Pick by city.
      </p>
      <div className="ic-field">
        <label htmlFor="pf-tz">Time zone</label>
        <input id="pf-tz" value={form.timezone} onChange={set("timezone")} placeholder="Africa/Lagos" />
        {fieldError.timezone && <p className="ic-field__error">{fieldError.timezone}</p>}
      </div>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(9rem, 1fr))", gap: "var(--ic-spacing-4)" }}>
        <div className="ic-field">
          <label htmlFor="pf-locale">Locale</label>
          <input id="pf-locale" value={form.locale} onChange={set("locale")} placeholder="en-NG" />
          {fieldError.locale && <p className="ic-field__error">{fieldError.locale}</p>}
        </div>
        <div className="ic-field">
          <label htmlFor="pf-lang">Language</label>
          <input id="pf-lang" value={form.language} onChange={set("language")} placeholder="en" />
          {fieldError.language && <p className="ic-field__error">{fieldError.language}</p>}
        </div>
        <div className="ic-field">
          <label htmlFor="pf-cc">Country</label>
          <input id="pf-cc" value={form.country_code} onChange={set("country_code")} placeholder="NG" maxLength={2} />
          {fieldError.country_code && <p className="ic-field__error">{fieldError.country_code}</p>}
        </div>
      </div>

      {error && (
        <div role="alert" style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-semantic-danger-light)" }}>
          {error.message}
        </div>
      )}
      <div style={{ display: "flex", alignItems: "center", gap: "var(--ic-spacing-4)", flexWrap: "wrap" }}>
        <button
          type="submit"
          className="ic-btn ic-btn--primary"
          disabled={saving || dirty.length === 0}
          style={{ width: "fit-content" }}
        >
          {saving ? "Saving…" : "Save changes"}
        </button>
        {dirty.length > 0 && !saving && (
          <span style={{ fontSize: "var(--ic-font-size-caption)", color: "var(--ic-color-neutral-500)" }}>
            {dirty.length} unsaved {dirty.length === 1 ? "field" : "fields"}
          </span>
        )}
        {dirty.length === 0 && savedNote && <SavedNote>{savedNote}</SavedNote>}
      </div>
    </form>
  );
}

function seed(p: Profile | null) {
  return {
    display_name: p?.display_name ?? "",
    username: p?.username ?? "",
    bio: p?.bio ?? "",
    avatar_url: p?.avatar_url ?? "",
    timezone: p?.timezone ?? "",
    locale: p?.locale ?? "",
    language: p?.language ?? "",
    country_code: p?.country_code ?? "",
  };
}

function shallowEqual(a: Record<string, string>, b: Record<string, string>) {
  const ka = Object.keys(a);
  const kb = Object.keys(b);
  return ka.length === kb.length && ka.every((k) => a[k] === b[k]);
}
