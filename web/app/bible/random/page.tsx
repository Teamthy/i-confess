import type { Metadata } from "next";
import { BibleRandomPage } from "@/components/bible-pages";

export const metadata: Metadata = {
  title: "Bible random passage",
  description:
    "Draw a passage from the pool of verses the reviewed confession corpus cites — never an arbitrary verse.",
  alternates: { canonical: "/bible/random" },
};

export default function Page() {
  return <BibleRandomPage />;
}
