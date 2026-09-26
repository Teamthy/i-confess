/** Settings — playback. Backed by GET/PATCH /me/preferences. */
import Link from "next/link";
import { PreferencesClient } from "@/components/PreferencesClient";

export default function Page() {
  return (
    <div className="ia-page">
      <Link href="/app/settings" className="ic-backlink" style={{ color: "var(--ic-color-neutral-500)" }}>
        ← Settings
      </Link>
      <div className="ia-page__head">
        <h1>Playback</h1>
        <p>How audio behaves by default.</p>
      </div>
      <PreferencesClient section="playback" />
    </div>
  );
}
