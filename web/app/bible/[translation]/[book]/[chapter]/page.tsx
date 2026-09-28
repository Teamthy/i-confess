import type { Metadata } from "next";
import { BibleReader } from "@/components/bible-reader";
import { bookByID, displayRef } from "@/lib/canon";

/* /bible/{translation}/{book}/{chapter} — the canonical deep link. It is the
   URL a share produces, the one the mobile app opens and the one the reader
   keeps in the address bar while you read. */

type Props = { params: Promise<{ translation: string; book: string; chapter: string }> };

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { translation, book, chapter } = await params;
  const canonical = bookByID(decodeURIComponent(book));
  const number = Number(chapter);
  const reference = canonical && number ? displayRef(canonical.id, number) : "Bible";
  return {
    title: `${reference}${translation && translation !== "-" ? ` — ${translation.toUpperCase()}` : ""}`,
    description: canonical
      ? `Read ${reference} in the iCONFESS Bible reader: highlight, bookmark and follow the verse back to the confessions written from it.`
      : "Read Scripture in the iCONFESS Bible reader.",
    alternates: {
      canonical: `/bible/${encodeURIComponent(translation)}/${encodeURIComponent(book)}/${chapter}`,
    },
    openGraph: { title: reference, type: "article" },
  };
}

export default async function Page({ params }: Props) {
  const { translation, book, chapter } = await params;
  const number = Number(chapter);
  return (
    <BibleReader
      initialTranslation={decodeURIComponent(translation)}
      initialBook={decodeURIComponent(book)}
      initialChapter={Number.isFinite(number) && number > 0 ? number : 1}
    />
  );
}
