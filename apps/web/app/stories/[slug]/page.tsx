/** Story detail — placeholder for future editorial content. */

import type { Metadata } from "next";
import Link from "next/link";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";

export const metadata: Metadata = {
  title: "Story — iCONFESS",
};

export default async function StoryDetailPage({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params;
  return (
    <div>
      <SiteHeader />
      <main id="main">
        <section className="ic-page-hero">
          <div className="ic-container">
            <Link href="/stories" className="ic-backlink">← Stories</Link>
            <h1>Story</h1>
            <p className="ic-lede">This story is being prepared.</p>
          </div>
        </section>
        <section className="ic-section">
          <div className="ic-container ic-container--text">
            <div className="ic-state">
              <h3>Story not yet published</h3>
              <p>This editorial story has not been published yet.</p>
              <Link href="/stories" className="ic-btn ic-btn--secondary">Back to stories</Link>
            </div>
          </div>
        </section>
      </main>
      <SiteFooter />
    </div>
  );
}
