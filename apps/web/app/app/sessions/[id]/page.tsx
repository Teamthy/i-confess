/** One session — its queue and state, from GET /sessions/{id}. Playback lives in the player. */
import Link from "next/link";
import { SessionDetailClient } from "@/components/SessionDetailClient";

export default async function SessionDetailPage({ params }: { params: Promise<{ id: string }> }) {
  // The id is resolved on the server so the client island needs no dynamic
  // read of its own (same shape as the player page).
  const { id } = await params;

  return (
    <div className="ia-page">
      <Link href="/app/sessions" className="ic-backlink" style={{ color: "var(--ic-color-neutral-500)" }}>← Sessions</Link>
      <SessionDetailClient sessionId={id} />
    </div>
  );
}
