/** Readable source text, not timed captions. Do not infer sentence timing from
 * total track progress: only verified, asset-bound timings can drive highlighting. */
export function TranscriptText({ text, enabled }: { text: string; enabled: boolean }) {
  if (!enabled) {
    return <p className="ps-text">Text is hidden. Use the text control to show it.</p>;
  }
  if (!text.trim()) {
    return <p className="ps-text">Text is not available for this item.</p>;
  }
  return <div className="player-transcript" aria-label="Text for this item">
    <p className="ps-text transcript-text">{text}</p>
    <p className="transcript-note">Text is not synchronized and may differ from the recording.</p>
  </div>;
}
