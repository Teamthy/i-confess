import type { Metadata } from "next";
import { BibleBooksPage } from "@/components/bible-pages";

export const metadata: Metadata = {
  title: "Bible books",
  description:
    "Every book of the canon in order — two testaments, ten sections, chapter and verse counts — straight from the generated structure, no request required.",
  alternates: { canonical: "/bible/books" },
};

export default function Page() {
  return <BibleBooksPage />;
}
