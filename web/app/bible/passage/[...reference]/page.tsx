import type { Metadata } from "next";
import { BibleReader } from "@/components/bible-reader";
import { parseReference } from "@/lib/canon";

/* /bible/passage/John.3.16, /bible/passage/John/3/16, /bible/passage/Psalm 23
   — every shape a shared reference arrives in resolves to the same verse. */

type Props = { params: Promise<{ reference: string[] }> };

const asReference = (parts: string[]) => {
  const decoded = parts.map(decodeURIComponent);
  if (decoded.length === 1) return decoded[0].replace(/\+/g, " ");
  const [book, chapter, verse] = decoded;
  if (chapter && /^\d+$/.test(chapter)) return `${book} ${chapter}${verse && /^\d+(-\d+)?$/.test(verse) ? `:${verse}` : ""}`;
  return decoded.join(" ");
};

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { reference } = await params;
  const text = asReference(reference);
  const parsed = parseReference(text);
  return {
    title: parsed ? text : "Bible passage",
    description: parsed ? `Read ${text} in the iCONFESS Bible reader.` : "Read Scripture in the iCONFESS Bible reader.",
    alternates: { canonical: `/bible/passage/${reference.map(encodeURIComponent).join("/")}` },
  };
}

export default async function Page({ params }: Props) {
  const { reference } = await params;
  return <BibleReader initialReference={asReference(reference)} />;
}
