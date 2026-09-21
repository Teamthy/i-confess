/** Listening history — your playback record. */

export default function HistoryPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>History</h1>
        <p>Your listening history. Every confession you have heard, in the order you heard it.</p>
      </div>
      <div className="ic-state">
        <h3>Your history will appear here</h3>
        <p>Start listening to confessions and your history will build over time.</p>
        <a href="/app/explore" className="ic-btn ic-btn--secondary">Explore confessions</a>
      </div>
    </div>
  );
}
