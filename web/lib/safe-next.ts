/**
 * A same-origin path from `?next=`, else "/app".
 *
 * Browsers read a backslash as a slash, so "/\evil.com" means "//evil.com"
 * (another host). A string prefix check alone is how open redirects happen,
 * so the value is rejected if it holds a backslash, then resolved against
 * this origin and kept only if it stays here.
 */
export function safeNext(
  search: string,
  origin: string = typeof window === "undefined" ? "http://localhost" : window.location.origin,
): string {
  const raw = new URLSearchParams(search).get("next") || "";
  if (!raw.startsWith("/") || raw.startsWith("//") || raw.includes("\\")) return "/app";
  try {
    const u = new URL(raw, origin);
    if (u.origin !== origin) return "/app";
    return u.pathname + u.search + u.hash;
  } catch {
    return "/app";
  }
}
