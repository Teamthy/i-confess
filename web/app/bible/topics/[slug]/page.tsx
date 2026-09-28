import type { Metadata } from "next";
import { BibleTopicPage } from "@/components/bible-pages";

type Props = { params: Promise<{ slug: string }> };

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { slug } = await params;
  const name = slug.replace(/-/g, " ");
  return {
    title: `Bible verses about ${name}`,
    description: `The passages the reviewed iCONFESS corpus stands on for ${name}.`,
    alternates: { canonical: `/bible/topics/${encodeURIComponent(slug)}` },
  };
}

export default async function Page({ params }: Props) {
  const { slug } = await params;
  return <BibleTopicPage slug={slug} />;
}
