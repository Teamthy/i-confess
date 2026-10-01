"use client";
import { useCallback, useEffect, useState } from "react";
import { appApi, type AppApiFail } from "./app-api";
import type { Voice, Category } from "./api";

/** Public catalogues must work signed-out, but a failed API is never a licence
 * to fabricate available voices. Device speech is a separate, labelled path. */
export function usePublicCatalogue<T>(path: string) {
  const [state, setState] = useState<{ loading: boolean; data: T | null; error: AppApiFail | null }>({ loading: true, data: null, error: null });
  const [nonce, setNonce] = useState(0);
  useEffect(() => {
    const ac = new AbortController();
    setState({ loading: true, data: null, error: null });
    void appApi<T>(path, { signal: ac.signal }).then((r) => {
      if (ac.signal.aborted) return;
      setState(r.ok ? { loading: false, data: r.data, error: null } : { loading: false, data: null, error: r });
    });
    return () => ac.abort();
  }, [path, nonce]);
  const reload = useCallback(() => setNonce((n) => n + 1), []);
  return { ...state, reload };
}
export function useVoiceCatalogue() { return usePublicCatalogue<Voice[]>("/voices"); }
export function useCategoryCatalogue() { return usePublicCatalogue<Category[]>("/categories"); }
export function preferredVoiceId(voices: Voice[], preference: string): string {
  return voices.find((v) => v.id === preference || v.name.toLowerCase() === preference.toLowerCase())?.id || preference;
}
export function playableSample(url: string | undefined): boolean {
  // The API supplies signed same-origin/CDN links, never private object keys.
  return !!url && (/^https?:\/\//i.test(url) || /^\/media\//.test(url));
}
