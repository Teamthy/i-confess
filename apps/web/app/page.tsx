// I CONFESS — Public website landing (Next.js App Router)
// Spec §62 — Hero → What It Is → How It Works → Daily Ritual → Categories → Audio → Voices → Featured → Community → Pricing → FAQ → CTA
// This is a shell that type-checks without node_modules installed — no runtime dependency on generated API yet.


// JSON-LD for SEO §63 — Organization + WebSite (canonical, no hard-coded categories)
const orgJsonLd = {
  "@context": "https://schema.org",
  "@type": "Organization",
  name: "I CONFESS",
  url: "https://iconfess.app",
  logo: "https://iconfess.app/icon.png",
};
const siteJsonLd = {
  "@context": "https://schema.org",
  "@type": "WebSite",
  name: "I CONFESS",
  url: "https://iconfess.app",
  potentialAction: {
    "@type": "SearchAction",
    target: "https://iconfess.app/search?q={search_term_string}",
    "query-input": "required name=search_term_string",
  },
};

export const metadata = {
  title: "I CONFESS — Daily Scripture confessions, spoken over your life",
  description: "Choose what you want to speak over your life. Build a daily confession ritual — Scripture, audio sessions, scheduling, offline.",
  openGraph: {
    title: "I CONFESS",
    description: "Daily Scripture confessions. Audio sessions. Habit.",
    type: "website",
  },
};

function Section({ title, kicker, children }: { title: string; kicker?: string; children: React.ReactNode }) {
  return (
    <section className="py-16 border-t border-[#2d3350]">
      {kicker && <p className="text-[11px] tracking-[0.12em] uppercase text-[#9aa1c0] mb-3">{kicker}</p>}
      <h2 className="font-serif text-2xl text-[#e8c67a] mb-4">{title}</h2>
      <div className="text-[#9aa1c0] leading-relaxed">{children}</div>
    </section>
  );
}

export default function LandingPage() {
  return (
    <main className="min-h-screen bg-[#0f1220] text-[#e8eaf6]">
      <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: JSON.stringify(orgJsonLd) }} />
      <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: JSON.stringify(siteJsonLd) }} />
      {/* Hero */}
      <div className="max-w-6xl mx-auto px-6 pt-16 pb-10">
        <p className="text-[11px] tracking-[0.14em] uppercase text-[#9aa1c0]">I CONFESS · Daily Confession Ritual</p>
        <h1 className="font-serif text-4xl md:text-5xl leading-tight mt-4 max-w-3xl">
          Speak <span className="text-[#e8c67a]">Scripture</span> over your life — <span className="text-[#b7a1ff]">daily</span>.
        </h1>
        <p className="text-[#9aa1c0] mt-4 max-w-2xl">
          Choose categories like Healing, Peace, Faith and Purpose. Pick a voice and duration. I CONFESS builds a structured audio session you can schedule for 6:00 AM and play offline.
        </p>
        <div className="flex gap-3 mt-8">
          <a href="/signup" className="bg-[#7c8cf8] text-[#0b0e1c] font-semibold rounded-lg px-5 py-3 text-sm">Start your morning session</a>
          <a href="/categories" className="border border-[#2d3350] rounded-lg px-5 py-3 text-sm">Explore 39 categories</a>
        </div>
        <div className="grid grid-cols-3 gap-4 mt-10 max-w-xl text-center">
          <div className="bg-[#1c2138] border border-[#2d3350] rounded-xl p-4"><div className="font-serif text-xl">39</div><div className="text-xs uppercase tracking-wide text-[#9aa1c0]">Categories</div></div>
          <div className="bg-[#1c2138] border border-[#2d3350] rounded-xl p-4"><div className="font-serif text-xl">10–180m</div><div className="text-xs uppercase tracking-wide text-[#9aa1c0]">Sessions</div></div>
          <div className="bg-[#1c2138] border border-[#2d3350] rounded-xl p-4"><div className="font-serif text-xl">Offline</div><div className="text-xs uppercase tracking-wide text-[#9aa1c0]">Licenced</div></div>
        </div>

        <Section title="What I CONFESS is" kicker="Not motivational filler">
          <p>A Christian practice engine: every confession is rooted in Scripture, reviewed theologically, and delivered as calm audio. Not a quote generator, not a chatbot.</p>
        </Section>

        <Section title="How it works" kicker="Three steps">
          <ol className="list-decimal ml-6 space-y-2">
            <li><b className="text-[#e8eaf6]">Choose</b> what you want to confess — Healing, Peace, Purpose, Joy… with weighting.</li>
            <li><b className="text-[#e8eaf6]">Create</b> a session — 10 to 180 minutes, voice, intensity — preview 12 confessions before you start.</li>
            <li><b className="text-[#e8eaf6]">Schedule</b> it — 6:00 AM, your timezone, local notification + one-tap start. Repeat daily.</li>
          </ol>
        </Section>

        <Section title="Daily ritual" kicker="The habit">
          <p><b className="text-[#e8eaf6]">The product is not content. It is a habit.</b> Consistency · Repetition · Intentionality · Scripture · Personalization · Audio immersion · Habit formation — every screen reinforces the loop: Discover → Choose → Create/Schedule → Listen → Confess → Complete → Return.</p>
        </Section>

        <Section title="Audio experience" kicker="Calm, not noisy">
          <p>Background audio, lock-screen controls, Bluetooth, sleep timer, queue, progress, speed control — and a completion screen that feels like peace, not a game.</p>
        </Section>

        <div className="py-12 flex flex-col items-center gap-3">
          <a href="/signup" className="bg-[#e8c67a] text-[#241b05] font-semibold rounded-lg px-8 py-3">Download the app</a>
          <a href="/pricing" className="text-[#9aa1c0] text-sm">See pricing — Free · Trial · Premium →</a>
        </div>

        <footer className="border-t border-[#2d3350] pt-6 text-xs text-[#6b7294] flex gap-4 flex-wrap">
          <a href="/privacy">Privacy</a><a href="/terms">Terms</a><a href="/faq">FAQ</a><a href="/contact">Contact</a><span className="ml-auto">© I CONFESS 2026 — Global. NGN · USD · GBP · EUR · PHP</span>
        </footer>
      </div>
    </main>
  );
}
