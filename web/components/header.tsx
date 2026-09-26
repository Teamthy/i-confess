"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { usePathname } from "next/navigation";
import { Logo } from "./ui";

export default function Header() {
  const [open, setOpen] = useState(false);
  const [scrolled, setScrolled] = useState(false);
  const pathname = usePathname();
  const darkHero = pathname === "/" && !scrolled;
  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 10);
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);
  useEffect(() => {
    const saved = window.localStorage.getItem("ic-theme");
    if (saved && saved !== "light") document.documentElement.setAttribute("data-theme", saved);
    else if (!saved && window.matchMedia?.("(prefers-color-scheme: dark)").matches) document.documentElement.setAttribute("data-theme", "dark");
  }, []);
  useEffect(() => { setOpen(false); }, [pathname]);
  const links = [
    { href: "/how-it-works", label: "How it works" },
    { href: "/explore", label: "Explore" },
    { href: "/bible", label: "Bible" },
    { href: "/journal", label: "Journal" },
    { href: "/premium", label: "Premium" },
    { href: "/faq", label: "FAQ" },
  ];
  return (
    <header className={"site-header" + (scrolled ? " scrolled" : "") + (darkHero ? " on-dark" : "")}>
      <div className="header-in container-wide">
      <Link className={"logo" + (darkHero ? " on-dark" : "")} href="/" aria-label="iCONFESS home">
        <svg className="mark" width="30" height="30" viewBox="0 0 100 96" fill="none" stroke="currentColor" strokeWidth="9" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
          <path d="M50 5 L63 23 L50 40 L37 23 Z" />
          <path d="M47 51 C40 42 27 42 20 52 C12 63 13 77 24 81 C32 84 40 79 47 71 L58 59" />
          <path d="M53 51 C60 42 73 42 80 52 C88 63 87 77 76 81 C68 84 60 79 53 71 L42 59" />
        </svg>
        <span className="lw"><b>I CONFESS</b></span>
      </Link>
      <nav className={"main-nav" + (open ? " open" : "")} aria-label="Main">
        {links.map((l) => (
          <Link key={l.href} href={l.href} aria-current={pathname === l.href ? "page" : undefined}>
            {l.label}
          </Link>
        ))}
      </nav>
      <div className="header-actions">
        <ThemeToggle />
        <Link className="grid-btn" href="/app" title="Open the app" aria-label="Open the app">
          <svg className="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true"><rect x="3" y="3" width="7" height="7" rx="1.5" /><rect x="14" y="3" width="7" height="7" rx="1.5" /><rect x="3" y="14" width="7" height="7" rx="1.5" /><rect x="14" y="14" width="7" height="7" rx="1.5" /></svg>
        </Link>
        <Link className="login" href="/login">Log in</Link>
        <Link className="btn btn-primary" href="/register">Get Started</Link>
        <button className="menu-btn" aria-label="Menu" aria-expanded={open} onClick={() => setOpen(!open)}>
          <span /><span /><span />
        </button>
      </div>
      </div>
      <div className={"mobile-menu" + (open ? " open" : "")}>
        <div className="mm-top">
          <Logo onDark />
          <button className="mm-close" aria-label="Close menu" onClick={() => setOpen(false)}>
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true"><path d="M6 6l12 12M18 6L6 18" /></svg>
          </button>
        </div>
        <nav aria-label="Mobile">
          {links.map((l) => <Link key={l.href} href={l.href}>{l.label}</Link>)}
          <Link href="/categories">Categories</Link>
          <Link href="/community">Community</Link>
          <Link href="/about">About</Link>
          <Link href="/help">Help</Link>
        </nav>
        <div className="mm-actions">
          <Link className="grid-btn" href="/app" aria-label="Open the app">
            <svg className="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true"><rect x="3" y="3" width="7" height="7" rx="1.5" /><rect x="14" y="3" width="7" height="7" rx="1.5" /><rect x="3" y="14" width="7" height="7" rx="1.5" /><rect x="14" y="14" width="7" height="7" rx="1.5" /></svg>
          </Link>
          <Link className="btn btn-ghost on-dark" href="/login">Log in</Link>
          <Link className="btn btn-primary" href="/register">Get Started</Link>
        </div>
      </div>
    </header>
  );
}

function ThemeToggle() {
  const [mode, setMode] = useState("light");
  useEffect(() => {
    setMode(window.localStorage.getItem("ic-theme") || (document.documentElement.getAttribute("data-theme") ?? "light"));
  }, []);
  const order = ["light", "dark", "contrast", "dys"];
  const toggle = () => {
    const next = order[(order.indexOf(mode) + 1) % order.length];
    setMode(next);
    if (next === "light") document.documentElement.removeAttribute("data-theme");
    else document.documentElement.setAttribute("data-theme", next);
    window.localStorage.setItem("ic-theme", next);
  };
  const dark = mode !== "light";
  return (
    <button className="theme-toggle" onClick={toggle} aria-label={"Theme: " + mode + " — tap to change"} title={"Theme: " + mode}>
      {mode === "contrast" ? (
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true"><circle cx="12" cy="12" r="9" /><path d="M12 3v18" fill="currentColor" /><path d="M12 3a9 9 0 0 1 0 18Z" fill="currentColor" stroke="none" /></svg>
      ) : mode === "dys" ? (
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true"><path d="M4 7h16M4 12h10M4 17h16" /></svg>
      ) : dark ? (
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true"><circle cx="12" cy="12" r="4" /><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" /></svg>
      ) : (
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true"><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8Z" /></svg>
      )}
    </button>
  );
}
