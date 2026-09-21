/** Email, password, display name. */

export default function AccountSettingsPage() {
  return (
    <div className="ia-page">
      <a href="/app/settings" className="ic-backlink" style={{ color: "var(--ic-color-neutral-500)" }}>← Settings</a>
      <div className="ia-page__head">
        <h1>Account</h1>
        <p>Your email, password, and display name.</p>
      </div>
      <div className="ia-settings">
        <div className="ia-settings__group">
          <h2>Display name</h2>
          <div className="ic-field">
            <label htmlFor="display-name">Name</label>
            <input id="display-name" type="text" placeholder="Your display name" />
          </div>
          <button type="button" className="ic-btn ic-btn--primary" style={{ width: "fit-content" }}>Save</button>
        </div>
        <div className="ia-settings__group">
          <h2>Change password</h2>
          <div className="ic-field">
            <label htmlFor="current-pw">Current password</label>
            <input id="current-pw" type="password" />
          </div>
          <div className="ic-field">
            <label htmlFor="new-pw">New password</label>
            <input id="new-pw" type="password" />
          </div>
          <button type="button" className="ic-btn ic-btn--primary" style={{ width: "fit-content" }}>Update password</button>
        </div>
      </div>
    </div>
  );
}
