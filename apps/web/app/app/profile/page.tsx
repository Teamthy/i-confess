/** Your account profile. */

export default function ProfilePage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Profile</h1>
        <p>Your account details and public identity.</p>
      </div>
      <div className="ia-settings">
        <div className="ia-settings__group">
          <h2>Account</h2>
          <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)" }}>
            Sign in to view and edit your profile details.
          </p>
          <a href="/app/settings" className="ic-btn ic-btn--secondary" style={{ width: "fit-content" }}>Go to settings</a>
        </div>
      </div>
    </div>
  );
}
