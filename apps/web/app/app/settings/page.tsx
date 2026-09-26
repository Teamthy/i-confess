/** Account settings hub. */
import Link from "next/link";
const SETTING_LINKS = [
  { href: "/app/settings/account", label: "Account", desc: "Email, password, and display name" },
  { href: "/app/settings/privacy", label: "Privacy", desc: "Data, visibility, and deletion" },
  { href: "/app/settings/notifications", label: "Notifications", desc: "Push, email, and in-app alerts" },
  { href: "/app/settings/playback", label: "Playback", desc: "Audio speed, auto-play, and quality" },
  { href: "/app/settings/accessibility", label: "Accessibility", desc: "Motion, contrast, and screen reader" },
] as const;

export default function SettingsPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Settings</h1>
        <p>Manage your account, privacy, notifications, and preferences.</p>
      </div>
      <div style={{ display: "grid", gap: "var(--ic-spacing-3)" }}>
        {SETTING_LINKS.map((s) => (
          <Link key={s.href} href={s.href} className="ic-card ic-card--hover" style={{ padding: "var(--ic-spacing-5)", textDecoration: "none", display: "flex", justifyContent: "space-between", alignItems: "center" }}>
            <div>
              <h3 style={{ fontWeight: "var(--ic-font-weight-semibold)", fontSize: "var(--ic-font-size-body)" }}>{s.label}</h3>
              <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-500)", marginTop: "var(--ic-spacing-1)" }}>{s.desc}</p>
            </div>
            <span style={{ color: "var(--ic-color-neutral-400)" }}>→</span>
          </Link>
        ))}
      </div>
    </div>
  );
}
