import type { Metadata } from "next";
import { BibleTranslationsPage } from "@/components/bible-pages";

export const metadata: Metadata = {
  title: "Bible translations",
  description: "Every Bible translation approved for iCONFESS, with the licence and the rights it was reviewed under.",
  alternates: { canonical: "/bible/translations" },
};

export default function Page() {
  return <BibleTranslationsPage />;
}
