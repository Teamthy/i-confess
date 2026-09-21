/** Resolve a shared template or confession. */

export default async function SharedPage({ params }: { params: Promise<{ token: string }> }) {
  const { token } = await params;
  return (
    <div className="ia-page">
      <div className="ia-page__head" style={{ textAlign: "center" }}>
        <h1>Shared with you</h1>
        <p>Someone shared an iCONFESS experience with you.</p>
      </div>
      <div className="ic-state">
        <h3>Loading shared content</h3>
        <p>Resolving shared link.</p>
      </div>
    </div>
  );
}
