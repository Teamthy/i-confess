"use client";

/**
 * Authenticated application shell.
 *
 * Desktop: 64-column sidebar + main content area.
 * Mobile: bottom tab bar (5 destinations per tokens.json §12) + top bar.
 * The shell wraps all /app/* routes and manages the mini-player slot.
 */

import Link from "next/link";
import { usePathname } from "next/navigation";
import type { ReactNode } from "react";
import { useAuth } from "@/lib/auth-context";
import { usePlayer } from "@/lib/player-context";
import { MiniPlayer } from "@/components/MiniPlayer";
import { ConsentBanner } from "@/components/ConsentBanner";

const NAV_ITEMS = [
  { href: "/app", label: "Home", icon: "◉" },
  { href: "/app/bible", label: "Bible", icon: "▤" },
  { href: "/app/explore", label: "Explore", icon: "◎" },
  { href: "/app/categories", label: "Categories", icon: "≡" },
  { href: "/app/sessions", label: "Sessions", icon: "▷" },
  { href: "/app/voices", label: "Voices", icon: "♫" },
  { href: "/app/history", label: "History", icon: "↻" },
  { href: "/app/favorites", label: "Favorites", icon: "♡" },
  { href: "/app/routines", label: "Routines", icon: "◫" },
  { href: "/app/schedule", label: "Schedule", icon: "◷" },
  { href: "/app/community", label: "Community", icon: "⌂" },
  { href: "/app/search", label: "Search", icon: "⌕" },
] as const;

const BOTTOM_TABS = [
  { href: "/app", label: "Home", icon: "◉" },
  { href: "/app/bible", label: "Bible", icon: "▤" },
  { href: "/app/confessions", label: "Create", icon: "+" },
  { href: "/app/history", label: "Activity", icon: "↻" },
  { href: "/app/profile", label: "Profile", icon: "⊙" },
] as const;

function isActive(pathname: string, href: string): boolean {
  if (href === "/app") return pathname === "/app";
  return pathname.startsWith(href);
}

export function AppShell({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const { user, logout } = useAuth();

  return (
    <div className="ia-shell">
      {/* Desktop sidebar */}
      <aside className="ia-sidebar" aria-label="Main navigation">
        <div className="ia-sidebar__brand">
          <Link href="/app" className="ia-sidebar__logo">iCONFESS</Link>
        </div>

        <nav className="ia-sidebar__nav">
          {NAV_ITEMS.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              className={`ia-sidebar__link ${isActive(pathname, item.href) ? "ia-sidebar__link--active" : ""}`}
              aria-current={isActive(pathname, item.href) ? "page" : undefined}
            >
              <span className="ia-sidebar__icon" aria-hidden="true">{item.icon}</span>
              <span>{item.label}</span>
            </Link>
          ))}
        </nav>

        <div className="ia-sidebar__footer">
          <Link
            href="/app/profile"
            className={`ia-sidebar__link ${isActive(pathname, "/app/profile") ? "ia-sidebar__link--active" : ""}`}
          >
            <span className="ia-sidebar__icon" aria-hidden="true">⊙</span>
            <span>{user?.display_name ?? "Profile"}</span>
          </Link>
          <Link
            href="/app/settings"
            className={`ia-sidebar__link ${isActive(pathname, "/app/settings") ? "ia-sidebar__link--active" : ""}`}
          >
            <span className="ia-sidebar__icon" aria-hidden="true">⚙</span>
            <span>Settings</span>
          </Link>
          <button
            type="button"
            className="ia-sidebar__link ia-sidebar__link--muted"
            onClick={logout}
          >
            <span className="ia-sidebar__icon" aria-hidden="true">→</span>
            <span>Sign out</span>
          </button>
        </div>
      </aside>

      {/* Mobile top bar */}
      <header className="ia-topbar">
        <Link href="/app" className="ia-topbar__brand">iCONFESS</Link>
        <div className="ia-topbar__actions">
          <Link href="/app/notifications" aria-label="Notifications" className="ia-topbar__btn">🔔</Link>
          <Link href="/app/profile" aria-label="Profile" className="ia-topbar__btn">⊙</Link>
        </div>
      </header>

      {/* Main content */}
      <main className="ia-main" id="main">
        {children}
      </main>

      {/* Mobile bottom navigation — exactly five, per §12 */}
      <nav className="ia-bottom" aria-label="Bottom navigation">
        {BOTTOM_TABS.map((tab) => (
          <Link
            key={tab.href}
            href={tab.href}
            className={`ia-bottom__tab ${isActive(pathname, tab.href) ? "ia-bottom__tab--active" : ""}`}
            aria-current={isActive(pathname, tab.href) ? "page" : undefined}
          >
            <span className="ia-bottom__icon" aria-hidden="true">{tab.icon}</span>
            <span className="ia-bottom__label">{tab.label}</span>
          </Link>
        ))}
      </nav>

      {/* Persistent mini-player — reflects the single provider element. */}
      <MiniPlayer />
      <ConsentBanner />
    </div>
  );
}
