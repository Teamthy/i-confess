import type { Metadata } from "next";
import { BibleLibraryPage } from "@/components/bible-pages";

export const metadata: Metadata = {
  title: "Bible collections",
  alternates: { canonical: "/bible/collections" },
  robots: { index: false, follow: false },
};

export default function Page() {
  return <BibleLibraryPage kind="collections" />;
}
