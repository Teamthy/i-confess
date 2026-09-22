/** Offline listening — time-bounded download licences, from GET /me/downloads. */
import { DownloadsClient } from "@/components/DownloadsClient";

export default function DownloadsPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Downloads</h1>
        <p>Confessions saved for offline listening, each for as long as its licence runs.</p>
      </div>
      <DownloadsClient />
    </div>
  );
}
