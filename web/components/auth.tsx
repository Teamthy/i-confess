"use client";
import React, { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Icon, Logo } from "./ui";
import { useApp, mutate, track } from "@/lib/store";
import { useAuth } from "@/lib/auth-context";
import { authFetch } from "@/lib/auth-client";
import { appApi } from "@/lib/app-api";
import { useToast } from "@/lib/ui";
import { CATEGORIES } from "@/lib/data";
import { ALL_DAYS, deviceTimezone } from "@/lib/schedules";

type PasswordFieldProps = Omit<React.InputHTMLAttributes<HTMLInputElement>, "type"> & {
  label: string;
  hint?: string;
};

/** Passwords remain hidden by default, with an explicit, keyboard-accessible
 * reveal control wherever the auth flow asks a person to enter one. */
function PasswordField({ label, hint, id, ...inputProps }: PasswordFieldProps) {
  const [visible, setVisible] = useState(false);
  const generatedID = React.useId();
  const fieldID = id || generatedID;
  const hintID = hint ? `${fieldID}-hint` : undefined;
  const describedBy = [inputProps["aria-describedby"], hintID].filter(Boolean).join(" ") || undefined;
  return (
    <div className="field password-field">
      <label htmlFor={fieldID}>{label}</label>
      <div className="password-control">
        <input {...inputProps} id={fieldID} aria-describedby={describedBy} type={visible ? "text" : "password"} />
        <button
          className="password-toggle"
          type="button"
          aria-label={visible ? `Hide ${label.toLowerCase()}` : `Show ${label.toLowerCase()}`}
          aria-pressed={visible}
          onClick={() => setVisible((shown) => !shown)}
        >
          <Icon n={visible ? "eyeOff" : "eye"} s={17} />
        </button>
      </div>
      {hint && <p id={hintID} className="password-hint">{hint}</p>}
    </div>
  );
}

export default function AuthPage({ kind }: { kind: string }) {
  const router = useRouter(); const toast = useToast(); const st = useApp();
  const auth = useAuth();
  const [interests, setInterests] = useState<string[]>(st.user?.interests || []);
  // The query string is read after mount so server and client render the same markup.
  const [search, setSearch] = useState("");
  React.useEffect(() => { setSearch(window.location.search); }, []);

  /**
   * Sign in for real, and fall back honestly.
   *
   * The merged app is both the marketing site and the product: it has to work
   * as a browsable site with no backend in front of it, and as the real client
   * when the API is there. So the form calls the API first — a wrong password
   * is a wrong password, and the server says so — and only when the API cannot
   * be reached at all (status 0) does it open a local, clearly-labelled
   * session so the demo surfaces stay walkable. It never pretends a failed
   * password succeeded.
   */
  // `name` is the account's display name when the server supplied one; the
  // email-derived name is only for the no-API demo session.
  const localSession = (email: string, name?: string) => {
    mutate((s) => { s.user = { name: name?.trim() || email.split("@")[0].replace(/[._]/g, " "), email, plan: "free", interests: s.user?.interests || [] }; });
  };

  const signIn = async (email: string, password: string) => {
    const result = await auth.login(email, password);
    if (result.ok) {
      localSession(email, result.user?.display_name);
      track("account_created", { type: "login" });
      toast("Welcome back");
      router.push(safeNext(window.location.search));
      return;
    }
    if (result.mfaRequired) { toast(result.error || "Enter the code from your authenticator app."); return; }
    // status 0 is "no API reachable", which is the demo deployment.
    if (result.error && /couldn't reach the service/i.test(result.error)) {
      localSession(email);
      toast("No API here — opened a local demo session");
      router.push("/app");
      return;
    }
    toast(result.error || "That didn't work.");
  };

  const submit = (e: React.FormEvent, fn: (fd: FormData) => void) => {
    e.preventDefault();
    const form = e.target as HTMLFormElement;
    if (!form.reportValidity()) return;
    fn(new FormData(form));
  };

  if (kind === "welcome") {
    return (
      <div className="auth-card rv in" style={{ width: "min(560px,100%)" }}>
        <Logo /><h1>Welcome{st.user ? ", " + st.user.name.split(" ")[0] : ""}.</h1>
        <p className="sub">Choose the areas of life you want to start with — this shapes your recommendations. You can change this anytime.</p>
        <div className="chip-row" style={{ marginTop: 20 }}>
          {CATEGORIES.slice(0, 12).map((c) => (
            <button className="chip" key={c.slug} aria-pressed={interests.includes(c.slug)} onClick={() => {
              const on = interests.includes(c.slug);
              const next = on ? interests.filter((x) => x !== c.slug) : [...interests, c.slug];
              setInterests(next);
              mutate((s) => { if (s.user) s.user.interests = next; });
            }}>{c.name}</button>
          ))}
        </div>
        <Link className="btn btn-primary btn-lg" style={{ width: "100%", justifyContent: "center", marginTop: 24 }} href="/app">Enter I CONFESS</Link>
      </div>
    );
  }

  const card: Record<string, React.ReactNode> = {
    login: (
      <>
        <Logo /><h1>Welcome back</h1><p className="sub">Your words are where you left them.</p>
        <form onSubmit={(e) => submit(e, (fd) => { void signIn(String(fd.get("email")), String(fd.get("pw"))); })}>
          <div className="field"><label>Email</label><input name="email" type="email" required autoComplete="email" /></div>
          <PasswordField label="Password" name="pw" required autoComplete="current-password" />
          <button className="btn btn-primary btn-lg" style={{ width: "100%", justifyContent: "center", marginTop: 22 }} type="submit">Log In</button>
        </form>
        <p className="auth-foot"><Link href="/forgot-password">Forgot password?</Link></p>
        <p className="auth-foot">New here? <Link href={"/register" + search}>Create an account</Link></p>
      </>
    ),
    register: <Onboarding />,
    forgot: (
      <>
        <Logo /><h1>Reset password</h1><p className="sub">Enter your email and we'll send a reset link.</p>
        <form onSubmit={(e) => submit(e, async (fd) => {
          // The API answers every address the same way, registered or not.
          const r = await authFetch("/auth/request-password-reset", { email: String(fd.get("email")).trim() });
          if (!r.ok && r.status !== 0) { toast(r.message); return; }
          toast("If that address has an account, a reset link is on its way.");
          router.push("/login");
        })}>
          <div className="field"><label>Email</label><input name="email" type="email" required /></div>
          <button className="btn btn-primary btn-lg" style={{ width: "100%", justifyContent: "center", marginTop: 20 }} type="submit">Send Reset Link</button>
        </form>
        <p className="auth-foot"><Link href="/login">Back to log in</Link></p>
      </>
    ),
    reset: (
      <>
        <Logo /><h1>Choose a new password</h1>
        <form onSubmit={(e) => submit(e, async (fd) => {
          if (fd.get("p1") !== fd.get("p2")) { toast("Passwords don't match"); return; }
          const token = new URLSearchParams(window.location.search).get("token") || "";
          if (!token) { toast("Open this page from the link in your reset email."); return; }
          const r = await authFetch("/auth/reset-password", { token, password: String(fd.get("p1")) });
          if (!r.ok) { toast(r.message); return; }
          toast("Password updated — sign in with the new one");
          router.push("/login");
        })}>
          <PasswordField label="New password" name="p1" minLength={8} required autoComplete="new-password" hint="At least 8 characters. Avoid common passwords." />
          <PasswordField label="Repeat password" name="p2" minLength={8} required autoComplete="new-password" />
          <button className="btn btn-primary btn-lg" style={{ width: "100%", justifyContent: "center", marginTop: 20 }} type="submit">Save Password</button>
        </form>
      </>
    ),
    verifyEmail: <VerifyEmail />,
    verifyPhone: (
      <>
        <Logo /><h1>Phone verification</h1>
        <p className="sub">Phone numbers aren't verified on the web yet. Your account works without one.</p>
        <Link className="btn btn-primary btn-lg" href="/app/settings" style={{ width: "100%", justifyContent: "center", marginTop: 20 }}>Back to settings</Link>
      </>
    ),
  };
  return <div className="auth-card rv in">{card[kind]}</div>;
}

/** A same-origin path from `?next=`, else the app. */
function safeNext(search: string) {
  const raw = new URLSearchParams(search).get("next") || "";
  return raw.startsWith("/") && !raw.startsWith("//") ? raw : "/app";
}

/** Opened from the verification email: `?token=` is consumed once, on mount. */
function VerifyEmail() {
  const [state, setState] = useState<"working" | "done" | "failed">("working");
  const [msg, setMsg] = useState("");
  React.useEffect(() => {
    const token = new URLSearchParams(window.location.search).get("token") || "";
    if (!token) { setState("failed"); setMsg("Open this page from the link in your verification email."); return; }
    void authFetch("/auth/verify-email", { token }).then((r) => {
      setState(r.ok ? "done" : "failed");
      if (!r.ok) setMsg(r.message || "That link has expired or was already used.");
    });
  }, []);
  return (
    <>
      <Logo /><h1>{state === "done" ? "Email verified" : state === "failed" ? "We couldn't verify that link" : "Verifying your email…"}</h1>
      <p className="sub" role="status">{state === "done" ? "Thank you. Your account is confirmed." : msg}</p>
      {state !== "working" && <Link className="btn btn-primary btn-lg" href={state === "done" ? "/app" : "/login"} style={{ width: "100%", justifyContent: "center", marginTop: 20 }}>{state === "done" ? "Open I CONFESS" : "Go to sign in"}</Link>}
    </>
  );
}

function Onboarding() {
  const router = useRouter(); const toast = useToast(); const auth = useAuth();
  // Where the visitor was headed (a home CTA, a gated page). Same-origin paths only.
  const [next, setNext] = useState("/app");
  React.useEffect(() => { setNext(safeNext(window.location.search)); }, []);
  const [step, setStep] = useState(0);
  const [busy, setBusy] = useState(false);
  const [name, setName] = useState(""); const [email, setEmail] = useState("");
  const [interests, setInterests] = useState<string[]>([]);
  const [time, setTime] = useState("06:00"); const [len, setLen] = useState("10");
  const [remind, setRemind] = useState(true);

  // Step 1 creates the real account. The API answers an address that is
  // already registered exactly like a fresh one (no membership oracle), with
  // `pending` and no session — that visitor is sent to check their email.
  const account = async (e: React.FormEvent) => {
    e.preventDefault();
    const f = e.target as HTMLFormElement;
    if (!f.reportValidity()) return;
    const password = String(new FormData(f).get("password") || "");
    setBusy(true);
    const r = await authFetch("/auth/register", { email: email.trim(), password, display_name: name.trim(), timezone: deviceTimezone() });
    setBusy(false);
    // No API in front of the site (the static demo): keep the setup walkable locally.
    if (!r.ok && r.status === 0) { setStep(2); return; }
    if (!r.ok) { toast(r.message || "We couldn't create your account."); return; }
    if (!r.token || !r.user) { setStep(1); return; }
    auth.setToken(r.token, r.user);
    track("account_created");
    setStep(2);
  };
  const finish = async () => {
    const cats = interests.slice(0, 3);
    setBusy(true);
    const token = auth.token;
    let saved = true;
    if (token) {
      // Signed in: the practice lives on the account, so the reminder fires on
      // every device. Interests and schedules are keyed by category id; the
      // static library is keyed by slug.
      const list = await appApi<{ id: string; slug: string }[]>("/categories");
      const rows = list.ok && Array.isArray(list.data) ? list.data : [];
      const idOf = (slug: string) => rows.find((c) => c.slug === slug)?.id;
      const interestIds = interests.map(idOf).filter((x): x is string => !!x);
      const practiceIds = cats.map(idOf).filter((x): x is string => !!x);
      if (interestIds.length) {
        const r = await appApi("/me/interests", { token, method: "PUT", body: { category_ids: interestIds, source: "ONBOARDING" } });
        saved = saved && r.ok;
      }
      const sc = await appApi("/schedules", {
        token, method: "POST", idempotencyKey: crypto.randomUUID(),
        body: { label: "Daily practice", time, days_of_week: [...ALL_DAYS], timezone: deviceTimezone(), duration_seconds: Number(len) * 60, category_ids: practiceIds, enabled: true },
      });
      if (!sc.ok) { saved = false; toast(sc.message || "We couldn't save your practice time."); }
      const pr = await appApi("/me/notifications", { token, method: "PATCH", body: { scheduled_sessions: remind } });
      saved = saved && pr.ok;
    }
    setBusy(false);
    mutate((s) => {
      s.user = { name: name.trim(), email: email.trim(), plan: "free", interests };
      s.routines = [{ slot: time, cats }];
      if (!token) {
        // No API (the static demo): keep the practice on this device only.
        s.schedule = [...s.schedule.filter((sc) => sc.localOwner !== email.trim()), { id: `local-${Date.now()}`, label: "Daily practice", time, days_of_week: [...ALL_DAYS], timezone: deviceTimezone(), duration_seconds: Number(len) * 60, category_ids: cats, enabled: true, localOwner: email.trim() }];
        s.settings.notifications.reminders = remind;
      }
    });
    track("onboarding_complete", { interests: interests.length });
    if (saved) toast("Your practice is set up");
    router.push(next);
  };

  const steps = ["Account", "Email", "Interests", "Practice"];
  return (
    <>
      <Logo />
      <div className="ob-steps" role="list" aria-label="Onboarding progress">
        {steps.map((t, i) => <i key={t} className={i <= step ? "on" : ""} aria-label={t + (i <= step ? " (done)" : "")} role="listitem" />)}
      </div>
      <p className="sub" aria-live="polite">Step {step + 1} of 4 — {steps[step]}</p>
      {step === 0 && (
        <>
          <h1>Start your experience</h1><p className="sub">Free forever for the core practice. Premium adds depth.</p>
          <form onSubmit={account}>
            <div className="field"><label>Name</label><input value={name} onChange={(e) => setName(e.target.value)} required autoComplete="name" /></div>
            <div className="field"><label>Email</label><input value={email} onChange={(e) => setEmail(e.target.value)} type="email" required autoComplete="email" /></div>
            <PasswordField label="Password" name="password" required minLength={8} autoComplete="new-password" hint="At least 8 characters. Avoid common passwords." />
            <label style={{ display: "flex", gap: 10, alignItems: "flex-start", marginTop: 16, fontSize: 12.5, color: "var(--n600)" }}><input type="checkbox" required style={{ width: "auto", marginTop: 2 }} /> I consent to product analytics and understand my private content is never reviewed unless I submit it.</label>
            <button className="btn btn-primary btn-lg" style={{ width: "100%", justifyContent: "center", marginTop: 20 }} type="submit" disabled={busy}>{busy ? "Creating account…" : "Create account"}</button>
          </form>
          <p className="auth-foot">Already have an account? <Link href={`/login?next=${encodeURIComponent(next)}`}>Log in</Link></p>
        </>
      )}
      {step === 1 && (
        <>
          <h1>Check your email</h1>
          <p className="sub">We sent a link to <b>{email}</b>. Open it to finish setting up, then sign in. If you already have an account, the email says so.</p>
          <div style={{ display: "flex", gap: 10, marginTop: 20 }}>
            <button className="btn btn-ghost btn-lg" type="button" onClick={() => setStep(0)} style={{ flex: 1, justifyContent: "center" }}>Back</button>
            <Link className="btn btn-primary btn-lg" href={`/login?next=${encodeURIComponent(next)}`} style={{ flex: 2, justifyContent: "center" }}>Go to sign in</Link>
          </div>
        </>
      )}
      {step === 2 && (
        <>
          <h1>What are you carrying?</h1>
          <p className="sub">Pick 3 or more areas — this shapes your recommendations and default practice. You can change this anytime.</p>
          <div className="chip-row" style={{ marginTop: 18 }}>
            {CATEGORIES.map((c) => (
              <button className="chip" key={c.slug} aria-pressed={interests.includes(c.slug)} onClick={() => setInterests((p) => p.includes(c.slug) ? p.filter((x) => x !== c.slug) : [...p, c.slug])}>{c.name}</button>
            ))}
          </div>
          <p className="sub" style={{ marginTop: 14 }}>{interests.length} selected{interests.length < 3 ? " — pick at least 3" : ""}</p>
          <div style={{ display: "flex", gap: 10, marginTop: 8 }}>
            <button className="btn btn-ghost btn-lg" type="button" onClick={() => router.push(next)} style={{ flex: 1, justifyContent: "center" }}>Skip</button>
            <button className="btn btn-primary btn-lg" type="button" disabled={interests.length < 3} onClick={() => setStep(3)} style={{ flex: 2, justifyContent: "center" }}>Continue</button>
          </div>
        </>
      )}
      {step === 3 && (
        <>
          <h1>Set up your practice</h1><p className="sub">Two minutes now, a habit for later.</p>
          <div className="field"><label>When do you want to practice?</label>
            <input type="time" value={time} onChange={(e) => setTime(e.target.value)} /></div>
          <div className="field"><label>Session length</label>
            <div className="ob-opts">
              {[["5", "5 min"], ["10", "10 min"], ["15", "15 min"]].map(([v, l]) => (
                <button key={v} className="ob-opt" aria-pressed={len === v} onClick={() => setLen(v)}>{l}</button>
              ))}
            </div></div>
          <label className="setting-row" style={{ marginTop: 12 }}><span>Daily reminder{remind ? " on" : " off"}</span>
            <button className="switch" type="button" role="switch" aria-checked={remind} onClick={() => setRemind(!remind)} aria-label="Daily reminder" /></label>
          <div style={{ display: "flex", gap: 10, marginTop: 22 }}>
            <button className="btn btn-ghost btn-lg" type="button" onClick={() => setStep(2)} style={{ flex: 1, justifyContent: "center" }}>Back</button>
            <button className="btn btn-primary btn-lg" type="button" onClick={finish} disabled={busy} style={{ flex: 2, justifyContent: "center" }}>{busy ? "Saving…" : "Finish setup"}</button>
          </div>
        </>
      )}
    </>
  );
}
