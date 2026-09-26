/** FAQ — public frequently asked questions. */

import type { Metadata } from "next";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";

export const metadata: Metadata = {
  title: "FAQ — iCONFESS",
  description: "Frequently asked questions about iCONFESS — what it is, how it works, and how to get started.",
};

const FAQS = [
  ["What is iCONFESS?", "iCONFESS is a daily confession practice. Choose an area of life, hear a confession spoken over you, and return to it until it becomes part of how you think and live."],
  ["How does iCONFESS work?", "Pick an area of life and how long you have. iCONFESS assembles a session of confessions and Scripture in a curated voice, and guides you through it."],
  ["What are categories?", "Categories are 39 areas of life — from peace and healing to purpose and provision. Each contains curated confessions and Scripture."],
  ["Can I create my own confession?", "Yes. Write confessions in your own words. Keep them private, share them with people you choose, or offer them for review."],
  ["Can my confession remain private?", "Yes. Private confessions are yours alone. Publication only ever happens to content you explicitly offer for review."],
  ["How does audio work?", "Every confession can be heard as well as read. Choose a curated voice and listen at the pace that suits you."],
  ["What is Premium?", "Premium unlocks the complete library, extended session lengths, premium voices, and personalised routines."],
  ["Can I use iCONFESS on the web?", "Yes. The full product experience is available in the browser."],
  ["Can I use iCONFESS on mobile?", "Yes. The mobile app is available for iOS and Android."],
  ["How do I manage my subscription?", "From your subscription settings in the app, at any time."],
  ["How is my data handled?", "Private content is never exposed publicly or to search engines. Public sharing only happens through explicit review and publication."],
] as const;

export default function FAQPage() {
  return (
    <div>
      <SiteHeader />
      <main id="main">
        <section className="ic-page-hero">
          <div className="ic-container">
            <h1>Questions about iCONFESS?</h1>
            <p className="ic-lede">Finally, some answers.</p>
          </div>
        </section>
        <section className="ic-section">
          <div className="ic-container" style={{ maxWidth: "48rem" }}>
            <div className="ic-faq">
              {FAQS.map(([q, a]) => (
                <details key={q}>
                  <summary>{q}</summary>
                  <p>{a}</p>
                </details>
              ))}
            </div>
          </div>
        </section>
      </main>
      <SiteFooter />
    </div>
  );
}
