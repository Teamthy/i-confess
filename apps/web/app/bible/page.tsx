import type { Metadata } from "next";
import { BibleReader } from "@/components/BibleReader";

export const metadata: Metadata = {
  title: "Bible reader",
  description: "Read and search Scripture with the iCONFESS Bible reader.",
  alternates: { canonical: "/bible" },
  robots: { index: false, follow: true },
};

export default function BiblePage() {
  return <BibleReader />;
}
