import type { Metadata } from "next";
import { LegalPage } from "@/components/LegalPage";
import { GUIDELINES } from "@/content/legal";

export const metadata: Metadata = {
  title: "Community guidelines",
  description: "What belongs in the iCONFESS community, what does not, and how review decisions are made.",
  alternates: { canonical: "/community-guidelines" },
};

export default function CommunityGuidelinesPage() {
  return <LegalPage {...GUIDELINES} />;
}
