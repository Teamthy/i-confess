import type { Metadata } from "next";
import { BibleReader } from "@/components/bible-reader";
import { bookByID } from "@/lib/canon";

type Props = { params: Promise<{ translation: string; book: string }> };

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { translation, book } = await params;
  const canonical = bookByID(decodeURIComponent(book));
  return {
    title: canonical ? canonical.name : "Bible",
    description: canonical
      ? `${canonical.name} — ${canonical.chapter_count} chapters in the iCONFESS Bible reader.`
      : "Read Scripture in the iCONFESS Bible reader.",
    alternates: { canonical: `/bible/${encodeURIComponent(translation)}/${encodeURIComponent(book)}` },
  };
}

export default async function Page({ params }: Props) {
  const { translation, book } = await params;
  return <BibleReader initialTranslation={decodeURIComponent(translation)} initialBook={decodeURIComponent(book)} initialChapter={1} />;
}
