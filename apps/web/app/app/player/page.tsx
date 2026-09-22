/** The immersive player. Opens a session built by POST /sessions, from a resume link, or from history. */
import { PlayerClient } from "@/components/PlayerClient";

export default async function PlayerPage({
  searchParams,
}: {
  searchParams: Promise<{ session?: string }>;
}) {
  // The session id is resolved on the server (a page-level `await` of
  // searchParams), so the client island needs no dynamic read of its own.
  const sp = await searchParams;
  const sessionId = typeof sp.session === "string" && sp.session.length > 0 ? sp.session : null;

  return (
    <div className="ia-page">
      <div className="ia-page__head" style={{ textAlign: "center" }}>
        <p className="ia-section-title">{sessionId ? "Now playing" : "Player"}</p>
      </div>
      <PlayerClient sessionId={sessionId} />
    </div>
  );
}
