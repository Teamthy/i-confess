/**
 * Legal content model.
 *
 * INTEGRATION BOUNDARY: the repository's PRIVACY.md and TERMS.md are the
 * engineering-facing versions of these commitments; the pages below are their
 * product-facing rendering. Copy is reviewed with the same care as
 * confessions — plain sentences, no dark patterns, commitments we can keep
 * because the server already enforces them (audit logs, moderation gates,
 * deletion tooling). When the company's counsel supplies final text, only
 * this file changes.
 */

export type LegalSection = { heading: string; body: string[] };
export type LegalDoc = { title: string; lede: string; updated: string; sections: LegalSection[] };

export const PRIVACY: LegalDoc = {
  title: "Privacy at iCONFESS",
  lede: "What we collect, why we collect it, and the lines we do not cross.",
  updated: "2026-09-21",
  sections: [
    {
      heading: "The short version",
      body: [
        "Your private confessions are private. They are stored so you can return to them, they are never published, and they are excluded from search engines by construction — the public systems physically cannot serve them.",
        "We collect the minimum needed to run the practice: your account details, the content you create, your playback and session history, and the preferences you set. We do not sell personal information, and we do not run advertising trackers.",
      ],
    },
    {
      heading: "What we collect",
      body: [
        "Account data: your email address, display name, timezone, and authentication credentials (passwords are stored only as modern hashes).",
        "Content you create: personal confessions and community submissions, together with the visibility state you chose for each.",
        "Practice data: sessions you start, items you complete or skip, favourites, schedules, and playback history. This is what powers your history and recommendations.",
        "Technical records: sign-in sessions and devices, and security events such as password changes — kept so you can audit your own account access.",
      ],
    },
    {
      heading: "What we never do",
      body: [
        "We never publish your private content. Publication is a deliberate state change that only happens to content you explicitly offer for review, and only after a person approves it.",
        "We never sell or share your personal information with advertisers. Analytics events are counted without personal identifiers.",
        "We never decide your entitlements in the app. Whether content is available to you is always answered by the server.",
      ],
    },
    {
      heading: "Your controls",
      body: [
        "You can export everything we hold about you as a machine-readable download, from your account settings.",
        "You can delete individual content, or schedule full account deletion. Deletion removes your content and personal data; a short grace period lets you cancel, and afterwards the erasure is complete.",
        "You can review and revoke active sign-in sessions at any time, and every privileged action on your account is written to an audit log.",
      ],
    },
    {
      heading: "Contact",
      body: [
        "Questions about privacy — including access, correction, or deletion requests — reach a person at support@iconfess.app.",
      ],
    },
  ],
};

export const TERMS: LegalDoc = {
  title: "Terms of service",
  lede: "The agreement between you and iCONFESS, in plain sentences.",
  updated: "2026-09-21",
  sections: [
    {
      heading: "Using iCONFESS",
      body: [
        "iCONFESS provides a library of reviewed confessions, guided sessions, and tools for a personal practice. You need an account to create content and use the full experience; the public library is open to everyone.",
        "You must be old enough to consent to data processing in your jurisdiction, and you are responsible for keeping your credentials safe.",
      ],
    },
    {
      heading: "Your content",
      body: [
        "Confessions you write remain yours. By keeping them private, you grant us only the technical licence needed to store and display them to you.",
        "If you offer content for community publication, you grant us the licence to publish that specific content after review. Rejection or later withdrawal is always available to you.",
        "You may not upload content you do not have the right to share, content that impersonates others, or material that breaks the community guidelines.",
      ],
    },
    {
      heading: "Our content",
      body: [
        "The canonical library, audio renders, and the iCONFESS name and identity belong to iCONFESS. They are provided to you for personal practice; reproduction for other purposes needs permission.",
        "Narration voices are used under explicit licence. Their availability can change if a licence ends — if that happens, affected sessions are rebuilt rather than silently broken.",
      ],
    },
    {
      heading: "Subscriptions and trials",
      body: [
        "Premium is billed through the app stores or as offered at checkout. Trials convert only if you let them, and cancellation is available at any time — your practice and history remain yours on the free tier.",
        "Prices are set regionally and shown before you confirm any purchase.",
      ],
    },
    {
      heading: "Changes and ending",
      body: [
        "We may update these terms as the product evolves; material changes are announced in the product before they take effect. You can end this agreement at any time by deleting your account.",
      ],
    },
  ],
};

export const COOKIES: LegalDoc = {
  title: "Cookie policy",
  lede: "The small set of cookies iCONFESS uses, and the many it does not.",
  updated: "2026-09-21",
  sections: [
    {
      heading: "What we use",
      body: [
        "Essential session cookies keep you signed in and protect forms against cross-site request forgery. Without them the product cannot function, and they are scoped to the iCONFESS site alone.",
        "Preference cookies remember the choices you make in the product — such as your selected audio settings — so the practice feels continuous.",
      ],
    },
    {
      heading: "What we do not use",
      body: [
        "No advertising cookies. No cross-site trackers. No third-party behavioural analytics.",
        "Aggregate product analytics count events without personal identifiers, which is why we do not need a tracking cookie to understand the product.",
      ],
    },
    {
      heading: "Your choices",
      body: [
        "You can clear or block cookies in your browser at any time; the cost is simply being signed out. The site remains readable without them.",
      ],
    },
  ],
};

export const GUIDELINES: LegalDoc = {
  title: "Community guidelines",
  lede: "What belongs in the community, what does not, and how decisions are made.",
  updated: "2026-09-21",
  sections: [
    {
      heading: "The spirit of the community",
      body: [
        "The community exists so people can share words that helped them. Confessions offered here are read by a person before publication — not because we doubt you, but because a shared library is only worth having if it is trustworthy.",
      ],
    },
    {
      heading: "What belongs",
      body: [
        "Original confessions and reflections in your own words, standing honestly on the Scripture they reference.",
        "Words offered to help — the test we apply is simple: would reading this strengthen someone going through a hard season?",
      ],
    },
    {
      heading: "What does not belong",
      body: [
        "Content that demeans, threatens, or harasses; content that impersonates another person; plagiarism; medical, financial, or legal claims presented as guarantees; commercial solicitation; and anything illegal in the jurisdictions we serve.",
        "Private content about other people — their names, struggles, or messages — without their consent.",
      ],
    },
    {
      heading: "How review works",
      body: [
        "Every submission is read by a moderator. Approved content is published with the visibility you offered. Rejected content comes back with a reason, and a rejected submission can be revised and offered again.",
        "If you believe a decision was wrong, the appeal process is built into your account: a dismissed report or a rejected confession can be appealed once, and the appeal is decided by a different pass, not the same one.",
      ],
    },
    {
      heading: "Enforcement",
      body: [
        "Content that breaks these guidelines is not published, and repeated violations end participation. Serious violations — threats, illegal content — result in immediate suspension.",
        "Every moderation decision is recorded, which is how the process stays accountable to the community it serves.",
      ],
    },
  ],
};
