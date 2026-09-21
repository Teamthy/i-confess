/** Data visibility and deletion. */

export default function PrivacySettingsPage() {
  return (
    <div className="ia-page">
      <a href="/app/settings" className="ic-backlink" style={{ color: "var(--ic-color-neutral-500)" }}>← Settings</a>
      <div className="ia-page__head">
        <h1>Privacy</h1>
        <p>Your data, your visibility, and your right to leave.</p>
      </div>
      <div className="ia-settings">
        <div className="ia-settings__group">
          <h2>Content visibility</h2>
          <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)" }}>
            Your confessions are private by default. You choose what to share.
          </p>
        </div>
        <div className="ia-settings__group">
          <h2>Your data</h2>
          <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", marginBottom: "var(--ic-spacing-4)" }}>
            Download everything we hold about you, or request account deletion.
          </p>
          <div style={{ display: "flex", gap: "var(--ic-spacing-3)", flexWrap: "wrap" }}>
            <a href="/api/me/export" className="ic-btn ic-btn--secondary">Download my data</a>
            <button type="button" className="ic-btn ic-btn--secondary" style={{ color: "var(--ic-color-semantic-danger-light)", borderColor: "var(--ic-color-semantic-danger-light)" }}>Delete account</button>
          </div>
        </div>
      </div>
    </div>
  );
}
