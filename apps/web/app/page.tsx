import type { Metadata } from "next";
import Link from "next/link";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";
import { Reveal } from "@/components/Reveal";
import { CategoryRail } from "@/components/CategoryRail";
import { SectionHead, ConfessionCard } from "@/components/Sections";
import { api, type Category, type Confession, type Voice } from "@/lib/api";
import { SITE_URL } from "@/lib/site";

export const metadata: Metadata = {
  alternates: { canonical: "/" },
};

/**
 * The homepage journey: PAUSE → FEEL → UNDERSTAND → EXPERIENCE → BELIEVE →
 * BEGIN. All catalogue data is fetched from the Go API at request time with
 * ISR; nothing on the page is fabricated. If the API is down the page still
 * renders with its editorial content and honest error states in the
 * data-driven rails.
 */

// Real confession text fetched from the API, used inside the hero card.
// The fallback line is the product's own positioning, not a fabricated quote.
function heroText(
  confession: Confession | undefined,
  category: Category | undefined,
): { kicker: string; text: string } {
  if (confession) {
    return {
      kicker: `${category?.name ?? "Peace"} · spoken confession`,
      text:
        confession.long_text ||
        confession.medium_text ||
        confession.short_text ||
        "A confession to speak over your life.",
    };
  }
  return {
    kicker: "Peace · spoken confession",
    text: "Speak words that steady you. iCONFESS turns a few intentional minutes into something you return to.",
  };
}

export default async function HomePage() {
  const [categoriesRes, voicesRes] = await Promise.all([api.categories(), api.voices()]);
  const categories = categoriesRes.ok ? categoriesRes.data : [];
  const voices = voicesRes.ok ? voicesRes.data : [];

  // Featured category: Peace, per the homepage concept; falls back to the
  // first published category if Peace is absent.
  const peace =
    categories.find((c) => c.slug === "peace") ?? categories[0];
  const peaceConfessionsRes = peace
    ? await api.categoryConfessions(peace.id)
    : ({ ok: false, status: 404, message: "" } as const);
  const peaceConfessions = peaceConfessionsRes.ok ? peaceConfessionsRes.data : [];

  const featured = peaceConfessions[0];
  const more = peaceConfessions.slice(1, 4);
  const hero = heroText(featured, peace);

  const jsonLd = {
    "@context": "https://schema.org",
    "@type": "WebSite",
    name: "iCONFESS",
    url: SITE_URL,
    description:
      "Spoken confession and reflection, made a daily practice across 39 areas of life.",
  };

  return (
    <div>
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd) }}
      />
      <SiteHeader />

      <main id="main">
        {/* ------------------------------------------------ 01 · PAUSE (Hero) */}
        <section className="ic-hero on-ink" aria-labelledby="hero-title">
          <div className="ic-container ic-hero__inner">
            <div>
              <p className="ic-eyebrow">A new way to speak what you believe</p>
              <h1 id="hero-title" style={{ marginTop: "var(--ic-spacing-5)" }}>
                Experience words differently.
              </h1>
              <p className="ic-hero__sub">
                iCONFESS turns spoken confession into an intentional daily
                practice — choose an area of life, hear the words, and return
                to them until they become part of you.
              </p>
              <div className="ic-hero__cta">
                <Link href="/register" className="ic-btn ic-btn--on-dark">
                  Start your experience
                </Link>
                <Link href="/explore" className="ic-btn ic-btn--secondary on-ink">
                  Explore
                </Link>
              </div>
            </div>

            <Reveal delay={120}>
              <div className="ic-hero-card">
                <div className="ic-hero-card__eyebrow">
                  <span>{hero.kicker}</span>
                  <span className="ic-wave" aria-hidden="true">
                    <span /><span /><span /><span /><span />
                  </span>
                </div>
                <blockquote className="ic-scripture">
                  {hero.text.length > 240 ? `${hero.text.slice(0, 240).trimEnd()}…` : hero.text}
                </blockquote>
                <div className="ic-hero-card__meta">
                  <span>{featured ? "From the library" : "The iCONFESS practice"}</span>
                  {featured && (
                    <Link href={`/confessions/${featured.id}`} className="ic-btn--text" style={{ fontSize: "var(--ic-font-size-caption)" }}>
                      Hear it
                    </Link>
                  )}
                </div>
              </div>
            </Reveal>
          </div>
        </section>

        {/* ------------------------------------------- 02 · FEEL (The idea) */}
        <section className="ic-section" aria-labelledby="idea-title">
          <div className="ic-container">
            <Reveal>
              <SectionHead
                id="idea-title"
                eyebrow="The idea"
                title="Your words can become a daily practice."
                lede="iCONFESS transforms spoken confession and reflection into an intentional experience. Not scrolling. Not skimming. Speaking words worth repeating, and hearing them again tomorrow."
              />
            </Reveal>
            <div className="ic-grid ic-grid--3" style={{ marginTop: "var(--ic-spacing-6)" }}>
              {[
                {
                  name: "Speak",
                  body: "Choose an area of life and say the words out loud — written to be spoken, not skimmed.",
                },
                {
                  name: "Hear",
                  body: "Listen as the words come back to you in a curated voice, at a pace you choose.",
                },
                {
                  name: "Repeat",
                  body: "Return tomorrow. A ritual is not a moment; it is a return.",
                },
              ].map((block, i) => (
                <Reveal key={block.name} delay={i * 90}>
                  <article className="ic-step" style={{ border: "none", paddingBlock: 0 }}>
                    <span className="ic-step__num" aria-hidden="true">
                      {String(i + 1).padStart(2, "0")}
                    </span>
                    <div>
                      <h3>{block.name}</h3>
                      <p>{block.body}</p>
                    </div>
                  </article>
                </Reveal>
              ))}
            </div>
          </div>
        </section>

        {/* ------------------------------------- 03 · EXPERIENCE (Product) */}
        <section className="ic-section ic-section--mist" aria-labelledby="experience-title">
          <div className="ic-container">
            <div className="ic-section-head--split ic-section-head" style={{ marginBottom: "var(--ic-spacing-8)" }}>
              <Reveal>
                <SectionHead
                  id="experience-title"
                  index="01"
                  eyebrow="The experience"
                  title="Turn a few intentional minutes into something you return to."
                />
              </Reveal>
              <Reveal delay={100}>
                <p className="ic-lede">
                  A session is built for the time you actually have. Choose an
                  area of life and a length; iCONFESS assembles the words and a
                  voice, and guides you through.
                </p>
              </Reveal>
            </div>
            <Reveal>
              <div className="ic-frame" role="img" aria-label="Preview of an iCONFESS session: the Peace category, a spoken confession, playback controls">
                <div className="ic-frame__bar" aria-hidden="true">
                  <span className="ic-frame__dot" />
                  <span className="ic-frame__dot" />
                  <span className="ic-frame__dot" />
                  <span style={{ marginLeft: "auto" }}>iCONFESS · session</span>
                </div>
                <div className="ic-frame__body">
                  <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline", gap: "var(--ic-spacing-4)" }}>
                    <span className="ic-frame__kicker">Category — {peace?.name ?? "Peace"}</span>
                    <span style={{ fontSize: "var(--ic-font-size-caption)", color: "var(--ic-color-neutral-400)", fontVariantNumeric: "tabular-nums" }}>
                      03:12 / 05:00
                    </span>
                  </div>
                  <p className="ic-scripture" style={{ color: "var(--ic-color-neutral-100)" }}>
                    {hero.text.length > 180 ? `${hero.text.slice(0, 180).trimEnd()}…` : hero.text}
                  </p>
                  <div>
                    <div className="ic-player-line" aria-hidden="true">
                      <i /><b />
                    </div>
                    <div style={{ display: "flex", alignItems: "center", gap: "var(--ic-spacing-5)", marginTop: "var(--ic-spacing-5)" }}>
                      <span style={{ display: "inline-flex", alignItems: "center", justifyContent: "center", width: 48, height: 48, borderRadius: "var(--ic-radius-full)", background: "var(--ic-color-brand-400)", color: "var(--ic-color-neutral-950)" }} aria-hidden="true">
                        <svg width="18" height="18" viewBox="0 0 18 18"><path d="M5 3l10 6-10 6V3z" fill="currentColor" /></svg>
                      </span>
                      <span style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-300)" }}>Play</span>
                      <span style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-300)", marginLeft: "auto" }}>Save · Share</span>
                    </div>
                  </div>
                </div>
              </div>
            </Reveal>
          </div>
        </section>

        {/* --------------------------------- 04 · UNDERSTAND (39 categories) */}
        <section className="ic-section" aria-labelledby="categories-title">
          <div className="ic-container">
            <Reveal>
              <SectionHead
                id="categories-title"
                index="02"
                eyebrow="Explore your experience"
                title="Find the words that meet you where you are."
                lede={`${categories.length || 39} areas of life — from peace and healing to purpose and provision. Open a spine, or walk them with your arrow keys.`}
                split
              />
            </Reveal>
            {categoriesRes.ok ? (
              <Reveal>
                <CategoryRail categories={categories.slice(0, 12)} />
              </Reveal>
            ) : (
              <ErrorStateInline />
            )}
            <div style={{ marginTop: "var(--ic-spacing-7)", display: "flex", justifyContent: "center" }}>
              <Link href="/categories" className="ic-btn ic-btn--secondary">
                See all {categories.length || 39} categories
              </Link>
            </div>
          </div>
        </section>

        {/* ------------------------------ 05 · EXPERIENCE (Featured band) */}
        {peace && (
          <section
            className="ic-section ic-section--ink on-ink"
            aria-labelledby="featured-cat-title"
            style={{ ["--rail" as string]: "#00072D" }}
          >
            <div className="ic-container ic-section-head--split ic-section-head">
              <Reveal>
                <p className="ic-eyebrow">Featured area of life</p>
                <h2 className="ic-display" id="featured-cat-title" style={{ marginTop: "var(--ic-spacing-4)" }}>
                  {peace.name}
                </h2>
                <p className="ic-lede" style={{ marginTop: "var(--ic-spacing-4)" }}>
                  {peace.description}
                </p>
                <div className="ic-btn-row" style={{ marginTop: "var(--ic-spacing-6)" }}>
                  <Link href={`/categories/${peace.slug}`} className="ic-btn ic-btn--on-dark">
                    Explore {peace.name}
                  </Link>
                  {featured && (
                    <Link href={`/confessions/${featured.id}`} className="ic-btn ic-btn--secondary on-ink">
                      Hear a confession
                    </Link>
                  )}
                </div>
              </Reveal>
              {more.length > 0 && (
                <Reveal delay={120}>
                  <div style={{ display: "grid", gap: "var(--ic-spacing-3)" }}>
                    {more.map((c) => (
                      <Link
                        key={c.id}
                        href={`/confessions/${c.id}`}
                        className="ic-card ic-confession-card"
                        style={{ background: "var(--ic-color-neutral-900)", borderColor: "var(--ic-color-neutral-800)", textDecoration: "none" }}
                      >
                        <h3 style={{ color: "var(--ic-color-neutral-0)" }}>{c.title}</h3>
                        <p style={{ color: "var(--ic-color-neutral-400)", WebkitLineClamp: 2 }}>{c.short_text || c.medium_text}</p>
                      </Link>
                    ))}
                  </div>
                </Reveal>
              )}
            </div>
          </section>
        )}

        {/* ------------------------------------------ 06 · DAILY RITUAL */}
        <section className="ic-section ic-section--mist" aria-labelledby="ritual-title">
          <div className="ic-container">
            <Reveal>
              <SectionHead
                id="ritual-title"
                index="03"
                eyebrow="Make it part of your day"
                title="Three quiet appointments with yourself."
                lede="Morning, midday, night — the words meet the day you are actually having."
              />
            </Reveal>
            <div className="ic-ritual">
              {[
                {
                  time: "6:00 AM",
                  name: "Morning",
                  body: "Start intentionally, before the noise begins.",
                  sample: "“This is the day the Lord has made; I will rejoice and be glad in it.”",
                },
                {
                  time: "12:30 PM",
                  name: "Midday",
                  body: "Reset your focus in the middle of everything.",
                  sample: "“I am not anxious about anything; peace guards my heart and my mind.”",
                },
                {
                  time: "10:00 PM",
                  name: "Night",
                  body: "Release the day, and rest.",
                  sample: "“I lay down in peace and sleep, for the Lord keeps me safe.”",
                },
              ].map((slot, i) => (
                <Reveal key={slot.name} delay={i * 90}>
                  <article className="ic-ritual__slot">
                    <time>{slot.time}</time>
                    <h3>{slot.name}</h3>
                    <p>{slot.body}</p>
                    <p className="ic-ritual__sample">{slot.sample}</p>
                  </article>
                </Reveal>
              ))}
            </div>
          </div>
        </section>

        {/* --------------------------------------- 07 · VOICE EXPERIENCE */}
        <section className="ic-section" aria-labelledby="voices-title">
          <div className="ic-container">
            <Reveal>
              <SectionHead
                id="voices-title"
                index="04"
                eyebrow="Hear the words differently"
                title="Choose the voice that helps the words land."
                lede="Every confession can be heard as well as read. The voices are curated and licensed — hear the difference they make."
                split
              />
            </Reveal>
            {voicesRes.ok && voices.length > 0 ? (
              <div className="ic-grid ic-grid--3">
                {voices.slice(0, 3).map((v, i) => (
                  <Reveal key={v.id} delay={i * 90}>
                    <article className="ic-card ic-card--hover" style={{ padding: "var(--ic-spacing-6)", display: "grid", gap: "var(--ic-spacing-4)" }}>
                      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                        <span className="ic-voice-avatar" aria-hidden="true">{v.name.charAt(0)}</span>
                        {v.premium && <span className="ic-chip">Premium voice</span>}
                      </div>
                      <div>
                        <h3 style={{ fontSize: "var(--ic-font-size-subheading)", fontWeight: "var(--ic-font-weight-semibold)" }}>{v.name}</h3>
                        <p style={{ color: "var(--ic-color-neutral-600)", fontSize: "var(--ic-font-size-bodySm)", marginTop: "var(--ic-spacing-2)", lineHeight: "var(--ic-font-lineHeight-relaxed)" }}>
                          {v.description}
                        </p>
                      </div>
                      <Link href={`/voices/${v.id}`} className="ic-btn--text" style={{ fontSize: "var(--ic-font-size-bodySm)" }}>
                        Meet {v.name}
                      </Link>
                    </article>
                  </Reveal>
                ))}
              </div>
            ) : (
              <ErrorStateInline />
            )}
          </div>
        </section>

        {/* ------------------------------------------- 08 · HOW IT WORKS */}
        <section className="ic-section ic-section--mist" aria-labelledby="how-title">
          <div className="ic-container ic-section-head--split ic-section-head">
            <Reveal>
              <SectionHead
                id="how-title"
                index="05"
                eyebrow="How it works"
                title="Four steps, then it's yours."
              />
            </Reveal>
            <Reveal delay={100}>
              <p className="ic-lede">
                No feed to refresh. The practice is small on purpose: choose,
                listen, repeat — until the words are part of how you live.
              </p>
            </Reveal>
          </div>
          <div className="ic-container">
            <div>
              {[
                { n: "01", t: "Choose", b: "Choose an area of life, or let today's suggestion find you." },
                { n: "02", t: "Listen", b: "Hear the confession spoken, with the Scripture it stands on." },
                { n: "03", t: "Repeat", b: "Return to the words — tomorrow, or whenever the moment asks." },
                { n: "04", t: "Live", b: "Make the experience part of your routine, at the pace you choose." },
              ].map((s) => (
                <div className="ic-step" key={s.n}>
                  <span className="ic-step__num" aria-hidden="true">{s.n}</span>
                  <div>
                    <h3>{s.t}</h3>
                    <p>{s.b}</p>
                  </div>
                </div>
              ))}
            </div>
            <div style={{ marginTop: "var(--ic-spacing-7)" }}>
              <Link href="/how-it-works" className="ic-btn ic-btn--secondary">
                See the full journey
              </Link>
            </div>
          </div>
        </section>

        {/* ---------------------------------------- 09 · PERSONALIZATION */}
        <section className="ic-section" aria-labelledby="personal-title">
          <div className="ic-container">
            <Reveal>
              <SectionHead
                id="personal-title"
                index="06"
                eyebrow="Made for your moment"
                title="The words meet the season you are in."
                lede="Tell iCONFESS what you are walking through and it shapes each session around it — your morning, your focus, your relationships, your peace. Signals come from what you choose to hear, not from a profile sold to anyone."
                split
              />
            </Reveal>
            <div className="ic-grid ic-grid--3">
              {["For your morning", "For your current season", "For your focus", "For your relationships", "For your goals", "For your peace"].map((label, i) => (
                <Reveal key={label} delay={(i % 3) * 80}>
                  <div className="ic-card ic-card--hover" style={{ padding: "var(--ic-spacing-5)", display: "grid", gap: "var(--ic-spacing-3)" }}>
                    <span className="ic-frame__kicker">{label}</span>
                    <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)" }}>
                      A session shaped by the interests you set and the moments you return to.
                    </p>
                  </div>
                </Reveal>
              ))}
            </div>
          </div>
        </section>

        {/* ---------------------------------------------- 10 · COMMUNITY */}
        <section className="ic-section ic-section--mist" aria-labelledby="community-title">
          <div className="ic-container ic-section-head--split ic-section-head">
            <Reveal>
              <SectionHead
                id="community-title"
                index="07"
                eyebrow="Your words matter too"
                title="Write confessions of your own."
                lede="Every account can write personal confessions — keep them private, or offer them for review to appear in the community. Reviewed before publication, always."
              />
            </Reveal>
            <Reveal delay={100}>
              <div style={{ display: "grid", gap: "var(--ic-spacing-3)" }}>
                {[
                  ["Private", "Only you ever see it."],
                  ["Shared", "You decide who can hear it."],
                  ["Community", "Reviewed by people, then published."],
                ].map(([name, body]) => (
                  <div key={name} className="ic-card" style={{ padding: "var(--ic-spacing-5)" }}>
                    <h3 style={{ fontSize: "var(--ic-font-size-body)", fontWeight: "var(--ic-font-weight-semibold)" }}>{name}</h3>
                    <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", marginTop: "var(--ic-spacing-1)" }}>{body}</p>
                  </div>
                ))}
                <Link href="/community" className="ic-btn ic-btn--secondary" style={{ marginTop: "var(--ic-spacing-3)" }}>
                  How community works
                </Link>
              </div>
            </Reveal>
          </div>
        </section>

        {/* ------------------------------------------------ 11 · PREMIUM */}
        <section className="ic-section ic-section--ink on-ink" aria-labelledby="premium-title">
          <div className="ic-container ic-section-head--split ic-section-head">
            <Reveal>
              <p className="ic-eyebrow">Premium</p>
              <h2 className="ic-display" id="premium-title" style={{ marginTop: "var(--ic-spacing-4)" }}>
                Go deeper.
              </h2>
            </Reveal>
            <Reveal delay={100}>
              <p className="ic-lede">
                Longer sessions, premium voices, deeper categories, and the full
                library — for the practice you keep, not the one you sample.
              </p>
              <div className="ic-btn-row" style={{ marginTop: "var(--ic-spacing-6)" }}>
                <Link href="/premium" className="ic-btn ic-btn--on-dark">Explore Premium</Link>
                <Link href="/pricing" className="ic-btn ic-btn--secondary on-ink">See plans</Link>
              </div>
            </Reveal>
          </div>
        </section>

        {/* ------------------------------------------ 12 · SOCIAL PROOF */}
        {featured && (
          <section className="ic-section" aria-labelledby="proof-title">
            <div className="ic-container ic-container--text" style={{ textAlign: "center" }}>
              <Reveal>
                <p className="ic-eyebrow" style={{ justifyContent: "center" }}>From the library</p>
                <blockquote className="ic-scripture" style={{ marginTop: "var(--ic-spacing-5)" }}>
                  “{featured.short_text || featured.medium_text || featured.long_text}”
                  <cite>— {featured.title}{peace ? `, ${peace.name}` : ""}</cite>
                </blockquote>
                <p style={{ marginTop: "var(--ic-spacing-6)", fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-500)" }}>
                  {peace?.name ?? "Peace"} is one of {categories.length || 39} areas of life in the library — every one with words reviewed before they are published.
                </p>
              </Reveal>
            </div>
          </section>
        )}

        {/* ------------------------------------------- 13 · APP DOWNLOAD */}
        <section className="ic-section ic-section--ink on-ink" aria-labelledby="download-title">
          <div className="ic-container ic-section-head--split ic-section-head" style={{ alignItems: "center" }}>
            <Reveal>
              <p className="ic-eyebrow">The app</p>
              <h2 className="ic-display" id="download-title" style={{ marginTop: "var(--ic-spacing-4)" }}>
                Your experience, wherever you are.
              </h2>
              <p className="ic-lede" style={{ marginTop: "var(--ic-spacing-4)" }}>
                Offline listening, scheduled reminders, and your practice in
                your pocket.
              </p>
              <div className="ic-btn-row" style={{ marginTop: "var(--ic-spacing-6)" }}>
                <Link href="/download" className="ic-btn ic-btn--on-dark">
                  Get the app
                </Link>
              </div>
            </Reveal>
            <Reveal delay={120}>
              <div className="ic-frame" style={{ maxWidth: "22rem", marginLeft: "auto" }}>
                <div className="ic-frame__body" style={{ textAlign: "center" }}>
                  <span className="ic-frame__kicker">iOS &amp; Android</span>
                  <p style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-font-size-heading)", lineHeight: 1.3 }}>
                    Morning light, evening quiet — the practice travels with you.
                  </p>
                  <Link href="/download" className="ic-btn ic-btn--on-dark ic-btn--small" style={{ justifyContent: "center" }}>
                    Download
                  </Link>
                </div>
              </div>
            </Reveal>
          </div>
        </section>

        {/* ------------------------------------------------------ 14 · FAQ */}
        <section className="ic-section" aria-labelledby="faq-title">
          <div className="ic-container">
            <Reveal>
              <SectionHead
                id="faq-title"
                index="08"
                eyebrow="Questions"
                title="Asked, answered."
              />
            </Reveal>
            <Reveal>
              <Faq />
            </Reveal>
          </div>
        </section>

        {/* --------------------------------------------- 15 · FINAL CTA */}
        <section className="ic-section ic-section--ink on-ink" aria-labelledby="final-title" style={{ textAlign: "center" }}>
          <div className="ic-container ic-container--text">
            <Reveal>
              <h2 className="ic-display" id="final-title" style={{ marginInline: "auto" }}>
                Start with one confession.
              </h2>
              <p className="ic-lede" style={{ margin: "var(--ic-spacing-5) auto 0" }}>
                One minute, one area of life, one voice. Hear the words — then
                decide.
              </p>
              <div className="ic-btn-row" style={{ justifyContent: "center", marginTop: "var(--ic-spacing-7)" }}>
                <Link href="/register" className="ic-btn ic-btn--on-dark">
                  Begin your experience
                </Link>
                <Link href="/explore" className="ic-btn ic-btn--secondary on-ink">
                  Explore first
                </Link>
              </div>
            </Reveal>
          </div>
        </section>
      </main>

      {/* -------------------------------------------------- 16 · FOOTER */}
      <SiteFooter />
    </div>
  );
}

function Faq() {
  return (
    <div className="ic-faq">
      {[
        [
          "What is iCONFESS?",
          "iCONFESS is a daily confession practice. Choose an area of life, hear a confession spoken over you, and return to it until it becomes part of how you think and live.",
        ],
        [
          "How does iCONFESS work?",
          "Pick an area of life and how long you have. iCONFESS assembles a session of confessions and Scripture in a curated voice, and guides you through it. Sessions can be scheduled, saved, and repeated.",
        ],
        [
          "Is iCONFESS free?",
          "The core experience is free, including a wide selection of categories and standard session lengths. Premium adds the full library, longer sessions, and premium voices.",
        ],
        [
          "What are premium experiences?",
          "Premium unlocks the complete library of 39 areas of life, extended session lengths, premium voices, and personalised routines.",
        ],
        [
          "Can I create my own confession?",
          "Yes. Write confessions in your own words. Keep them private, share them with people you choose, or offer them for review — nothing is published without review.",
        ],
        [
          "Can my confession remain private?",
          "Yes. Private confessions are yours alone, and deletion tools are built in. Publication only ever happens to content you explicitly offer for review.",
        ],
        [
          "Can I listen offline?",
          "The mobile app supports offline listening for saved sessions on Premium.",
        ],
        [
          "What voices are available?",
          "iCONFESS works with curated narration voices. Each has its own pace and character — choose the one that helps the words land.",
        ],
        [
          "How do I cancel Premium?",
          "From your subscription settings, at any time. Your practice and history remain yours.",
        ],
        [
          "Is my content private?",
          "Yes. Private content is never exposed publicly or to search engines, and public sharing only happens through explicit review and publication.",
        ],
      ].map(([q, a]) => (
        <details key={q}>
          <summary>{q}</summary>
          <p>{a}</p>
        </details>
      ))}
    </div>
  );
}

function ErrorStateInline() {
  return (
    <div className="ic-state" role="status">
      <h3>We couldn't reach the library</h3>
      <p>Something went wrong while loading this experience. It will be back shortly.</p>
      <Link href="/categories" className="ic-btn ic-btn--secondary">
        Browse categories
      </Link>
    </div>
  );
}
