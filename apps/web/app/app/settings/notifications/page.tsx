/** Settings — notifications. Backed by GET/PATCH /me/preferences. */
import Link from "next/link";
import { PreferencesClient } from "@/components/PreferencesClient";

export default function Page() {
  return (
    <div className="ia-page">
      <Link href="/app/settings" className="ic-backlink" style={{ color: "var(--ic-color-neutral-500)" }}>
        ← Settings
      </Link>
      <div className="ia-page__head">
        <h1>Notifications</h1>
        <p>What iCONFESS may send you, and over which channel.</p>
      </div>
      <PreferencesClient section="notifications" />
    </div>
  );
}
