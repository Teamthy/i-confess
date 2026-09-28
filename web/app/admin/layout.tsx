import React from "react";
import Link from "next/link";
import type { Metadata } from "next";
import { Mark } from "@/components/ui";

export const metadata: Metadata = {
  title: { default: "Admin console", template: "%s — iCONFESS admin" },
  robots: { index: false, follow: false },
};

const NAV_GROUPS: { label: string; items: [string, string, string][] }[] = [
  {
    label: "Platform",
    items: [
      ["/admin", "Overview", "grid"],
      ["/admin/super", "Super Admin", "shield"],
      ["/admin/system", "System Health", "chart"],
    ],
  },
  {
    label: "Content",
    items: [
      ["/admin/content", "Content", "book"],
      ["/admin/bible", "Bible", "book"],
      ["/admin/audio", "Audio & Voices", "mic"],
    ],
  },
  {
    label: "Community",
    items: [
      ["/admin/moderation", "Moderation", "eye"],
      ["/admin/queue", "Job Queue", "clock"],
    ],
  },
  {
    label: "People",
    items: [
      ["/admin/users", "Users", "user"],
      ["/admin/rbac", "Roles & RBAC", "lock"],
    ],
  },
  {
    label: "Business",
    items: [
      ["/admin/pricing", "Pricing", "spark"],
      ["/admin/audit", "Audit Log", "bell"],
      ["/admin/security", "Security", "shield"],
    ],
  },
];

export default function AdminLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="adm-layout">
      <aside className="adm-sidebar">
        <div className="adm-sidebar-head">
          <Link className="adm-brand" href="/">
            <Mark s={28} />
            <span>
              iCONFESS <b>admin</b>
            </span>
          </Link>
          <span className="adm-badge">SUPER ADMIN CONSOLE</span>
        </div>

        <nav className="adm-sidebar-nav" aria-label="Admin sections">
          {NAV_GROUPS.map((group) => (
            <div key={group.label} className="adm-nav-group">
              <span className="adm-nav-label">{group.label}</span>
              {group.items.map(([href, label]) => (
                <Link key={href} href={href} className="adm-nav-item">
                  <span className="adm-nav-dot" />
                  {label}
                </Link>
              ))}
            </div>
          ))}
        </nav>

        <div className="adm-sidebar-foot">
          <Link className="adm-exit" href="/app">
            Back to the app
          </Link>
          <p className="adm-footnote">
            Every action is audited. RBAC is enforced server-side. Super admin has full access.
          </p>
        </div>
      </aside>

      <div className="adm-content">
        <header className="adm-topbar">
          <div className="adm-topbar-left">
            <span className="adm-topbar-title">Admin Console</span>
            <span className="adm-topbar-sub">RBAC · Audit · System · Content</span>
          </div>
          <div className="adm-topbar-right">
            <Link href="/admin/super" className="adm-topbar-badge">
              Super Admin Mode
            </Link>
          </div>
        </header>
        <main className="adm-main" id="main">
          {children}
        </main>
      </div>
    </div>
  );
}
