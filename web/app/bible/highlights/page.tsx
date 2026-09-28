import type { Metadata } from "next";
import { BibleLibraryPage } from "@/components/bible-pages";

export const metadata: Metadata = {
  title: "Bible highlights",
  alternates: { canonical: "/bible/highlights" },
  robots: { index: false, follow: false },
};

export default function Page() {
  return <BibleLibraryPage kind="highlights" />;
}
