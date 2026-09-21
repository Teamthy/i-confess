/** Push, email, and in-app notifications. */

export default function NotificationSettingsPage() {
  return (
    <div className="ia-page">
      <a href="/app/settings" className="ic-backlink" style={{ color: "var(--ic-color-neutral-500)" }}>← Settings</a>
      <div className="ia-page__head">
        <h1>Notifications</h1>
        <p>Choose how and when iCONFESS reaches you.</p>
      </div>
      <div className="ia-settings">
        <div className="ia-settings__group">
          <h2>Email notifications</h2>
          <div className="ia-toggle">
            <div><p style={{ fontWeight: "var(--ic-font-weight-medium)" }}>Weekly digest</p><p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-500)" }}>A summary of your week in confessions.</p></div>
            <div className="ia-toggle__track" role="switch" aria-checked="false" tabIndex={0}><div className="ia-toggle__thumb" /></div>
          </div>
          <div className="ia-toggle">
            <div><p style={{ fontWeight: "var(--ic-font-weight-medium)" }}>Product updates</p><p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-500)" }}>New features and categories.</p></div>
            <div className="ia-toggle__track" role="switch" aria-checked="true" tabIndex={0}><div className="ia-toggle__thumb" /></div>
          </div>
        </div>
      </div>
    </div>
  );
}
