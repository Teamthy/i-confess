import type { Metadata } from "next";
import { LegalPage } from "@/components/LegalPage";
import { TERMS } from "@/content/legal";

export const metadata: Metadata = {
  title: "Terms of service",
  description: "The agreement between you and iCONFESS, in plain sentences.",
  alternates: { canonical: "/terms" },
};

export default function TermsPage() {
  return <LegalPage {...TERMS} />;
}
