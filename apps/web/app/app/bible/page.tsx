import type { Metadata } from "next";
import { BibleReader } from "@/components/BibleReader";

export const metadata: Metadata = { title: "Bible" };

export default function AppBiblePage() {
  return <BibleReader />;
}
