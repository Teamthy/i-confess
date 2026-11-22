/* Server components only. The published confession library, read from the API (GET /confessions) by
   server components. The bundled copy in lib/confs.json is used only when the
   API cannot be reached at all — never to fill in a library the server says
   is empty, which would publish words the editors have not released. */
import { API_URL } from "./api";
import { CONFESSIONS, type Confession, type Scripture } from "./data";

type Entry = { slug: string; title: string; category: string; short_text: string; medium_text: string; long_text: string; intensity: number; scriptures: Scripture[] | null };

export type Library = { confessions: Confession[]; source: "api" | "bundled" };

export async function getLibrary(): Promise<Library> {
  try {
    const res = await fetch(`${API_URL}/confessions`, { next: { revalidate: 60 }, signal: AbortSignal.timeout(4000) });
    if (!res.ok) throw new Error(String(res.status));
    const data = (await res.json()) as { confessions: Entry[] };
    return {
      source: "api",
      confessions: data.confessions.map((e) => ({
        slug: e.slug, title: e.title, category: e.category, intensity: e.intensity,
        short: e.short_text, medium: e.medium_text || e.short_text, long: e.long_text || e.medium_text || e.short_text,
        scriptures: e.scriptures || [],
      })),
    };
  } catch {
    return { source: "bundled", confessions: CONFESSIONS };
  }
}
