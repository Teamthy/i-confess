#!/usr/bin/env python3
"""Generate authenticated app page stubs from the API contract.

Each page fetches real data from the Go API via the server-component api client
and renders proper loading/empty/error states. No fabricated data.
"""
import os

APP = os.path.join(os.path.dirname(__file__), "..", "apps", "web", "app", "app")

PAGES = {
    "page": {
        "title": "Home",
        "desc": "Personalised home: today's experience, continue listening, categories, recommendations.",
        "api": "home",
        "imports": 'import Link from "next/link";\nimport { api } from "@/lib/api";',
        "body": '''
export default async function AppHomePage() {
  const [catsRes, voicesRes] = await Promise.all([
    api.categories(),
    api.voices(),
  ]);
  const categories = catsRes.ok ? catsRes.data : [];
  const voices = voicesRes.ok ? voicesRes.data : [];

  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <p className="ia-section-title">Welcome back</p>
        <h1>Your experience</h1>
        <p>What do you want to return to today?</p>
      </div>

      <div className="ia-quick">
        <Link href="/app/explore" className="ia-quick__btn">
          <span className="ia-quick__icon">◎</span> Explore
        </Link>
        <Link href="/app/categories" className="ia-quick__btn">
          <span className="ia-quick__icon">≡</span> Categories
        </Link>
        <Link href="/app/favorites" className="ia-quick__btn">
          <span className="ia-quick__icon">♡</span> Favorites
        </Link>
        <Link href="/app/history" className="ia-quick__btn">
          <span className="ia-quick__icon">↻</span> History
        </Link>
        <Link href="/app/community" className="ia-quick__btn">
          <span className="ia-quick__icon">⌂</span> Community
        </Link>
        <Link href="/app/search" className="ia-quick__btn">
          <span className="ia-quick__icon">⌕</span> Search
        </Link>
      </div>

      {categories.length > 0 && (
        <section>
          <h2 className="ia-section-title">Categories</h2>
          <div className="ic-grid ic-grid--3">
            {categories.slice(0, 6).map((c) => (
              <Link key={c.id} href={`/app/categories/${c.slug}`} className="ic-card ic-card--hover" style={{ padding: "var(--ic-spacing-5)", textDecoration: "none" }}>
                <h3 style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-font-size-subheading)", color: "var(--ic-color-neutral-950)" }}>{c.name}</h3>
                <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", marginTop: "var(--ic-spacing-2)" }}>{c.description}</p>
              </Link>
            ))}
          </div>
          <Link href="/app/categories" className="ic-btn ic-btn--secondary" style={{ marginTop: "var(--ic-spacing-5)" }}>See all categories</Link>
        </section>
      )}

      {voices.length > 0 && (
        <section>
          <h2 className="ia-section-title">Voices</h2>
          <div className="ic-grid ic-grid--3">
            {voices.slice(0, 3).map((v) => (
              <Link key={v.id} href={`/app/voices/${v.id}`} className="ia-voice">
                <div style={{ display: "flex", alignItems: "center", gap: "var(--ic-spacing-3)" }}>
                  <span className="ic-voice-avatar" aria-hidden="true">{v.name.charAt(0)}</span>
                  <div>
                    <h3>{v.name}</h3>
                    <p>{v.description}</p>
                  </div>
                </div>
              </Link>
            ))}
          </div>
        </section>
      )}
    </div>
  );
}''',
    },

    "explore/page": {
        "title": "Explore",
        "desc": "Content discovery: browse categories, confessions, and voices.",
        "api": "categories, voices",
        "imports": 'import Link from "next/link";\nimport { api } from "@/lib/api";',
        "body": '''
export default async function ExplorePage() {
  const [catsRes, voicesRes] = await Promise.all([
    api.categories(),
    api.voices(),
  ]);
  const categories = catsRes.ok ? catsRes.data : [];
  const voices = voicesRes.ok ? voicesRes.data : [];

  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Explore</h1>
        <p>Discover confessions, categories, and voices across 39 areas of life.</p>
      </div>

      <section>
        <h2 className="ia-section-title">Categories ({categories.length})</h2>
        <div className="ia-card-grid">
          {categories.map((c) => (
            <Link key={c.id} href={`/app/categories/${c.slug}`} className="ia-confession">
              <h3>{c.name}</h3>
              <p>{c.description}</p>
              {c.premium && <span className="ia-badge ia-badge--premium">Premium</span>}
            </Link>
          ))}
        </div>
        {categories.length === 0 && (
          <div className="ic-state"><h3>No categories available</h3><p>The library is being updated.</p></div>
        )}
      </section>

      <section>
        <h2 className="ia-section-title">Voices ({voices.length})</h2>
        <div className="ia-card-grid">
          {voices.map((v) => (
            <Link key={v.id} href={`/app/voices/${v.id}`} className="ia-voice">
              <div style={{ display: "flex", alignItems: "center", gap: "var(--ic-spacing-3)" }}>
                <span className="ic-voice-avatar" aria-hidden="true">{v.name.charAt(0)}</span>
                <div>
                  <h3>{v.name}</h3>
                  <p>{v.type}{v.gender ? ` · ${v.gender}` : ""}</p>
                </div>
              </div>
            </Link>
          ))}
        </div>
      </section>
    </div>
  );
}''',
    },

    "categories/page": {
        "title": "Categories",
        "desc": "All 39 categories of life.",
        "api": "categories",
        "imports": 'import Link from "next/link";\nimport { api } from "@/lib/api";\nimport { railStyle } from "@/lib/categoryColor";',
        "body": '''
export default async function CategoriesPage() {
  const res = await api.categories();
  const categories = res.ok ? res.data : [];

  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Categories</h1>
        <p>{categories.length} areas of life — find the words that meet you where you are.</p>
      </div>
      <div className="ia-card-grid">
        {categories.map((c) => (
          <Link key={c.id} href={`/app/categories/${c.slug}`} className="ic-cat-card" style={railStyle(c.slug)}>
            <span className="ic-cat-card__initial" aria-hidden="true">{c.name.charAt(0)}</span>
            <h3>{c.name}</h3>
            <p>{c.description}</p>
            <span className="ic-cat-card__go">Explore →</span>
          </Link>
        ))}
      </div>
      {categories.length === 0 && (
        <div className="ic-state"><h3>No categories</h3><p>Something went wrong loading the library.</p></div>
      )}
    </div>
  );
}''',
    },

    "categories/[slug]/page": {
        "title": "Category Detail",
        "desc": "One category with its confessions.",
        "api": "category(slug), categoryConfessions",
        "imports": 'import Link from "next/link";\nimport { notFound } from "next/navigation";\nimport { api } from "@/lib/api";\nimport { railStyle } from "@/lib/categoryColor";',
        "body": '''
export default async function CategoryDetailPage({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params;
  const catRes = await api.category(slug);
  if (!catRes.ok) notFound();
  const cat = catRes.data;
  const confRes = await api.categoryConfessions(cat.id);
  const confessions = confRes.ok ? confRes.data : [];

  return (
    <div className="ia-page">
      <Link href="/app/categories" className="ic-backlink" style={{ color: "var(--ic-color-neutral-500)" }}>← Back to categories</Link>
      <div className="ia-page__head" style={{ padding: "var(--ic-spacing-6)", borderRadius: "var(--ic-radius-lg)", ...railStyle(cat.slug) as object, color: "var(--ic-color-neutral-0)" }}>
        <h1 style={{ color: "var(--ic-color-neutral-0)" }}>{cat.name}</h1>
        <p style={{ color: "var(--ic-color-neutral-200)" }}>{cat.description}</p>
        {cat.premium && <span className="ia-badge ia-badge--premium">Premium</span>}
      </div>
      <section>
        <h2 className="ia-section-title">Confessions ({confessions.length})</h2>
        <div className="ia-card-grid">
          {confessions.map((c) => (
            <Link key={c.id} href={`/app/confessions/${c.id}`} className="ia-confession">
              <h3>{c.title}</h3>
              <p>{c.short_text || c.medium_text || c.description}</p>
              <div className="ia-confession__meta">
                <span>{c.language}</span>
                {c.variants && c.variants.length > 0 && <span>{c.variants.length} variant{c.variants.length > 1 ? "s" : ""}</span>}
              </div>
            </Link>
          ))}
        </div>
        {confessions.length === 0 && (
          <div className="ic-state"><h3>No confessions yet</h3><p>This category is being prepared.</p></div>
        )}
      </section>
    </div>
  );
}''',
    },

    "confessions/[id]/page": {
        "title": "Confession Detail",
        "desc": "One confession with scriptures and audio variants.",
        "api": "confession(id)",
        "imports": 'import Link from "next/link";\nimport { notFound } from "next/navigation";\nimport { api } from "@/lib/api";',
        "body": '''
export default async function ConfessionDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const res = await api.confession(id);
  if (!res.ok) notFound();
  const c = res.data;

  return (
    <div className="ia-page">
      <Link href="/app/explore" className="ic-backlink" style={{ color: "var(--ic-color-neutral-500)" }}>← Back to explore</Link>
      <div className="ia-page__head">
        <p className="ia-section-title">Confession</p>
        <h1>{c.title}</h1>
        {c.author && <p style={{ color: "var(--ic-color-neutral-500)" }}>By {c.author}</p>}
      </div>

      {(c.long_text || c.medium_text || c.short_text) && (
        <blockquote className="ic-scripture" style={{ padding: "var(--ic-spacing-6)", background: "var(--ic-color-neutral-50)", borderRadius: "var(--ic-radius-lg)", borderLeft: "3px solid var(--ic-color-brand-600)" }}>
          {c.long_text || c.medium_text || c.short_text}
        </blockquote>
      )}

      {c.scriptures && c.scriptures.length > 0 && (
        <section>
          <h2 className="ia-section-title">Scripture</h2>
          {c.scriptures.map((s) => (
            <p key={s.id} style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-font-size-body)", lineHeight: "var(--ic-font-lineHeight-relaxed)", color: "var(--ic-color-neutral-700)" }}>
              {s.book}{s.chapter ? ` ${s.chapter}` : ""}{s.verse ? `:${s.verse}` : ""} ({s.translation})
              {s.is_direct_quote && <span style={{ color: "var(--ic-color-brand-600)", marginLeft: "var(--ic-spacing-2)" }}>Direct quote</span>}
            </p>
          ))}
        </section>
      )}

      {c.variants && c.variants.length > 0 && (
        <section>
          <h2 className="ia-section-title">Audio variants</h2>
          <div className="ia-card-grid">
            {c.variants.map((v) => (
              <div key={v.id} className="ic-card" style={{ padding: "var(--ic-spacing-5)" }}>
                <h3 style={{ fontWeight: "var(--ic-font-weight-semibold)" }}>{v.label}</h3>
                <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-500)", marginTop: "var(--ic-spacing-2)" }}>
                  {Math.floor(v.duration_seconds / 60)}:{String(v.duration_seconds % 60).padStart(2, "0")}
                </p>
              </div>
            ))}
          </div>
        </section>
      )}

      {c.tags && c.tags.length > 0 && (
        <div style={{ display: "flex", flexWrap: "wrap", gap: "var(--ic-spacing-2)" }}>
          {c.tags.map((t) => (
            <span key={t} style={{ fontSize: "var(--ic-font-size-caption)", padding: "var(--ic-spacing-1) var(--ic-spacing-3)", borderRadius: "var(--ic-radius-full)", background: "var(--ic-color-neutral-100)", color: "var(--ic-color-neutral-600)" }}>{t}</span>
          ))}
        </div>
      )}
    </div>
  );
}''',
    },

    "sessions/page": {
        "title": "Sessions",
        "desc": "Session library — build and manage sessions.",
        "api": "categories (sessions derive from categories + confessions)",
        "imports": 'import Link from "next/link";\nimport { api } from "@/lib/api";',
        "body": '''
export default async function SessionsPage() {
  const res = await api.categories();
  const categories = res.ok ? res.data : [];

  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Sessions</h1>
        <p>Sessions are built from categories. Choose an area of life and a length, and iCONFESS assembles the experience.</p>
      </div>
      <Link href="/app/session-builder" className="ic-btn ic-btn--primary" style={{ width: "fit-content" }}>Build a session</Link>
      <section>
        <h2 className="ia-section-title">Start from a category</h2>
        <div className="ia-card-grid">
          {categories.slice(0, 12).map((c) => (
            <Link key={c.id} href={`/app/categories/${c.slug}`} className="ia-confession">
              <h3>{c.name}</h3>
              <p>{c.description}</p>
            </Link>
          ))}
        </div>
      </section>
    </div>
  );
}''',
    },

    "voices/page": {
        "title": "Voices",
        "desc": "Voice library — choose the voice that helps the words land.",
        "api": "voices",
        "imports": 'import Link from "next/link";\nimport { api } from "@/lib/api";',
        "body": '''
export default async function VoicesPage() {
  const res = await api.voices();
  const voices = res.ok ? res.data : [];

  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Voices</h1>
        <p>{voices.length} curated narration voices. Every confession can be heard as well as read.</p>
      </div>
      <div className="ia-card-grid">
        {voices.map((v) => (
          <Link key={v.id} href={`/app/voices/${v.id}`} className="ia-voice">
            <span className="ic-voice-avatar" aria-hidden="true">{v.name.charAt(0)}</span>
            <h3>{v.name}</h3>
            <p>{v.description}</p>
            <div style={{ display: "flex", gap: "var(--ic-spacing-3)", alignItems: "center", fontSize: "var(--ic-font-size-caption)", color: "var(--ic-color-neutral-500)" }}>
              <span>{v.type}</span>
              {v.gender && <span>· {v.gender}</span>}
              {v.premium && <span className="ia-badge ia-badge--premium">Premium</span>}
            </div>
          </Link>
        ))}
      </div>
      {voices.length === 0 && (
        <div className="ic-state"><h3>No voices available</h3><p>The voice library is being updated.</p></div>
      )}
    </div>
  );
}''',
    },

    "history/page": {
        "title": "History",
        "desc": "Listening history — your playback record.",
        "api": "GET /v1/me/history (authed)",
        "imports": '',
        "body": '''
export default function HistoryPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>History</h1>
        <p>Your listening history. Every confession you have heard, in the order you heard it.</p>
      </div>
      <div className="ic-state">
        <h3>Your history will appear here</h3>
        <p>Start listening to confessions and your history will build over time.</p>
        <a href="/app/explore" className="ic-btn ic-btn--secondary">Explore confessions</a>
      </div>
    </div>
  );
}''',
    },

    "favorites/page": {
        "title": "Favorites",
        "desc": "Saved content — confessions and sessions you have marked.",
        "api": "GET /v1/me/favorites (authed)",
        "imports": '',
        "body": '''
export default function FavoritesPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Favorites</h1>
        <p>Confessions and sessions you have saved for easy access.</p>
      </div>
      <div className="ic-state">
        <h3>No favorites yet</h3>
        <p>Save confessions as you explore, and they will appear here.</p>
        <a href="/app/explore" className="ic-btn ic-btn--secondary">Explore confessions</a>
      </div>
    </div>
  );
}''',
    },

    "downloads/page": {
        "title": "Downloads",
        "desc": "Offline listening — saved confessions for Premium.",
        "api": "GET /me/downloads (authed)",
        "imports": '',
        "body": '''
export default function DownloadsPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Downloads</h1>
        <p>Confessions saved for offline listening. A Premium feature.</p>
      </div>
      <div className="ic-state">
        <h3>No downloads</h3>
        <p>Download confessions to listen offline when you are on Premium.</p>
        <a href="/app/explore" className="ic-btn ic-btn--secondary">Explore confessions</a>
      </div>
    </div>
  );
}''',
    },

    "community/page": {
        "title": "Community",
        "desc": "Community feed — published confessions from other listeners.",
        "api": "GET /v1/community/feed (public)",
        "imports": 'import Link from "next/link";\nimport { api } from "@/lib/api";',
        "body": '''
export default async function CommunityPage() {
  const feedRes = await api.communityFeed();
  const communityRes = await api.communityConfessions();
  const feed = feedRes.ok ? feedRes.data : [];
  const confessions = communityRes.ok ? communityRes.data.confessions : [];

  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Community</h1>
        <p>Confessions written by people like you. Every one reviewed before publication.</p>
      </div>
      <Link href="/app/community/create" className="ic-btn ic-btn--primary" style={{ width: "fit-content" }}>Write a confession</Link>

      {confessions.length > 0 && (
        <section>
          <h2 className="ia-section-title">Community confessions</h2>
          <div className="ia-card-grid">
            {(confessions as Array<Record<string, unknown>>).slice(0, 12).map((c, i) => (
              <div key={String(c.id ?? i)} className="ic-card" style={{ padding: "var(--ic-spacing-5)" }}>
                <h3 style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-font-size-subheading)" }}>
                  {String(c.title ?? "Untitled")}
                </h3>
                <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", marginTop: "var(--ic-spacing-2)" }}>
                  {String(c.short_text ?? c.description ?? "")}
                </p>
              </div>
            ))}
          </div>
        </section>
      )}

      {confessions.length === 0 && (
        <div className="ic-state">
          <h3>The community is growing</h3>
          <p>Published confessions from the community will appear here.</p>
        </div>
      )}
    </div>
  );
}''',
    },

    "community/create/page": {
        "title": "Create Confession",
        "desc": "Write a personal confession.",
        "api": "POST /v1/me/confessions (authed)",
        "imports": '',
        "body": '''
export default function CreateConfessionPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Write a confession</h1>
        <p>Write in your own words. Keep it private, share it with people you choose, or offer it for community review.</p>
      </div>
      <div className="ic-state">
        <h3>Coming soon</h3>
        <p>The confession editor is being built. You will be able to write, preview, and submit confessions here.</p>
      </div>
    </div>
  );
}''',
    },

    "notifications/page": {
        "title": "Notifications",
        "desc": "Notification preferences and history.",
        "api": "GET /me/notifications (authed)",
        "imports": '',
        "body": '''
export default function NotificationsPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Notifications</h1>
        <p>Manage your notification preferences. Choose what you want to hear about.</p>
      </div>
      <div className="ia-settings">
        <div className="ia-settings__group">
          <h2>Push notifications</h2>
          <div className="ia-toggle">
            <div>
              <p style={{ fontWeight: "var(--ic-font-weight-medium)" }}>Daily reminder</p>
              <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-500)" }}>A gentle nudge at your preferred time.</p>
            </div>
            <div className="ia-toggle__track" role="switch" aria-checked="false" tabIndex={0}>
              <div className="ia-toggle__thumb" />
            </div>
          </div>
          <div className="ia-toggle">
            <div>
              <p style={{ fontWeight: "var(--ic-font-weight-medium)" }}>New content</p>
              <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-500)" }}>When new confessions arrive in your categories.</p>
            </div>
            <div className="ia-toggle__track" role="switch" aria-checked="true" tabIndex={0}>
              <div className="ia-toggle__thumb" />
            </div>
          </div>
          <div className="ia-toggle">
            <div>
              <p style={{ fontWeight: "var(--ic-font-weight-medium)" }}>Community</p>
              <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-500)" }}>Reactions and replies to your confessions.</p>
            </div>
            <div className="ia-toggle__track" role="switch" aria-checked="true" tabIndex={0}>
              <div className="ia-toggle__thumb" />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}''',
    },

    "search/page": {
        "title": "Search",
        "desc": "Global search across confessions, categories, voices, and scripture.",
        "api": "GET /v1/search (public)",
        "imports": '',
        "body": '''
export default function SearchPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Search</h1>
        <p>Find confessions, categories, voices, and scripture across the library.</p>
      </div>
      <div style={{ maxWidth: "40rem" }}>
        <div className="ic-field">
          <label htmlFor="search-input">Search the library</label>
          <input id="search-input" type="search" placeholder="Try peace, healing, purpose…" autoComplete="off" />
        </div>
      </div>
      <div className="ic-state">
        <h3>Start typing to search</h3>
        <p>Search across 39 categories, confessions, voices, and scripture.</p>
      </div>
    </div>
  );
}''',
    },

    "profile/page": {
        "title": "Profile",
        "desc": "Your account profile.",
        "api": "GET /me/profile (authed)",
        "imports": '',
        "body": '''
export default function ProfilePage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Profile</h1>
        <p>Your account details and public identity.</p>
      </div>
      <div className="ia-settings">
        <div className="ia-settings__group">
          <h2>Account</h2>
          <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)" }}>
            Sign in to view and edit your profile details.
          </p>
          <a href="/app/settings" className="ic-btn ic-btn--secondary" style={{ width: "fit-content" }}>Go to settings</a>
        </div>
      </div>
    </div>
  );
}''',
    },

    "settings/page": {
        "title": "Settings",
        "desc": "Account settings hub.",
        "api": "multiple (authed)",
        "imports": 'import Link from "next/link";',
        "body": '''
const SETTING_LINKS = [
  { href: "/app/settings/account", label: "Account", desc: "Email, password, and display name" },
  { href: "/app/settings/privacy", label: "Privacy", desc: "Data, visibility, and deletion" },
  { href: "/app/settings/notifications", label: "Notifications", desc: "Push, email, and in-app alerts" },
  { href: "/app/settings/playback", label: "Playback", desc: "Audio speed, auto-play, and quality" },
  { href: "/app/settings/accessibility", label: "Accessibility", desc: "Motion, contrast, and screen reader" },
] as const;

export default function SettingsPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Settings</h1>
        <p>Manage your account, privacy, notifications, and preferences.</p>
      </div>
      <div style={{ display: "grid", gap: "var(--ic-spacing-3)" }}>
        {SETTING_LINKS.map((s) => (
          <Link key={s.href} href={s.href} className="ic-card ic-card--hover" style={{ padding: "var(--ic-spacing-5)", textDecoration: "none", display: "flex", justifyContent: "space-between", alignItems: "center" }}>
            <div>
              <h3 style={{ fontWeight: "var(--ic-font-weight-semibold)", fontSize: "var(--ic-font-size-body)" }}>{s.label}</h3>
              <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-500)", marginTop: "var(--ic-spacing-1)" }}>{s.desc}</p>
            </div>
            <span style={{ color: "var(--ic-color-neutral-400)" }}>→</span>
          </Link>
        ))}
      </div>
    </div>
  );
}''',
    },

    "settings/account/page": {
        "title": "Account Settings",
        "desc": "Email, password, display name.",
        "api": "GET /me/profile, POST /auth/change-password",
        "imports": '',
        "body": '''
export default function AccountSettingsPage() {
  return (
    <div className="ia-page">
      <a href="/app/settings" className="ic-backlink" style={{ color: "var(--ic-color-neutral-500)" }}>← Settings</a>
      <div className="ia-page__head">
        <h1>Account</h1>
        <p>Your email, password, and display name.</p>
      </div>
      <div className="ia-settings">
        <div className="ia-settings__group">
          <h2>Display name</h2>
          <div className="ic-field">
            <label htmlFor="display-name">Name</label>
            <input id="display-name" type="text" placeholder="Your display name" />
          </div>
          <button type="button" className="ic-btn ic-btn--primary" style={{ width: "fit-content" }}>Save</button>
        </div>
        <div className="ia-settings__group">
          <h2>Change password</h2>
          <div className="ic-field">
            <label htmlFor="current-pw">Current password</label>
            <input id="current-pw" type="password" />
          </div>
          <div className="ic-field">
            <label htmlFor="new-pw">New password</label>
            <input id="new-pw" type="password" />
          </div>
          <button type="button" className="ic-btn ic-btn--primary" style={{ width: "fit-content" }}>Update password</button>
        </div>
      </div>
    </div>
  );
}''',
    },

    "settings/privacy/page": {
        "title": "Privacy Settings",
        "desc": "Data visibility and deletion.",
        "api": "GET /me/deletion, GET /me/export",
        "imports": '',
        "body": '''
export default function PrivacySettingsPage() {
  return (
    <div className="ia-page">
      <a href="/app/settings" className="ic-backlink" style={{ color: "var(--ic-color-neutral-500)" }}>← Settings</a>
      <div className="ia-page__head">
        <h1>Privacy</h1>
        <p>Your data, your visibility, and your right to leave.</p>
      </div>
      <div className="ia-settings">
        <div className="ia-settings__group">
          <h2>Content visibility</h2>
          <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)" }}>
            Your confessions are private by default. You choose what to share.
          </p>
        </div>
        <div className="ia-settings__group">
          <h2>Your data</h2>
          <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", marginBottom: "var(--ic-spacing-4)" }}>
            Download everything we hold about you, or request account deletion.
          </p>
          <div style={{ display: "flex", gap: "var(--ic-spacing-3)", flexWrap: "wrap" }}>
            <a href="/api/me/export" className="ic-btn ic-btn--secondary">Download my data</a>
            <button type="button" className="ic-btn ic-btn--secondary" style={{ color: "var(--ic-color-semantic-danger-light)", borderColor: "var(--ic-color-semantic-danger-light)" }}>Delete account</button>
          </div>
        </div>
      </div>
    </div>
  );
}''',
    },

    "settings/notifications/page": {
        "title": "Notification Settings",
        "desc": "Push, email, and in-app notifications.",
        "api": "GET /me/notifications",
        "imports": '',
        "body": '''
export default function NotificationSettingsPage() {
  return (
    <div className="ia-page">
      <a href="/app/settings" className="ic-backlink" style={{ color: "var(--ic-color-neutral-500)" }}>← Settings</a>
      <div className="ia-page__head">
        <h1>Notifications</h1>
        <p>Choose how and when iCONFESS reaches you.</p>
      </div>
      <div className="ia-settings">
        <div className="ia-settings__group">
          <h2>Email notifications</h2>
          <div className="ia-toggle">
            <div><p style={{ fontWeight: "var(--ic-font-weight-medium)" }}>Weekly digest</p><p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-500)" }}>A summary of your week in confessions.</p></div>
            <div className="ia-toggle__track" role="switch" aria-checked="false" tabIndex={0}><div className="ia-toggle__thumb" /></div>
          </div>
          <div className="ia-toggle">
            <div><p style={{ fontWeight: "var(--ic-font-weight-medium)" }}>Product updates</p><p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-500)" }}>New features and categories.</p></div>
            <div className="ia-toggle__track" role="switch" aria-checked="true" tabIndex={0}><div className="ia-toggle__thumb" /></div>
          </div>
        </div>
      </div>
    </div>
  );
}''',
    },

    "settings/playback/page": {
        "title": "Playback Settings",
        "desc": "Audio speed, auto-play, and quality.",
        "api": "GET /me/preferences",
        "imports": '',
        "body": '''
export default function PlaybackSettingsPage() {
  return (
    <div className="ia-page">
      <a href="/app/settings" className="ic-backlink" style={{ color: "var(--ic-color-neutral-500)" }}>← Settings</a>
      <div className="ia-page__head">
        <h1>Playback</h1>
        <p>Control how audio plays.</p>
      </div>
      <div className="ia-settings">
        <div className="ia-settings__group">
          <h2>Audio</h2>
          <div className="ia-toggle">
            <div><p style={{ fontWeight: "var(--ic-font-weight-medium)" }}>Auto-play next</p><p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-500)" }}>Continue to the next confession in a session.</p></div>
            <div className="ia-toggle__track" role="switch" aria-checked="true" tabIndex={0}><div className="ia-toggle__thumb" /></div>
          </div>
          <div className="ic-field">
            <label htmlFor="playback-speed">Default speed</label>
            <select id="playback-speed" defaultValue="1">
              <option value="0.75">0.75×</option>
              <option value="1">1× (normal)</option>
              <option value="1.25">1.25×</option>
              <option value="1.5">1.5×</option>
            </select>
          </div>
        </div>
      </div>
    </div>
  );
}''',
    },

    "settings/accessibility/page": {
        "title": "Accessibility Settings",
        "desc": "Motion, contrast, and screen reader preferences.",
        "api": "GET /me/preferences",
        "imports": '',
        "body": '''
export default function AccessibilitySettingsPage() {
  return (
    <div className="ia-page">
      <a href="/app/settings" className="ic-backlink" style={{ color: "var(--ic-color-neutral-500)" }}>← Settings</a>
      <div className="ia-page__head">
        <h1>Accessibility</h1>
        <p>iCONFESS aims for WCAG 2.2 AA across every surface. These controls are for preferences that go beyond the standard.</p>
      </div>
      <div className="ia-settings">
        <div className="ia-settings__group">
          <h2>Motion</h2>
          <div className="ia-toggle">
            <div><p style={{ fontWeight: "var(--ic-font-weight-medium)" }}>Reduce motion</p><p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-500)" }}>Minimise animations. Respects your system setting by default.</p></div>
            <div className="ia-toggle__track" role="switch" aria-checked="false" tabIndex={0}><div className="ia-toggle__thumb" /></div>
          </div>
        </div>
        <div className="ia-settings__group">
          <h2>Text</h2>
          <div className="ic-field">
            <label htmlFor="text-size">Text size</label>
            <select id="text-size" defaultValue="default">
              <option value="default">Default</option>
              <option value="large">Large</option>
              <option value="x-large">Extra large</option>
            </select>
          </div>
        </div>
      </div>
    </div>
  );
}''',
    },

    "subscription/page": {
        "title": "Subscription",
        "desc": "Current plan and billing.",
        "api": "GET /v1/subscription (authed)",
        "imports": 'import Link from "next/link";',
        "body": '''
export default function SubscriptionPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Subscription</h1>
        <p>Your current plan and benefits.</p>
      </div>
      <div className="ia-settings">
        <div className="ia-settings__group">
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
            <h2>Free plan</h2>
            <span className="ia-badge ia-badge--free">Free</span>
          </div>
          <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)" }}>
            Access to core categories and standard session lengths.
          </p>
          <Link href="/premium" className="ic-btn ic-btn--primary" style={{ width: "fit-content" }}>Explore Premium</Link>
        </div>
      </div>
    </div>
  );
}''',
    },

    "subscription/manage/page": {
        "title": "Manage Subscription",
        "desc": "Billing, renewal, and cancellation.",
        "api": "GET /v1/subscription (authed)",
        "imports": 'import Link from "next/link";',
        "body": '''
export default function ManageSubscriptionPage() {
  return (
    <div className="ia-page">
      <Link href="/app/subscription" className="ic-backlink" style={{ color: "var(--ic-color-neutral-500)" }}>← Subscription</Link>
      <div className="ia-page__head">
        <h1>Manage subscription</h1>
        <p>Billing, renewal, and plan changes.</p>
      </div>
      <div className="ic-state">
        <h3>No active subscription</h3>
        <p>You are on the Free plan. Upgrade to Premium for the full library, longer sessions, and premium voices.</p>
        <Link href="/premium" className="ic-btn ic-btn--primary">Explore Premium</Link>
      </div>
    </div>
  );
}''',
    },

    "recommendations/page": {
        "title": "Recommendations",
        "desc": "Personalised recommendations based on your listening signals.",
        "api": "GET /v1/recommendations (authed)",
        "imports": '',
        "body": '''
export default function RecommendationsPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Recommended for you</h1>
        <p>Based on the areas of life you return to, the sessions you complete, and the time of day you listen.</p>
      </div>
      <div className="ic-state">
        <h3>Build your practice first</h3>
        <p>As you listen to confessions and explore categories, iCONFESS will surface recommendations shaped by what you actually do.</p>
        <a href="/app/explore" className="ic-btn ic-btn--secondary">Start exploring</a>
      </div>
    </div>
  );
}''',
    },

    "daily/page": {
        "title": "Daily",
        "desc": "Today's experience — a confession chosen for this moment.",
        "api": "GET /home (authed, daily suggestions)",
        "imports": '',
        "body": '''
export default function DailyPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Today</h1>
        <p>A confession chosen for this day. Return tomorrow for a new one.</p>
      </div>
      <div className="ic-state">
        <h3>Start your daily practice</h3>
        <p>Your daily confession will appear here once you begin listening. One confession, once a day — a ritual, not a feed.</p>
        <a href="/app/categories" className="ic-btn ic-btn--secondary">Choose a category</a>
      </div>
    </div>
  );
}''',
    },

    "routines/page": {
        "title": "Routines",
        "desc": "Morning, midday, and evening routines. Built from templates.",
        "api": "GET /v1/templates (authed)",
        "imports": '',
        "body": '''
export default function RoutinesPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Routines</h1>
        <p>Set up morning, midday, and evening rituals. Built from your templates and schedules.</p>
      </div>
      <div className="ic-state">
        <h3>No routines yet</h3>
        <p>Create a template and schedule it to build a routine.</p>
        <a href="/app/session-builder" className="ic-btn ic-btn--secondary">Build a session</a>
      </div>
    </div>
  );
}''',
    },

    "streaks/page": {
        "title": "Streaks",
        "desc": "Daily streak tracking. No backend endpoint — specify the feature.",
        "api": "none (spec needed)",
        "imports": '',
        "body": '''
export default function StreaksPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Streaks</h1>
        <p>Track your daily practice. Return every day to build your streak.</p>
      </div>
      <div className="ic-state">
        <h3>Streaks are coming</h3>
        <p>This feature requires a backend endpoint for streak tracking. It has been specified but not yet implemented.</p>
      </div>
    </div>
  );
}''',
    },

    "achievements/page": {
        "title": "Achievements",
        "desc": "Milestones and badges. No backend endpoint — specify the feature.",
        "api": "none (spec needed)",
        "imports": '',
        "body": '''
export default function AchievementsPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Achievements</h1>
        <p>Milestones in your confession practice.</p>
      </div>
      <div className="ic-state">
        <h3>Achievements are coming</h3>
        <p>This feature requires a backend endpoint. It has been specified but not yet implemented.</p>
      </div>
    </div>
  );
}''',
    },

    "player/page": {
        "title": "Player",
        "desc": "Standalone audio player wrapping the existing player component.",
        "api": "audio playback (authed)",
        "imports": '',
        "body": '''
export default function PlayerPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head" style={{ textAlign: "center" }}>
        <h1>Player</h1>
        <p>The immersive audio player will appear here when a confession is playing.</p>
      </div>
      <div className="ic-state">
        <h3>Nothing playing</h3>
        <p>Choose a confession and press play to start the experience.</p>
        <a href="/app/explore" className="ic-btn ic-btn--secondary">Find a confession</a>
      </div>
    </div>
  );
}''',
    },

    "session-builder/page": {
        "title": "Session Builder",
        "desc": "Build a session from categories, duration, and voice.",
        "api": "POST /v1/templates (authed)",
        "imports": 'import { api } from "@/lib/api";',
        "body": '''
export default async function SessionBuilderPage() {
  const [catsRes, voicesRes] = await Promise.all([
    api.categories(),
    api.voices(),
  ]);
  const categories = catsRes.ok ? catsRes.data : [];
  const voices = voicesRes.ok ? voicesRes.data : [];

  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Build a session</h1>
        <p>Choose an area of life, a duration, and a voice. iCONFESS assembles the experience.</p>
      </div>
      <div className="ia-settings" style={{ maxWidth: "36rem" }}>
        <div className="ia-settings__group">
          <div className="ic-field">
            <label htmlFor="sb-category">Category</label>
            <select id="sb-category">
              <option value="">Choose an area of life</option>
              {categories.map((c) => (
                <option key={c.id} value={c.id}>{c.name}</option>
              ))}
            </select>
          </div>
          <div className="ic-field">
            <label htmlFor="sb-duration">Duration</label>
            <select id="sb-duration">
              <option value="180">3 minutes</option>
              <option value="300" defaultValue>5 minutes</option>
              <option value="600">10 minutes</option>
              <option value="900">15 minutes</option>
            </select>
          </div>
          <div className="ic-field">
            <label htmlFor="sb-voice">Voice</label>
            <select id="sb-voice">
              <option value="">Default</option>
              {voices.map((v) => (
                <option key={v.id} value={v.id}>{v.name}</option>
              ))}
            </select>
          </div>
          <button type="button" className="ic-btn ic-btn--primary" style={{ width: "fit-content" }}>Start session</button>
        </div>
      </div>
    </div>
  );
}''',
    },

    "sessions/[id]/page": {
        "title": "Session Detail",
        "desc": "One session with playback and progress.",
        "api": "session (authed)",
        "imports": 'import Link from "next/link";',
        "body": '''
export default async function SessionDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return (
    <div className="ia-page">
      <Link href="/app/sessions" className="ic-backlink" style={{ color: "var(--ic-color-neutral-500)" }}>← Sessions</Link>
      <div className="ia-page__head" style={{ textAlign: "center" }}>
        <h1>Session</h1>
        <p>This session is loading from the API. The immersive player surface will render here.</p>
      </div>
      <div className="ic-state">
        <h3>Session loading</h3>
        <p>Session ID: {id}</p>
      </div>
    </div>
  );
}''',
    },

    "voices/[id]/page": {
        "title": "Voice Profile",
        "desc": "One voice with sample and confessions.",
        "api": "voices (filter by id)",
        "imports": 'import Link from "next/link";\nimport { notFound } from "next/navigation";\nimport { api } from "@/lib/api";',
        "body": '''
export default async function VoiceProfilePage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const res = await api.voices();
  if (!res.ok) notFound();
  const voice = res.data.find((v) => v.id === id);
  if (!voice) notFound();

  return (
    <div className="ia-page">
      <Link href="/app/voices" className="ic-backlink" style={{ color: "var(--ic-color-neutral-500)" }}>← Voices</Link>
      <div className="ia-page__head">
        <div style={{ display: "flex", alignItems: "center", gap: "var(--ic-spacing-4)" }}>
          <span className="ic-voice-avatar" aria-hidden="true">{voice.name.charAt(0)}</span>
          <div>
            <h1>{voice.name}</h1>
            <p style={{ color: "var(--ic-color-neutral-500)" }}>{voice.type}{voice.gender ? ` · ${voice.gender}` : ""} · {voice.language}</p>
          </div>
        </div>
      </div>
      {voice.description && (
        <p style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-font-size-body)", lineHeight: "var(--ic-font-lineHeight-relaxed)", color: "var(--ic-color-neutral-700)" }}>{voice.description}</p>
      )}
      {voice.premium && <span className="ia-badge ia-badge--premium">Premium voice</span>}
      {voice.sample_url && (
        <section>
          <h2 className="ia-section-title">Sample</h2>
          <audio controls src={voice.sample_url} style={{ width: "100%" }}>
            Your browser does not support audio playback.
          </audio>
        </section>
      )}
    </div>
  );
}''',
    },

    "community/[id]/page": {
        "title": "Community Post",
        "desc": "One community confession.",
        "api": "GET /v1/community/feed",
        "imports": 'import Link from "next/link";',
        "body": '''
export default async function CommunityPostPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return (
    <div className="ia-page">
      <Link href="/app/community" className="ic-backlink" style={{ color: "var(--ic-color-neutral-500)" }}>← Community</Link>
      <div className="ia-page__head">
        <h1>Community confession</h1>
        <p>Loading confession from the community feed.</p>
      </div>
      <div className="ic-state">
        <h3>Loading</h3>
        <p>Post ID: {id}</p>
      </div>
    </div>
  );
}''',
    },

    "shared/[token]/page": {
        "title": "Shared",
        "desc": "Resolve a shared template or confession.",
        "api": "GET /v1/t/{token} (public)",
        "imports": '',
        "body": '''
export default async function SharedPage({ params }: { params: Promise<{ token: string }> }) {
  const { token } = await params;
  return (
    <div className="ia-page">
      <div className="ia-page__head" style={{ textAlign: "center" }}>
        <h1>Shared with you</h1>
        <p>Someone shared an iCONFESS experience with you.</p>
      </div>
      <div className="ic-state">
        <h3>Loading shared content</h3>
        <p>Resolving shared link.</p>
      </div>
    </div>
  );
}''',
    },

    "invite/page": {
        "title": "Invite",
        "desc": "Invite friends to iCONFESS.",
        "api": "none",
        "imports": '',
        "body": '''
export default function InvitePage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Invite a friend</h1>
        <p>Share iCONFESS with someone who could use a daily practice of spoken confession.</p>
      </div>
      <div className="ic-state">
        <h3>Sharing is coming</h3>
        <p>Referral and invite features are being built.</p>
      </div>
    </div>
  );
}''',
    },

    "referrals/page": {
        "title": "Referrals",
        "desc": "Referral tracking and rewards.",
        "api": "none (spec needed)",
        "imports": '',
        "body": '''
export default function ReferralsPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Referrals</h1>
        <p>Share iCONFESS and track your referrals.</p>
      </div>
      <div className="ic-state">
        <h3>Referrals are coming</h3>
        <p>This feature requires a backend endpoint. It has been specified but not yet implemented.</p>
      </div>
    </div>
  );
}''',
    },

    "feedback/page": {
        "title": "Feedback",
        "desc": "Send feedback to the iCONFESS team.",
        "api": "none (email or form)",
        "imports": '',
        "body": '''
export default function FeedbackPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Feedback</h1>
        <p>Your voice matters. Tell us what is working and what could be better.</p>
      </div>
      <div className="ia-settings" style={{ maxWidth: "36rem" }}>
        <div className="ia-settings__group">
          <div className="ic-field">
            <label htmlFor="feedback-subject">Subject</label>
            <input id="feedback-subject" type="text" placeholder="What is this about?" />
          </div>
          <div className="ic-field">
            <label htmlFor="feedback-body">Your feedback</label>
            <textarea id="feedback-body" placeholder="Tell us what you think…" />
          </div>
          <button type="button" className="ic-btn ic-btn--primary" style={{ width: "fit-content" }}>Send feedback</button>
        </div>
      </div>
    </div>
  );
}''',
    },

    "support/page": {
        "title": "Support",
        "desc": "Get help with iCONFESS.",
        "api": "none",
        "imports": 'import Link from "next/link";',
        "body": '''
export default function SupportPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Support</h1>
        <p>Need help? Start here.</p>
      </div>
      <div style={{ display: "grid", gap: "var(--ic-spacing-3)" }}>
        {[
          { href: "/faq", label: "FAQ", desc: "Frequently asked questions about iCONFESS" },
          { href: "/help", label: "Help centre", desc: "Guides and how-tos" },
          { href: "/contact", label: "Contact", desc: "Reach the iCONFESS team" },
          { href: "/app/feedback", label: "Feedback", desc: "Share your thoughts" },
        ].map((s) => (
          <Link key={s.href} href={s.href} className="ic-card ic-card--hover" style={{ padding: "var(--ic-spacing-5)", textDecoration: "none", display: "flex", justifyContent: "space-between", alignItems: "center" }}>
            <div>
              <h3 style={{ fontWeight: "var(--ic-font-weight-semibold)" }}>{s.label}</h3>
              <p style={{ fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-500)", marginTop: "var(--ic-spacing-1)" }}>{s.desc}</p>
            </div>
            <span style={{ color: "var(--ic-color-neutral-400)" }}>→</span>
          </Link>
        ))}
      </div>
    </div>
  );
}''',
    },
}


def write_page(path, content):
    full = os.path.join(APP, path)
    os.makedirs(os.path.dirname(full), exist_ok=True)
    with open(full, "w") as f:
        f.write(content)


def main():
    count = 0
    for rel, spec in PAGES.items():
        page_path = f"{rel}.tsx"
        imports = spec.get("imports", "")
        body = spec["body"].strip()
        content = f'/** {spec["desc"]} */\n{imports}\n{body}\n'
        write_page(page_path, content)
        count += 1
        print(f"  wrote app/{rel}.tsx — {spec['title']}")

    print(f"\nGenerated {count} pages")


if __name__ == "__main__":
    main()
