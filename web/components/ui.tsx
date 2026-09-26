import React from "react";
import Link from "next/link";
import { CAT_IMAGES, motifStyle, type Category, type Session, type Article } from "@/lib/data";

const IC: Record<string, React.ReactNode> = {
  arrow: <path d="M5 12h14M13 6l6 6-6 6" />, arrowL: <path d="M19 12H5M11 6l-6 6 6 6" />,
  play: <path d="M8 5.5v13l11-6.5z" fill="currentColor" stroke="none" />, pause: <path d="M8 5v14M16 5v14" strokeWidth="2.6" />,
  next: <path d="M5 5v14l9-7zM17 5v14" />, prev: <path d="M19 5v14l-9-7zM7 5v14" />,
  plus: <path d="M12 5v14M5 12h14" />, minus: <path d="M5 12h14" />,
  heart: <path d="M12 20s-7-4.6-9.3-9A5.4 5.4 0 0 1 12 6.6 5.4 5.4 0 0 1 21.3 11C19 15.4 12 20 12 20z" />,
  search: <><circle cx="11" cy="11" r="7" /><path d="m20 20-3.5-3.5" /></>,
  bell: <><path d="M6 9a6 6 0 1 1 12 0c0 5 2 6 2 6H4s2-1 2-6" /><path d="M10 20a2 2 0 0 0 4 0" /></>,
  shield: <path d="M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6z" />,
  home: <path d="M4 11l8-7 8 7v9a1 1 0 0 1-1 1h-5v-6h-4v6H5a1 1 0 0 1-1-1z" />,
  compass: <><circle cx="12" cy="12" r="9" /><path d="m15 9-2 5-4 1 2-5z" /></>,
  clock: <><circle cx="12" cy="12" r="9" /><path d="M12 7v5l3 3" /></>,
  user: <><circle cx="12" cy="8" r="4" /><path d="M4 21c1-4 4-6 8-6s7 2 8 6" /></>,
  gear: <><circle cx="12" cy="12" r="3.2" /><path d="M12 2.8l1.5 2.6 3-.6 1 2.8 2.9 1-.3 3 2.1 2.4-2.1 2.4.3 3-2.9 1-1 2.8-3-.6L12 21.2l-1.5-2.6-3 .6-1-2.8-2.9-1 .3-3L1.8 12l2.1-2.4-.3-3 2.9-1 1-2.8 3 .6z" strokeWidth="1.2" /></>,
  book: <><path d="M4 5a2 2 0 0 1 2-2h14v18H6a2 2 0 0 0-2 2z" /><path d="M4 19a2 2 0 0 1 2-2h14" /></>,
  mic: <><rect x="9" y="3" width="6" height="11" rx="3" /><path d="M5 11a7 7 0 0 0 14 0M12 18v3" /></>,
  cal: <><rect x="3" y="5" width="18" height="16" rx="2" /><path d="M3 10h18M8 3v4M16 3v4" /></>,
  dl: <path d="M12 3v12m0 0 4-4m-4 4-4-4M4 21h16" />,
  share: <><circle cx="6" cy="12" r="2.5" /><circle cx="17" cy="6" r="2.5" /><circle cx="17" cy="18" r="2.5" /><path d="m8.3 10.8 6.4-3.6M8.3 13.2l6.4 3.6" /></>,
  check: <path d="m5 12.5 4.5 4.5L19 7" />, x: <path d="M6 6l12 12M18 6 6 18" />,
  lock: <><rect x="5" y="11" width="14" height="9" rx="2" /><path d="M8 11V8a4 4 0 0 1 8 0v3" /></>,
  spark: <path d="M12 3v4M12 17v4M3 12h4M17 12h4M6 6l2.5 2.5M15.5 15.5 18 18M18 6l-2.5 2.5M8.5 15.5 6 18" />,
  wave: <path d="M3 12c2 0 2-4 4-4s2 8 4 8 2-8 4-8 2 8 4 8 2-4 4-4" strokeWidth="1.6" />,
  edit: <><path d="M4 20h4L20 8l-4-4L4 16z" /><path d="m13 7 4 4" /></>,
  chart: <path d="M4 20V6M4 20h16M8 16v-5M12 16V8M16 16v-3" />,
  sun: <><circle cx="12" cy="12" r="4" /><path d="M12 2v3M12 19v3M2 12h3M19 12h3M4.5 4.5l2 2M17.5 17.5l2 2M19.5 4.5l-2 2M6.5 17.5l-2 2" /></>,
  moon: <path d="M20 14.5A8.5 8.5 0 0 1 9.5 4a8.5 8.5 0 1 0 10.5 10.5z" />,
  exit: <path d="M15 4h4a1 1 0 0 1 1 1v14a1 1 0 0 1-1 1h-4M10 8l-4 4 4 4M6 12h10" />,
  info: <><circle cx="12" cy="12" r="9" /><path d="M12 10.5V17M12 7.2v.2" /></>,
  ne: <path d="M7 17 17 7M9 7h8v8" />,
  grid: <><circle cx="8" cy="8" r="1.6" fill="currentColor" stroke="none"/><circle cx="16" cy="8" r="1.6" fill="currentColor" stroke="none"/><circle cx="8" cy="16" r="1.6" fill="currentColor" stroke="none"/><circle cx="16" cy="16" r="1.6" fill="currentColor" stroke="none"/></>,
  ast: <path d="M12 4v16M5.1 8l13.8 8M18.9 8 5.1 16" strokeWidth="2.4" />,
};
export function Icon({ n, s = 18 }: { n: string; s?: number }) {
  return <svg className="ic" width={s} height={s} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.8} strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">{IC[n]}</svg>;
}

export function Mark({ s = 30 }: { s?: number }) {
  return (
    <svg className="mark" width={s} height={s} viewBox="0 0 100 96" fill="none" stroke="currentColor" strokeWidth="9" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M50 5 L63 23 L50 40 L37 23 Z" />
      <path d="M47 51 C40 42 27 42 20 52 C12 63 13 77 24 81 C32 84 40 79 47 71 L58 59" />
      <path d="M53 51 C60 42 73 42 80 52 C88 63 87 77 76 81 C68 84 60 79 53 71 L42 59" />
    </svg>
  );
}
export function Logo({ onDark, tagline }: { onDark?: boolean; tagline?: boolean }) {
  return (
    <Link className={"logo" + (onDark ? " on-dark" : "")} href="/" style={onDark ? { color: "#fff" } : undefined} aria-label="iCONFESS home">
      <Mark />
      <span className="lw"><b>I CONFESS</b>{tagline && <span>SPEAK IT. BELIEVE IT. LIVE IT.</span>}</span>
    </Link>
  );
}

export function SectionHead({ eyebrow, title, lede, right, onDark }: { eyebrow?: React.ReactNode; title?: React.ReactNode; lede?: React.ReactNode; right?: React.ReactNode; onDark?: boolean }) {
  return (
    <div className="section-head rv in">
      <div>
        {eyebrow && <span className={"eyebrow" + (onDark ? " on-dark" : "")}>{eyebrow}</span>}
        {title && <h2 className="h1" style={{ marginTop: eyebrow ? 14 : 0 }}>{title}</h2>}
        {lede && <p className="lede" style={{ marginTop: 14, maxWidth: "52ch" }}>{lede}</p>}
      </div>
      {right}
    </div>
  );
}

export function CatCard({ cat, i, featured }: { cat: Category; i: number; featured?: boolean }) {
  const img = CAT_IMAGES[cat.slug];
  return (
    <Link className={"cat-card rv in" + (featured ? " featured" : "")} href={`/categories/${cat.slug}`}>
      <div className="cc-blob">
        {img ? (
          <div className="blob"><img src={`/assets/${img}`} alt="" loading="lazy" /></div>
        ) : (
          <div className="blob motif" style={motifStyle(cat.slug)}><span className="m-glyph" style={{ fontSize: 44, color: "rgba(255,255,255,.9)" }}>{cat.name[0]}</span></div>
        )}
      </div>
      <div className="cc-name">{cat.name}</div>
      <div className="cc-tag">{cat.tagline}</div>
      <div className="cc-foot"><span className="cc-arrow"><Icon n="ne" s={15} /></span></div>
    </Link>
  );
}

export function SessionTile({ s }: { s: Session }) {
  return (
    <Link className="tile rv in" href={`/sessions/${s.slug}`}>
      <span className="t-meta">{s.category} · {s.minutes} min</span>
      <h4>{s.title}</h4>
      <p>{s.description}</p>
      <span className="textlink" style={{ marginTop: 6 }}>Start experience <Icon n="arrow" s={12} /></span>
    </Link>
  );
}

export function ArticleCard({ a, img }: { a: Article; img: string }) {
  return (
    <Link className="j-card rv in" href={`/journal/${a.slug}`}>
      <div className="j-media"><img src={`/assets/${img}`} alt="" loading="lazy" /></div>
      <div className="j-body"><span className="j-cat">{a.category}</span><h3>{a.title}</h3></div>
      <div className="j-foot"><span className="j-arrow2"><Icon n="ne" s={15} /></span></div>
    </Link>
  );
}

export function PhoneMock({ big }: { big?: boolean }) {
  return (
    <div className={"phone-mock" + (big ? " big" : "")} aria-hidden="true">
      <div className="pm-line short" /><div className="pm-line" /><div className="pm-line" />
      <div className="pm-card"><span className="play-chip"><Icon n="play" s={10} /></span><span className="pm-line" /></div>
      <div className="pm-line short" />
    </div>
  );
}

export function EmptyState({ icon, title, sub, cta }: { icon: string; title: string; sub: string; cta?: React.ReactNode }) {
  return (
    <div className="empty-state">
      <span className="e-ic"><Icon n={icon} s={18} /></span>
      <h3>{title}</h3><p>{sub}</p>
      {cta && <div style={{ marginTop: 16, display: "flex", gap: 10, justifyContent: "center" }}>{cta}</div>}
    </div>
  );
}

export function Reveal({ children, className, as }: any) {
  const Tag = as || "div";
  return <Tag className={"rv in " + (className || "")}>{children}</Tag>;
}
