/** Sign-in credentials. Identity fields live on the profile page — one editor per fact. */
import Link from "next/link";
import { PasswordChangeClient } from "@/components/PasswordChangeClient";

export default function AccountSettingsPage() {
  return (
    <div className="ia-page">
      <Link href="/app/settings" className="ic-backlink" style={{ color: "var(--ic-color-neutral-500)" }}>
        ← Settings
      </Link>
      <div className="ia-page__head">
        <h1>Account</h1>
        <p>Your password and sign-in security.</p>
      </div>
      <div className="ia-settings" style={{ maxWidth: "40rem" }}>
        <div className="ia-settings__group">
          <h2>Your name, photo and details</h2>
          <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", margin: "0 0 var(--ic-spacing-4)" }}>
            Those are profile fields — they are edited in one place so every surface agrees.
          </p>
          <Link href="/app/profile" className="ic-btn ic-btn--secondary" style={{ width: "fit-content" }}>
            Edit profile
          </Link>
        </div>
        <div className="ia-settings__group">
          <h2>Change password</h2>
          <PasswordChangeClient />
        </div>
      </div>
    </div>
  );
}
