import type { Metadata } from "next";
import { BibleTopicsPage } from "@/components/bible-pages";

export const metadata: Metadata = {
  title: "Bible topics",
  description:
    "Scripture by topic, derived from the passages the reviewed iCONFESS confession corpus stands on — faith, peace, healing, purpose and more.",
  alternates: { canonical: "/bible/topics" },
};

export default function Page() {
  return <BibleTopicsPage />;
}
