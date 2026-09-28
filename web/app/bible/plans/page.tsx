import type { Metadata } from "next";
import { BiblePlansPage } from "@/components/bible-pages";

export const metadata: Metadata = {
  title: "Bible reading plans",
  description: "Editorially reviewed reading plans through the Gospels, the Psalms, the New Testament and the whole Bible.",
  alternates: { canonical: "/bible/plans" },
};

export default function Page() {
  return <BiblePlansPage />;
}
