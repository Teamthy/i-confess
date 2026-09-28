import type { Metadata } from "next";
import { BibleReader } from "@/components/bible-reader";

type Props = { params: Promise<{ translation: string }> };

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { translation } = await params;
  return {
    title: `Bible — ${decodeURIComponent(translation).toUpperCase()}`,
    alternates: { canonical: `/bible/${encodeURIComponent(translation)}` },
  };
}

/* A translation on its own opens the reader in that edition. The book and
   chapter pickers are in the reader's toolbar, so there is no dead landing
   page between choosing an edition and reading it. */
export default async function Page({ params }: Props) {
  const { translation } = await params;
  return <BibleReader initialTranslation={decodeURIComponent(translation)} initialBook="John" initialChapter={1} />;
}
