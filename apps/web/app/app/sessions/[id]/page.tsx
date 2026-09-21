/** One session with playback and progress. */
import Link from "next/link";
export default async function SessionDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return (
    <div className="ia-page">
      <Link href="/app/sessions" className="ic-backlink" style={{ color: "var(--ic-color-neutral-500)" }}>← Sessions</Link>
      <div className="ia-page__head" style={{ textAlign: "center" }}>
        <h1>Session</h1>
        <p>This session is loading from the API. The immersive player surface will render here.</p>
      </div>
      <div className="ic-state">
        <h3>Session loading</h3>
        <p>Session ID: {id}</p>
      </div>
    </div>
  );
}
