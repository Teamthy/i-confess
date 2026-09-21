/** Personalised recommendations based on your listening signals. */

export default function RecommendationsPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Recommended for you</h1>
        <p>Based on the areas of life you return to, the sessions you complete, and the time of day you listen.</p>
      </div>
      <div className="ic-state">
        <h3>Build your practice first</h3>
        <p>As you listen to confessions and explore categories, iCONFESS will surface recommendations shaped by what you actually do.</p>
        <a href="/app/explore" className="ic-btn ic-btn--secondary">Start exploring</a>
      </div>
    </div>
  );
}
