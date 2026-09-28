/**
 * The API client for server components.
 *
 * Server components call the Go API directly over the loopback (IC_API_URL),
 * which keeps the API origin off the client. Every helper degrades to a typed
 * error result instead of throwing, so pages can render their real error
 * state rather than a crash boundary.
 *
 * NOTE: the token "tagline" referenced by the category seed of record
 * (server/internal/seed/categories_canonical.go) is not yet served by
 * GET /categories — only name/slug/description/icon are. The site therefore
 * renders the served `description` and does not hardcode taglines. That is a
 * recorded backend gap, not a content decision (docs/44-WEB-PLATFORM-AUDIT.md §5).
 */

export const API_URL = process.env.IC_API_URL || "http://127.0.0.1:8080";

/** One published category of life, as served by GET /categories. */
export type Category = {
  id: string;
  name: string;
  slug: string;
  description?: string;
  icon?: string;
  premium: boolean;
  status: string;
  sort_order: number;
  updated_at: string;
};

/** One canonical confession, as served by GET /confessions/{id} and category listings. */
export type Confession = {
  id: string;
  category_id: string;
  title: string;
  short_text?: string;
  medium_text?: string;
  long_text?: string;
  description?: string;
  tags?: string[];
  intensity: number;
  language: string;
  status: string;
  author?: string;
  version: number;
  published_at?: string;
  created_at: string;
  updated_at: string;
  variants?: { id: string; label: string; duration_seconds: number }[];
  scriptures?: {
    id: string;
    book: string;
    chapter?: number;
    verse?: string;
    translation: string;
    is_direct_quote: boolean;
  }[];
};

/** One narration voice, as served by GET /voices. */
export type Voice = {
  id: string;
  name: string;
  description?: string;
  type: string;
  provider?: string;
  gender?: string;
  language: string;
  premium: boolean;
  status: string;
  sample_url?: string;
};

export type ApiError = { ok: false; status: number; message: string };
export type ApiOk<T> = { ok: true; data: T };
export type ApiResult<T> = ApiOk<T> | ApiError;

/** Human-readable message for a failed fetch. Never leaks internals (§50). */
function messageFor(status: number): string {
  if (status === 404) return "We couldn't find that experience.";
  if (status === 401 || status === 403) return "You need to sign in to continue.";
  if (status === 429) return "Too many requests — please wait a moment.";
  if (status >= 500) return "Something went wrong while loading this experience.";
  return "Something went wrong. Please try again.";
}

async function get<T>(path: string, revalidate = 120): Promise<ApiResult<T>> {
  try {
    const res = await fetch(`${API_URL}${path}`, {
      next: { revalidate },
      headers: { accept: "application/json" },
    });
    if (!res.ok) return { ok: false, status: res.status, message: messageFor(res.status) };
    return { ok: true, data: (await res.json()) as T };
  } catch {
    return { ok: false, status: 0, message: "Something went wrong while loading this experience." };
  }
}

export const api = {
  categories: () => get<Category[]>("/categories", 300),
  category: (slugOrId: string): Promise<ApiResult<Category>> =>
    get<Category[]>("/categories").then((r) => {
      if (!r.ok) return r;
      const found = r.data.find((c) => c.slug === slugOrId || c.id === slugOrId);
      return found
        ? ({ ok: true, data: found } as ApiOk<Category>)
        : ({ ok: false, status: 404, message: messageFor(404) } as ApiError);
    }),
  /** The backend serves confessions by category id; resolve the id from the slug first. */
  categoryConfessions: (categoryId: string) =>
    get<Confession[]>(`/categories/${categoryId}/confessions`, 120),
  confession: (id: string) => get<Confession>(`/confessions/${id}`, 120),
  voices: () => get<Voice[]>("/voices", 300),
  search: (q: string) =>
    get<{ count: number; results: { id: string; type: string; title: string; description?: string }[] }>(
      `/search?q=${encodeURIComponent(q)}&limit=20`,
      0,
    ),
  communityConfessions: () =>
    get<{ confessions: Record<string, unknown>[] }>("/community/confessions", 60),
  communityFeed: () => get<Record<string, unknown>[]>("/community/feed", 60),
};
