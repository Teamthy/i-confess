/** Morning, midday, and evening routines. Built from templates. */

export default function RoutinesPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Routines</h1>
        <p>Set up morning, midday, and evening rituals. Built from your templates and schedules.</p>
      </div>
      <div className="ic-state">
        <h3>No routines yet</h3>
        <p>Create a template and schedule it to build a routine.</p>
        <a href="/app/session-builder" className="ic-btn ic-btn--secondary">Build a session</a>
      </div>
    </div>
  );
}
