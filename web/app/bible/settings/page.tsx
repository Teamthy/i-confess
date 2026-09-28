import type { Metadata } from "next";
import { BibleSettingsPage } from "@/components/bible-pages";

export const metadata: Metadata = {
  title: "Bible reader settings",
  description: "Typography, layout and the reading palette for the iCONFESS Bible reader.",
  alternates: { canonical: "/bible/settings" },
  robots: { index: false, follow: true },
};

export default function Page() {
  return <BibleSettingsPage />;
}
