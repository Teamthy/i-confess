/** Motion, contrast, and screen reader preferences. */

export default function AccessibilitySettingsPage() {
  return (
    <div className="ia-page">
      <a href="/app/settings" className="ic-backlink" style={{ color: "var(--ic-color-neutral-500)" }}>← Settings</a>
      <div className="ia-page__head">
        <h1>Accessibility</h1>
        <p>iCONFESS aims for WCAG 2.2 AA across every surface. These controls are for preferences that go beyond the standard.</p>
      </div>
      <div className="ia-settings">
        <div className="ia-settings__group">
          <h2>Motion</h2>
          <div className="ia-toggle">
            <div><p style={{ fontWeight: "var(--ic-font-weight-medium)" }}>Reduce motion</p><p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-500)" }}>Minimise animations. Respects your system setting by default.</p></div>
            <div className="ia-toggle__track" role="switch" aria-checked="false" tabIndex={0}><div className="ia-toggle__thumb" /></div>
          </div>
        </div>
        <div className="ia-settings__group">
          <h2>Text</h2>
          <div className="ic-field">
            <label htmlFor="text-size">Text size</label>
            <select id="text-size" defaultValue="default">
              <option value="default">Default</option>
              <option value="large">Large</option>
              <option value="x-large">Extra large</option>
            </select>
          </div>
        </div>
      </div>
    </div>
  );
}
