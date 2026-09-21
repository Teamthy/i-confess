/** Audio speed, auto-play, and quality. */

export default function PlaybackSettingsPage() {
  return (
    <div className="ia-page">
      <a href="/app/settings" className="ic-backlink" style={{ color: "var(--ic-color-neutral-500)" }}>← Settings</a>
      <div className="ia-page__head">
        <h1>Playback</h1>
        <p>Control how audio plays.</p>
      </div>
      <div className="ia-settings">
        <div className="ia-settings__group">
          <h2>Audio</h2>
          <div className="ia-toggle">
            <div><p style={{ fontWeight: "var(--ic-font-weight-medium)" }}>Auto-play next</p><p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-500)" }}>Continue to the next confession in a session.</p></div>
            <div className="ia-toggle__track" role="switch" aria-checked="true" tabIndex={0}><div className="ia-toggle__thumb" /></div>
          </div>
          <div className="ic-field">
            <label htmlFor="playback-speed">Default speed</label>
            <select id="playback-speed" defaultValue="1">
              <option value="0.75">0.75×</option>
              <option value="1">1× (normal)</option>
              <option value="1.25">1.25×</option>
              <option value="1.5">1.5×</option>
            </select>
          </div>
        </div>
      </div>
    </div>
  );
}
