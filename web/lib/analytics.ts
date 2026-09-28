"use client";

/**
 * Product analytics for the authenticated web app (§51).
 *
 * Three hard boundaries, all from the server contract:
 *
 * 1. Authenticated only — POST /analytics/batch stamps the caller from the
 *    session and 401s without one. Signed-out pages never call this module.
 * 2. Allowlisted names only — the server rejects the whole batch on an
 *    unknown name, so the set below mirrors analytics_batch.go exactly.
 *    Marketing-funnel names (homepage_viewed, hero_clicked …) are NOT sent:
 *    the endpoint cannot take them, and inventing a parallel pipeline would
 *    make web and mobile numbers unreconcilable.
 * 3. No content, ever — the prop blocklist mirrors the mobile client's
 *    AnalyticsBlocklist. IDs and counts only.
 *
 * Consent: product analytics are off until the listener accepts the banner
 * in the app shell. The decision is recorded server-side (POST
 * /auth/consent, category product_analytics) and mirrored locally so the
 * banner does not nag.
 */

import { useCallback, useEffect, useState } from "react";
import { appApi } from "@/lib/app-api";

/** Exact mirror of the server batch allowlist. */
export const ANALYTICS_EVENTS = {
  appOpened: "app_opened",
  sessionCreated: "session_created",
  sessionStarted: "session_started",
  sessionCompleted: "session_completed",
  scheduleCreated: "schedule_created",
  downloadCreated: "download_created",
  trialDayViewed: "trial_day_viewed",
  subscriptionVerified: "subscription_verified",
  subscriptionCancelled: "subscription_cancelled",
  searchPerformed: "search_performed",
  categoryViewed: "category_viewed",
  templateCreated: "template_created",
  playbackProgressSynced: "playback_progress_synced",
  trialStarted: "trial_started",
  trialConverted: "trial_converted",
  trialExpired: "trial_expired",
} as const;

type EventName = (typeof ANALYTICS_EVENTS)[keyof typeof ANALYTICS_EVENTS];

/** Props that must never leave the device (mirror of mobile's blocklist). */
const BLOCKED_PROPS = new Set([
  "email", "password", "token", "scripture", "scripture_text", "confession_text",
  "body", "text", "note", "notes", "query", "title", "name", "bio",
]);

const CONSENT_KEY = "ic_analytics_consent";
export const CONSENT_CATEGORY = "product_analytics";
const CONSENT_VERSION = "1";

export function getConsent(): boolean | null {
  try {
    const v = localStorage.getItem(CONSENT_KEY);
    if (v === "granted") return true;
    if (v === "denied") return false;
    return null;
  } catch {
    return null;
  }
}

export async function recordConsent(token: string | null, granted: boolean): Promise<void> {
  try {
    localStorage.setItem(CONSENT_KEY, granted ? "granted" : "denied");
  } catch {
    // Private mode: the banner returns next visit. Annoying, not broken.
  }
  if (token) {
    // Best-effort mirror; a failure here must never block the product.
    await appApi("/auth/consent", {
      token,
      method: "POST",
      body: { category: CONSENT_CATEGORY, granted, version: CONSENT_VERSION },
    }).catch(() => ({ ok: false as const, status: 0, code: "NETWORK", message: "" }));
  }
  window.dispatchEvent(new CustomEvent("iconfess:consent", { detail: granted }));
}

export function useConsent(): [boolean | null, (granted: boolean) => void] {
  const [consent, setConsentState] = useState<boolean | null>(null);
  const [ready, setReady] = useState(false);
  useEffect(() => {
    setConsentState(getConsent());
    setReady(true);
    const onChange = (e: Event) => setConsentState(Boolean((e as CustomEvent).detail));
    window.addEventListener("iconfess:consent", onChange);
    return () => window.removeEventListener("iconfess:consent", onChange);
  }, []);
  const set = useCallback((granted: boolean) => {
    setConsentState(granted);
    window.dispatchEvent(new CustomEvent("iconfess:consent", { detail: granted }));
  }, []);
  return [ready ? consent : null, set];
}

type QueuedEvent = { name: EventName; props?: Record<string, string | number | boolean>; timestamp: string };

const queue: QueuedEvent[] = [];
let flushing = false;
let flushTimer: ReturnType<typeof setTimeout> | null = null;

function scrub(props: Record<string, unknown> | undefined): Record<string, string | number | boolean> | undefined {
  if (!props) return undefined;
  const out: Record<string, string | number | boolean> = {};
  for (const [k, v] of Object.entries(props)) {
    if (BLOCKED_PROPS.has(k)) continue;
    if (typeof v === "string" || typeof v === "number" || typeof v === "boolean") out[k] = v;
  }
  return out;
}

async function flush(token: string): Promise<void> {
  if (flushing || queue.length === 0) return;
  flushing = true;
  try {
    const batch = queue.splice(0, 100);
    await appApi("/analytics/batch", { token, method: "POST", body: { events: batch } }).catch(() => null);
  } finally {
    flushing = false;
    if (queue.length > 0) scheduleFlush(token);
  }
}

function scheduleFlush(token: string): void {
  if (flushTimer) return;
  flushTimer = setTimeout(() => {
    flushTimer = null;
    void flush(token);
  }, 5000);
  if (queue.length >= 10) {
    if (flushTimer) clearTimeout(flushTimer);
    flushTimer = null;
    void flush(token);
  }
}

if (typeof window !== "undefined") {
  window.addEventListener("visibilitychange", () => {
    // Flush on hide with the last known token — the queue is tiny and the
    // call is fire-and-forget; a missed flush is retried next event.
    if (document.visibilityState === "hidden" && lastToken) void flush(lastToken);
  });
}

let lastToken: string | null = null;

/**
 * Queue one event. Silent unless: signed in, consented, allowlisted name.
 * Analytics must never throw, never block, never delay the product.
 */
export function track(token: string | null | undefined, name: EventName, props?: Record<string, unknown>): void {
  try {
    if (!token || getConsent() !== true) return;
    lastToken = token;
    queue.push({ name, props: scrub(props), timestamp: new Date().toISOString() });
    scheduleFlush(token);
  } catch {
    // Analytics is the last thing allowed to break a page.
  }
}
