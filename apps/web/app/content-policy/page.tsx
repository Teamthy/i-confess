/** Content Policy — rules governing confession content and moderation. */

import type { Metadata } from "next";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";

export const metadata: Metadata = {
  title: "Content Policy — iCONFESS",
  description: "Rules governing confession content, community posts, and moderation on iCONFESS.",
};

export default function ContentPolicyPage() {
  return (
    <div>
      <SiteHeader />
      <main id="main">
        <section className="ic-page-hero">
          <div className="ic-container">
            <h1>Content policy</h1>
            <p className="ic-lede">How content is governed on iCONFESS.</p>
          </div>
        </section>
        <section className="ic-section">
          <div className="ic-container ic-container--text">
            <div className="ic-prose">
              <h2>Canon confessions</h2>
              <p>The canonical library of 39 categories and 78 confessions is editorial content, reviewed and maintained by the iCONFESS team. Every confession passes a theological review gate before publication.</p>

              <h2>User confessions</h2>
              <p>Users can write confessions in their own words. These are private by default. A user may choose to submit a confession for community review, but nothing is published automatically.</p>

              <h3>Visibility states</h3>
              <ul>
                <li><strong>Private</strong> — visible only to the author.</li>
                <li><strong>Shared</strong> visible to people the author chooses.</li>
                <li><strong>Pending review</strong> — submitted for moderation review.</li>
                <li><strong>Approved</strong> — published to the community after human review.</li>
                <li><strong>Rejected</strong> — did not pass review. The author is notified with a reason.</li>
                <li><strong>Archived</strong> — withdrawn by the author or the moderation team.</li>
              </ul>

              <h2>Community posts</h2>
              <p>Community posts are moderated before publication. Content that is harmful, misleading, or violates the spirit of the platform will not be published.</p>

              <h2>Reporting</h2>
              <p>Any published content can be reported. Reports are reviewed by the moderation team and decisions are recorded in the audit log.</p>

              <h2>Appeals</h2>
              <p>If your content is rejected or removed, you can appeal the decision. Appeals are reviewed independently.</p>
            </div>
          </div>
        </section>
      </main>
      <SiteFooter />
    </div>
  );
}
