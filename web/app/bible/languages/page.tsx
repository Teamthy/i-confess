import type { Metadata } from "next";
import { BibleLanguagesPage } from "@/components/bible-pages";

export const metadata: Metadata = {
  title: "Bible languages",
  description: "The languages iCONFESS serves Scripture in — listed only where an approved translation exists.",
  alternates: { canonical: "/bible/languages" },
};

export default function Page() {
  return <BibleLanguagesPage />;
}
