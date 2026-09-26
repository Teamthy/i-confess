import type { Metadata } from "next";
import Image from "next/image";
import Link from "next/link";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";
import { Reveal } from "@/components/Reveal";
import { SectionHead } from "@/components/Sections";
import { VoicePreview } from "@/components/VoicePreview";
import { HomeExperienceCards, type CuratedCards } from "@/components/HomeExperienceCards";
import { NewsletterForm } from "@/components/NewsletterForm";
import { motifStyle } from "@/lib/categoryColor";
import { api, type Category, type Confession, type Voice } from "@/lib/api";
import { ARTICLES } from "@/content/articles";
import { SITE_URL } from "@/lib/site";

export const metadata: Metadata = {
  alternates: { canonical: "/" },
};

/**
 * Homepage — the reference rhythm (ledger 47, motifs M2–M12):
 *
 *   light editorial hero → overlapping cards → light story →
 *   DARK category panel → light principles → immersive image →
 *   light voices → mist community → light statement → light journal →
 *   mist FAQ → newsletter split → light final CTA → DARK footer.
 *
 * All catalogue data (categories, confessions, voices, counts) is fetched
 * from the Go API at request time with ISR; nothing is fabricated. If the
 * API is down the page still renders its editorial content with honest
 * error states in the data-driven rails.
 */

const FAQ: Array<[string, string]> = [
  [
    "What is iCONFESS?",
    "iCONFESS is a daily confession practice. Choose an area of life, hear a confession spoken over you, and return to it until it becomes part of how you think and live.",
  ],
  [
    "How does iCONFESS work?",
    "Pick an area of life and how long you have. iCONFESS assembles a session of confessions and Scripture in a curated voice, and guides you through it. Sessions can be saved, scheduled, and repeated.",
  ],
  [
    "What are categories?",
    "Categories are the 39 areas of life the library is organised around — peace, healing, faith, provision, relationships, and more. Each one holds confessions written for that area, and sessions are built from them.",
  ],
  [
    "Can I create my own confession?",
    "Yes. Write confessions in your own words from your account. Keep them private, share them with people you choose, or offer them for review — nothing reaches the community without review.",
  ],
  [
    "Can my confession remain private?",
    "Yes. Private confessions are yours alone, and deletion tools are built in. Publication only ever happens to content you explicitly offer for review.",
  ],
  [
    "How does audio work?",
    "Every confession can be heard as well as read, in a curated voice you choose. Sessions stream with playback controls — play, pause, seek, speed — and the mobile app supports offline listening for saved sessions on Premium.",
  ],
  [
    "What is Premium?",
    "Premium unlocks the complete library, extended session lengths, premium voices, and personalised routines. The core experience — including a wide selection of categories — is free.",
  ],
  [
    "Can I use iCONFESS on the web?",
    "Yes. The full experience — sessions, audio, routines, schedules, community, settings — runs in the browser. Sign in once and your practice follows you.",
  ],
  [
    "Can I use iCONFESS on mobile?",
    "Yes. The iOS and Android apps carry the same practice with offline listening and reminders. See the download page for availability in your region.",
  ],
  [
    "How do I manage my subscription?",
    "From your subscription settings, at any time — upgrade, cancel, or restore a purchase. Your practice and history remain yours whatever you choose.",
  ],
  [
    "How is my data handled?",
    "Private content is never exposed publicly or to search engines. The privacy policy names exactly what is collected and why, and account deletion is self-serve.",
  ],
];

const PRINCIPLES = [
  {
    n: "01",
    t: "Intention",
    b: "Every session begins with a choice — an area of life, a length, a voice. Nothing plays by accident.",
    active: false,
  },
  {
    n: "02",
    t: "Presence",
    b: "One confession at a time, spoken then rested. No feed, no queue of infinite nexts.",
    active: true,
  },
  {
    n: "03",
    t: "Repetition",
    b: "The words return daily until they stop being affirmations and start being descriptions.",
    active: false,
  },
  {
    n: "04",
    t: "Personal experience",
    b: "Your shelf, your routines, your schedule. The practice shapes around the life you have.",
    active: false,
  },
];

const JOURNAL_IMAGES = ["/images/story.jpg", "/images/newsletter.jpg", "/images/community.jpg"];

export default async function HomePage() {
  const [categoriesRes, voicesRes] = await Promise.all([api.categories(), api.voices()]);
  const categories: Category[] = categoriesRes.ok ? categoriesRes.data : [];
  const voices: Voice[] = voicesRes.ok ? voicesRes.data.filter((v) => v.status === "active") : [];

  const peace = categories.find((c) => c.slug === "peace") ?? categories[0];
  const peaceConfessionsRes = peace
    ? await api.categoryConfessions(peace.id)
    : ({ ok: false, status: 404, message: "" } as const);
  const peaceConfessions: Confession[] = peaceConfessionsRes.ok ? peaceConfessionsRes.data : [];

  const featured = peaceConfessions[0];
  const featuredVoice = voices.find((v) => !v.premium) ?? voices[0];
  const spotlight = categories.find((c) => c.slug === "healing") ?? categories[1] ?? peace;

  const curated: CuratedCards = {
    confession: featured
      ? { id: featured.id, title: featured.title, categoryName: peace?.name ?? "Peace" }
      : null,
    category: spotlight
      ? { slug: spotlight.slug, name: spotlight.name, description: spotlight.description ?? "" }
      : null,
    voice: featuredVoice
      ? { id: featuredVoice.id, name: featuredVoice.name, description: featuredVoice.description ?? "" }
      : null,
    categoryCount: categories.length,
  };

  const statement = featured?.short_text || featured?.medium_text || featured?.long_text || "";

  const jsonLd = {
    "@context": "https://schema.org",
    "@type": "WebSite",
    name: "iCONFESS",
    url: SITE_URL,
    description: "Spoken confession and reflection, made a daily practice across 39 areas of life.",
  };

  return (
    <div>
      <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd) }} />
      <SiteHeader light />

      <main id="main">
        {/* ------------------------------------------------ 01 · HERO (M2) */}
        <section className="ic-edhero" aria-labelledby="hero-title">
          <div className="ic-edhero__type">
            <p className="ic-eyebrow">iCONFESS</p>
            <h1 className="ic-edhero__title" id="hero-title">
              Speak it. Hear&nbsp;it. Live&nbsp;it.
            </h1>
            <p className="ic-edhero__sub">
              Turn the words you believe in into an experience you can return to
              every day — spoken confession, guided sessions, and voices that
              help the words land.
            </p>
            <div className="ic-edhero__cta">
              <Link href="/register" className="ic-btn ic-btn--primary">
                Start your experience
              </Link>
              <Link href="/explore" className="ic-btn ic-btn--secondary">
                Explore iCONFESS
              </Link>
            </div>
          </div>
          <div className="ic-container">
            <div className="ic-edhero__media">
              <Image
                src="/images/hero.jpg"
                alt="A woman listening to iCONFESS with headphones by a bright window at dawn"
                fill
                priority
                sizes="(max-width: 76rem) 100vw, 76rem"
              />
            </div>
          </div>
        </section>

        {/* ----------------------------------- 02 · FLOATING CARDS (M3) */}
        <section className="ic-float" aria-label="Featured experiences">
          <div className="ic-container">
            <HomeExperienceCards curated={curated} />
          </div>
        </section>

        {/* ------------------------------------------ 03 · STORY (M4) */}
        <section className="ic-section" aria-labelledby="story-title">
          <div className="ic-container ic-story">
            <Reveal>
              <div>
                <SectionHead
                  id="story-title"
                  eyebrow="Our philosophy"
                  title="Words are more powerful when you return to them."
                  lede="iCONFESS exists for one reason: the words you believe should be words you live with. Not scrolled past — spoken, heard, and repeated until they become part of you."
                />
                <div style={{ marginTop: "var(--ic-spacing-6)" }}>
                  <Link href="/about" className="ic-btn ic-btn--secondary">
                    Why iCONFESS exists
                  </Link>
                </div>
                <dl className="ic-story__facts">
                  <div className="ic-story__fact">
                    <strong>{categories.length || 39}</strong>
                    <span>areas of life</span>
                  </div>
                  <div className="ic-story__fact">
                    <strong>{voices.length || 3}</strong>
                    <span>curated voices</span>
                  </div>
                  <div className="ic-story__fact">
                    <strong>5-min</strong>
                    <span>starter session</span>
                  </div>
                </dl>
              </div>
            </Reveal>
            <Reveal delay={120}>
              <div className="ic-story__media">
                <Image
                  src="/images/story.jpg"
                  alt="Hands holding an open book in soft window light"
                  fill
                  sizes="(max-width: 56rem) 100vw, 40vw"
                />
              </div>
            </Reveal>
          </div>
        </section>

        {/* --------------------------------- 04 · DARK CATEGORIES (M5) */}
        <section className="ic-section" style={{ paddingTop: 0 }} aria-labelledby="categories-title">
          <div className="ic-container">
            <div className="ic-panel">
              <Reveal>
                <p className="ic-eyebrow">Explore your experience</p>
                <h2 className="ic-panel__title" id="categories-title">
                  Find the words that meet you where you are.
                </h2>
                <p className="ic-panel__lede">
                  {categories.length || 39} areas of life — from peace and healing
                  to purpose and provision. Open one, or walk them all.
                </p>
              </Reveal>
              {categoriesRes.ok && categories.length > 0 ? (
                <div className="ic-snap ic-snap--peek ic-snap--4" role="list" aria-label="Categories">
                  {categories.slice(0, 8).map((c) => (
                    <Link key={c.id} href={`/categories/${c.slug}`} className="ic-cat" role="listitem">
                      <span className="ic-cat__visual" style={motifStyle(c.slug)} aria-hidden="true">
                        <span className="ic-cat__orb" />
                        <span className="ic-cat__initial">{c.name.charAt(0)}</span>
                      </span>
                      <span className="ic-cat__body">
                        <h3>{c.name}</h3>
                        {c.description && <p>{c.description}</p>}
                        <span className="ic-cat__go">Explore →</span>
                      </span>
                    </Link>
                  ))}
                </div>
              ) : (
                <ErrorStateInline />
              )}
              <div style={{ marginTop: "var(--ic-spacing-6)", display: "flex", justifyContent: "center" }}>
                <Link href="/categories" className="ic-btn ic-btn--on-dark">
                  See all {categories.length || 39} categories
                </Link>
              </div>
            </div>
          </div>
        </section>

        {/* --------------------------------------- 05 · PRINCIPLES (M6) */}
        <section className="ic-section" style={{ paddingTop: 0 }} aria-labelledby="principles-title">
          <div className="ic-container">
            <Reveal>
              <SectionHead
                id="principles-title"
                eyebrow="Our approach"
                title="Designed around how you actually experience words."
                split
              />
            </Reveal>
            <div className="ic-prin" style={{ marginTop: "var(--ic-spacing-7)" }}>
              {PRINCIPLES.map((p, i) => (
                <Reveal key={p.n} delay={i * 80}>
                  <article className={`ic-prin__card${p.active ? " ic-prin__card--active" : ""}`}>
                    <span className="ic-prin__num" aria-hidden="true">{p.n}</span>
                    <h3>{p.t}</h3>
                    <p>{p.b}</p>
                  </article>
                </Reveal>
              ))}
            </div>
          </div>
        </section>

        {/* ---------------------------------------- 06 · IMMERSIVE (M7) */}
        <section className="ic-section" style={{ paddingTop: 0 }} aria-labelledby="immersive-title">
          <div className="ic-container">
            <Reveal>
              <div className="ic-immers">
                <Image
                  src="/images/immersive.jpg"
                  alt="A lone figure walking a quiet coastal path at blue hour"
                  fill
                  sizes="(max-width: 76rem) 100vw, 76rem"
                />
                <div className="ic-immers__card">
                  <p className="ic-eyebrow">For this moment</p>
                  <h2 id="immersive-title">You&apos;ll find what you need for this moment.</h2>
                  <p>
                    Morning light or midnight quiet — choose the hour, and
                    iCONFESS shapes the words around it.
                  </p>
                  <div>
                    <Link href="/explore" className="ic-btn ic-btn--primary">
                      Explore the experience
                    </Link>
                  </div>
                </div>
              </div>
            </Reveal>
          </div>
        </section>

        {/* -------------------------------------------- 07 · VOICES (M8) */}
        <section className="ic-section" style={{ paddingTop: 0 }} aria-labelledby="voices-title">
          <div className="ic-container">
            <Reveal>
              <SectionHead
                id="voices-title"
                eyebrow="Hear the words differently"
                title="Choose the voice that helps the words land."
                lede="Every confession can be heard as well as read. The voices are curated and licensed — press play and hear the difference."
                split
              />
            </Reveal>
            {voicesRes.ok && voices.length > 0 ? (
              <div className="ic-snap" role="list" aria-label="Voices">
                {voices.map((v) => (
                  <article key={v.id} className="ic-voice" role="listitem">
                    <div className="ic-voice__top">
                      <span className="ic-voice__avatar" aria-hidden="true">{v.name.charAt(0)}</span>
                      <div>
                        <h3 className="ic-voice__name">{v.name}</h3>
                        <p className="ic-voice__style">
                          {[v.gender, v.type].filter(Boolean).join(" · ") || "Narration"}
                          {v.premium ? " · Premium" : ""}
                        </p>
                      </div>
                    </div>
                    {v.description && <p className="ic-voice__desc">{v.description}</p>}
                    <VoicePreview sampleUrl={v.sample_url} voiceName={v.name} />
                    <Link href={`/voices/${v.id}`} className="ic-btn--text" style={{ fontSize: "var(--ic-font-size-bodySm)" }}>
                      Explore {v.name} →
                    </Link>
                  </article>
                ))}
              </div>
            ) : (
              <ErrorStateInline />
            )}
          </div>
        </section>

        {/* ---------------------------------------- 08 · COMMUNITY (M9) */}
        <section className="ic-section ic-section--mist" aria-labelledby="community-title">
          <div className="ic-container ic-split">
            <Reveal>
              <div className="ic-split__media">
                <Image
                  src="/images/community.jpg"
                  alt="Two friends in quiet conversation over a table in warm evening light"
                  fill
                  sizes="(max-width: 56rem) 100vw, 45vw"
                />
              </div>
            </Reveal>
            <Reveal delay={100}>
              <div>
                <SectionHead
                  id="community-title"
                  eyebrow="Your words matter too"
                  title="Write confessions of your own."
                  lede="Keep them private, share them intentionally, or offer them to the community. Reviewed by people before publication — always."
                />
                <div className="ic-state-row">
                  <div className="ic-state-row__item">
                    <strong>Private</strong>
                    <span>Only you ever see it.</span>
                  </div>
                  <div className="ic-state-row__item">
                    <strong>Shared</strong>
                    <span>You decide who can hear it.</span>
                  </div>
                  <div className="ic-state-row__item">
                    <strong>Public</strong>
                    <span>Reviewed by people, then published.</span>
                  </div>
                </div>
                <div className="ic-btn-row" style={{ marginTop: "var(--ic-spacing-6)" }}>
                  <Link href="/register" className="ic-btn ic-btn--primary">
                    Create your confession
                  </Link>
                  <Link href="/community" className="ic-btn ic-btn--secondary">
                    How community works
                  </Link>
                </div>
              </div>
            </Reveal>
          </div>
        </section>

        {/* ----------------------------------------- 09 · STATEMENT (M10) */}
        <section className="ic-section" aria-labelledby="statement-title">
          <div className="ic-container">
            <Reveal>
              <p className="ic-eyebrow" id="statement-title">From the library</p>
            </Reveal>
            <div className="ic-quote" style={{ marginTop: "var(--ic-spacing-5)" }}>
              <Reveal>
                <blockquote>
                  “{statement || "An experience I can actually return to every day."}”
                  {featured && (
                    <cite>— {featured.title}{peace ? `, ${peace.name}` : ""}</cite>
                  )}
                </blockquote>
              </Reveal>
              <Reveal delay={100}>
                <p className="ic-quote__side">
                  {peace?.name ?? "Peace"} is one of {categories.length || 39} areas
                  of life in the library — every one with words reviewed before
                  they are published.
                </p>
              </Reveal>
            </div>
          </div>
        </section>

        {/* ------------------------------------------- 10 · JOURNAL (M11) */}
        <section className="ic-section" style={{ paddingTop: 0 }} aria-labelledby="journal-title">
          <div className="ic-container">
            <Reveal>
              <SectionHead
                id="journal-title"
                eyebrow="Words to return to"
                title="Hear directly from iCONFESS."
                split
              />
            </Reveal>
            <div className="ic-journal" style={{ marginTop: "var(--ic-spacing-7)" }}>
              {ARTICLES.slice(0, 3).map((a, i) => (
                <Reveal key={a.slug} delay={i * 80}>
                  <Link href={`/journal/${a.slug}`} className="ic-journal__card">
                    <span className="ic-journal__img" aria-hidden="true">
                      <Image
                        src={JOURNAL_IMAGES[i % JOURNAL_IMAGES.length]}
                        alt=""
                        fill
                        sizes="(max-width: 56rem) 100vw, 33vw"
                      />
                    </span>
                    <span className="ic-journal__body">
                      <span className="ic-journal__cat">{a.category}</span>
                      <h3>{a.title}</h3>
                      <p>{a.excerpt}</p>
                    </span>
                  </Link>
                </Reveal>
              ))}
            </div>
            <div style={{ marginTop: "var(--ic-spacing-6)" }}>
              <Link href="/journal" className="ic-btn ic-btn--secondary">
                Read the journal
              </Link>
            </div>
          </div>
        </section>

        {/* ----------------------------------------------- 11 · FAQ (M12) */}
        <section className="ic-section ic-section--mist" aria-labelledby="faq-title">
          <div className="ic-container ic-faq2">
            <div className="ic-faq2__head">
              <Reveal>
                <p className="ic-eyebrow">Finally, some answers</p>
                <h2 className="ic-display" id="faq-title" style={{ marginTop: "var(--ic-spacing-4)" }}>
                  Questions about iCONFESS?
                </h2>
                <p className="ic-lede" style={{ marginTop: "var(--ic-spacing-4)" }}>
                  The short version of everything people ask before they begin.
                </p>
                <div style={{ marginTop: "var(--ic-spacing-6)" }}>
                  <Link href="/faq" className="ic-btn ic-btn--secondary">
                    All questions
                  </Link>
                </div>
              </Reveal>
            </div>
            <Reveal>
              <div className="ic-faq">
                {FAQ.map(([q, a]) => (
                  <details key={q}>
                    <summary>{q}</summary>
                    <p>{a}</p>
                  </details>
                ))}
              </div>
            </Reveal>
          </div>
        </section>

        {/* ------------------------------------------ 12 · DOWNLOAD */}
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

        {/* ----------------------------------------- 13 · NEWSLETTER (M12) */}
        <section className="ic-section" aria-labelledby="news-title">
          <div className="ic-container">
            <Reveal>
              <div className="ic-news">
                <div className="ic-news__body">
                  <p className="ic-eyebrow">Latest from iCONFESS</p>
                  <h2 id="news-title">The weekly words, in your inbox.</h2>
                  <p>
                    One confession, one reflection, one invitation to return —
                    every week. It arrives with your account, and you can
                    silence it any time from notification settings.
                  </p>
                  <NewsletterForm />
                </div>
                <div className="ic-news__img">
                  <Image
                    src="/images/newsletter.jpg"
                    alt="An open journal and glasses in soft morning light"
                    fill
                    sizes="(max-width: 56rem) 100vw, 50vw"
                  />
                </div>
              </div>
            </Reveal>
          </div>
        </section>

        {/* ------------------------------------------ 14 · FINAL CTA */}
        <section className="ic-section" style={{ paddingTop: 0, textAlign: "center" }} aria-labelledby="final-title">
          <div className="ic-container ic-container--text">
            <Reveal>
              <p className="ic-eyebrow" style={{ justifyContent: "center" }}>Ready to begin?</p>
              <h2 className="ic-display" id="final-title" style={{ margin: "var(--ic-spacing-4) auto 0" }}>
                Start with one confession.
              </h2>
              <p className="ic-lede" style={{ margin: "var(--ic-spacing-5) auto 0" }}>
                One minute, one area of life, one voice. Hear the words — then
                decide.
              </p>
              <div className="ic-btn-row" style={{ justifyContent: "center", marginTop: "var(--ic-spacing-7)" }}>
                <Link href="/register" className="ic-btn ic-btn--primary">
                  Start your experience
                </Link>
                <Link href="/download" className="ic-btn ic-btn--secondary">
                  Download the app
                </Link>
              </div>
            </Reveal>
          </div>
        </section>
      </main>

      {/* -------------------------------------------------- 15 · FOOTER */}
      <SiteFooter />
    </div>
  );
}

function ErrorStateInline() {
  return (
    <div className="ic-state" role="status" style={{ marginTop: "var(--ic-spacing-6)" }}>
      <h3>We couldn&apos;t reach the library</h3>
      <p>Something went wrong while loading this experience. It will be back shortly.</p>
      <Link href="/categories" className="ic-btn ic-btn--secondary">
        Browse categories
      </Link>
    </div>
  );
}
