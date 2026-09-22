"use client";

/**
 * View-event islands for server-rendered app pages (§51).
 *
 * Renders nothing. Fires once per mount when signed in and consented —
 * the analytics module itself enforces both, so these islands stay dumb.
 */

import { useEffect, useRef } from "react";
import { useAuth } from "@/lib/auth-context";
import { ANALYTICS_EVENTS, track } from "@/lib/analytics";

export function TrackCategoryView({ categoryId }: { categoryId: string }) {
  const { token } = useAuth();
  const fired = useRef(false);
  useEffect(() => {
    if (!fired.current && token) {
      fired.current = true;
      track(token, ANALYTICS_EVENTS.categoryViewed, { category_id: categoryId });
    }
  }, [token, categoryId]);
  return null;
}
