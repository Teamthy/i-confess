/** Build a session from categories, duration, and voice. */
import { api } from "@/lib/api";
export default async function SessionBuilderPage() {
  const [catsRes, voicesRes] = await Promise.all([
    api.categories(),
    api.voices(),
  ]);
  const categories = catsRes.ok ? catsRes.data : [];
  const voices = voicesRes.ok ? voicesRes.data : [];

  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Build a session</h1>
        <p>Choose an area of life, a duration, and a voice. iCONFESS assembles the experience.</p>
      </div>
      <div className="ia-settings" style={{ maxWidth: "36rem" }}>
        <div className="ia-settings__group">
          <div className="ic-field">
            <label htmlFor="sb-category">Category</label>
            <select id="sb-category">
              <option value="">Choose an area of life</option>
              {categories.map((c) => (
                <option key={c.id} value={c.id}>{c.name}</option>
              ))}
            </select>
          </div>
          <div className="ic-field">
            <label htmlFor="sb-duration">Duration</label>
            <select id="sb-duration" defaultValue="300">
              <option value="180">3 minutes</option>
              <option value="300">5 minutes</option>
              <option value="600">10 minutes</option>
              <option value="900">15 minutes</option>
            </select>
          </div>
          <div className="ic-field">
            <label htmlFor="sb-voice">Voice</label>
            <select id="sb-voice">
              <option value="">Default</option>
              {voices.map((v) => (
                <option key={v.id} value={v.id}>{v.name}</option>
              ))}
            </select>
          </div>
          <button type="button" className="ic-btn ic-btn--primary" style={{ width: "fit-content" }}>Start session</button>
        </div>
      </div>
    </div>
  );
}
