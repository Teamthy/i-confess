/** Stories — editorial content about the iCONFESS practice. */

import type { Metadata } from "next";
import Link from "next/link";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";

export const metadata: Metadata = {
  title: "Stories — iCONFESS",
  description: "Stories about confession, reflection, and the practice of returning to words that matter.",
};

export default function StoriesPage() {
  return (
    <div>
      <SiteHeader />
      <main id="main">
        <section className="ic-page-hero">
          <div className="ic-container">
            <h1>Stories</h1>
            <p className="ic-lede">Reflections on confession, words, and the practice of returning.</p>
          </div>
        </section>
        <section className="ic-section">
          <div className="ic-container">
            <div className="ic-state">
              <h3>Stories are coming</h3>
              <p>Editorial stories about the iCONFESS practice will be published here.</p>
              <Link href="/journal" className="ic-btn ic-btn--secondary">Read the journal</Link>
            </div>
          </div>
        </section>
      </main>
      <SiteFooter />
    </div>
  );
}
