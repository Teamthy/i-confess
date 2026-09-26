/** Search across confessions, categories, collections, and voices — GET /search. */
import { SearchClient } from "@/components/SearchClient";

export default function SearchPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Search</h1>
        <p>Find confessions, categories, collections, and voices across the library.</p>
      </div>
      <SearchClient />
    </div>
  );
}
