import React from "react";
import Link from "next/link";
import type { Metadata } from "next";
import { Mark } from "@/components/ui";

/* The admin console shares this app, its design system and its session with
   the public site — it is a section, not a second product. It is never
   indexed, and the API authorises every request it makes. */

export const metadata: Metadata = {
  title: { default: "Admin console", template: "%s — iCONFESS admin" },
  robots: { index: false, follow: false },
};

const NAV: [string, string][] = [
  ["/admin", "Overview"],
  ["/admin/content", "Content"],
  ["/admin/bible", "Bible"],
  ["/admin/moderation", "Moderation"],
  ["/admin/users", "People"],
  ["/admin/audit", "Audit"],
];

export default function AdminLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="adm">
      <header className="adm-bar">
        <Link className="adm-brand" href="/">
          <Mark s={24} />
          <span>
            iCONFESS <b>admin</b>
          </span>
        </Link>
        <nav className="adm-nav" aria-label="Admin sections">
          {NAV.map(([href, label]) => (
            <Link key={href} href={href}>
              {label}
            </Link>
          ))}
        </nav>
        <Link className="adm-exit" href="/app">
          Back to the app
        </Link>
      </header>
      <main className="adm-main" id="main">
        {children}
      </main>
    </div>
  );
}
