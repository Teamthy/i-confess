/** Notification preferences and history. */

export default function NotificationsPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Notifications</h1>
        <p>Manage your notification preferences. Choose what you want to hear about.</p>
      </div>
      <div className="ia-settings">
        <div className="ia-settings__group">
          <h2>Push notifications</h2>
          <div className="ia-toggle">
            <div>
              <p style={{ fontWeight: "var(--ic-font-weight-medium)" }}>Daily reminder</p>
              <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-500)" }}>A gentle nudge at your preferred time.</p>
            </div>
            <div className="ia-toggle__track" role="switch" aria-checked="false" tabIndex={0}>
              <div className="ia-toggle__thumb" />
            </div>
          </div>
          <div className="ia-toggle">
            <div>
              <p style={{ fontWeight: "var(--ic-font-weight-medium)" }}>New content</p>
              <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-500)" }}>When new confessions arrive in your categories.</p>
            </div>
            <div className="ia-toggle__track" role="switch" aria-checked="true" tabIndex={0}>
              <div className="ia-toggle__thumb" />
            </div>
          </div>
          <div className="ia-toggle">
            <div>
              <p style={{ fontWeight: "var(--ic-font-weight-medium)" }}>Community</p>
              <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-500)" }}>Reactions and replies to your confessions.</p>
            </div>
            <div className="ia-toggle__track" role="switch" aria-checked="true" tabIndex={0}>
              <div className="ia-toggle__thumb" />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
