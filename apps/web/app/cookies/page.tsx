import type { Metadata } from "next";
import { LegalPage } from "@/components/LegalPage";
import { COOKIES } from "@/content/legal";

export const metadata: Metadata = {
  title: "Cookie policy",
  description: "The small set of cookies iCONFESS uses, and the many it does not.",
  alternates: { canonical: "/cookies" },
};

export default function CookiesPage() {
  return <LegalPage {...COOKIES} />;
}
