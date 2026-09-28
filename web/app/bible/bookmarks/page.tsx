import type { Metadata } from "next";
import { BibleLibraryPage } from "@/components/bible-pages";

export const metadata: Metadata = {
  title: "Bible bookmarks",
  alternates: { canonical: "/bible/bookmarks" },
  robots: { index: false, follow: false },
};

export default function Page() {
  return <BibleLibraryPage kind="bookmarks" />;
}
