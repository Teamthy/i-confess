"use client";
import React, { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Icon } from "./ui";
import { useAudio, useToast, useSearch } from "@/lib/ui";
import { useApp, mutate, track } from "@/lib/store";
import { confBySlug, queueItem, sessionBySlug, buildQueue, catBySlug, CONFESSIONS, type Confession, type Category } from "@/lib/data";

export function HeroCtas() {
  return (
    <>
      <Link className="btn btn-primary btn-lg" href="/register" onClick={() => track("hero_clicked")}>Start Your Experience</Link>
      <Link className="btn btn-ghost btn-lg" href="/explore">Explore iCONFESS</Link>
    </>
  );
}
export function PlayBtn({ slug, small, big, label = "Play" }: { slug: string; small?: boolean; big?: boolean; label?: string }) {
  const audio = useAudio(); const toast = useToast();
  const c = confBySlug(slug); if (!c) return null;
  return (
    <button className={"btn btn-primary" + (small ? " btn-sm" : big ? " btn-lg" : "")} onClick={() => { audio.play([queueItem(c, "long")]); toast("Playing — " + c.title); }}>
      <Icon n="play" s={small ? 12 : 14} /> {label}
    </button>
  );
}
export function PlayChip({ slug }: { slug: string }) {
  const audio = useAudio(); const c = confBySlug(slug); if (!c) return null;
  return <button className="play-chip" aria-label={"Play " + c.title} onClick={() => audio.play([queueItem(c, "long")])}><Icon n="play" s={13} /></button>;
}
export function PlaySessionBtn({ slug, big }: { slug: string; big?: boolean }) {
  const audio = useAudio(); const router = useRouter();
  return (
    <button className={"btn btn-light" + (big ? " btn-lg" : "")} onClick={() => {
      const s = sessionBySlug(slug); if (!s) return;
      audio.play(buildQueue(s)); track("session_started", { slug }); router.push("/app/player");
    }}><Icon n="play" s={14} /> Start Session</button>
  );
}
export function FavBtn({ slug, ghost }: { slug: string; ghost?: boolean }) {
  const st = useApp(); const toast = useToast();
  const fav = st.favs.includes(slug);
  return (
    <button className={ghost ? "btn btn-ghost" : "icon-btn"} style={!ghost ? { width: 32, height: 32 } : undefined} aria-pressed={fav}
      aria-label={fav ? "Remove from favorites" : "Save to favorites"}
      onClick={() => { mutate((s) => { s.favs = fav ? s.favs.filter((x) => x !== slug) : [...s.favs, slug]; }); track("content_saved", { slug }); toast(fav ? "Removed from favorites" : "Saved to favorites"); }}>
      <Icon n="heart" s={ghost ? 14 : 15} />{ghost && " Save"}
    </button>
  );
}
export function ShareBtn({ slug }: { slug: string }) {
  const toast = useToast();
  return (
    <button className="btn btn-ghost" onClick={() => {
      const token = "sh-" + slug;
      mutate((s) => { s.shared[token] = { slug, at: Date.now() }; });
      track("content_shared", { token });
      toast("Share link created — " + location.origin + "/app/shared/" + token);
    }}><Icon n="share" s={14} /> Share</button>
  );
}
export function VoicePreviewBtn({ light }: { light?: boolean }) {
  const audio = useAudio(); const toast = useToast();
  return (
    <button className={"btn " + (light ? "btn-light" : "btn-primary")} onClick={() => {
      track("voice_played", { voice: "grace" });
      audio.play([{ slug: "voice-preview", title: "Grace — voice preview", category: "peace", text: "Peace beyond understanding. Speak it, hear it, and let the words return to you." }]);
      toast("Playing voice preview — Grace");
    }}><Icon n="play" s={13} /> Preview voice</button>
  );
}
export function ConfTile({ c }: { c: Confession }) {
  return (
    <div className="tile rv in">
      <div className="t-top"><span className="t-meta">{c.category}</span><FavBtn slug={c.slug} /></div>
      <h4>{c.title}</h4>
      <p>{c.short}</p>
      <div style={{ display: "flex", gap: 8, marginTop: 6 }}>
        <PlayBtn slug={c.slug} small />
        <a className="btn btn-ghost btn-sm" href={`/confessions/${c.slug}`}>Read</a>
      </div>
    </div>
  );
}
export function ConfessionFilter() {
  const cats = [...new Set(CONFESSIONS.map((c) => c.category))];
  const [f, setF] = useState("All");
  const list = f === "All" ? CONFESSIONS : CONFESSIONS.filter((c) => c.category === f);
  return (
    <>
      <div className="chip-row" style={{ marginBottom: 28 }}>
        {["All", ...cats.slice(0, 12)].map((c) => (
          <button key={c} className="chip" aria-pressed={f === c} onClick={() => setF(c)}>{c}</button>
        ))}
      </div>
      <div className="grid4">{list.map((c) => <ConfTile key={c.slug} c={c} />)}</div>
    </>
  );
}
export function FaqList({ items }: { items: [string, string][] }) {
  const [open, setOpen] = useState<number | null>(null);
  return (
    <div>
      {items.map(([q, a], i) => (
        <div className={"faq-item" + (open === i ? " open" : "")} key={i}>
          <button className="faq-q" aria-expanded={open === i} onClick={() => setOpen(open === i ? null : i)}>
            {q}<span className="fx"><Icon n={open === i ? "minus" : "plus"} s={14} /></span>
          </button>
          <div className="faq-a" style={{ maxHeight: open === i ? 240 : 0 }}><p>{a}</p></div>
        </div>
      ))}
    </div>
  );
}
export function SearchBtn({ pill, children }: { pill?: boolean; children?: React.ReactNode }) {
  const open = useSearch();
  return <button className={pill ? "pill" : "icon-btn bare"} onClick={open} aria-label="Search">{children || <Icon n="search" s={16} />}</button>;
}
export function RailScroll({ target }: { target: string }) {
  const go = (d: number) => document.getElementById(target)?.scrollBy({ left: d, behavior: "smooth" });
  return (
    <div style={{ display: "flex", gap: 8 }}>
      <button className="icon-btn" aria-label="Scroll left" onClick={() => go(-320)}><Icon n="arrowL" s={16} /></button>
      <button className="icon-btn" aria-label="Scroll right" onClick={() => go(320)}><Icon n="arrow" s={16} /></button>
    </div>
  );
}
export function NewsletterForm() {
  const toast = useToast();
  return (
    <form className="news-form" onSubmit={(e) => { e.preventDefault(); const f = e.target as HTMLFormElement; if (!f.reportValidity()) return; track("newsletter_subscribed"); toast("Subscribed — one email when new words land."); f.reset(); }}>
      <input type="email" required placeholder="Your email address" aria-label="Email address" />
      <button className="btn btn-blue" type="submit">Subscribe</button>
    </form>
  );
}
export function ContactForm() {
  const toast = useToast();
  return (
    <form className="form-card rv in" style={{ maxWidth: 640 }} onSubmit={(e) => { e.preventDefault(); if (!(e.target as HTMLFormElement).reportValidity()) return; toast("Message sent — we read everything."); (e.target as HTMLFormElement).reset(); }}>
      <div className="field"><label>Your name</label><input required /></div>
      <div className="field"><label>Email</label><input type="email" required /></div>
      <div className="field"><label>Message</label><textarea rows={5} required /></div>
      <button className="btn btn-primary" style={{ marginTop: 18 }} type="submit">Send message</button>
    </form>
  );
}

export function StoryTabs() {
  const TABS = [
    "iCONFESS is built by people who keep a practice themselves — curated words, licensed voices, and a library checked line by line before you ever see it.",
    "Every confession is drafted, edited and verified against the Scripture it claims to stand on. Nothing publishes automatically — not ever.",
    "The design is quiet on purpose: no streak guilt, no autoplay, no noise. The loudest thing in iCONFESS should be the words themselves.",
  ];
  const [i, setI] = useState(0);
  return (
    <div>
      <div className="tabs" role="tablist">{["01", "02", "03"].map((t, k) => <button key={t} role="tab" aria-selected={i === k} onClick={() => setI(k)}>{t}</button>)}</div>
      <p style={{ fontSize: 15, color: "var(--n600)", lineHeight: 1.7 }}>{TABS[i]}</p>
    </div>
  );
}

export function ConfDetailTabs({ c, cat }: { c: Confession; cat: Category | undefined }) {
  const [tab, setTab] = useState<"text" | "scripture" | "use">("text");
  const [tr, setTr] = useState<Record<string, string>>({});
  return (
    <>
      <div className="tabrow" role="tablist" aria-label="Confession detail">
        {([["text", "The words"], ["scripture", "Scripture check"], ["use", "How to use it"]] as const).map(([k, l]) => (
          <button key={k} role="tab" aria-selected={tab === k} className={"tabbtn" + (tab === k ? " on" : "")} onClick={() => setTab(k)}>{l}</button>
        ))}
      </div>
      <div className="form-card rv in" style={{ marginTop: 22 }}>
        {tab === "text" && (
          <div className="prose">
            <p className="lede" style={{ fontSize: 20 }}>{c.medium}</p>
            <p>{c.long}</p>
            <p className="small">Say it at speaking volume, in a room where no one is performing for you. The point is not volume — it is that the sentence has to survive your own voice.</p>
          </div>
        )}
        {tab === "scripture" && (
          <div>
            <p className="lede" style={{ marginBottom: 18 }}>Every quotation here was checked against the text it cites before this page published.</p>
            <div className="field" style={{ marginBottom: 14 }}><label>Cite scriptures as</label>
              <div className="chip-row">{(Array.from(new Set([c.scriptures[0]?.translation || "WEB", "KJV", "WEB", "ESV", "NLT"]))).map((t) => <button className="chip" key={t} aria-pressed={(tr.all || c.scriptures[0]?.translation) === t} onClick={() => setTr({ all: t })}>{t}</button>)}</div>
              <p className="small" style={{ marginTop: 8 }}>We show references, not verse bodies — your Bible app opens the full text in the translation you choose. Quoted/allusion labels never change.</p></div>
            {c.scriptures.map((sc, i) => (
              <div className="list-row" key={i}>
                <div className="lr-main"><h4>{sc.book} {sc.chapter}:{sc.verse}</h4><p>{tr.all || sc.translation} · {sc.direct ? "quoted directly" : "referenced, not quoted"}</p></div>
                <span className={"tag " + (sc.direct ? "public" : "private")}>{sc.direct ? "Quoted" : "Allusion"}</span>
              </div>
            ))}
            <p className="small" style={{ marginTop: 16 }}>If a line alludes rather than quotes, we label it that way. Nothing is dressed up as Scripture that is not.</p>
          </div>
        )}
        {tab === "use" && (
          <div className="prose">
            <p>Place this confession in a routine slot you already keep — a morning you do not have to negotiate for. Five minutes is a complete practice: the words, their Scripture, a voice, and a moment of stillness at the end.</p>
            <div style={{ display: "flex", gap: 12, marginTop: 18, flexWrap: "wrap" }}>
              <Link className="btn btn-primary" href={`/app/session-builder?cat=${encodeURIComponent(c.category)}`}>Build a session around it</Link>
              <Link className="btn btn-ghost" href="/app/routines">Add to my routine</Link>
            </div>
          </div>
        )}
      </div>
      {cat && <p className="small" style={{ marginTop: 14 }}>Part of the {cat.name} library — {cat.tagline}</p>}
    </>
  );
}

export function ReviewLog() {
  const st = useApp();
  const published = CONFESSIONS.length;
  const quoted = CONFESSIONS.reduce((a, c) => a + c.scriptures.filter((x) => x.direct).length, 0);
  const allusions = CONFESSIONS.reduce((a, c) => a + c.scriptures.filter((x) => !x.direct).length, 0);
  return (
    <section className="section container" style={{ paddingTop: "calc(var(--header-h) + 56px)" }}>
      <div className="c-head rv in">
        <span className="eyebrow">Review log</span>
        <h1 className="h-display" style={{ marginTop: 18 }}>Nothing publishes <span className="accent">automatically.</span></h1>
        <p className="lede c-lede">The transparency ledger for the library: every published confession passed a human review, and every Scripture line is labelled quoted or allusion.</p>
      </div>
      <div className="stats-strip rv in">
        <div><b>{published}</b><span>confessions published</span></div>
        <div><b>{published}</b><span>human-reviewed before publish</span></div>
        <div><b>{quoted}</b><span>scripture lines quoted directly</span></div>
        <div><b>{allusions}</b><span>labelled as allusion</span></div>
      </div>
      <div className="form-card rv in" style={{ marginTop: 24, maxWidth: 860 }}>
        <h3 className="h4" style={{ marginBottom: 12 }}>Your submissions</h3>
        {st.community.length ? st.community.map((c) => (
          <div className="list-row" key={c.slug}><div className="lr-main"><h4>{c.title}</h4><p>{new Date(c.at).toLocaleDateString()} · visibility {c.visibility}</p></div><span className={"tag " + (c.status === "pending_review" || c.status === "mentor_review" ? "pending" : c.status === "public" ? "public" : "private")}>{c.status.replace(/_/g, " ")}</span></div>
        )) : <p className="small">You haven't submitted anything yet. When you do, every decision — approved or declined with a reason — is recorded here.</p>}
      </div>
      <div className="form-card rv in" style={{ marginTop: 16, maxWidth: 860 }}>
        <h3 className="h4" style={{ marginBottom: 12 }}>How a decision is recorded</h3>
        <div className="prose"><p>Draft → review → published is one-way and enforced by the platform. A reviewer reads each submission, approves or declines with a written reason, and the decision is stored with a timestamp. Withdrawn submissions disappear from the public surface immediately.</p></div>
      </div>
    </section>
  );
}

export function MixBtn({ cats, label = "Play mix", big }: { cats: string[]; label?: string; big?: boolean }) {
  const audio = useAudio(); const toast = useToast();
  return (
    <button className={"btn btn-primary" + (big ? " btn-lg" : "")} onClick={() => {
      const q = cats.map((sl) => CONFESSIONS.filter((c) => c.category === (catBySlug(sl)?.name || "")).slice(0, 1)).flat().map((c) => queueItem(c));
      if (!q.length) { toast("Nothing in this mix yet"); return; }
      audio.play(q); toast("Mix queued — " + q.length + " confessions");
    }}>{label}</button>
  );
}

export function RadioBtn({ slug }: { slug: string }) {
  const audio = useAudio(); const toast = useToast();
  return (
    <button className="btn btn-ghost" onClick={() => {
      const base = confBySlug(slug); if (!base) return;
      const same = CONFESSIONS.filter((c) => c.category === base.category && c.slug !== slug);
      const near = CONFESSIONS.filter((c) => c.category !== base.category && Math.abs(c.intensity - base.intensity) <= 1 && c.slug !== slug);
      const q = [base, ...same, ...near].slice(0, 8).map((c) => queueItem(c));
      audio.play(q); toast("Radio started from “" + base.title + "”");
    }}><svg className="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true" style={{ width: 14, height: 14 }}><circle cx="12" cy="12" r="2" /><path d="M7.8 16.2a6 6 0 0 1 0-8.4M16.2 7.8a6 6 0 0 1 0 8.4M4.9 19.1a10 10 0 0 1 0-14.2M19.1 4.9a10 10 0 0 1 0 14.2" /></svg> Start radio</button>
  );
}

export function SnippetBtn({ slug }: { slug: string }) {
  const toast = useToast();
  return (
    <button className="btn btn-ghost" onClick={() => {
      const url = location.origin + "/confessions/" + slug + "?sn=1";
      (navigator.clipboard?.writeText(url) || Promise.reject()).then(() => toast("15-second audio card link copied")).catch(() => toast(url));
    }}>Share 15s card</button>
  );
}

export function Annotations({ slug }: { slug: string }) {
  const st = useApp(); const toast = useToast();
  const [val, setVal] = useState(st.annotations[slug] || "");
  return (
    <div className="form-card rv in" style={{ marginTop: 22 }}>
      <h3 className="h4" style={{ marginBottom: 8 }}>Private annotation</h3>
      <p className="small" style={{ marginBottom: 10 }}>A note pinned to this confession. Only you can ever see it.</p>
      <textarea rows={3} value={val} onChange={(e) => setVal(e.target.value)} placeholder="What does this line meet in you?" style={{ width: "100%" }} />
      <button className="btn btn-primary" style={{ marginTop: 10 }} onClick={() => { mutate((s) => { s.annotations[slug] = val; }); toast(val ? "Annotation saved" : "Annotation cleared"); }}>Save note</button>
    </div>
  );
}

export function SnippetAuto({ slug }: { slug: string }) {
  const audio = useAudio();
  React.useEffect(() => {
    if (typeof window === "undefined") return;
    if (new URLSearchParams(window.location.search).get("sn") !== "1") return;
    const c = confBySlug(slug); if (!c) return;
    audio.play([queueItem(c, "short")]);
    const t = setTimeout(() => audio.stop(), 15000);
    return () => clearTimeout(t);
  }, [slug]);
  return null;
}
