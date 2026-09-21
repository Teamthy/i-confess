/** Today's experience — a confession chosen for this moment. */

export default function DailyPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Today</h1>
        <p>A confession chosen for this day. Return tomorrow for a new one.</p>
      </div>
      <div className="ic-state">
        <h3>Start your daily practice</h3>
        <p>Your daily confession will appear here once you begin listening. One confession, once a day — a ritual, not a feed.</p>
        <a href="/app/categories" className="ic-btn ic-btn--secondary">Choose a category</a>
      </div>
    </div>
  );
}
