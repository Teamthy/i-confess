import type { Metadata } from "next";
import { BibleAudioPage } from "@/components/bible-pages";

export const metadata: Metadata = {
  title: "Audio Bible",
  description: "Rights-gated Bible audio: approved recordings, served through short-lived signed URLs.",
  alternates: { canonical: "/bible/audio" },
};

export default function Page() {
  return <BibleAudioPage />;
}
