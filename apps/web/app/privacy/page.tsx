import type { Metadata } from "next";
import { LegalPage } from "@/components/LegalPage";
import { PRIVACY } from "@/content/legal";

export const metadata: Metadata = {
  title: "Privacy policy",
  description: "What iCONFESS collects, why, and the lines we do not cross.",
  alternates: { canonical: "/privacy" },
};

export default function PrivacyPolicyPage() {
  return <LegalPage {...PRIVACY} />;
}
