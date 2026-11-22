import React from "react";
import Link from "next/link";
import { Icon, SessionTile, ArticleCard, SectionHead, PhoneMock, EmptyState, Logo, Mark } from "./ui";
import { PlayChip, PlayBtn, PlaySessionBtn, FavBtn, ShareBtn, VoicePreviewBtn, ConfTile, ConfessionFilter, ConfDetailTabs, FaqList, NewsletterForm, ContactForm, RailScroll, SearchBtn, HeroCtas, AuthLink, ReviewLog, RadioBtn, SnippetBtn, Annotations, SnippetAuto } from "./islands";
import { CATEGORIES, CONFESSIONS, ARTICLES, VOICES, PLANS, FAQS, catBySlug, confBySlug, artBySlug, sessionsFor, ALL_SESSIONS, sessionBySlug, motifStyle, CAT_IMAGES, confessionsFor } from "@/lib/data";
import { ART_W, ART_H, shapeCentre, shapeForIndex, shapePaths } from "@/lib/card-shapes";
import { MARK_PATH, MARK_VIEWBOX } from "@/lib/brand-mark";
import type { Category } from "@/lib/data";
import { CANON } from "@/lib/canon";
import { VoiceLibrary } from "@/components/voices";

/* The canon totals on the home page come from the generated canon, never from
   typed numbers — a structure change upstream has to move this copy too. */
const CANON_CHAPTERS = CANON.chapter_count;
const CANON_VERSES = CANON.verse_count;

/* ================= HOME ================= */
export function Home() {
  return (
    <>
      {/* 01 DARK HERO */}
      <section className="dhero">
        <div className="dhero-bg" aria-hidden="true">
          <video className="chrome" autoPlay muted loop playsInline src="/assets/hero-media.mp4" />
          <div className="veil" />
        </div>
        <span className="badge"><i aria-hidden="true" /> Designed for daily practice</span>
        <h1>Confessions<br />for the <span style={{ color: "var(--tint1)" }}>real you.</span></h1>
        <p className="sub">Speak the truth about yourself, receive the Word against it, and build a confession practice that becomes part of how you live.</p>
        <div className="dh-cta">
          <span className="btn-orb"><AuthLink className="btn btn-accent btn-lg" href="/app">Begin your practice</AuthLink><span className="orb" aria-hidden="true" style={{ background: "var(--blue2)", color: "#fff" }}><Icon n="ne" s={16} /></span></span>
          <Link className="btn btn-dim btn-lg" href="/explore">Explore the library</Link>
        </div>
        <div className="logo-row" aria-label="Brand promises">
          <span>Speak it.</span><span>Believe it.</span><span>Live it.</span><span>{CATEGORIES.length} categories</span><span>{CONFESSIONS.length} confessions</span>
        </div>
        <div className="scroll-cue" aria-hidden="true">
          <span className="mouse" />
          Scroll down
        </div>
      </section>

      {/* 03 WHO WE ARE — three pillars */}
      <section className="section container hm-intro">
        <div className="hm-intro-copy rv in">
          <span className="eyebrow">Who we are</span>
          <h2 className="h1">Words grow stronger when you <span className="accent">return</span> to them.</h2>
          <p className="lede">I CONFESS is a daily practice of spoken confession: short, Scripture-grounded declarations, read in a voice you trust, sized to the time you have.</p>
          <div className="hm-actions">
            <Link className="btn btn-primary" href="/about">About I CONFESS</Link>
            <Link className="btn btn-ghost" href="/how-it-works">How it works</Link>
          </div>
        </div>
        <ol className="hm-pillars rv in">
          <li>
            <span className="hm-num">01</span>
            <div><h3>Reviewed words</h3><p>{CONFESSIONS.length} confessions across {CATEGORIES.length} categories, each read by a person before it is published.</p></div>
          </li>
          <li>
            <span className="hm-num">02</span>
            <div><h3>A licensed voice</h3><p>Every narration voice is released only after its identity and usage rights are cleared.</p></div>
          </li>
          <li>
            <span className="hm-num">03</span>
            <div><h3>Sized to your day</h3><p>Five, ten or fifteen minutes, morning or evening — built from the areas you choose.</p></div>
          </li>
        </ol>
      </section>

      {/* 04 CATEGORIES — navy panel, shaped photo cards */}
      <section className="section" style={{ paddingBottom: 96 }}>
        <div className="container-wide">
          <CategoryPanel
            eyebrow="Categories"
            title={<>Every season has<br />a <span className="cp-accent">confession.</span></>}
            lede="Pick the ones that name where you are. The practice follows you there."
            action={<Link className="btn btn-light" href="/categories">All {CATEGORIES.length} categories <Icon n="arrow" s={14} /></Link>}
          >
            <div className="cp-row" role="list">{CATEGORIES.slice(0, 8).map((c) => <div role="listitem" key={c.slug}><TCard c={c} /></div>)}</div>
          </CategoryPanel>
        </div>
      </section>

      {/* 05 APPROACH — four commitments */}
      <section className="section" style={{ background: "var(--paper)" }}><div className="container">
        <div className="c-head rv in">
          <span className="eyebrow">Our approach</span>
          <h2 className="h2" style={{ marginTop: 18 }}>You speak, <span className="accent">we keep it honest,</span><br />you return.</h2>
          <p className="lede c-lede">Four commitments shape every session, and none of them is negotiable.</p>
        </div>
        <div className="hm-commit">
          {([
            ["compass", "Intention", "One area of life at a time. A session asks for a few honest minutes, not an hour of scrolling."],
            ["moon", "Presence", "Nothing autoplays and nothing pings. The loudest thing in I CONFESS is the words themselves."],
            ["clock", "Repetition", "A sentence said thirty times stops being an affirmation and starts being a description."],
            ["lock", "Privacy", "Your confessions are private by default and shared only when you choose to share them."],
          ] as const).map(([ic, t, d]) => (
            <article className="hm-commit-card rv in" key={t}>
              <span className="hm-chip"><Icon n={ic} s={20} /></span>
              <h3>{t}</h3>
              <p>{d}</p>
            </article>
          ))}
        </div>
      </div></section>

      {/* 06 LARGE IMMERSIVE IMAGE + CARD */}
      <section className="band rv in">
        <img src="/assets/immersive.jpg" alt="A person listening with headphones, eyes closed, in soft morning light" loading="lazy" />
        <div className="band-card">
          <div className="bc-top"><span className="eyebrow">Why I CONFESS</span></div>
          <h3 style={{ fontSize: "clamp(24px,2.6vw,32px)" }}>Five minutes or fifteen.<br /><mark>Sized to your day.</mark></h3>
          <div className="bc-foot">
            <p style={{ margin: 0 }}>Start from a category, a single confession or a ready-made session. No gimmicks, no streak guilt.</p>
            <Link className="btn btn-primary" href="/explore">Explore the library</Link>
          </div>
        </div>
      </section>

      {/* 07 VOICE & LICENSING */}
      <section className="section container hm-voice">
        <div className="hm-voice-copy rv in">
          <span className="eyebrow">Voice &amp; licensing</span>
          <h2 className="h1">A voice, built on <span className="accent">trust.</span></h2>
          <p className="lede">The voice you hear every morning should be one you can trust. That is why no voice is listed until it has been cleared.</p>
          <ul className="hm-checks">
            <li><Icon n="check" s={15} /> Identity confirmed with the speaker</li>
            <li><Icon n="check" s={15} /> Sample reviewed before release</li>
            <li><Icon n="check" s={15} /> Usage rights licensed in writing</li>
          </ul>
          <Link className="btn btn-primary" href="/voices">Explore the voice library</Link>
        </div>
        <div className="voice-feature-grid hm-voice-cards">
          <Link className="v-card highlight rv in" href="/voices/grace">
            <div className="v-top"><img className="avatar" src="/assets/voice-grace.jpg" alt="Portrait of Grace, I CONFESS narration voice" /></div>
            <div className="v-div"><div><h3>Grace</h3><span className="v-role">Warm · Calm · Licensed</span></div><span className="v-arrow"><Icon n="ne" s={15} /></span></div>
          </Link>
          <Link className="v-card voice-policy-card rv in" href="/voices">
            <div className="v-top"><span className="avatar-slot" style={{ width: 120, height: 120 }}><Icon n="shield" s={22} /></span></div>
            <div className="v-div"><div><h3>Rights come first</h3><span className="v-role">Review before release</span></div><span className="v-arrow"><Icon n="ne" s={15} /></span></div>
            <p className="v-desc">No placeholder listings. A voice appears only after its identity, sample and usage rights are cleared.</p>
          </Link>
        </div>
      </section>

      {/* 07c SCRIPTURE */}
      <section className="home-bible">
        <div className="hb-copy">
          <span className="eyebrow on-dark">Scripture</span>
          <h2 className="h1" style={{ marginTop: 18 }}>The whole Bible, inside the practice.</h2>
          <p className="lede" style={{ marginTop: 16 }}>
            {CANON.book_count} books, {CANON_CHAPTERS.toLocaleString()} chapters and{" "}
            {CANON_VERSES.toLocaleString()} verses — the reference distribution the reader draws from, open beside
            every confession on the platform.
          </p>
          <div className="hb-stats">
            <div><b>{CANON.book_count}</b><span>books</span></div>
            <div><b>{CANON_CHAPTERS.toLocaleString()}</b><span>chapters</span></div>
            <div><b>{CANON_VERSES.toLocaleString()}</b><span>verses</span></div>
            <div><b>7</b><span>highlight colours</span></div>
          </div>
          <div className="hb-actions">
            <Link className="btn btn-light" href="/bible">Read the Bible</Link>
            <Link className="btn btn-ghost on-dark" href="/bible/topics">Browse topics</Link>
          </div>
          <div className="hb-links">
            <Link href="/bible/search">Search</Link>
            <Link href="/bible/compare">Compare</Link>
            <Link href="/bible/plans">Plans</Link>
            <Link href="/bible/translations">Translations</Link>
          </div>
        </div>
        <div className="hb-preview" aria-hidden="true">
          <div className="hbp-card">
            <div className="hbp-bar">
              <span className="hbp-dots"><i /><i /><i /></span>
              <span>John 3 · KJV</span>
            </div>
            <div className="hbp-body">
              <p><sup>15</sup>That whosoever believeth…</p>
              <p className="hbp-mark"><sup>16</sup>For God so loved the world…</p>
              <p><sup>17</sup>For God sent not his Son…</p>
            </div>
            <div className="hbp-foot">
              <span className="hbp-chip">Highlight</span>
              <span className="hbp-chip">Note</span>
              <span className="hbp-chip">Compare</span>
              <span className="hbp-chip on">Confess from this verse</span>
            </div>
          </div>
        </div>
      </section>

      {/* 08 TRUSTED SPLIT */}
      <section className="split-full rv in">
        <div className="split-img"><img src="/assets/community.jpg" alt="Two friends in meaningful conversation" loading="lazy" /></div>
        <div className="split-card">
          <div className="sc-top"><span className="eyebrow on-dark">Your own words</span></div>
          <h3 style={{ marginTop: 28 }}>Private by default.<br />Reviewed by people.</h3>
          <ul className="hm-share">
            <li><span><Icon n="lock" s={16} /></span><div><b>Private</b>Only you can hear it. This is where every confession starts.</div></li>
            <li><span><Icon n="user" s={16} /></span><div><b>Shared</b>Send it to one person you choose, by link.</div></li>
            <li><span><Icon n="heart" s={16} /></span><div><b>Community</b>Offered to everyone — after a person reviews it.</div></li>
          </ul>
          <div className="sc-foot">
            <AuthLink className="btn btn-light" href="/app/community/create">Write your confession</AuthLink>
            <Link className="btn btn-ghost on-dark" href="/community">How sharing works</Link>
          </div>
        </div>
      </section>

      {/* 09 OUR PROMISE */}
      <section className="section container hm-promise">
        <figure className="hm-quote rv in">
          <span className="hm-qmark" aria-hidden="true">“</span>
          <blockquote>I CONFESS is built for a few honest minutes a day. No streak guilt, no noise, no autoplay — just words you chose, in a voice you trust, returned to until they become yours.</blockquote>
          <figcaption><Mark s={28} /> <span><b>Our promise</b> · The I CONFESS team</span></figcaption>
        </figure>
        <div className="hm-promise-side rv in">
          <div><b>{CONFESSIONS.length}</b><span>reviewed confessions</span></div>
          <div><b>{CATEGORIES.length}</b><span>categories</span></div>
          <div><b>0</b><span>ads or autoplay</span></div>
          <Link className="btn btn-primary" href="/how-it-works">See how a session works</Link>
        </div>
      </section>

      {/* 10 CATEGORY MARQUEE */}
      <div className="marquee" aria-hidden="true"><div className="marquee-in">{[...CATEGORIES, ...CATEGORIES].map((c, i) => <span key={i}>{c.name}</span>)}</div></div>

      {/* 11 JOURNAL */}
      <section className="section band-gray"><div className="container-wide">
        <div className="c-head rv in">
          <span className="eyebrow">Journal</span>
          <h2 className="h1" style={{ marginTop: 18 }}>Hear <span className="accent">directly</span><br />from I CONFESS.</h2>
          <Link className="btn btn-primary" href="/journal" style={{ marginTop: 26 }}>Read the journal</Link>
        </div>
        <div className="journal-grid snap" style={{ gridTemplateColumns: "repeat(3,1fr)" }}>
          <ArticleCard a={ARTICLES[0]} img="journal-1.jpg" />
          <Link className="j-card dark rv in" href="/download">
            <div className="jd-title">The I CONFESS app — carry the practice with you.</div>
            <div className="jd-blob" aria-hidden="true" />
            <PhoneMock />
            <div className="jd-meta"><Icon n="ast" s={14} /> 2026</div>
            <span className="j-arrow"><Icon n="ne" s={15} /></span>
          </Link>
          <ArticleCard a={ARTICLES[2]} img="cat-faith.jpg" />
        </div>
      </div></section>

      {/* 12 FAQ */}
      <section className="section" style={{ background: "var(--paper)" }}><div className="container faq">
        <div className="faq-left rv in" style={{ display: "flex", flexDirection: "column", alignItems: "flex-start" }}>
          <span className="eyebrow">FAQ</span>
          <h2 className="h1" style={{ marginTop: 22 }}>Questions about<br />I CONFESS?</h2>
          <p className="lede" style={{ marginTop: 16, fontSize: 14 }}>Common questions on confessions, sessions, voices and privacy.</p>
          <div className="hm-faq-actions"><Link className="btn btn-primary" href="/faq">All questions</Link><Link className="btn btn-ghost" href="/contact">Contact us</Link></div>
        </div>
        <div className="rv in"><FaqList items={FAQS.slice(0, 4)} /></div>
      </div></section>

      {/* 13 NEWSLETTER */}
      <section className="news-full"><div className="news rv in">
        <div className="news-left">
          <span className="ast"><Icon n="ast" s={18} /></span>
          <h2>Latest words,<br />when they're ready.</h2>
          <p>One email when a new category, voice or essay lands. Never more, never sold.</p>
          <NewsletterForm />
        </div>
        <div className="news-right"><img src="/assets/newsletter.jpg" alt="A journal and phone on a quiet desk" loading="lazy" /></div>
      </div></section>

      {/* 14 FINAL CTA */}
      <section className="final-full">
        <div className="rv in" style={{ display: "flex", flexDirection: "column", alignItems: "center", textAlign: "center" }}>
          <Logo />
          <h2 className="h1" style={{ marginTop: 26 }}>Begin with <span className="accent">five minutes</span><br />tomorrow morning.</h2>
          <p className="final-cap">Free for the core practice. Premium adds depth when you want it.</p>
          <div style={{ marginTop: 34 }} className="hm-actions">
            <AuthLink className="btn btn-primary btn-lg" href="/app">Start your practice</AuthLink>
            <Link className="btn btn-ghost btn-lg" href="/pricing">See plans</Link>
          </div>
        </div>
        <div className="rv in">
          <h4 style={{ fontSize: 11, letterSpacing: ".14em", textTransform: "uppercase", color: "var(--n500)" }}>Contact us</h4>
          <div className="contact-grid">
            <div><div className="cg-l">Email</div><b><a href="mailto:hello@iconfess.app" style={{ color: "inherit" }}>hello@iconfess.app</a></b></div>
            <div><div className="cg-l">Support</div><b><Link href="/help" style={{ color: "inherit" }}>Help Center</Link></b></div>
            <div><div className="cg-l">Write to us</div><b><Link href="/contact" style={{ color: "inherit" }}>Contact form</Link></b></div>
          </div>
        </div>
      </section>
    </>
  );
}

/** The navy panel category cards sit on: eyebrow pill, heading, one action. */
export function CategoryPanel({ eyebrow, title, lede, action, children, as: H = "h2" }: { eyebrow: string; title: React.ReactNode; lede?: string; action?: React.ReactNode; children: React.ReactNode; as?: "h1" | "h2" }) {
  return (
    <div className="cat-panel rv in">
      <div className="cp-head">
        <div>
          <span className="cp-eyebrow">{eyebrow}</span>
          <H className="h1 cp-title">{title}</H>
          {lede && <p className="cp-lede">{lede}</p>}
        </div>
        {action}
      </div>
      {children}
    </div>
  );
}

/** Category card: the photo seen through a shaped window with a halo band,
 *  title bottom-left and a round arrow bottom-right. The shape is fixed per
 *  category (by catalogue position), so a category looks the same everywhere. */
export function TCard({ c }: { c: Category }) {
  const n = confessionsFor(c).length;
  const shape = shapeForIndex(Math.max(0, CATEGORIES.findIndex((x) => x.slug === c.slug)));
  const paths = shapePaths(shape);
  const img = CAT_IMAGES[c.slug];
  return (
    <Link href={"/categories/" + c.slug} className="tcard" aria-label={`${c.name} — ${n} confession${n === 1 ? "" : "s"}`}>
      <span className="t-art">
        <svg viewBox={`0 0 ${ART_W} ${ART_H}`} preserveAspectRatio="xMidYMin slice" aria-hidden="true" focusable="false">
          <g className="t-halo">{paths.map((d, k) => <path key={k} d={d} />)}</g>
          <g className="t-fill">{paths.map((d, k) => <path key={k} d={d} />)}</g>
          {img ? (
            <image className="t-img" href={`/assets/${img}`} x="0" y="0" width={ART_W} height={ART_H} preserveAspectRatio="xMidYMid slice" clipPath={`url(#tshape-${shape})`} />
          ) : (
            <g clipPath={`url(#tshape-${shape})`}>
              <rect width={ART_W} height={ART_H} fill="url(#tshape-motif)" />
              <svg x={shapeCentre(shape)[0] - 28} y={shapeCentre(shape)[1] - 28} width="56" height="56" viewBox={MARK_VIEWBOX}><path fill="rgba(255,255,255,.9)" fillRule="evenodd" d={MARK_PATH} /></svg>
            </g>
          )}
        </svg>
      </span>
      <span className="t-foot">
        <span className="t-text">
          <h3>{c.name}</h3>
          <span className="t-meta">{n} confession{n === 1 ? "" : "s"}</span>
        </span>
        <span className="t-orb" aria-hidden="true"><Icon n="ne" s={16} /></span>
      </span>
    </Link>
  );
}

function PageStats({ items }: { items: [string, string][] }) {
  return (
    <div className="stats-strip rv in">
      {items.map(([v, l]) => <div key={l}><b>{v}</b><span>{l}</span></div>)}
    </div>
  );
}
function CtaBand({ title, cta = "Begin your practice", href = "/register" }: { title: string; cta?: string; href?: string }) {
  return (
    <div className="cta-band rv in">
      <h3 className="h2">{title}</h3>
      <span className="btn-orb" style={{ position: "relative", zIndex: 1 }}>
        <Link className="btn btn-light btn-lg" href={href}>{cta}</Link>
        <Link className="orb" href={href} aria-label={cta} style={{ background: "#fff", color: "var(--deep)" }}><Icon n="ne" s={15} /></Link>
      </span>
    </div>
  );
}

const catBySlugCat = (name: string) => CATEGORIES.find((x) => x.name === name)?.slug || "";
const HEAVY = ["overcoming-fear", "emotional-strength", "forgiveness", "hope", "rest"];
function CareNote() {
  return (
    <div className="care-note rv in">
      <span className="b-ic"><Icon n="shield" s={16} /></span>
      <div><b>Some seasons are heavy.</b><p>If this is more than a theme right now, a human voice helps. <a href="https://findahelpline.com" target="_blank" rel="noopener noreferrer">Find a free, confidential helpline</a> — these words are a practice, not a substitute for care.</p></div>
    </div>
  );
}
export function ReviewLogPage() { return <ReviewLog />; }

/* ================= OTHER MARKETING PAGES ================= */
export function Explore() {
  return (
    <section className="section container" style={{ paddingTop: "calc(var(--header-h) + 56px)" }}>
      <SectionHead as="h1" eyebrow="Explore" title={<>What do you want to <span className="accent">return to</span> today?</>} right={<SearchBtn pill><Icon n="search" s={13} /> Search</SearchBtn>} />
      <div className="app-section"><div className="section-head"><h3 className="h3">Categories</h3><Link className="textlink" href="/categories">All 39 <Icon n="arrow" s={12} /></Link></div>
        <div className="tgrid snap">{CATEGORIES.slice(0, 8).map((c) => <TCard key={c.slug} c={c} />)}</div></div>
      <div className="app-section"><div className="section-head"><h3 className="h3">Confessions</h3><Link className="textlink" href="/confessions">Library <Icon n="arrow" s={12} /></Link></div>
        <div className="grid4 snap">{CONFESSIONS.slice(0, 4).map((c) => <ConfTile key={c.slug} c={c} />)}</div></div>
      <div className="app-section"><div className="section-head"><h3 className="h3">Sessions</h3><Link className="textlink" href="/sessions">All sessions <Icon n="arrow" s={12} /></Link></div>
        <div className="card-row three snap">{ALL_SESSIONS.slice(0, 3).map((s) => <SessionTile key={s.slug} s={s} />)}</div></div>
      <div className="app-section"><div className="section-head"><h3 className="h3">Voice &amp; licensing</h3><Link className="textlink" href="/voices">Voice library <Icon n="arrow" s={12} /></Link></div>
        <div className="big-tile"><div><h3>Grace — warm, calm, licensed.</h3><p>Grace is the licensed voice listed today. Any future voice will be named only after its identity, sample and usage rights are approved.</p></div><Link className="btn btn-light" href="/voices">Explore the library</Link></div></div>
      <PageStats items={[[String(CATEGORIES.length), "categories"], [String(CONFESSIONS.length), "reviewed confessions"], [String(ALL_SESSIONS.length), "ready sessions"], ["Rights", "reviewed before release"]]} />
      <CtaBand title="Not sure where to start? Begin with one honest sentence." />
    </section>
  );
}

export function Categories() {
  return (
    <section className="section container-wide" style={{ paddingTop: "calc(var(--header-h) + 56px)" }}>
      <CategoryPanel as="h1"
        eyebrow="Categories"
        title={<>{CATEGORIES.length} areas of life. <span className="cp-accent">One practice.</span></>}
        lede="Every category is a reviewed library of confessions. Order leads with the areas people reach for first in trouble."
        action={<Link className="btn btn-light" href="/register">Choose mine <Icon n="arrow" s={14} /></Link>}
      >
        <div className="tgrid">{CATEGORIES.map((c) => <TCard key={c.slug} c={c} />)}</div>
      </CategoryPanel>
      <PageStats items={[[String(CATEGORIES.length), "areas of life"], [String(CONFESSIONS.length), "confessions in the library"], ["Every one", "reviewed before publishing"], ["1–15 min", "sessions to fit your day"]]} />
      <div className="form-card rv in" style={{ marginTop: 48, maxWidth: 820 }}>
        <h3 className="h4" style={{ marginBottom: 12 }}>How to choose a category</h3>
        <div className="prose"><p>Start with the one that names the thing you would not say out loud in a group. That is usually the honest entry point. Pick up to three at registration — your routine and recommendations build around them, and you can change them any time in Settings.</p></div>
        <div style={{ marginTop: 16 }}><Link className="btn btn-primary" href="/register">Choose mine</Link></div>
      </div>
      <CtaBand title="Pick the season you are in. The words are already written." />
    </section>
  );
}

export function CategoryDetail({ slug }: { slug: string }) {
  const cat = catBySlug(slug); if (!cat) return <NotFoundBody />;
  const confs = CONFESSIONS.filter((c) => c.category === cat.name);
  const s5 = sessionsFor(cat.slug)[0];
  const related = CATEGORIES.filter((c) => c.slug !== cat.slug).slice(0, 4);
  return (
    <section className="container-wide" style={{ paddingTop: "calc(var(--header-h) + 40px)", paddingBottom: 96 }}>
      <div className="dark-block cd-hero rv in">
        {CAT_IMAGES[cat.slug] ? (
          <img className="cd-bg" src={`/assets/${CAT_IMAGES[cat.slug]}`} alt="" />
        ) : (
          <div className="cd-bg motif" style={motifStyle(cat.slug)} aria-hidden="true" />
        )}
        <div className="cd-veil" aria-hidden="true" />
        <div className="cd-in">
          <span className="eyebrow on-dark">Category</span>
          <h1 className="h-display" style={{ marginTop: 14 }}>{cat.name}</h1>
          <p className="lede" style={{ color: "var(--tint1)", marginTop: 14, maxWidth: "52ch" }}>{cat.tagline} {cat.description}</p>
          <div style={{ display: "flex", gap: 12, marginTop: 28, flexWrap: "wrap" }}>
            <PlaySessionBtn slug={s5.slug} big />
            <Link className="btn btn-ghost on-dark" href={`/app/session-builder?cat=${cat.slug}`}>Build a session</Link>
          </div>
        </div>
      </div>
      {HEAVY.includes(cat.slug) && <div className="container" style={{ marginTop: 24 }}><CareNote /></div>}
      <div className="app-section" style={{ marginTop: 64 }}>
        <div className="section-head"><h3 className="h3">Featured in {cat.name}</h3></div>
        {confs.length ? <div className="grid4 snap">{confs.map((c) => <ConfTile key={c.slug} c={c} />)}</div> : <EmptyState icon="book" title="Library in review" sub={`New ${cat.name} confessions are with the editorial team.`} />}
      </div>
      <div className="app-section"><div className="section-head"><h3 className="h3">Related categories</h3></div>
        <div className="tgrid snap">{related.map((c) => <TCard key={c.slug} c={c} />)}</div></div>
    </section>
  );
}

export function Confessions() {
  return (
    <section className="section container-wide" style={{ paddingTop: "calc(var(--header-h) + 56px)" }}>
      <SectionHead as="h1" eyebrow="Confession Library" title={<>Words that were <span className="accent">checked</span> before you saw them.</>} />
      <ConfessionFilter />
      <PageStats items={[[String(CONFESSIONS.length), "confessions"], ["1–5", "intensity scale"], ["Quoted or labelled", "every Scripture line"], ["One-way", "draft → review → published"]]} />
      <CtaBand title="Find the sentence you have been trying to say." />
    </section>
  );
}

export function ConfessionDetail({ slug }: { slug: string }) {
  const c = confBySlug(slug); if (!c) return <NotFoundBody />;
  const cat = CATEGORIES.find((x) => x.name === c.category);
  const siblings = CONFESSIONS.filter((x) => x.category === c.category && x.slug !== c.slug);
  return (
    <>
      <section className="container detail-hero">
        <Link className="textlink" href="/confessions"><Icon n="arrowL" s={12} /> Library</Link>
        <div style={{ display: "flex", gap: 10, marginTop: 20, flexWrap: "wrap" }}>
          <span className="tag private">{c.category}</span>
          <span className="tag public">Intensity {c.intensity}/5</span>
          {c.scriptures.map((sc, i) => <span className="tag shared" key={i}>{sc.book} {sc.chapter}:{sc.verse}</span>)}
        </div>
        <h1 className="h-display" style={{ marginTop: 18, fontSize: "clamp(34px,5vw,60px)", maxWidth: "24ch" }}>{c.title}</h1>
        <p className="lede" style={{ marginTop: 18, maxWidth: "52ch" }}>{c.short}</p>
        <div style={{ display: "flex", gap: 12, margin: "26px 0", flexWrap: "wrap" }}>
          <PlayBtn slug={c.slug} label="Play" />
          <FavBtn slug={c.slug} ghost />
          <ShareBtn slug={c.slug} />
          <RadioBtn slug={c.slug} />
          <SnippetBtn slug={c.slug} />
        </div>
        <SnippetAuto slug={c.slug} />
        <div className="prose rv in">
          <p style={{ fontSize: 19, color: "var(--deep)", fontWeight: 550 }}>“{c.long}”</p>
        </div>
      </section>

      {HEAVY.includes(catBySlugCat(c.category)) && <div className="container"><CareNote /></div>}
      <section className="section container" style={{ paddingTop: 0 }}>
        <ConfDetailTabs c={c} cat={cat} />
        <Annotations slug={c.slug} />
      </section>

      {siblings.length > 0 && (
        <section className="section container-wide" style={{ paddingTop: 0 }}>
          <div className="container"><div className="section-head"><h3 className="h3">More in {c.category}</h3><Link className="textlink" href="/confessions">Full library <Icon n="arrow" s={12} /></Link></div></div>
          <div className="container"><div className="grid4 snap">{siblings.slice(0, 4).map((x) => <ConfTile key={x.slug} c={x} />)}</div></div>
        </section>
      )}

      {cat && (
        <section className="section container" style={{ paddingTop: 0 }}>
          <div className="split rv in">
            <div className="split-img">{CAT_IMAGES[cat.slug] ? <img src={`/assets/${CAT_IMAGES[cat.slug]}`} alt="" loading="lazy" /> : <div className="motif" style={{ ...motifStyle(cat.slug), width: "100%", height: "100%" }} />}</div>
            <div className="split-card">
              <span className="eyebrow on-dark">Category</span>
              <h3>{cat.name}</h3>
              <p>{cat.tagline}</p>
              <div className="btns"><Link className="btn btn-light" href={`/categories/${cat.slug}`}>Open category</Link><PlaySessionBtn slug={sessionsFor(cat.slug)[0].slug} /></div>
            </div>
          </div>
        </section>
      )}
    </>
  );
}

export function Sessions() {
  return (
    <section className="section container-wide" style={{ paddingTop: "calc(var(--header-h) + 56px)" }}>
      <SectionHead as="h1" eyebrow="Session Library" title={<>An experience sized to <span className="accent">the day you had.</span></>} lede="Sessions are assembled by the engine from reviewed confessions to fit 5, 10 or 15 minutes." />
      <div className="card-row three snap" style={{ gridTemplateColumns: "repeat(3,1fr)" }}>{ALL_SESSIONS.map((s) => <SessionTile key={s.slug} s={s} />)}</div>
      <div className="container" style={{ marginTop: 8 }}>
        <PageStats items={[["5–15 min", "session lengths"], [String(ALL_SESSIONS.length), "ready sessions"], ["Assembled from", "reviewed confessions only"], ["Yours", "build your own any time"]]} />
        <CtaBand title="Five minutes today beats an hour you never schedule." cta="Build a session" href="/app/session-builder" />
      </div>
    </section>
  );
}

export function SessionDetail({ slug }: { slug: string }) {
  const s = sessionBySlug(slug); if (!s) return <NotFoundBody />;
  const cat = catBySlug(s.category)!;
  return (
    <section className="container detail-hero">
      <Link className="textlink" href="/sessions"><Icon n="arrowL" s={12} /> Sessions</Link>
      <span className="tag private" style={{ marginTop: 20, display: "inline-block" }}>{cat.name} · {s.minutes} min</span>
      <h1 className="h1" style={{ marginTop: 14 }}>{s.title}</h1>
      <p className="lede" style={{ marginTop: 14 }}>{s.description}</p>
      <div style={{ margin: "26px 0" }}><PlaySessionBtn slug={s.slug} big /></div>
      <div className="form-card rv in" style={{ maxWidth: 640 }}>
        <h3 className="h4" style={{ marginBottom: 12 }}>In this session</h3>
        {s.items.map((c) => <div className="list-row" key={c.slug}><div className="lr-main"><h4>{c.title}</h4><p>{c.category} · {c.short}</p></div><PlayChip slug={c.slug} /></div>)}
      </div>
      <div style={{ maxWidth: 640, marginTop: 28 }}>
        <div className="split-card" style={{ background: "var(--n50)", color: "inherit", borderRadius: 20, padding: 24 }}>
          <h3 style={{ color: "var(--deep)" }}>Make it a habit</h3>
          <p style={{ color: "var(--n600)" }}>Add this session to your routine and it returns at the time you choose — no streak guilt, no autoplay.</p>
          <div className="btns" style={{ marginTop: 14 }}><Link className="btn btn-primary" href="/app/routines">Add to routine</Link><Link className="btn btn-ghost" href={`/categories/${cat.slug}`}>More in {cat.name}</Link></div>
        </div>
      </div>
    </section>
  );
}

export function Voices() {
  return <VoiceLibrary />;
}

export function VoiceDetail({ slug }: { slug: string }) {
  if (slug !== "grace") return <NotFoundBody />;
  return (
    <section className="container detail-hero">
      <Link className="textlink" href="/voices"><Icon n="arrowL" s={12} /> Voices</Link>
      <div style={{ display: "flex", gap: 28, alignItems: "center", marginTop: 28, flexWrap: "wrap" }}>
        <img src="/assets/voice-grace.jpg" alt="Grace" style={{ width: 140, height: 140, borderRadius: "50%", objectFit: "cover" }} />
        <div>
          <h1 className="h1">Grace</h1>
          <p className="lede" style={{ marginTop: 8 }}>Warm, calm licensed narration voice. English.</p>
          <div style={{ display: "flex", gap: 10, marginTop: 14 }}><span className="tag shared">Listed</span><span className="tag private">Licensed · i-confess studio · global TTS</span></div>
          <div style={{ marginTop: 18 }}><VoicePreviewBtn /></div>
          <p className="small" style={{ marginTop: 10, maxWidth: "52ch" }}>This web sample uses your device’s speech engine; it is not a studio recording of Grace.</p>
        </div>
      </div>
      <div className="app-section" style={{ marginTop: 64 }}>
        <div className="section-head"><h3 className="h3">Sample words</h3></div>
        <p className="small" style={{ marginBottom: 16 }}>These reviewed confessions also play through browser speech on the web.</p>
        <div className="grid4 snap">{CONFESSIONS.slice(0, 4).map((c) => <ConfTile key={c.slug} c={c} />)}</div>
      </div>
    </section>
  );
}

export function HowItWorks() {
  return (
    <section className="section container" style={{ paddingTop: "calc(var(--header-h) + 56px)" }}>
      <SectionHead as="h1" eyebrow="How It Works" title={<>Three movements. <span className="accent">One practice.</span></>} />
      <div className="principle-grid snap" style={{ gridTemplateColumns: "repeat(3,1fr)" }}>
        <div className="p-card rv in"><span className="venn v1"><i /><i /></span><div className="p-in"><h3>Hear it</h3><p>On the web, your browser’s speech engine reads the confession in a voice available on your device. Listening is the invitation.</p></div></div>
        <div className="p-card active rv in"><span className="venn v3"><i /><i /></span><div className="p-in"><h3>Speak it</h3><p>Then you say it — out loud, at speaking volume. The words have to survive your own voice.</p></div></div>
        <div className="p-card rv in"><span className="venn v4"><i /><i /></span><div className="p-in"><h3>Return</h3><p>Daily, in a routine you build. Repetition turns a quote into a posture.</p></div></div>
      </div>
      <div className="band rv in" style={{ marginTop: 72 }}>
        <img src="/assets/immersive.jpg" alt="" loading="lazy" />
        <div className="band-card"><h3>Five minutes is <mark>a complete practice</mark>.</h3><p>A confession, its Scripture, browser speech, and a moment of stillness at the end. Nothing withheld.</p><Link className="btn btn-primary" href="/sessions">Browse Sessions</Link></div>
      </div>
      <div className="app-section" style={{ marginTop: 64 }}>
        <div className="section-head"><h3 className="h3">What happens in your first week</h3></div>
        <div className="card-row snap">
          {[["Day 1", "You pick three categories and a time. The routine is built around the day you actually have."], ["Day 2–3", "The same confession returns. Repetition is the method, not a defect."], ["Day 4–5", "A new confession enters the slot when the first one has settled."], ["Day 7", "You review the week in History and keep what worked."]].map(([d, t]) => (
            <div className="tile" key={d}><h4>{d}</h4><p>{t}</p></div>
          ))}
        </div>
      </div>
      <CtaBand title="The practice is three movements. Start the first one." />
    </section>
  );
}

export function Premium() {
  const feats = (yearly: boolean) => ["Additional licensed voices when available", "Offline downloads on mobile", yearly ? "Two months free vs monthly" : "Longer guided sessions", "7-day trial, cancel anytime"];
  const plan = (p: typeof PLANS[0], gold: boolean) => (
    <div className={"plan-card rv in" + (gold ? " gold" : "")}>
      <div className="pc-top">
        <span className="eyebrow" style={gold ? { color: "var(--tint2)" } : { color: "var(--blue2)" }}>{p.interval === "year" ? "Annual · Best value" : "Monthly"}</span>
        <span className="orb sm" aria-hidden="true" style={gold ? { background: "var(--gold)", color: "var(--navy)" } : { background: "var(--light)", color: "var(--blue2)" }}><Icon n="ast" s={12} /></span>
      </div>
      <h3 className="h3" style={{ marginTop: 20 }}>{p.name}</h3>
      <div style={{ marginTop: 8 }}><span className="price">{p.prices.USD}</span> <span className="per">/ {p.interval} · also {p.prices.NGN} NGN</span></div>
      <ul style={{ marginTop: 14 }}>{feats(p.interval === "year").map((f, i) => <li key={i}><Icon n="check" s={14} /> {f}</li>)}</ul>
      <div style={{ marginTop: "auto", paddingTop: 26, display: "flex", flexDirection: "column", gap: 12 }}>
        <Link className={"btn btn-lg " + (gold ? "btn-light" : "btn-primary")} style={{ justifyContent: "center" }} href="/register">Start 7-day trial</Link>
        <span className="small" style={{ textAlign: "center", ...(gold ? { color: "var(--tint2)" } : {}) }}>Regional pricing: GBP {p.prices.GBP} · EUR {p.prices.EUR} · PHP {p.prices.PHP}</span>
      </div>
    </div>
  );
  return (
    <>
      <section className="section container" style={{ paddingTop: "calc(var(--header-h) + 72px)" }}>
        <div className="c-head rv in">
          <span className="eyebrow">Premium</span>
          <h1 className="h-display" style={{ marginTop: 18 }}>Go deeper, <span className="accent">quietly.</span></h1>
          <p className="lede c-lede">The free practice is complete. Premium adds depth — not pressure.</p>
        </div>
        <div className="plan-grid" style={{ margin: "8px auto 0" }}>{plan(PLANS[0], false)}{plan(PLANS[1], true)}</div>
      </section>
      <section className="section container" style={{ paddingTop: 0 }}>
        <div className="premium-duo">
          <div className="form-card rv in">
            <h3 className="h4" style={{ marginBottom: 14 }}>Free vs Premium</h3>
            {[["39 categories & full library", "✓", "✓"], ["Sessions up to 15 minutes", "✓", "✓"], ["Grace voice", "✓", "✓"], ["Additional licensed voices (when available)", "—", "As approved"], ["Offline downloads (mobile)", "—", "✓"], ["Extended sessions", "—", "✓"]].map(([f, a, b], i) => (
              <div className="setting-row" key={i}><b>{f}</b><span style={{ display: "flex", gap: 40 }}><span>{a}</span><span>{b}</span></span></div>
            ))}
          </div>
          <div className="form-card rv in">
            <h3 className="h4" style={{ marginBottom: 12 }}>Premium, honestly</h3>
            <div className="prose"><p>The free practice is not a teaser — it is complete. Premium adds offline downloads for bad-network mornings and longer guided sessions; additional licensed voices appear only after their rights are approved. The 7-day trial converts only with notice, and cancelling keeps your library and history intact.</p></div>
            <div style={{ marginTop: 20 }}><Link className="btn btn-primary" href="/register">Start 7-day trial</Link></div>
          </div>
        </div>
        <CtaBand title="Try Premium for seven days. Keep the practice either way." />
      </section>
    </>
  );
}

export function Community() {
  return (
    <section className="section container" style={{ paddingTop: "calc(var(--header-h) + 56px)" }}>
      <SectionHead as="h1" eyebrow="Community" title="Your words matter too." lede="Create a confession. Keep it private. Share it intentionally. Discover community experiences — after human review." />
      <div className="principle-grid snap" style={{ gridTemplateColumns: "repeat(3,1fr)" }}>
        <div className="p-card rv in"><span className="tag private">Private</span><h3>Only you</h3><p>The default. Encrypted to your account, never reviewed, never seen.</p></div>
        <div className="p-card rv in"><span className="tag shared">Shared</span><h3>One person</h3><p>A signed link to someone you choose. It expires when you say so.</p></div>
        <div className="p-card active rv in"><span className="tag public">Public</span><h3>Community</h3><p>Offered to review, approved by a human, then published with your permission.</p></div>
      </div>
      <div className="split rv in" style={{ marginTop: 72 }}>
        <div className="split-img"><img src="/assets/community.jpg" alt="" loading="lazy" /></div>
        <div className="split-card"><span className="eyebrow on-dark">Guidelines</span><h3>Review is the product.</h3><p>Nothing publishes automatically — not ever. A person reads every submission, approves or declines with a reason, and the decision is recorded.</p><div className="btns"><Link className="btn btn-light" href="/app/community/create">Create Your Confession</Link><Link className="btn btn-ghost on-dark" href="/community-guidelines">Community Guidelines</Link></div></div>
      </div>
      <div className="app-section" style={{ marginTop: 64 }}>
        <div className="section-head"><h3 className="h3">The life of a community confession</h3></div>
        <div className="card-row snap">
          {[["1 · Written", "Your words, standing on Scripture honestly. It stays private until you decide otherwise."], ["2 · Submitted", "You offer it for review. A reviewer reads it — not an algorithm."], ["3 · Decided", "Approved or declined with a reason. The decision is recorded and you can withdraw any time."], ["4 · Published", "If approved, it appears with your permission. Private stays private, always."]].map(([d, t]) => (
            <div className="tile" key={d}><h4>{d}</h4><p>{t}</p></div>
          ))}
        </div>
      </div>
      <CtaBand title="Write the sentence only you can say." cta="Create your confession" href="/app/community/create" />
    </section>
  );
}

export function About() {
  return (
    <section className="section container" style={{ paddingTop: "calc(var(--header-h) + 56px)" }}>
      <div className="story"><div className="rv in"><span className="eyebrow">About</span><h1 className="h1" style={{ marginTop: 16 }}>Built for a few <span className="accent">honest minutes</span> a day.</h1>
        <p className="lede" style={{ marginTop: 20 }}>I CONFESS began with a simple observation: every meaningful commitment in history was spoken before it was written. Speech is slower than reading. It costs breath. It makes words harder to dismiss.</p>
        <p className="lede" style={{ marginTop: 16 }}>So we built a home for that practice — curated confessions checked against the Scripture they stand on, a licensed narration voice, and sessions sized to real days.</p>
        <div className="story-cta"><Link className="btn btn-primary" href="/mission">Our Mission</Link></div></div>
        <aside className="story-side rv in"><p>We do not use streak guilt. Motion is sparing. Nothing autoplays. The loudest thing in I CONFESS should be the words themselves.</p><p className="small" style={{ marginTop: 16 }}>Design principle, from “Designing a quiet app on purpose” — the I CONFESS journal.</p></aside></div>
      <div className="brand-panel rv in" style={{ marginTop: 72 }}>
        <div className="brand-lockup" role="img" aria-label="I CONFESS — Speak it. Believe it. Live it.">
          <Mark s={88} />
          <b>I CONFESS</b>
          <span>Speak it. Believe it. Live it.</span>
        </div>
        <p className="lede" style={{ color: "var(--tint2)", maxWidth: "44ch", marginTop: 26 }}>Speak it. Believe it. Live it. — the order is the product. Speaking comes first because speech is where belief is tested, and living is where it is proven.</p>
      </div>
      <PageStats items={[[String(CATEGORIES.length), "categories"], [String(CONFESSIONS.length), "reviewed confessions"], ["Human", "review on everything published"], ["Quiet", "by design, on purpose"]]} />
      <CtaBand title="A few honest minutes a day. That is the whole pitch." />
    </section>
  );
}
export function Mission() {
  return (
    <section className="section container" style={{ paddingTop: "calc(var(--header-h) + 56px)" }}>
      <SectionHead as="h1" eyebrow="Mission" title={<>Orientation, <span className="accent">not hype.</span></>} />
      <div className="prose rv in"><p>A sentence you have said thirty times stops being an affirmation and starts being a description — of who you are choosing to be. That is the quiet ambition of this product: not hype, but orientation.</p><p>We measure ourselves by returns, not reaches: did the words meet you where you were, on the fortieth morning as on the first?</p></div>
      <CtaBand title="Orientation starts with one spoken sentence." />
    </section>
  );
}

export function Journal() {
  const imgs = ["journal-1.jpg", "cat-peace.jpg", "cat-faith.jpg", "immersive.jpg", "cat-purpose.jpg"];
  return (
    <section className="section container-wide" style={{ paddingTop: "calc(var(--header-h) + 56px)" }}>
      <SectionHead as="h1" eyebrow="The Journal" title={<>Words to <span className="accent">return to.</span></>} lede="Essays written for this product and signed I CONFESS — no invented authors." />
      <div className="journal-grid" style={{ gridTemplateColumns: "repeat(3,1fr)" }}>{ARTICLES.map((a, i) => <ArticleCard key={a.slug} a={a} img={imgs[i]} />)}</div>
      <CtaBand title="Read less. Say more. Start the practice." />
    </section>
  );
}
export function Article({ slug }: { slug: string }) {
  const a = artBySlug(slug); if (!a) return <NotFoundBody />;
  return (
    <section className="container detail-hero">
      <Link className="textlink" href="/journal"><Icon n="arrowL" s={12} /> Journal</Link>
      <span className="j-cat" style={{ display: "block", marginTop: 20 }}>{a.category}</span>
      <h1 className="h1" style={{ marginTop: 12, maxWidth: "24ch" }}>{a.title}</h1>
      <p className="small" style={{ marginTop: 14 }}>By {a.author} · {a.date} · {a.readingTime}</p>
      <div className="prose rv in" style={{ marginTop: 28 }}>{a.body.map((b, i) => <p key={i}>{b}</p>)}</div>
    </section>
  );
}
export function Stories({ slug }: { slug?: string }) {
  return (
    <section className="section container" style={{ paddingTop: "calc(var(--header-h) + 56px)" }}>
      <SectionHead as="h1" eyebrow="Stories" title={<>Real words, <span className="accent">reviewed.</span></>} />
      <div style={{ maxWidth: 640 }}><EmptyState icon="book" title="Stories arrive with the community launch" sub="Every story is reviewed by a human before publishing. We will never fill this space with invented voices." cta={<Link className="btn btn-primary" href="/app/community/create">Write the first one</Link>} /></div>
    </section>
  );
}

export function Download() {
  return (
    <section className="section container" style={{ paddingTop: "calc(var(--header-h) + 56px)" }}>
      <div className="split">
        <div className="rv in"><span className="eyebrow">Download</span><h1 className="h1" style={{ marginTop: 16 }}>Carry the practice <span className="accent">with you.</span></h1>
          <p className="lede" style={{ marginTop: 18 }}>One account across mobile and web. Favorites, history, routines and streaks travel with you.</p>
          <div style={{ display: "flex", gap: 12, marginTop: 26, flexWrap: "wrap" }}>
            <Link className="btn btn-primary btn-lg" href="/app">Open the Web App</Link>
            <span className="btn btn-ghost btn-lg">App Store — beta soon</span>
            <span className="btn btn-ghost btn-lg">Google Play — beta soon</span>
          </div>
          <p className="small" style={{ marginTop: 14 }}>Listings appear when the beta opens. The web app is fully live today.</p></div>
        <div className="rv in" style={{ display: "flex", justifyContent: "center" }}><PhoneMock big /></div>
      </div>
    </section>
  );
}

export function NotFoundBody() {
  return (
    <section className="section container" style={{ paddingTop: "calc(var(--header-h) + 80px)", textAlign: "center" }}>
      <span className="eyebrow" style={{ justifyContent: "center" }}>404</span>
      <h1 className="h-display" style={{ marginTop: 14 }}>This page went <span className="accent">quiet.</span></h1>
      <p className="lede" style={{ margin: "18px auto", maxWidth: "40ch" }}>The address doesn't match anything in I CONFESS.</p>
      <div style={{ display: "flex", gap: 12, justifyContent: "center" }}><Link className="btn btn-primary" href="/">Go home</Link><Link className="btn btn-ghost" href="/explore">Explore</Link></div>
    </section>
  );
}

export function Legal({ kind }: { kind: string }) {
  const L: Record<string, [string, string[]]> = {
    privacy: ["Privacy", ["Your private confessions are yours. They are stored against your account, never indexed for search, never reviewed unless you explicitly submit them, and never used to train anything.", "We collect the minimum needed to run the service: an email address, your content, and playback events that make “continue listening” possible. Analytics events are product events, not advertising profiles.", "If you sign up for the newsletter we keep only that address, and every email carries a one-click unsubscribe link. Messages sent through the contact form are kept so we can reply.", "When you describe what you need to the session builder, that description alone may be sent to Anthropic's Claude to interpret it — never your name, email or saved confessions.", "You can export or delete your account and its content from Settings. Deletion is complete, not a hide."]],
    terms: ["Terms", ["I CONFESS provides curated confession content, audio narration and session tooling. You may use it personally; you may not scrape, resell or redistribute the library.", "Content you create remains yours. If you submit it for community publication you grant us a licence to publish it after review, revocable by withdrawing the submission.", "Premium subscriptions renew until cancelled and are billed by the stores or our billing provider; the 7-day trial converts only with notice."]],
    cookies: ["Cookies", ["We use a session cookie for sign-in and a preferences cookie for settings. We do not use third-party advertising cookies.", "Analytics are first-party product events (a session started, a confession played) and respect your consent choice at registration."]],
    guidelines: ["Community Guidelines", ["Community confessions must stand on Scripture honestly, harm no person or group, and be your own words.", "Nothing publishes automatically. A human reviewer approves or declines with a reason; repeated violations close the submission path.", "Private stays private. Sharing someone else's words requires their explicit share link."]],
    policy: ["Content Policy", ["Every confession is drafted, edited and checked against the Scripture it claims to stand on. Quotations must quote; allusions must allude honestly.", "The pipeline is one-way: draft → review → published, enforced by the platform, not by habit."]],
  };
  const [title, paras] = L[kind] || L.privacy;
  return (
    <section className="container detail-hero">
      <span className="eyebrow">Legal</span>
      <h1 className="h1" style={{ marginTop: 14 }}>{title}</h1>
      <div className="prose rv in">{paras.map((p, i) => <p key={i}>{p}</p>)}</div>
    </section>
  );
}

export function HelpCenter() {
  return (
    <section className="section container" style={{ paddingTop: "calc(var(--header-h) + 56px)" }}>
      <SectionHead as="h1" eyebrow="Help Center" title={<>What do you need <span className="accent">right now?</span></>} />
      <div className="card-row snap">{[["/faq", "FAQ", "The eleven honest answers."], ["/app/settings", "Settings", "Playback, privacy, notifications, accessibility."], ["/app/subscription", "Subscription", "Plans, trial, cancel, restore."], ["/community-guidelines", "Guidelines", "What publishes, and how."]].map(([h, t, s]) => (
        <Link className="tile" href={h} key={t}><h4>{t}</h4><p>{s}</p><span className="textlink" style={{ marginTop: 6 }}>Open <Icon n="arrow" s={12} /></span></Link>
      ))}</div>
      <div className="app-section" style={{ marginTop: 48 }}>
        <div className="section-head"><h3 className="h3">Popular topics</h3></div>
        <div className="card-row snap">
          {[["/faq", "The eleven honest answers", "Practice, privacy, pricing — answered plainly."], ["/voices", "Voices & licensing", "Why only licensed voices ship."], ["/content-policy", "How review works", "The one-way pipeline: draft → review → published."], ["/app/history", "Your history & streak", "Where past sessions live."], ["/download", "Mobile & offline", "What works today, what is coming."], ["/contact", "Talk to a human", "We read everything."]].map(([h, t, s]) => (
            <Link className="tile" href={h} key={t}><h4>{t}</h4><p>{s}</p><span className="textlink" style={{ marginTop: 6 }}>Open <Icon n="arrow" s={12} /></span></Link>
          ))}
        </div>
      </div>
      <CtaBand title="Still stuck? A person reads every message." cta="Contact us" href="/contact" />
    </section>
  );
}
export function FaqPage() {
  return (
    <section className="section container faq">
      <div className="faq-left rv in"><span className="eyebrow">FAQ</span><h1 className="h1" style={{ marginTop: 16 }}>Finally, some <span className="accent">answers.</span></h1><p className="lede" style={{ marginTop: 16 }}>Everything people actually ask.</p><div style={{ marginTop: 24 }}><Link className="btn btn-primary" href="/help">Help Center</Link></div></div>
      <div className="rv in"><FaqList items={FAQS} /></div>
    </section>
  );
}
export function Contact() {
  return (
    <section className="section container" style={{ paddingTop: "calc(var(--header-h) + 56px)" }}>
      <SectionHead as="h1" eyebrow="Contact" title={<>We read <span className="accent">everything.</span></>} />
      <ContactForm />
    </section>
  );
}
