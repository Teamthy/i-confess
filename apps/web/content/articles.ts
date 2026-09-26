/**
 * Journal content model.
 *
 * INTEGRATION BOUNDARY (docs/44-WEB-PLATFORM-AUDIT.md §5): the backend has no
 * articles API yet. These are the journal's launch essays, stored as typed
 * content separate from presentation (§70), shaped exactly like the future
 * Article model (category, title, excerpt, author, date, reading time, body).
 * When GET /articles exists, only the two journal pages change their data
 * source — this file's type is the contract.
 *
 * All essays are written for this product and signed "iCONFESS" — no invented
 * named authors, no fabricated external writers (§86).
 */

export type Article = {
  slug: string;
  category: string;
  title: string;
  excerpt: string;
  author: "iCONFESS";
  date: string; // ISO
  readingTime: string;
  /** Paragraphs. Kept deliberately short — this is a journal, not a book. */
  body: string[];
};

export const ARTICLES: Article[] = [
  {
    slug: "words-you-repeat",
    category: "Reflections",
    title: "The words you repeat become the voice you hear",
    excerpt:
      "Why speaking a confession out loud does something silent reading never quite does — and how repetition turns words into belief.",
    author: "iCONFESS",
    date: "2026-09-01",
    readingTime: "4 min read",
    body: [
      "There is a reason every meaningful commitment in history was spoken before it was written. Speech is slower than reading. It costs breath. It passes through the body on the way out, and something about that passage makes the words harder to dismiss.",
      "When you read silently, you can skim past a sentence that convicts you. When you say it — really say it, at speaking volume, alone in a room — the sentence has to survive your own voice. That is a higher bar. It is also the point.",
      "iCONFESS is built around that difference. A confession is written to be spoken: short sentences, present-tense claims, words that stand on Scripture you can check. You hear it in a curated voice, and then you say it yourself. The voice models the rhythm; the repetition makes it yours.",
      "Repetition is not magic. It is maintenance. A sentence you have said thirty times stops being an affirmation and starts being a description — of who you are choosing to be. That is the quiet ambition of the practice: not hype, but orientation.",
      "Start with one minute. One area of life. Say the words out loud where no one can hear you, and notice what it feels like to mean them a little more on the third day than the first.",
    ],
  },
  {
    slug: "five-minute-practice",
    category: "Growth",
    title: "A five-minute practice is not a lesser practice",
    excerpt:
      "Short sessions are not a compromise on the way to something more serious. They are the thing itself.",
    author: "iCONFESS",
    date: "2026-08-18",
    readingTime: "3 min read",
    body: [
      "We treat shortness as a defect, as if a practice only counts once it costs an hour. But the practices that actually last are rarely the dramatic ones. They are the ones small enough to survive a bad week.",
      "A five-minute session has a property an hour-long one does not: it fits inside the day you actually had. Not the ideal day — the real one, with the traffic and the interrupted morning and the evening that disappeared.",
      "That is why iCONFESS builds sessions around explicit lengths. Five minutes is a complete experience: a confession, its Scripture, a voice, a moment of stillness at the end. Nothing is withheld to make you upgrade your exhaustion into commitment.",
      "Do the short practice daily and the long one occasionally, and you will get further than the reverse. Consistency is the multiplier. Length is just a setting.",
    ],
  },
  {
    slug: "words-reviewed-before-published",
    category: "Product",
    title: "Why every confession is reviewed before you ever see it",
    excerpt:
      "An open library is a trusted library. Inside iCONFESS's editorial and theological review — and why nothing publishes automatically.",
    author: "iCONFESS",
    date: "2026-08-04",
    readingTime: "5 min read",
    body: [
      "Most content platforms optimise for volume. A confession platform cannot. The words are the product, and a wrong or hollow sentence repeated daily does real harm — not dramatically, but by degrees.",
      "So the iCONFESS pipeline is deliberately slow. A confession is drafted, edited, and checked against the Scripture it claims to stand on. Quotations must quote; allusions must allude honestly. A reviewer records the check. Only then does the confession move toward published — and the movement is one-way and enforced, not a habit of the software.",
      "The same gate applies to the community. A confession you write can stay private forever, or you can offer it for review. Nothing publishes automatically — not ever. A person reads it, approves or declines with a reason, and the decision is recorded.",
      "This costs us speed. That is the trade we chose on purpose. You will never open iCONFESS and wonder who checked the words, because the answer is always: we did, before you ever saw them.",
    ],
  },
  {
    slug: "choose-your-voice",
    category: "Mindset",
    title: "Choose the voice that helps the words land",
    excerpt:
      "The same sentence in a different voice is a different experience. On narration, pacing, and why voices are curated rather than generated on demand.",
    author: "iCONFESS",
    date: "2026-07-21",
    readingTime: "3 min read",
    body: [
      "Hearing a sentence and reading it are different events in the body. Hearing it in the right voice is a third thing still — steadier, slower, closer to being spoken over than spoken at.",
      "That is why the voice is not an afterthought in iCONFESS. Each narration voice is chosen for pace and warmth, cleared for use, and kept consistent, so a session sounds the same on the fortieth morning as the first.",
      "Try the same confession in two voices and notice which one you settle under. That one is yours. The practice should feel like being reminded by someone who means it.",
    ],
  },
  {
    slug: "the-quiet-app",
    category: "Lifestyle",
    title: "Designing a quiet app on purpose",
    excerpt:
      "No badges, no noise, no anxiety mechanics. The design choices behind a product that wants to be part of your morning, not your scrolling.",
    author: "iCONFESS",
    date: "2026-07-07",
    readingTime: "4 min read",
    body: [
      "Most apps compete for attention. iCONFESS competes for a different resource: a few honest minutes. The design reflects that from the first screen — deep calm surfaces, one suggestion, one next step.",
      "We do not use streak guilt. If you miss a morning, the app does not punish you; it simply offers the words again at midday. Missing is part of being human, and the practice is designed to survive it.",
      "Motion is used sparingly, sound is never forced, and nothing autoplays. The loudest thing in iCONFESS should be the words themselves.",
    ],
  },
];
