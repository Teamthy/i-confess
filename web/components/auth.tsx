"use client";
import React, { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Logo } from "./ui";
import { useApp, mutate, track } from "@/lib/store";
import { useToast } from "@/lib/ui";
import { CATEGORIES } from "@/lib/data";

export default function AuthPage({ kind }: { kind: string }) {
  const router = useRouter(); const toast = useToast(); const st = useApp();
  const [interests, setInterests] = useState<string[]>(st.user?.interests || []);

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
        <Link className="btn btn-primary btn-lg" style={{ width: "100%", justifyContent: "center", marginTop: 24 }} href="/app">Enter iCONFESS</Link>
      </div>
    );
  }

  const card: Record<string, React.ReactNode> = {
    login: (
      <>
        <Logo /><h1>Welcome back</h1><p className="sub">Your words are where you left them.</p>
        <form onSubmit={(e) => submit(e, (fd) => { const email = String(fd.get("email")); mutate((s) => { s.user = { name: email.split("@")[0].replace(/[._]/g, " "), email, plan: "free", interests: s.user?.interests || [] }; }); track("account_created", { type: "login" }); toast("Welcome back"); router.push("/app"); })}>
          <div className="field"><label>Email</label><input name="email" type="email" required autoComplete="email" /></div>
          <div className="field"><label>Password</label><input name="pw" type="password" required minLength={6} autoComplete="current-password" /><div className="err">At least 6 characters.</div></div>
          <button className="btn btn-primary btn-lg" style={{ width: "100%", justifyContent: "center", marginTop: 22 }} type="submit">Log In</button>
        </form>
        <p className="auth-foot"><Link href="/forgot-password">Forgot password?</Link></p>
        <p className="auth-foot">New here? <Link href="/register">Create an account</Link></p>
      </>
    ),
    register: <Onboarding />,
    forgot: (
      <>
        <Logo /><h1>Reset password</h1><p className="sub">Enter your email and we'll send a reset link.</p>
        <form onSubmit={(e) => submit(e, () => { toast("Reset link sent (demo)"); router.push("/reset-password"); })}>
          <div className="field"><label>Email</label><input name="email" type="email" required /></div>
          <button className="btn btn-primary btn-lg" style={{ width: "100%", justifyContent: "center", marginTop: 20 }} type="submit">Send Reset Link</button>
        </form>
        <p className="auth-foot"><Link href="/login">Back to log in</Link></p>
      </>
    ),
    reset: (
      <>
        <Logo /><h1>Choose a new password</h1>
        <form onSubmit={(e) => submit(e, (fd) => { if (fd.get("p1") !== fd.get("p2")) { toast("Passwords don't match"); return; } toast("Password updated"); router.push("/login"); })}>
          <div className="field"><label>New password</label><input name="p1" type="password" minLength={6} required /></div>
          <div className="field"><label>Repeat password</label><input name="p2" type="password" minLength={6} required /></div>
          <button className="btn btn-primary btn-lg" style={{ width: "100%", justifyContent: "center", marginTop: 20 }} type="submit">Save Password</button>
        </form>
      </>
    ),
    verifyEmail: verify("email", () => { toast("Verified"); router.push("/app"); }),
    verifyPhone: verify("phone", () => { toast("Verified"); router.push("/app"); }),
  };
  return <div className="auth-card rv in">{card[kind]}</div>;

  function verify(kind2: string, done: () => void) {
    return (
      <>
        <Logo /><h1>Verify your {kind2}</h1><p className="sub">Demo environment: use code <b>000000</b>.</p>
        <form onSubmit={(e) => submit(e, (fd) => { String(fd.get("code")) === "000000" ? done() : toast("Wrong code — demo code is 000000"); })}>
          <div className="field"><label>6-digit code</label><input name="code" inputMode="numeric" pattern="[0-9]{6}" maxLength={6} required /></div>
          <button className="btn btn-primary btn-lg" style={{ width: "100%", justifyContent: "center", marginTop: 20 }} type="submit">Verify</button>
        </form>
      </>
    );
  }
}

function Onboarding() {
  const router = useRouter(); const toast = useToast();
  const [step, setStep] = useState(0);
  const [name, setName] = useState(""); const [email, setEmail] = useState("");
  const [interests, setInterests] = useState<string[]>([]);
  const [time, setTime] = useState("06:00"); const [len, setLen] = useState("10");
  const [remind, setRemind] = useState(true);

  const account = (e: React.FormEvent) => {
    e.preventDefault();
    const f = e.target as HTMLFormElement;
    if (!f.reportValidity()) return;
    setStep(1);
  };
  const verifyCode = (e: React.FormEvent) => {
    e.preventDefault();
    const fd = new FormData(e.target as HTMLFormElement);
    if (String(fd.get("code")) === "000000") setStep(2); else toast("Wrong code — demo code is 000000");
  };
  const finish = () => {
    const cats = interests.slice(0, 3);
    mutate((s) => {
      s.user = { name: name.trim(), email: email.trim(), plan: "free", interests };
      s.routines = [{ slot: time, cats }];
      s.schedule = cats.map((c, i) => ({ id: Date.now() + i, category: c, time, days: "Every day" }));
      s.settings.notifications.reminders = remind;
    });
    track("account_created"); track("onboarding_complete", { interests: interests.length });
    toast("Your practice is set up");
    router.push("/app");
  };

  const steps = ["Account", "Verify", "Interests", "Practice"];
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
            <div className="field"><label>Password</label><input type="password" required minLength={6} autoComplete="new-password" /><div className="err">At least 6 characters.</div></div>
            <label style={{ display: "flex", gap: 10, alignItems: "flex-start", marginTop: 16, fontSize: 12.5, color: "var(--n600)" }}><input type="checkbox" required style={{ width: "auto", marginTop: 2 }} /> I consent to product analytics and understand my private content is never reviewed unless I submit it.</label>
            <button className="btn btn-primary btn-lg" style={{ width: "100%", justifyContent: "center", marginTop: 20 }} type="submit">Continue</button>
          </form>
          <p className="auth-foot">Already have an account? <Link href="/login">Log in</Link></p>
        </>
      )}
      {step === 1 && (
        <>
          <h1>Verify your email</h1><p className="sub">We sent a 6-digit code to <b>{email}</b>. Demo environment: use <b>000000</b>.</p>
          <form onSubmit={verifyCode}>
            <div className="field"><label>6-digit code</label><input name="code" inputMode="numeric" pattern="[0-9]{6}" maxLength={6} required autoFocus /></div>
            <div style={{ display: "flex", gap: 10, marginTop: 20 }}>
              <button className="btn btn-ghost btn-lg" type="button" onClick={() => setStep(0)} style={{ flex: 1, justifyContent: "center" }}>Back</button>
              <button className="btn btn-primary btn-lg" type="submit" style={{ flex: 2, justifyContent: "center" }}>Verify</button>
            </div>
          </form>
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
            <button className="btn btn-ghost btn-lg" type="button" onClick={() => setStep(1)} style={{ flex: 1, justifyContent: "center" }}>Back</button>
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
            <button className="btn btn-primary btn-lg" type="button" onClick={finish} style={{ flex: 2, justifyContent: "center" }}>Finish setup</button>
          </div>
        </>
      )}
    </>
  );
}
