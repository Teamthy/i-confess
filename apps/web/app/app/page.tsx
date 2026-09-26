/** Personalised home: continue, today, categories, recommendations, rhythm, recent. */
import type { Metadata } from "next";
import { AppHomeClient } from "@/components/AppHomeClient";

export const metadata: Metadata = { title: "Your experience" };

export default function AppHomePage() {
  return (
    <div className="ia-page">
      <AppHomeClient />
    </div>
  );
}
