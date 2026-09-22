/* iCONFESS content layer — sourced from the canonical repo seeds:
   server/internal/seed/categories_canonical.go (39 categories)
   server/internal/seed/confessions_canonical.go (78 confessions)
   server/internal/seed/seed.go (voice "Grace" + licence)
   server/internal/billing/pricing.go (plan catalog)
   apps/web/content/articles.ts (journal essays)               */
import catsJson from "./cats.json";
import confsJson from "./confs.json";

export type Category = { name: string; slug: string; description: string; tagline: string; icon: string };
export type Scripture = { book: string; chapter: number; verse: string; translation: string; direct: boolean };
export type Confession = { category: string; title: string; short: string; medium: string; long: string; intensity: number; scriptures: Scripture[]; slug: string };
export type Article = { slug: string; category: string; title: string; excerpt: string; author: string; date: string; readingTime: string; body: string[] };
export type Voice = { slug: string; name: string; description: string; type: string; gender: string; language: string; premium: boolean; status: string; rights: string };
export type Plan = { id: string; name: string; interval: string; trialDays: number; prices: Record<string, string>; features: string[] };
export type Session = { slug: string; category: string; minutes: number; title: string; description: string; items: Confession[] };

export const CATEGORIES = catsJson as Category[];
const slugify = (t: string) => t.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/(^-|-$)/g, "");
export const CONFESSIONS = (confsJson as Confession[]).map((c) => ({ ...c, slug: c.slug || slugify(c.title) }));

export const ARTICLES: Article[] = [
  { slug: "words-you-repeat", category: "Reflections", title: "The words you repeat become the voice you hear", excerpt: "Why speaking a confession out loud does something silent reading never quite does — and how repetition turns words into belief.", author: "iCONFESS", date: "2026-09-01", readingTime: "4 min read", body: ["There is a reason every meaningful commitment in history was spoken before it was written. Speech is slower than reading. It costs breath. It passes through the body on the way out, and something about that passage makes the words harder to dismiss.", "When you read silently, you can skim past a sentence that convicts you. When you say it — really say it, at speaking volume, alone in a room — the sentence has to survive your own voice. That is a higher bar. It is also the point.", "iCONFESS is built around that difference. A confession is written to be spoken: short sentences, present-tense claims, words that stand on Scripture you can check. You hear it in a curated voice, and then you say it yourself. The voice models the rhythm; the repetition makes it yours.", "Repetition is not magic. It is maintenance. A sentence you have said thirty times stops being an affirmation and starts being a description — of who you are choosing to be. That is the quiet ambition of the practice: not hype, but orientation.", "Start with one minute. One area of life. Say the words out loud where no one can hear you, and notice what it feels like to mean them a little more on the third day than the first."] },
  { slug: "five-minute-practice", category: "Growth", title: "A five-minute practice is not a lesser practice", excerpt: "Short sessions are not a compromise on the way to something more serious. They are the thing itself.", author: "iCONFESS", date: "2026-08-18", readingTime: "3 min read", body: ["We treat shortness as a defect, as if a practice only counts once it costs an hour. But the practices that actually last are rarely the dramatic ones. They are the ones small enough to survive a bad week.", "A five-minute session has a property an hour-long one does not: it fits inside the day you actually had. Not the ideal day — the real one, with the traffic and the interrupted morning and the evening that disappeared.", "That is why iCONFESS builds sessions around explicit lengths. Five minutes is a complete experience: a confession, its Scripture, a voice, a moment of stillness at the end. Nothing is withheld to make you upgrade your exhaustion into commitment.", "Do the short practice daily and the long one occasionally, and you will get further than the reverse. Consistency is the multiplier. Length is just a setting."] },
  { slug: "words-reviewed-before-published", category: "Product", title: "Why every confession is reviewed before you ever see it", excerpt: "An open library is a trusted library. Inside iCONFESS's editorial and theological review — and why nothing publishes automatically.", author: "iCONFESS", date: "2026-08-04", readingTime: "5 min read", body: ["Most content platforms optimise for volume. A confession platform cannot. The words are the product, and a wrong or hollow sentence repeated daily does real harm — not dramatically, but by degrees.", "So the iCONFESS pipeline is deliberately slow. A confession is drafted, edited, and checked against the Scripture it claims to stand on. Quotations must quote; allusions must allude honestly. A reviewer records the check. Only then does the confession move toward published — and the movement is one-way and enforced, not a habit of the software.", "The same gate applies to the community. A confession you write can stay private forever, or you can offer it for review. Nothing publishes automatically — not ever. A person reads it, approves or declines with a reason, and the decision is recorded.", "This costs us speed. That is the trade we chose on purpose. You will never open iCONFESS and wonder who checked the words, because the answer is always: we did, before you ever saw them."] },
  { slug: "choose-your-voice", category: "Mindset", title: "Choose the voice that helps the words land", excerpt: "The same sentence in a different voice is a different experience. On narration, pacing, and why voices are curated rather than generated on demand.", author: "iCONFESS", date: "2026-07-21", readingTime: "3 min read", body: ["Hearing a sentence and reading it are different events in the body. Hearing it in the right voice is a third thing still — steadier, slower, closer to being spoken over than spoken at.", "That is why the voice is not an afterthought in iCONFESS. Each narration voice is chosen for pace and warmth, cleared for use, and kept consistent, so a session sounds the same on the fortieth morning as the first.", "Try the same confession in two voices and notice which one you settle under. That one is yours. The practice should feel like being reminded by someone who means it."] },
  { slug: "the-quiet-app", category: "Lifestyle", title: "Designing a quiet app on purpose", excerpt: "No badges, no noise, no anxiety mechanics. The design choices behind a product that wants to be part of your morning, not your scrolling.", author: "iCONFESS", date: "2026-07-07", readingTime: "4 min read", body: ["Most apps compete for attention. iCONFESS competes for a different resource: a few honest minutes. The design reflects that from the first screen — deep calm surfaces, one suggestion, one next step.", "We do not use streak guilt. If you miss a morning, the app does not punish you; it simply offers the words again at midday. Missing is part of being human, and the practice is designed to survive it.", "Motion is used sparingly, sound is never forced, and nothing autoplays. The loudest thing in iCONFESS should be the words themselves."] },
];

export const VOICES: Voice[] = [{ slug: "grace", name: "Grace", description: "Warm, calm professional narration voice.", type: "professional", gender: "female", language: "en", premium: false, status: "active", rights: "Licensed — i-confess studio, global TTS" }];

export const PLANS: Plan[] = [
  { id: "monthly", name: "Premium Monthly", interval: "month", trialDays: 7, prices: { NGN: "₦1,500", USD: "$4.99", GBP: "£3.99", EUR: "€4.99", PHP: "₱299" }, features: ["premium_voices", "offline_downloads", "long_sessions"] },
  { id: "annual", name: "Premium Annual", interval: "year", trialDays: 7, prices: { NGN: "₦12,000", USD: "$39.99", GBP: "£32.99", EUR: "€39.99", PHP: "₱1,999" }, features: ["premium_voices", "offline_downloads", "long_sessions", "annual_saving"] },
];

export const FAQS: [string, string][] = [
  ["What is iCONFESS?", "A digital confession and guided-session platform. You speak confessions — short, Scripture-checked declarations — out loud, hear them in a curated voice, and return to them daily."],
  ["How does iCONFESS work?", "Choose a category, pick a length, and the session engine assembles confessions to fit. You hear each one narrated, then say it yourself. That's the whole practice."],
  ["What are categories?", "The areas of life you confess over — Healing, Peace, Finance, Family, Purpose and 34 more. 39 in total, each with its own reviewed library."],
  ["Can I create my own confession?", "Yes. Write it, keep it private, or offer it for review. A human reviewer reads every submission before it can become shared or public."],
  ["Can my confession remain private?", "Forever, if you want. Private is the default. Nothing ever publishes automatically."],
  ["How does audio work?", "Confessions are narrated by licensed voices — Grace is our first. On the web, previews and sessions play through your browser's speech engine using the voice you choose in settings."],
  ["What is Premium?", "A subscription that adds premium voices, offline downloads and longer sessions. Monthly and annual plans, with a 7-day trial."],
  ["Can I use iCONFESS on the web?", "Yes — this site is the full product: explore, listen, build sessions, create confessions and manage your account from the browser."],
  ["Can I use iCONFESS on mobile?", "Yes. The iOS and Android app shares your account, favorites, history and routines with the web."],
  ["How do I manage my subscription?", "From Profile → Subscription in the app or web. Upgrade, cancel or restore a purchase any time; store purchases are managed by Apple or Google."],
  ["How is my data handled?", "Privately. Your confessions are yours; private content is never reviewed unless you submit it. See the Privacy page for the full account."],
];

/* ---------- helpers ---------- */
export const catBySlug = (s: string) => CATEGORIES.find((c) => c.slug === s);
export const confBySlug = (s: string) => CONFESSIONS.find((c) => c.slug === s);
export const artBySlug = (s: string) => ARTICLES.find((a) => a.slug === s);

/* deterministic navy motif per slug — port of apps/web/lib/categoryColor.ts */
const TONES = ["#00072D", "#051650", "#081D61", "#0A2472", "#123499", "#141918", "#0A0E0D", "#051650"];
export function railColor(slug: string) { let h = 0; for (const ch of slug) h = (h * 31 + ch.charCodeAt(0)) >>> 0; return TONES[h % TONES.length]; }
const PAIRS: Record<string, [string, string]> = { "#00072D": ["#123499", "#00072D"], "#051650": ["#425EAF", "#051650"], "#081D61": ["#123499", "#081D61"], "#0A2472": ["#7288C4", "#0A2472"], "#123499": ["#A3B3DA", "#123499"], "#141918": ["#425EAF", "#141918"], "#0A0E0D": ["#123499", "#0A0E0D"] };
export function motifStyle(slug: string): React.CSSProperties { const b = railColor(slug); const p = PAIRS[b] || ["#123499", "#00072D"]; return { background: `linear-gradient(135deg, ${p[0]} 0%, ${p[1]} 78%)` }; }

export const CAT_IMAGES: Record<string, string> = Object.fromEntries(CATEGORIES.map((c) => [c.slug, `cat-${c.slug}.jpg`]));

export function sessionsFor(catSlug: string): Session[] {
  const cat = catBySlug(catSlug); if (!cat) return [];
  const items = CONFESSIONS.filter((c) => c.category === cat.name);
  return [5, 10, 15].map((m) => ({ slug: `${catSlug}-${m}-min`, category: catSlug, minutes: m, title: `${cat.name} — ${m} minute${m > 1 ? "s" : ""}`, description: `An engine-assembled ${m}-minute experience over ${cat.name.toLowerCase()}.`, items }));
}
export const ALL_SESSIONS: Session[] = CATEGORIES.slice(0, 12).flatMap((c) => sessionsFor(c.slug));
export const sessionBySlug = (s: string) => ALL_SESSIONS.find((x) => x.slug === s);

export type QueueItem = { slug: string; title: string; category: string; text: string };
export const queueItem = (c: Confession, variant: "short" | "medium" | "long" = "medium"): QueueItem => ({ slug: c.slug, title: c.title, category: c.category, text: c[variant] });
export function buildQueue(s: Session): QueueItem[] {
  const target = s.minutes * 60; const wps = 2.4; const q: QueueItem[] = []; let t = 0; let g = 0;
  const pool = s.items.length ? s.items : CONFESSIONS.slice(0, 6);
  while (t < target - 15 && g < 24) {
    const c = pool[g % pool.length];
    const variant = t + 40 > target ? "short" : t + 90 > target ? "medium" : "long";
    q.push(queueItem(c, variant)); t += c[variant].split(/\s+/).length / wps + 6; g++;
  }
  if (!q.length) q.push(queueItem(pool[0], "short"));
  return q;
}

export const MOMENTS: { id: string; title: string; desc: string; cats: string[] }[] = [
  { id: "five-am", title: "Words for 5am", desc: "Before the day speaks to you.", cats: ["discipline", "purpose", "direction", "confidence"] },
  { id: "hard-conversation", title: "Before a hard conversation", desc: "Slow the tongue, steady the heart.", cats: ["peace", "forgiveness", "love", "wisdom"] },
  { id: "bad-news", title: "After bad news", desc: "Hold the promise while the room is quiet.", cats: ["hope", "peace", "prayer", "emotional-strength"] },
  { id: "cant-sleep", title: "When you cannot sleep", desc: "Set the day down out loud.", cats: ["rest", "peace", "gratitude", "protection"] },
];
export const PACKS: { id: string; title: string; desc: string; cats: string[] }[] = [
  { id: "exam-week", title: "Exam week pack", desc: "Discipline, direction and rest for the crunch.", cats: ["discipline", "direction", "wisdom", "rest"] },
  { id: "new-baby", title: "New baby pack", desc: "Words for the smallest hours.", cats: ["children", "family", "rest", "provision"] },
  { id: "new-job", title: "New job pack", desc: "Walk in prepared.", cats: ["career", "favor", "confidence", "leadership"] },
];
