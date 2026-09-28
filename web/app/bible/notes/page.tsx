import type { Metadata } from "next";
import { BibleLibraryPage } from "@/components/bible-pages";

export const metadata: Metadata = {
  title: "Bible notes",
  alternates: { canonical: "/bible/notes" },
  robots: { index: false, follow: false },
};

export default function Page() {
  return <BibleLibraryPage kind="notes" />;
}
