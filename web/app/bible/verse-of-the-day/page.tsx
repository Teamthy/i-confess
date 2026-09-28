import type { Metadata } from "next";
import { BibleVerseOfDayPage } from "@/components/bible-pages";

export const metadata: Metadata = {
  title: "Bible verse of the day",
  description:
    "Today's reviewed verse, served by the API — read it in context, reflect on it, or confess from it.",
  alternates: { canonical: "/bible/verse-of-the-day" },
};

export default function Page() {
  return <BibleVerseOfDayPage />;
}
