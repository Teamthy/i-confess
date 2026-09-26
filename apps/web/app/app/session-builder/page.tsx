/** Build a session — categories, duration and voice, composed by POST /sessions. */
import { SessionBuilderClient } from "@/components/SessionBuilderClient";

export default async function SessionBuilderPage({
  searchParams,
}: {
  searchParams: Promise<{ category?: string | string[] }>;
}) {
  const sp = await searchParams;
  const raw = sp.category;
  const initial = raw ? (Array.isArray(raw) ? raw : [raw]).slice(0, 12) : [];

  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Build a session</h1>
        <p>Choose areas of life, a duration, and a voice. iCONFESS assembles the experience from published content.</p>
      </div>
      <SessionBuilderClient initialCategoryIds={initial} />
    </div>
  );
}
