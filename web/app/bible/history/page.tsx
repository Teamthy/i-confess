import type { Metadata } from "next";
import { BibleLibraryPage } from "@/components/bible-pages";

export const metadata: Metadata = {
  title: "Bible history",
  alternates: { canonical: "/bible/history" },
  robots: { index: false, follow: false },
};

export default function Page() {
  return <BibleLibraryPage kind="history" />;
}
