import type { Metadata } from "next";
import { BibleSearchPage } from "@/components/bible-pages";

export const metadata: Metadata = {
  title: "Search Scripture",
  description: "Search the Bible by phrase or open a reference — John 3:16, Psalm 23, 1 Corinthians 13:4-7.",
  alternates: { canonical: "/bible/search" },
};

type Props = { searchParams: Promise<{ q?: string; translation?: string }> };

export default async function Page({ searchParams }: Props) {
  const { q, translation } = await searchParams;
  return <BibleSearchPage initialQuery={q} initialTranslation={translation} />;
}
