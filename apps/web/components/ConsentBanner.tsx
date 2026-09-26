"use client";

/**
 * Analytics consent banner.
 *
 * Shown once, inside the app shell, only to signed-in listeners who have
 * not decided. Accepting enables ID-only product analytics (no content ever
 * leaves the device — see lib/analytics.ts); declining disables it. The
 * decision is recorded server-side and mirrored locally.
 */

import { useAuth } from "@/lib/auth-context";
import { recordConsent, useConsent } from "@/lib/analytics";

export function ConsentBanner() {
  const { token } = useAuth();
  const [consent, setConsent] = useConsent();

  if (!token || consent !== null) return null;

  async function decide(granted: boolean) {
    setConsent(granted);
    await recordConsent(token, granted);
  }

  return (
    <div className="ia-consent" role="dialog" aria-label="Analytics consent" aria-live="polite">
      <p>
        <strong>Help improve iCONFESS?</strong> Anonymous listening counts only —
        never your words, never Scripture text. You can change this in Settings.
      </p>
      <div className="ia-consent__actions">
        <button type="button" className="ic-btn ic-btn--primary ic-btn--small" onClick={() => void decide(true)}>
          Allow
        </button>
        <button type="button" className="ic-btn ic-btn--secondary ic-btn--small" onClick={() => void decide(false)}>
          Not now
        </button>
      </div>
    </div>
  );
}
