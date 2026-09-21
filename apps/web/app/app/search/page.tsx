/** Global search across confessions, categories, voices, and scripture. */

export default function SearchPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Search</h1>
        <p>Find confessions, categories, voices, and scripture across the library.</p>
      </div>
      <div style={{ maxWidth: "40rem" }}>
        <div className="ic-field">
          <label htmlFor="search-input">Search the library</label>
          <input id="search-input" type="search" placeholder="Try peace, healing, purpose…" autoComplete="off" />
        </div>
      </div>
      <div className="ic-state">
        <h3>Start typing to search</h3>
        <p>Search across 39 categories, confessions, voices, and scripture.</p>
      </div>
    </div>
  );
}
