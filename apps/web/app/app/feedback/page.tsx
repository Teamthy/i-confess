/** Send feedback to the iCONFESS team. */

export default function FeedbackPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Feedback</h1>
        <p>Your voice matters. Tell us what is working and what could be better.</p>
      </div>
      <div className="ia-settings" style={{ maxWidth: "36rem" }}>
        <div className="ia-settings__group">
          <div className="ic-field">
            <label htmlFor="feedback-subject">Subject</label>
            <input id="feedback-subject" type="text" placeholder="What is this about?" />
          </div>
          <div className="ic-field">
            <label htmlFor="feedback-body">Your feedback</label>
            <textarea id="feedback-body" placeholder="Tell us what you think…" />
          </div>
          <button type="button" className="ic-btn ic-btn--primary" style={{ width: "fit-content" }}>Send feedback</button>
        </div>
      </div>
    </div>
  );
}
