import type { Metadata } from "next";
import { BiblePlanPage } from "@/components/bible-pages";

type Props = { params: Promise<{ slug: string }> };

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { slug } = await params;
  return { title: "Reading plan", alternates: { canonical: `/bible/plans/${encodeURIComponent(slug)}` } };
}

export default async function Page({ params }: Props) {
  const { slug } = await params;
  return <BiblePlanPage slug={slug} />;
}
