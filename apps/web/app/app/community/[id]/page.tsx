/** One community confession. */
import Link from "next/link";
export default async function CommunityPostPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return (
    <div className="ia-page">
      <Link href="/app/community" className="ic-backlink" style={{ color: "var(--ic-color-neutral-500)" }}>← Community</Link>
      <div className="ia-page__head">
        <h1>Community confession</h1>
        <p>Loading confession from the community feed.</p>
      </div>
      <div className="ic-state">
        <h3>Loading</h3>
        <p>Post ID: {id}</p>
      </div>
    </div>
  );
}
