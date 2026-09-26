/** Saved content — confessions, categories, voices and sessions you have marked. */
import { FavoritesClient } from "@/components/FavoritesClient";

export default function FavoritesPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Favorites</h1>
        <p>Everything you have saved — confessions, categories, voices, and sessions — in one place.</p>
      </div>
      <FavoritesClient />
    </div>
  );
}
