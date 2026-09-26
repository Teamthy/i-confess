import type { Metadata } from "next";
import { BibleReader } from "@/components/BibleReader";

type Props = { params: Promise<{ reference: string[] }> };

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { reference } = await params;
  return {
    title: "Bible passage",
    robots: { index: false, follow: true },
    alternates: { canonical: `/bible/${reference.map(encodeURIComponent).join("/")}` },
  };
}

function parseDeepLink(parts: string[]) {
  if (parts[0]?.toLowerCase() === "passage") {
    const referenceParts = parts.slice(1);
    const canonical = /^([A-Za-z0-9]+)\.(\d+)\.(\d+)(?:-(\d+))?$/.exec(referenceParts.join("."));
    if (canonical) return { reference: `${canonical[1]} ${canonical[2]}:${canonical[3]}${canonical[4] ? `-${canonical[4]}` : ""}` };
    const chapterAndVerse = /^(\d+):(\d+(?:-\d+)?)$/.exec(referenceParts[1] ?? "");
    if (referenceParts[0] && chapterAndVerse) return { reference: `${referenceParts[0]} ${chapterAndVerse[1]}:${chapterAndVerse[2]}` };
    if (referenceParts.length >= 2 && /^\d+$/.test(referenceParts[1])) {
      return { reference: `${referenceParts[0]} ${referenceParts[1]}${referenceParts[2] && /^\d+$/.test(referenceParts[2]) ? `:${referenceParts[2]}` : ""}` };
    }
    return { reference: referenceParts.join(" ") };
  }
  const [translation, book, chapter, verse] = parts;
  if (translation && book && chapter && /^\d+$/.test(chapter)) {
    return {
      translation,
      reference: `${book} ${chapter}${verse && /^\d+$/.test(verse) ? `:${verse}` : ""}`,
    };
  }
  return { translation: translation || undefined };
}

export default async function BibleDeepLinkPage({ params }: Props) {
  const parts = (await params).reference;
  const deepLink = parseDeepLink(parts);
  return <BibleReader initialTranslation={deepLink.translation} initialReference={deepLink.reference} />;
}
