"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { Wordmark } from "./Wordmark";

const NAV = [
  { href: "/explore", label: "Explore" },
  { href: "/categories", label: "Categories" },
  { href: "/how-it-works", label: "How it works" },
  { href: "/voices", label: "Voices" },
  { href: "/about", label: "About" },
];

/**
 * The public header.
 *
 * Over the ink hero it starts transparent with light text; after scroll (or on
 * light pages) it gains a solid blurred surface. State is scroll-position only
 * — never hover-dependent — and the mobile drawer is a real focus-trapped
 * dialog pattern with Escape support.
 */
export function SiteHeader({ light = false }: { light?: boolean }) {
  const [solid, setSolid] = useState(false);
  const [open, setOpen] = useState(false);
  const pathname = usePathname();

  useEffect(() => {
    const onScroll = () => setSolid(window.scrollY > 24);
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  // Close the drawer on navigation and lock body scroll while it is open.
  useEffect(() => setOpen(false), [pathname]);
  useEffect(() => {
    document.body.style.overflow = open ? "hidden" : "";
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    window.addEventListener("keydown", onKey);
    return () => {
      document.body.style.overflow = "";
      window.removeEventListener("keydown", onKey);
    };
  }, [open]);

  return (
    <>
      <header
        className={`ic-header ${solid ? "ic-header--solid" : ""} ${light ? "ic-header--light" : ""}`}
      >
        <div className="ic-container ic-header__inner">
          <Link href="/" aria-label="iCONFESS home">
            <Wordmark />
          </Link>
          <nav className="ic-nav" aria-label="Primary">
            {NAV.map((item) => (
              <Link
                key={item.href}
                href={item.href}
                aria-current={pathname === item.href ? "page" : undefined}
              >
                {item.label}
              </Link>
            ))}
          </nav>
          <div className="ic-header__actions">
            <Link href="/login" className="ic-header__login">
              Log in
            </Link>
            <Link href="/register" className="ic-btn ic-btn--on-dark ic-btn--small">
              Get started
            </Link>
          </div>
          <button
            type="button"
            className="ic-menu-btn"
            aria-expanded={open}
            aria-controls="mobile-drawer"
            onClick={() => setOpen(true)}
          >
            <span className="ic-visually-hidden">Open menu</span>
            <svg width="22" height="22" viewBox="0 0 22 22" aria-hidden="true">
              <path d="M2 5.5h18M2 11h18M2 16.5h18" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
            </svg>
          </button>
        </div>
      </header>

      <div
        id="mobile-drawer"
        className="ic-drawer"
        data-open={open}
        role="dialog"
        aria-modal="true"
        aria-label="Menu"
        hidden={!open ? true : undefined}
      >
        <div className="ic-drawer__top">
          <Wordmark onDark />
          <button
            type="button"
            className="ic-menu-btn"
            onClick={() => setOpen(false)}
          >
            <span className="ic-visually-hidden">Close menu</span>
            <svg width="20" height="20" viewBox="0 0 20 20" aria-hidden="true">
              <path d="M4 4l12 12M16 4L4 16" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
            </svg>
          </button>
        </div>
        <nav aria-label="Mobile">
          {NAV.map((item) => (
            <Link key={item.href} href={item.href}>
              {item.label}
            </Link>
          ))}
          <Link href="/premium">Premium</Link>
          <Link href="/community">Community</Link>
          <Link href="/download">Download</Link>
        </nav>
        <div className="ic-drawer__actions">
          <Link href="/register" className="ic-btn ic-btn--on-dark">
            Get started
          </Link>
          <Link href="/login" className="ic-btn ic-btn--secondary on-ink">
            Log in
          </Link>
        </div>
      </div>
    </>
  );
}
