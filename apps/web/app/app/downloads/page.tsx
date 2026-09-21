/** Offline listening — saved confessions for Premium. */

export default function DownloadsPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Downloads</h1>
        <p>Confessions saved for offline listening. A Premium feature.</p>
      </div>
      <div className="ic-state">
        <h3>No downloads</h3>
        <p>Download confessions to listen offline when you are on Premium.</p>
        <a href="/app/explore" className="ic-btn ic-btn--secondary">Explore confessions</a>
      </div>
    </div>
  );
}
