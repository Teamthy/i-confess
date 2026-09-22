/** Listening history — your playback record, from GET /me/history. */
import { HistoryClient } from "@/components/HistoryClient";

export default function HistoryPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>History</h1>
        <p>Your listening history. Every confession you have heard, in the order you heard it.</p>
      </div>
      <HistoryClient />
    </div>
  );
}
