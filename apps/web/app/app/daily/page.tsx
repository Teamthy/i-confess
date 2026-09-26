/**
 * Today's experience — resume what's open, or start with what is suggested.
 * Backed by GET /home and GET /recommendations; nothing here invents a
 * daily pick the backend does not make.
 */
import { DailyClient } from "@/components/DailyClient";

export default function DailyPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Today</h1>
        <p>What the practice already holds for this moment — and where to begin if it holds nothing yet.</p>
      </div>
      <DailyClient />
    </div>
  );
}
