/** Saved content — confessions and sessions you have marked. */

export default function FavoritesPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Favorites</h1>
        <p>Confessions and sessions you have saved for easy access.</p>
      </div>
      <div className="ic-state">
        <h3>No favorites yet</h3>
        <p>Save confessions as you explore, and they will appear here.</p>
        <a href="/app/explore" className="ic-btn ic-btn--secondary">Explore confessions</a>
      </div>
    </div>
  );
}
