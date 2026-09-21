/** Standalone audio player wrapping the existing player component. */

export default function PlayerPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head" style={{ textAlign: "center" }}>
        <h1>Player</h1>
        <p>The immersive audio player will appear here when a confession is playing.</p>
      </div>
      <div className="ic-state">
        <h3>Nothing playing</h3>
        <p>Choose a confession and press play to start the experience.</p>
        <a href="/app/explore" className="ic-btn ic-btn--secondary">Find a confession</a>
      </div>
    </div>
  );
}
