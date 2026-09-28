import type { Metadata } from "next";
import { BibleComparePage } from "@/components/bible-pages";

export const metadata: Metadata = {
  title: "Compare translations",
  description: "Read one passage in two to four approved Bible translations side by side.",
  alternates: { canonical: "/bible/compare" },
};

type Props = { searchParams: Promise<{ reference?: string }> };

export default async function Page({ searchParams }: Props) {
  const { reference } = await searchParams;
  return <BibleComparePage initialReference={reference} />;
}
