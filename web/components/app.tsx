"use client";
import React, { useEffect, useState } from "react";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { Icon, Logo, SessionTile, EmptyState, PhoneMock } from "./ui";
import { ConfTile, PlayBtn, PlayChip, PlaySessionBtn, FavBtn, ShareBtn, MixBtn, RadioBtn, SnippetBtn, Annotations } from "./islands";
import { TCard } from "./marketing";
import { useApp, mutate, track } from "@/lib/store";
import { useAudio, useToast, useSearch } from "@/lib/ui";
import { CATEGORIES, CONFESSIONS, PLANS as PLANS_LOCAL, catBySlug, confBySlug, sessionsFor, ALL_SESSIONS, sessionBySlug, buildQueue, motifStyle, queueItem, CAT_IMAGES, MOMENTS, PACKS, type Session, type QueueItem } from "@/lib/data";
import { VoiceCard, SoundPanel, VoiceOrb, VoicePreview } from "./voice-catalog";
import { VOICE_CATALOG, effectiveVoices, searchVoices, voiceAvailable } from "@/lib/voices";
import { logAE } from "@/lib/audio-engine";

const SIDE: [string, string, string][] = [["/app", "home", "Home"], ["/app/explore", "compass", "Explore"], ["/app/categories", "book", "Categories"], ["/app/sessions", "clock", "Sessions"], ["/app/voices", "mic", "Voices"], ["/app/history", "clock", "History"], ["/app/journal", "edit", "Journal"], ["/app/memory", "ast", "Memory"], ["/app/favorites", "heart", "Favorites"], ["/app/routines", "spark", "Routines"], ["/app/schedule", "cal", "Schedule"]];
const BOTTOM: [string, string, string][] = [["/app", "home", "Home"], ["/app/explore", "compass", "Explore"], ["/app/community/create", "plus", "Create"], ["/app/history", "chart", "Activity"], ["/app/profile", "user", "Profile"]];

export function AppShell({ children }: { children: React.ReactNode }) {
  const path = usePathname();
  return (
    <div className="app-shell">
      <aside className="app-side">
        <Logo onDark />
        <nav aria-label="App">{SIDE.map(([h, ic, l]) => <Link key={h} href={h} aria-current={path === h ? "page" : undefined}><Icon n={ic} s={16} /> {l}</Link>)}</nav>
        <div className="side-bottom">
          <Link href="/app/profile" aria-current={path.startsWith("/app/profile") ? "page" : undefined}><Icon n="user" s={16} /> Profile</Link>
          <Link href="/app/settings" aria-current={path.startsWith("/app/settings") ? "page" : undefined}><Icon n="gear" s={16} /> Settings</Link>
        </div>
      </aside>
      <main className="app-main">{children}</main>
      <div className="bottom-nav"><nav aria-label="App primary">
        {BOTTOM.map(([h, ic, l]) => l === "Create"
          ? <Link key={h} href={h} aria-label="Create"><span className="create-btn"><Icon n="plus" s={18} /></span>{l}</Link>
          : <Link key={h} href={h} aria-current={path === h ? "page" : undefined}><Icon n={ic} s={18} />{l}</Link>)}
      </nav></div>
    </div>
  );
}

const Head = ({ title, sub, right }: { title: React.ReactNode; sub?: string; right?: React.ReactNode }) => (
  <div className="app-top"><div><h2 className="h2" style={{ fontSize: "clamp(22px,3vw,32px)" }}>{title}</h2>{sub && <p className="small" style={{ marginTop: 4 }}>{sub}</p>}</div>{right}</div>
);
const NeedAuth = () => (
  <div style={{ maxWidth: 520, margin: "40px auto" }}><EmptyState icon="lock" title="Sign in to continue" sub="This area belongs to your iCONFESS account." cta={<><Link className="btn btn-primary" href="/login">Log in</Link><Link className="btn btn-ghost" href="/register">Create account</Link></>} /></div>
);
const greet = () => { const h = new Date().getHours(); return h < 12 ? "Good morning" : h < 17 ? "Good afternoon" : "Good evening"; };
const slugOfName = (name: string) => CATEGORIES.find((c) => c.name === name)?.slug || "";
const isGentleTime = (st: ReturnType<typeof useApp>) => new Date().getHours() >= 21 || (st.history.length > 0 && Date.now() - st.history[0].at > 6 * 864e5);

export function AppPage({ kind, slug, token }: { kind: string; slug?: string; token?: string }) {
  const st = useApp();
  const needsAuth = !["player", "shared"].includes(kind);
  useEffect(() => { }, [kind, slug]);
  if (needsAuth && !st.user) return <NeedAuth />;
  switch (kind) {
    case "home": return <AppHome />;
    case "explore": return <><Head title="Explore" />
      <div className="app-section"><div className="section-head"><h3>Moments</h3></div><div className="card-row snap">{MOMENTS.map((m) => <div className="tile" key={m.id}><h4>{m.title}</h4><p>{m.desc}</p><div style={{ marginTop: 10 }}><MixBtn cats={m.cats} label="Queue moment" /></div></div>)}</div></div>
      <div className="app-section"><div className="section-head"><h3>Moods</h3></div><div className="card-row snap">{[["Calm", ["peace", "rest", "gratitude"]], ["Focus", ["discipline", "productivity", "direction"]], ["Sleep", ["rest", "peace", "prayer"]], ["Courage", ["confidence", "overcoming-fear", "breakthrough"]]].map(([m, cats]) => <div className="tile" key={m as string}><h4>{m}</h4><div className="chip-row" style={{ marginTop: 8 }}>{(cats as string[]).map((c) => <Link className="chip" key={c} href={`/categories/${c}`}>{catBySlug(c)?.name}</Link>)}</div></div>)}</div></div>
      <div className="app-section"><div className="tgrid snap">{CATEGORIES.slice(0, 8).map((c) => <TCard key={c.slug} c={c} />)}</div></div><div className="app-section"><div className="section-head"><h3>Sessions</h3></div><div className="card-row three snap">{ALL_SESSIONS.slice(0, 6).map((s) => <SessionTile key={s.slug} s={s} />)}</div></div></>;
    case "categories": return <><Head title="Categories" sub="39 areas of life" /><div className="tgrid">{CATEGORIES.map((c) => <TCard key={c.slug} c={c} />)}</div></>;
    case "category": return <AppCategory slug={slug!} />;
    case "confessions": return <><Head title="Confessions" sub="The reviewed library" /><div className="grid4">{CONFESSIONS.map((c) => <ConfTile key={c.slug} c={c} />)}</div></>;
    case "confession": return <AppConfession slug={slug!} />;
    case "sessions": return <><Head title="Sessions" sub="Assembled to fit your minutes" right={<Link className="btn btn-primary btn-sm" href="/app/session-builder"><Icon n="plus" s={12} /> Build</Link>} />
      <div className="app-section" style={{ marginTop: 0, marginBottom: 24 }}><div className="section-head"><h3>Offline mixtapes · Premium</h3></div><div className="card-row snap">{PACKS.map((p) => <PackCard key={p.id} id={p.id} title={p.title} desc={p.desc} cats={p.cats} />)}</div></div>
      <div className="card-row three" style={{ gridTemplateColumns: "repeat(3,1fr)" }}>{ALL_SESSIONS.map((s) => <SessionTile key={s.slug} s={s} />)}</div></>;
    case "session": return <AppSession slug={slug!} />;
    case "builder": return <Builder />;
    case "player": return <Player />;
    case "voices": return <AppVoices />;
    case "voice": return <AppVoiceDetail slug={slug!} />;
    case "journal": return <JournalApp />;
    case "memory": return <MemoryApp />;
    case "partners": return <PartnersApp />;
    case "family": return <FamilyApp />;
    case "languages": return <LanguagesApp />;
    case "widget": return <WidgetApp />;
    case "wrapped": return <WrappedApp />;
    case "history": { const cnt: Record<string, number> = {}; st.history.forEach((h) => { cnt[h.slug] = (cnt[h.slug] || 0) + 1; }); const chart = Object.entries(cnt).sort((a, b) => b[1] - a[1]).slice(0, 5).map(([sl, n]) => ({ c: confBySlug(sl), n })).filter((x) => x.c); return <><Head title="History" sub="Everything you've returned to" />
      {chart.length > 0 && <div className="app-section" style={{ marginTop: 0, marginBottom: 24 }}><div className="section-head"><h3>Your chart — most returned to</h3></div><div className="form-card">{chart.map((x, i) => <div className="list-row" key={x.c!.slug}><div className="lr-main"><h4>{i + 1}. {x.c!.title}</h4><p>{x.c!.category} · {x.n} return{x.n === 1 ? "" : "s"}</p></div><PlayChip slug={x.c!.slug} /></div>)}</div><p className="small" style={{ marginTop: 8 }}>Your data only. We never publish global charts.</p></div>}{st.history.length ? st.history.map((h, i) => <div className="list-row" key={i}><div className="lr-main"><h4>{h.title}</h4><p>{h.category} · {new Date(h.at).toLocaleString()}</p></div><PlayChip slug={h.slug} /></div>) : <EmptyState icon="clock" title="No history yet" sub="Play a confession or start a session and it will appear here." cta={<Link className="btn btn-primary" href="/app/explore">Explore</Link>} />}</>; }
    case "favorites": { const favs = st.favs.map(confBySlug).filter(Boolean); return <><Head title="Favorites" sub="Saved words" />{favs.length ? <div className="grid4">{favs.map((c) => <ConfTile key={c!.slug} c={c!} />)}</div> : <EmptyState icon="heart" title="No favorites yet" sub="Tap the heart on any confession to keep it here." />}</>; }
    case "downloads": { const dl = st.downloads.filter((d) => !d.includes(":")).map(sessionBySlug).filter(Boolean); const pins = st.downloads.filter((d) => d.startsWith("cat:")); const packs = st.downloads.filter((d) => d.startsWith("pack:")); return <><Head title="Downloads" sub="Premium · available offline on this device" />
      {packs.length > 0 && <div className="app-section" style={{ marginTop: 0 }}><div className="section-head"><h3>Mixtapes</h3></div><div className="card-row snap">{packs.map((p) => { const pk = PACKS.find((x) => "pack:" + x.id === p); return pk ? <div className="tile" key={p}><span className="t-meta">Pack</span><h4>{pk.title}</h4><p>{pk.desc}</p></div> : null; })}</div></div>}
      {pins.length > 0 && <div className="app-section"><div className="section-head"><h3>Pinned categories</h3></div><div className="tgrid snap">{pins.map((p) => { const c = catBySlug(p.slice(4)); return c ? <TCard key={p} c={c} /> : null; })}</div></div>}
      {dl.length ? <div className="app-section"><div className="section-head"><h3>Sessions</h3></div><div className="card-row three" style={{ gridTemplateColumns: "repeat(3,1fr)" }}>{dl.map((x) => <SessionTile key={x!.slug} s={x!} />)}</div></div> : !pins.length && !packs.length ? <EmptyState icon="dl" title="Nothing saved offline yet" sub="Pin a category, grab a mixtape pack, or save any session — Premium keeps it on this device for bad-network mornings." cta={<Link className="btn btn-primary" href="/app/sessions">Browse sessions</Link>} /> : null}</>; }
    case "schedule": return <Schedule />;
    case "routines": return <Routines />;
    case "notifications": return <><Head title="Notifications" /><div className="form-card" style={{ maxWidth: 560 }}><Switch title="Daily words" sub="A quiet nudge with today's confession" path="notifications.daily" /><Switch title="Reminders" sub="If you miss a morning, we offer the words again at midday" path="notifications.reminders" /><Switch title="Community" sub="When a submission you follow is approved" path="notifications.community" /><Switch title="Weekly review digest" sub="One optional Sunday email: the words you returned to, plus a journal prompt. Never more." path="notifications.digest" /><div className="setting-row"><div><b>Quiet hours</b><span>A hard blackout. Nothing can nudge you inside this window.</span></div><div style={{ display: "flex", gap: 8, alignItems: "center" }}><input type="time" defaultValue={st.settings.notifications.quietStart} style={{ width: 110 }} onChange={(e) => mutate((x) => { x.settings.notifications.quietStart = e.target.value; })} aria-label="Quiet hours start" /><span>–</span><input type="time" defaultValue={st.settings.notifications.quietEnd} style={{ width: 110 }} onChange={(e) => mutate((x) => { x.settings.notifications.quietEnd = e.target.value; })} aria-label="Quiet hours end" /></div></div></div></>;
    case "community": { const pub = st.community.filter((c) => c.status === "public" || c.status === "approved"); return <><Head title="Community" sub="Private by default. Shared intentionally." right={<Link className="btn btn-primary btn-sm" href="/app/community/create"><Icon n="plus" s={12} /> Create</Link>} />
      <div className="form-card rv in" style={{ maxWidth: 640, marginBottom: 20 }}><div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}><div><h4>Your creator page</h4><p className="small">{pub.length} published · {st.community.filter((c) => c.status === "pending_review").length} in review · decisions always recorded with a reason</p></div><span className="tag shared">reviewed</span></div><p className="small" style={{ marginTop: 8 }}>When your words publish, this page lists them. Followers get a quiet "new words" notice — never a feed, never a feed algorithm.</p></div>{st.community.length ? st.community.map((c) => <Link className="list-row" href={`/app/community/${c.slug}`} key={c.slug} style={{ display: "flex" }}><div className="lr-main"><h4>{c.title}</h4><p>{new Date(c.at).toLocaleDateString()}</p></div><span className={"tag " + (c.status === "pending_review" ? "pending" : c.visibility)}>{c.status.replace("_", " ")}</span></Link>) : <EmptyState icon="edit" title="No confessions yet" sub="Write your first words — they stay private unless you say otherwise." />}</>; }
    case "communityCreate": return <CreateConfession />;
    case "communityDetail": return <CommunityDetail slug={slug!} />;
    case "profile": return <Profile />;
    case "settings": return <><Head title="Settings" /><div className="card-row snap">{[["/app/settings/account", "Account", "Name, email, password"], ["/app/settings/privacy", "Privacy", "Defaults and data"], ["/app/settings/notifications", "Notifications", "Nudges and reminders"], ["/app/settings/playback", "Playback", "Voice, speed, volume"], ["/app/settings/accessibility", "Accessibility", "Text, captions, motion"], ["/admin", "Audio Studio", "Internal · voices, rights, queue"]].map(([h, t, s]) => <Link className="tile" href={h} key={h}><h4>{t}</h4><p>{s}</p><span className="textlink" style={{ marginTop: 6 }}>Open <Icon n="arrow" s={12} /></span></Link>)}</div></>;
    case "settingsAccount": return <><Head title="Account" /><div className="form-card" style={{ maxWidth: 560 }}><div className="setting-row"><div><b>Name</b><span>{st.user!.name}</span></div></div><div className="setting-row"><div><b>Email</b><span>{st.user!.email}</span></div></div><div className="setting-row"><div><b>Password</b><span>••••••••</span></div><Link className="btn btn-ghost btn-sm" href="/reset-password">Change</Link></div></div></>;
    case "settingsPrivacy": return <><Head title="Privacy" /><div className="form-card" style={{ maxWidth: 560 }}><Switch title="Private by default" sub="New confessions start private" path="privacy.privateByDefault" /><Switch title="Show streak" sub="Let your streak be visible in your profile" path="privacy.showStreak" /><Switch title="Pause history" sub="Practise without recording anything. Streaks and charts pause too." path="privacy.pauseHistory" /><div className="setting-row"><div><b>Your data</b><span>Export or delete everything</span></div><ExportBtn /></div><Switch title="Monthly auto-export" sub="A human-readable backup of your journal and confessions, emailed to you on the 1st." path="notifications.autoExport" /><div className="field" style={{ marginTop: 12 }}><label>Export email</label><input type="email" defaultValue={st.settings.notifications.autoExportEmail} placeholder="you@example.com" onBlur={(e) => mutate((x) => { x.settings.notifications.autoExportEmail = e.target.value; })} /></div></div></>;
    case "settingsNotifications": return <AppPage kind="notifications" />;
    case "settingsPlayback": return <Playback />;
    case "settingsAccess": return <><Head title="Accessibility" /><div className="form-card" style={{ maxWidth: 560 }}><Switch title="Captions / transcripts" sub="Always show the words being spoken" path="accessibility.captions" /><Switch title="Large text" sub="Increase reading sizes" path="accessibility.largeText" /><DataSaverSwitch /><div className="setting-row"><div><b>Reduced motion</b><span>Respects your device preference automatically</span></div><span className="tag private">Auto</span></div></div></>;
    case "subscription": return <Sub manage={false} />;
    case "subscriptionManage": return <Sub manage />;
    case "search": return <SearchPage />;
    case "recommendations": { const rec = (st.user!.interests?.length ? st.user!.interests : ["peace", "healing", "faith", "finance"]).map(catBySlug); const gentle = isGentleTime(st); return <><div className="rec-band rv in" style={{ marginBottom: 24 }}><img src="/assets/immersive.jpg" alt="" loading="lazy" /><div className="rec-in"><Head title="Recommended for you" sub="Based on the categories you follow — never on hidden guesses." />{gentle && <span className="tag shared" style={{ marginTop: 8, display: "inline-block" }}>Gentler words for tonight</span>}</div></div><div className="grid4 snap">{rec.flatMap((c) => CONFESSIONS.filter((x) => x.category === c!.name && (!gentle || x.intensity <= 2)).slice(0, 2)).map((c) => <div key={c.slug} className="reason-wrap"><span className="reason-chip">because you follow {c.category}</span><ConfTile c={c} /></div>)}</div></>; }
    case "daily": { const hr = new Date().getHours(); const morning = hr >= 5 && hr < 17; const poolM = ["confidence", "purpose", "discipline", "favor", "career", "success", "direction", "leadership"]; const poolE = ["peace", "rest", "hope", "forgiveness", "gratitude", "family", "love", "prayer"]; const pool = CONFESSIONS.filter((c) => (morning ? poolM : poolE).includes(slugOfName(c.category))); const t = pool[new Date().getDate() % Math.max(pool.length, 1)] || CONFESSIONS[0]; return <><Head title="Today" sub={new Date().toLocaleDateString(undefined, { weekday: "long", month: "long", day: "numeric" })} /><div className="big-tile rv in"><div><span className="eyebrow on-dark">{morning ? "Morning liturgy" : "Evening liturgy"}</span><h3 style={{ marginTop: 8 }}>{t.title}</h3><p>“{t.medium}”</p><p className="small" style={{ marginTop: 8, color: "var(--tint2)" }}>{morning ? "Words to walk out with — spoken before the day speaks to you." : "Words to set the day down with — quieter, closer, kept."}</p></div><PlayBtn slug={t.slug} /></div><div className="form-card" style={{ marginTop: 16 }}>{t.scriptures.map((sc, i) => <div className="scripture" key={i}><b>{sc.book} {sc.chapter}:{sc.verse}</b> · {sc.translation}</div>)}</div></>; }
    case "streaks": { const days = [...new Set(st.history.map((h) => new Date(h.at).toDateString()))]; const counts: Record<string, number> = {}; st.history.forEach((h) => { const k = new Date(h.at).toDateString(); counts[k] = (counts[k] || 0) + 1; }); const cells = Array.from({ length: 84 }, (_, i) => { const d = new Date(Date.now() - (83 - i) * 864e5); const n = counts[d.toDateString()] || 0; return <span key={i} className={"rm-cell l" + Math.min(n, 3)} title={d.toDateString() + (n ? " · " + n : "")} />; }); return <><Head title="Streaks" sub="Returning is the practice — missing is human, and unpunished." /><div className="card-row snap" style={{ gridTemplateColumns: "repeat(3,1fr)" }}><div className="tile"><span className="t-meta">Current</span><h4 style={{ fontSize: 30 }}>{st.streak.count}d</h4></div><div className="tile"><span className="t-meta">Days played</span><h4 style={{ fontSize: 30 }}>{days.length}</h4></div><div className="tile"><span className="t-meta">This week</span><h4 style={{ fontSize: 30 }}>{days.slice(-7).length}</h4></div></div>
      <div className="form-card rv in" style={{ marginTop: 16, maxWidth: 640 }}><h3 className="h4" style={{ marginBottom: 12 }}>Returns map — last 12 weeks</h3><div className="returns-map">{cells}</div><p className="small" style={{ marginTop: 10 }}>A map, not a score. Empty squares are rest days, not failures.</p></div>
      {st.streak.count >= 10 && st.streak.count % 10 === 0 && <div className="form-card rv in" style={{ marginTop: 16, maxWidth: 640 }}><span className="eyebrow">Milestone reflection</span><h4 style={{ marginTop: 10 }}>{st.streak.count} returns. What changed?</h4><p className="small" style={{ marginTop: 6 }}>No badge, no confetti — a question instead. Answer it in your journal while it's fresh.</p><Link className="btn btn-primary" style={{ marginTop: 14 }} href="/app/journal">Write the reflection</Link></div>}</>; }
    case "achievements": { const n = st.history.length; const A: [string, boolean][] = [["First words", n >= 1], ["Three returns", st.streak.count >= 3], ["A saved library", st.favs.length >= 3], ["Builder", st.routines.length >= 1 || st.schedule.length >= 1]]; return <><Head title="Achievements" sub="Quiet markers, not pressure." /><div className="card-row snap">{A.map(([t, on]) => <div className="tile" key={t} style={on ? undefined : { opacity: .55 }}><span className="t-meta">{on ? "Unlocked" : "Locked"}</span><h4>{t}</h4><p>{on ? "Done." : "Kept for when you get there."}</p></div>)}</div></>; }
    case "shared": return <Shared token={token!} />;
    case "invite": case "referrals": return <Invite />;
    case "feedback": return <><Head title="Feedback" sub="Tell us what to fix." /><FeedbackForm /></>;
    case "support": return <><Head title="Support" /><div className="card-row snap">{[["/help", "Help Center", "Guides and answers"], ["/faq", "FAQ", "The honest eleven"], ["/contact", "Contact", "We read everything"]].map(([h, t, s]) => <Link className="tile" href={h} key={t}><h4>{t}</h4><p>{s}</p></Link>)}</div></>;
    default: return <EmptyState icon="info" title="Not found" sub="This area doesn't exist in your iCONFESS." cta={<Link className="btn btn-primary" href="/app">Go home</Link>} />;
  }
}

function ExportBtn() { const toast = useToast(); return <button className="btn btn-ghost btn-sm" onClick={() => toast("Export queued — you'll receive a download link (demo).")}>Export</button>; }

function AppHome() {
  const st = useApp(); const u = st.user!;
  const cont = st.history[0] ? confBySlug(st.history[0].slug) : null;
  const todays = CONFESSIONS[new Date().getDate() % CONFESSIONS.length];
  const recCats = (u.interests?.length ? u.interests : CATEGORIES.slice(0, 4).map((c) => c.slug)).map(catBySlug).filter(Boolean);
  const openSearch = useSearch();
  return (
    <>
      <Head title={`${greet()}, ${u.name.split(" ")[0]}.`} sub="What do you want to return to today?" right={<><button className="icon-btn" onClick={openSearch} aria-label="Search"><Icon n="search" s={16} /></button><Link className="icon-btn" href="/app/notifications" aria-label="Notifications"><Icon n="bell" s={16} /></Link></>} />
      <HandoffCard />
      <div className="app-section"><div className="section-head"><h3>Morning Mix</h3><Link className="textlink" href="/app/recommendations">Why? <Icon n="arrow" s={12} /></Link></div>
        <div className="big-tile rv in" style={{ background: "linear-gradient(140deg, var(--navy), var(--deep))" }}><div><span className="eyebrow on-dark">Made for you</span><h3 style={{ marginTop: 8 }}>Built from the categories you follow</h3><p>One confession from each of your areas — queued, never autoplayed.</p></div><MixBtn cats={recCats.map((c) => c!.slug)} label="Play Morning Mix" /></div></div>
      {cont && <div className="app-section"><div className="list-row"><div className="lr-main"><h4>Continue listening — {cont.title}</h4><p>{cont.category}</p></div><PlayChip slug={cont.slug} /></div></div>}
      <div className="app-section"><div className="big-tile rv in"><div><span className="eyebrow on-dark">Today's Experience</span><h3 style={{ marginTop: 8 }}>{todays.title}</h3><p>{todays.medium}</p></div><div style={{ display: "flex", gap: 10 }}><PlayBtn slug={todays.slug} /><Link className="btn btn-ghost on-dark" href="/app/daily">Daily</Link></div></div></div>
      <div className="app-section"><div className="section-head"><h3>Explore Categories</h3><Link className="textlink" href="/app/categories">All <Icon n="arrow" s={12} /></Link></div><div className="tgrid snap">{recCats.slice(0, 4).map((c) => <TCard key={c!.slug} c={c!} />)}</div></div>
      <div className="app-section"><div className="rec-band rv in">
        <img src="/assets/immersive.jpg" alt="" loading="lazy" />
        <div className="rec-in">
          <div className="section-head"><h3 style={{ color: "#fff" }}>Recommended For You</h3><Link className="textlink" style={{ color: "var(--tint1)" }} href="/app/recommendations">Why? <Icon n="arrow" s={12} /></Link></div>
          <div className="grid4 snap">{(CONFESSIONS.filter((c) => recCats.some((r) => r!.name === c.category) && (!isGentleTime(st) || c.intensity <= 2)).slice(0, 4).length ? CONFESSIONS.filter((c) => recCats.some((r) => r!.name === c.category) && (!isGentleTime(st) || c.intensity <= 2)).slice(0, 4) : CONFESSIONS.slice(0, 4)).map((c) => <ConfTile key={c.slug} c={c} />)}</div>
        </div>
      </div></div>
      <div className="app-section"><div className="section-head"><h3>Your Journal</h3><Link className="textlink" href="/app/journal">Open <Icon n="arrow" s={12} /></Link></div>
        {st.journals.length ? <div className="list-row"><div className="lr-main"><h4>{st.journals[0].title}</h4><p>{st.journals[0].mood} · {new Date(st.journals[0].date).toLocaleDateString()}</p></div><Link className="btn btn-ghost btn-sm" href="/app/journal">Continue</Link></div> : <EmptyState icon="edit" title="A quiet page, kept for you" sub="Journal entries stay on your account. No review, no publishing — ever." cta={<Link className="btn btn-primary" href="/app/journal">Write today's entry</Link>} />}</div>
      <div className="app-section"><div className="section-head"><h3>More practice tools</h3></div>
        <div className="card-row snap">{[["/app/memory", "Scripture memory", "Spaced returns to the verses your words stand on."], ["/app/partners", "Practice partner", "One confession a week, shared with consent."], ["/app/family", "Family space", "Guardians and kids, age-appropriate words. Premium."], ["/app/languages", "Languages", "English live; Yoruba, Igbo and Hausa in review."], ["/app/widget", "Home-screen widget", "Today's line, one tap from your phone."], ["/review-log", "Review log", "The transparency ledger for everything published."], ["/app/wrapped", "Year in Words", "Your private annual review of returned words."]].map(([h, t, x]) => <Link className="tile" href={h} key={h}><h4>{t}</h4><p>{x}</p><span className="textlink" style={{ marginTop: 6 }}>Open <Icon n="arrow" s={12} /></span></Link>)}</div></div>
      <div className="app-section"><div className="section-head"><h3>Your Routines</h3><Link className="textlink" href="/app/routines">Manage <Icon n="arrow" s={12} /></Link></div>
        <div className="card-row snap" style={{ gridTemplateColumns: "repeat(2,1fr)" }}>{["Morning", "Evening"].map((slot) => { const r = st.routines.find((x) => x.slot === slot); return <div className="tile" key={slot}><span className="t-meta">{slot}</span><h4>{r && r.cats.length ? r.cats.map((s) => catBySlug(s)?.name || s).join(" · ") : "Nothing yet"}</h4><p>{r && r.cats.length ? "Ready when you are." : "Add a category to begin."}</p><Link className="textlink" href="/app/routines" style={{ marginTop: 4 }}>Open <Icon n="arrow" s={12} /></Link></div>; })}</div></div>
      <div className="app-section"><div className="section-head"><h3>Recently Played</h3><Link className="textlink" href="/app/history">History <Icon n="arrow" s={12} /></Link></div>
        {st.history.length ? st.history.slice(0, 5).map((h, i) => <div className="list-row" key={i}><div className="lr-main"><h4>{h.title}</h4><p>{h.category} · {new Date(h.at).toLocaleDateString()}</p></div><PlayChip slug={h.slug} /></div>) : <EmptyState icon="play" title="Nothing played yet" sub="Your history appears after your first session." />}</div>
    </>
  );
}

function AppCategory({ slug }: { slug: string }) {
  const cat = catBySlug(slug); const st2 = useApp(); const toast2 = useToast(); if (!cat) return <EmptyState icon="info" title="Not found" sub="Unknown category." />;
  const s5 = sessionsFor(cat.slug)[0];
  return (
    <>
      <Head title={cat.name} sub={cat.tagline} />
      <div className="big-tile rv in" style={{ marginBottom: 28 }}><div><p style={{ color: "var(--tint1)" }}>{cat.description}</p></div><div style={{ display: "flex", gap: 10, flexWrap: "wrap" }}><PlaySessionBtn slug={s5.slug} /><button className="btn btn-ghost on-dark" onClick={() => { if (st2.user?.plan === "free") { toast2("Offline pins are a Premium feature"); return; } const on = st2.downloads.includes("cat:" + cat.slug); mutate((x) => { x.downloads = on ? x.downloads.filter((d) => d !== "cat:" + cat.slug) : [...x.downloads, "cat:" + cat.slug]; }); toast2(on ? "Category unpinned" : "Category pinned offline"); }}>{st2.downloads.includes("cat:" + cat.slug) ? "Pinned ✓" : "Pin category offline"}</button></div></div>
      <div className="grid4 snap">{CONFESSIONS.filter((c) => c.category === cat.name).map((c) => <ConfTile key={c.slug} c={c} />)}</div>
    </>
  );
}
function AppConfession({ slug }: { slug: string }) {
  const c = confBySlug(slug); if (!c) return <EmptyState icon="info" title="Not found" sub="Unknown confession." />;
  return (
    <>
      <Link className="textlink" href="/app/confessions"><Icon n="arrowL" s={12} /> Confessions</Link>
      <Head title={c.title} sub={c.category} />
      <div style={{ display: "flex", gap: 10, marginBottom: 20 }}><PlayBtn slug={c.slug} /><FavBtn slug={c.slug} ghost /></div>
      <div className="form-card"><p style={{ fontSize: 16, lineHeight: 1.7 }}>“{c.long}”</p>{c.scriptures.map((s, i) => <div className="scripture" key={i}><b>{s.book} {s.chapter}:{s.verse}</b> · {s.translation}</div>)}</div>
    </>
  );
}
function AppSession({ slug }: { slug: string }) {
  const s = sessionBySlug(slug); if (!s) return <EmptyState icon="info" title="Not found" sub="Unknown session." />;
  const cat = catBySlug(s.category)!;
  return (
    <>
      <Head title={s.title} sub={`${cat.name} · ${s.minutes} min`} />
      <div style={{ marginBottom: 24, display: "flex", gap: 12, flexWrap: "wrap" }}><PlaySessionBtn slug={s.slug} big /><SaveOffline slug={s.slug} /></div>
      <div className="form-card">{s.items.map((c) => <div className="list-row" key={c.slug}><div className="lr-main"><h4>{c.title}</h4><p>{c.short}</p></div><PlayChip slug={c.slug} /></div>)}</div>
    </>
  );
}

function Builder() {
  const sp = useSearchParams();
  const [cat, setCat] = useState(sp.get("cat") || "peace");
  const [min, setMin] = useState(5);
  const [tone, setTone] = useState("any");
  const [out, setOut] = useState<Session | null>(null);
  const audio = useAudio(); const router = useRouter();
  const preview = () => {
    let items = CONFESSIONS.filter((c) => c.category === catBySlug(cat)!.name);
    if (tone === "gentle") items = items.filter((c) => c.intensity <= 2) || items;
    if (tone === "urgent") items = items.filter((c) => c.intensity >= 3) || items;
    if (!items.length) items = CONFESSIONS.filter((c) => c.category === catBySlug(cat)!.name);
    setOut({ slug: "built", category: cat, minutes: min, title: `${catBySlug(cat)!.name} — ${min} minutes (built)`, description: "Your custom session.", items });
  };
  return (
    <>
      <Head title="Session Builder" sub="Choose a category and a length — the engine assembles the rest." />
      <form className="form-card rv in" style={{ maxWidth: 640 }} onSubmit={(e) => { e.preventDefault(); preview(); }}>
        <div className="field"><label>Category</label><select value={cat} onChange={(e) => setCat(e.target.value)}>{CATEGORIES.map((c) => <option value={c.slug} key={c.slug}>{c.name}</option>)}</select></div>
        <div className="field"><label>Length</label><div className="chip-row">{[5, 10, 15].map((m) => <button type="button" className="chip" key={m} aria-pressed={min === m} onClick={() => setMin(m)}>{m} minutes</button>)}</div></div>
        <div className="field"><label>Tone</label><select value={tone} onChange={(e) => setTone(e.target.value)}><option value="any">Any intensity</option><option value="gentle">Gentle (1–2)</option><option value="urgent">Urgent (3+)</option></select></div>
        <button className="btn btn-primary btn-lg" type="submit" style={{ marginTop: 20 }}>Preview Session</button>
      </form>
      {out && <div className="form-card" style={{ marginTop: 24, maxWidth: 640 }}><h3 className="h4" style={{ marginBottom: 12 }}>{buildQueue(out).length} confessions · ~{out.minutes} min</h3>
        {buildQueue(out).map((q, i) => <div className="list-row" key={i}><div className="lr-main"><h4>{q.title}</h4><p>{q.category}</p></div></div>)}
        <button className="btn btn-primary btn-lg" style={{ marginTop: 16 }} onClick={() => { audio.play(buildQueue(out)); track("session_started", { built: true }); router.push("/app/player"); }}><Icon n="play" s={14} /> Start Session</button></div>}
    </>
  );
}

function Player() {
  const audio = useAudio(); const router = useRouter(); const st = useApp(); const toast = useToast();
  const it = audio.current();
  const [speed, setSpeed] = useState(1);
  const [timer, setTimer] = useState(0);
  const tRef = React.useRef<ReturnType<typeof setTimeout> | null>(null);
  const [still, setStill] = useState(0);
  const ambient = !!st.settings.ambienceId;
  const chime = () => { try { const C = window.AudioContext; const ctx = new C(); const o = ctx.createOscillator(); const g = ctx.createGain(); o.frequency.value = 528; g.gain.setValueAtTime(0.07, ctx.currentTime); g.gain.exponentialRampToValueAtTime(0.0001, ctx.currentTime + 1.4); o.connect(g); g.connect(ctx.destination); o.start(); o.stop(ctx.currentTime + 1.5); } catch { } };
  React.useEffect(() => { if (audio.status === "completed") setStill(30); }, [audio.status]);
  React.useEffect(() => { if (still <= 0) return; if (still === 1) chime(); const t = setTimeout(() => setStill((v) => v - 1), 1000); return () => clearTimeout(t); }, [still]);
  const toggleAmbient = () => {
    if (st.user?.plan === "free") { toast("Ambient bed is a Premium feature"); return; }
    const next = st.settings.ambienceId ? "" : "rain";
    mutate((x) => { x.settings.ambienceId = next; });
    if (next) toast("Ambience: rain — pick others in Sound"); else toast("Ambience off");
    logAE("ambience_changed", { id: next });
  };
  const whisper = () => {
    const SR = (window as any).SpeechRecognition || (window as any).webkitSpeechRecognition;
    if (!SR || !it) { toast("Speech recognition isn't available in this browser"); return; }
    const r = new SR(); r.lang = "en-US"; r.interimResults = false;
    toast("Listening — say the words out loud");
    r.onresult = (e: any) => { const said = String(e.results[0][0].transcript).toLowerCase(); const target = it.text.toLowerCase().split(/[^a-z]+/).filter((w: string) => w.length > 3); const hit = target.filter((w: string) => said.includes(w)).length; toast(target.length ? Math.round((hit / target.length) * 100) + "% of the words spoken — it counts when you say it" : "Heard you — keep going"); };
    r.onerror = () => toast("Couldn't hear you — try again somewhere quiet");
    r.start();
  };
  const cycleTimer = () => {
    const opts = [0, 5, 10, 15];
    const next = opts[(opts.indexOf(timer) + 1) % opts.length];
    if (next > 0 && st.user?.plan === "free") { toast("Sleep timer is a Premium feature"); return; }
    setTimer(next);
    if (tRef.current) clearTimeout(tRef.current);
    if (next > 0) { tRef.current = setTimeout(() => { audio.pause(); }, next * 60000); toast("Sleep timer: " + next + " min"); }
  };
  const catSlug = it ? catBySlug(it.category)?.slug || "" : "";
  const sents = it ? (it.text.match(/[^.!?]+[.!?]?/g) || [it.text]) : [];
  const activeLine = Math.min(sents.length - 1, Math.floor(audio.progress * sents.length));
  const cycleRepeat = () => { const opts = [1, 3, 7]; const next = opts[(opts.indexOf(st.settings.repeat || 1) + 1) % opts.length]; mutate((x) => { x.settings.repeat = next; }); toast(next === 1 ? "Repeat off" : "Repeat " + next + "×"); };
  const handoff = () => {
    if (st.user?.plan === "free") { toast("Device handoff is a Premium feature"); return; }
    localStorage.setItem("ic-handoff", JSON.stringify({ slugs: audio.queue.map((q) => q.slug), idx: audio.idx }));
    toast("Handoff saved — open iCONFESS on another device to resume");
  };
  useEffect(() => { }, [audio.status, audio.progress, audio.idx]);
  return (
    <div className="player-stage">
      {it && CAT_IMAGES[catSlug] && <img className="ps-canvas" src={`/assets/${CAT_IMAGES[catSlug]}`} alt="" aria-hidden="true" />}
      <Link className="ps-voice" href="/app/voices" style={{ display: "inline-flex", alignItems: "center", gap: 8 }}><img src="/assets/voice-grace.jpg" alt="" style={{ display: "none" }} /> Voice · {audio.voiceName()} · change <Icon n="arrow" s={12} /></Link>
      <Link className="btn btn-ghost on-dark btn-sm ps-exit" href="/app"><Icon n="exit" s={14} /> Exit</Link>
      <span className="ps-cat">{it ? it.category : "—"}</span>
      <h1>{it ? it.title : "Nothing playing"}</h1>
      {it && st.settings.accessibility.captions ? (
        <p className="ps-text tr-sync">{sents.map((sn, i) => <span key={i} className={"tr-line" + (i === activeLine && audio.status === "playing" ? " on" : "")}>{sn} </span>)}</p>
      ) : (
        <p className="ps-text">{it ? "“" + it.text + "”" : "Start a session from Sessions or the Builder, or play any confession — the words appear here as Grace speaks them."}</p>
      )}
      <div className="player-controls">
        <button className="pc-btn" onClick={audio.prev} aria-label="Previous"><Icon n="prev" s={18} /></button>
        <button className="pc-btn main" onClick={() => (audio.status === "playing" ? audio.pause() : audio.status === "paused" ? audio.resume() : router.push("/app/sessions"))} aria-label="Play or pause"><Icon n={audio.status === "playing" ? "pause" : "play"} s={22} /></button>
        <button className="pc-btn" onClick={audio.next} aria-label="Next"><Icon n="next" s={18} /></button>
        <button className="pc-btn" style={{ fontSize: 12, fontWeight: 700 }} aria-label="Speed" onClick={() => { const r = speed >= 1.4 ? 0.8 : +(speed + 0.2).toFixed(1); setSpeed(r); mutate((s) => { s.settings.rate = r; }); }}>{speed}×</button>
        <button className="pc-btn" style={{ fontSize: 12, fontWeight: 700 }} aria-label="Sleep timer" title="Sleep timer (Premium)" onClick={cycleTimer}>{timer ? timer + "m" : <Icon n="moon" s={16} />}</button>
        <button className="pc-btn" aria-label="Say it — whisper check" title="Whisper check: say the words, on-device only" onClick={whisper}><Icon n="mic" s={16} /></button>
        <button className="pc-btn" aria-label="Ambient bed" title="Ambient bed (Premium)" onClick={toggleAmbient} style={ambient ? { background: "var(--blue2)", color: "#fff" } : undefined}><Icon n="spark" s={16} /></button>
        <button className="pc-btn" aria-label="30 seconds of stillness" title="Stillness outro" onClick={() => setStill(30)}><Icon n="ast" s={16} /></button>
        <button className="pc-btn" style={{ fontSize: 12, fontWeight: 700 }} aria-label="Repeat mode" title="Repeat this confession" onClick={cycleRepeat}>{(st.settings.repeat || 1) > 1 ? st.settings.repeat + "×" : "1×"}</button>
        <button className="pc-btn" style={{ fontSize: 11, fontWeight: 700 }} aria-label="Continue on another device" title="Device handoff (Premium)" onClick={handoff}><Icon n="exit" s={14} /></button>
      </div>
      {it && audio.queue.length > audio.idx + 1 && (
        <div className="ps-queue" aria-label="Up next">
          <h4 style={{ color: "var(--tint2)", fontSize: 11, letterSpacing: ".14em", textTransform: "uppercase", marginBottom: 8 }}>Up next</h4>
          {audio.queue.slice(audio.idx + 1).map((q2, i) => (
            <div className="psq-row" key={q2.slug + i}>
              <div className="lr-main" style={{ minWidth: 0 }}><h4 style={{ color: "#fff", fontSize: 13 }}>{q2.title}</h4><p style={{ color: "var(--tint3)", fontSize: 11 }}>{q2.category}</p></div>
              <button className="pc-btn" style={{ width: 30, height: 30 }} aria-label={"Play " + q2.title + " next"} onClick={() => { const nq = [...audio.queue]; const [mv] = nq.splice(audio.idx + 1 + i, 1); nq.splice(audio.idx + 1, 0, mv); audio.updateQueue(nq, audio.idx + 1); }}><Icon n="next" s={12} /></button>
              <button className="pc-btn" style={{ width: 30, height: 30 }} aria-label={"Remove " + q2.title} onClick={() => { const nq = audio.queue.filter((_, j) => j !== audio.idx + 1 + i); audio.updateQueue(nq, audio.idx); }}><Icon n="x" s={12} /></button>
            </div>
          ))}
        </div>
      )}
      <div style={{ maxWidth: 600, margin: "28px auto 0", width: "100%" }}><SoundPanel compact /></div>
      <div role="status" aria-live="polite" className="sr-only">{it ? `Now ${audio.status}: ${it.title}, ${it.category}` : "Nothing playing"}</div>
      {still > 0 && (
        <div className="still-veil" role="dialog" aria-label="Stillness">
          <div className="still-card">
            <span className="eyebrow on-dark">Stillness</span>
            <div className="still-num">{still}</div>
            <p>Let the words settle. Nothing else is asked of you.</p>
            <button className="btn btn-ghost on-dark" onClick={() => setStill(0)}>End stillness</button>
          </div>
        </div>
      )}
      <div className="ps-progress">
        <div className="progress"><i style={{ width: Math.round(audio.progress * 100) + "%" }} /></div>
        <div className="ps-meta"><span>{audio.status}</span><span>{it ? `${audio.idx + 1} / ${audio.queue.length}` : ""}</span></div>
      </div>
    </div>
  );
}

function Switch({ title, sub, path }: { title: string; sub: string; path: string }) {
  const st = useApp();
  const parts = path.split(".");
  let val: any = st.settings; for (const p of parts) val = val?.[p];
  return (
    <div className="setting-row">
      <div><b>{title}</b><span>{sub}</span></div>
      <button className="switch" role="switch" aria-checked={!!val} aria-label={title} onClick={() => mutate((s) => { let o: any = s.settings; while (parts.length > 1) o = o[parts.shift()!]; o[parts[0]] = !val; })} />
    </div>
  );
}

function Schedule() {
  const st = useApp(); const toast = useToast();
  const [cat, setCat] = useState("peace"); const [time, setTime] = useState("07:00"); const [days, setDays] = useState("Daily");
  return (
    <>
      <Head title="Schedule" sub="Sessions at the times you choose" />
      <form className="form-card rv in" style={{ maxWidth: 560, marginBottom: 24 }} onSubmit={(e) => { e.preventDefault(); mutate((s) => { s.schedule.push({ id: Date.now(), category: cat, time, days }); }); toast("Scheduled"); }}>
        <div className="field"><label>Category</label><select value={cat} onChange={(e) => setCat(e.target.value)}>{CATEGORIES.map((c) => <option key={c.slug}>{c.slug}</option>)}</select></div>
        <div style={{ display: "flex", gap: 12 }}>
          <div className="field" style={{ flex: 1 }}><label>Time</label><input type="time" value={time} onChange={(e) => setTime(e.target.value)} required /></div>
          <div className="field" style={{ flex: 1 }}><label>Days</label><select value={days} onChange={(e) => setDays(e.target.value)}><option>Daily</option><option>Weekdays</option><option>Weekends</option></select></div>
        </div>
        <button className="btn btn-primary" type="submit" style={{ marginTop: 16 }}><Icon n="plus" s={13} /> Add to schedule</button>
      </form>
      {st.schedule.length ? st.schedule.map((s) => <div className="list-row" key={s.id}><div className="lr-main"><h4>{catBySlug(s.category)?.name || s.category} · {s.time}</h4><p>{s.days}</p></div><button className="icon-btn" aria-label="Delete" onClick={() => mutate((x) => { x.schedule = x.schedule.filter((y) => y.id !== s.id); })}><Icon n="x" s={14} /></button></div>) : <EmptyState icon="cal" title="Nothing scheduled" sub="The practice survives a bad week when it's scheduled." />}
    </>
  );
}

function Routines() {
  const st = useApp(); const toast = useToast();
  return (
    <>
      <Head title="Routines" sub="Morning and evening anchors" />
      {["Morning", "Evening"].map((slot) => {
        const r = st.routines.find((x) => x.slot === slot);
        return (
          <div className="app-section" key={slot}>
            <div className="section-head"><h3>{slot}</h3></div>
            <div className="form-card">{r && r.cats.length ? r.cats.map((s) => <div className="list-row" key={s}><div className="lr-main"><h4>{catBySlug(s)?.name || s}</h4><p>{catBySlug(s)?.tagline || ""}</p></div><PlaySessionBtn slug={`${s}-5-min`} /></div>) : <p className="small">Add categories below.</p>}</div>
            <div className="chip-row" style={{ marginTop: 12 }}>{CATEGORIES.slice(0, 10).map((c) => <button className="chip" key={c.slug} onClick={() => { mutate((s) => { const rt = s.routines.find((x) => x.slot === slot) || (s.routines.push({ slot, cats: [] }), s.routines.find((x) => x.slot === slot))!; if (!rt.cats.includes(c.slug)) rt.cats.push(c.slug); }); toast(`Added to ${slot} routine`); }}>+ {c.name}</button>)}</div>
          </div>
        );
      })}
    </>
  );
}

function CreateConfession() {
  const router = useRouter(); const toast = useToast();
  const [text, setText] = useState("");
  const onFile = (e: React.ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0]; if (!f) return;
    if (f.size > 200000) { toast("File too large — 200KB max"); return; }
    const r = new FileReader();
    r.onload = () => { setText(String(r.result || "").slice(0, 4000)); toast("Loaded from " + f.name); };
    r.readAsText(f);
  };
  return (
    <>
      <Head title="Create a confession" sub="Type it or upload it. Your words, your visibility. Human review for anything public." />
      <form className="form-card rv in" style={{ maxWidth: 640 }} onSubmit={(e) => {
        e.preventDefault();
        const f = new FormData(e.currentTarget);
        const vis = String(f.get("vis"));
        mutate((s) => { s.community.unshift({ slug: "mine-" + Date.now().toString(36), title: String(f.get("title") || "My confession"), text: text, visibility: vis === "mentor" ? "private" : vis, status: vis === "public" ? "pending_review" : vis === "mentor" ? "mentor_review" : vis, category: String(f.get("cat")), at: Date.now() }); });
        track("content_saved", { kind: "community" });
        toast(vis === "public" ? "Submitted for human review" : "Saved — " + vis);
        router.push("/app/community");
      }}>
        <div className="field"><label>Title</label><input name="title" required maxLength={60} /></div>
        <div className="field"><label>Your confession</label><textarea name="text" rows={6} required value={text} onChange={(e) => setText(e.target.value)} placeholder="Write in the first person, present tense…" /></div>
        <div className="field"><label>…or upload it</label>
          <label className="upload-box" htmlFor="conf-file"><Icon n="dl" s={16} /> <span>{text ? "Replace from a file (.txt / .md)" : "Upload a text file (.txt / .md)"}</span></label>
          <input id="conf-file" type="file" accept=".txt,.md,text/plain,text/markdown" style={{ display: "none" }} onChange={onFile} /></div>
        <div className="field"><label>Category</label><select name="cat">{CATEGORIES.map((c) => <option value={c.slug} key={c.slug}>{c.name}</option>)}</select></div>
        <div className="field"><label>Visibility</label><select name="vis"><option value="private">Private — only me, forever</option><option value="shared">Shared — anyone with my link</option><option value="public">Public — submit for human review</option><option value="mentor">Mentor — Premium written feedback, never published</option></select></div>
        <button className="btn btn-primary btn-lg" type="submit" style={{ marginTop: 18 }}>Save</button>
      </form>
    </>
  );
}
function CommunityDetail({ slug }: { slug: string }) {
  const st = useApp(); const c = st.community.find((x) => x.slug === slug);
  if (!c) return <EmptyState icon="info" title="Not found" sub="This confession doesn't exist." />;
  return <><Head title={c.title} sub={"Visibility: " + c.status.replace("_", " ")} /><div className="form-card"><p style={{ fontSize: 16, lineHeight: 1.7 }}>“{c.text}”</p></div><div style={{ marginTop: 16 }}><ShareBtn slug={c.slug} /></div></>;
}

function Profile() {
  const st = useApp(); const u = st.user!; const router = useRouter(); const toast = useToast();
  return (
    <>
      <Head title="Profile" />
      <div className="form-card rv in" style={{ maxWidth: 640 }}>
        <div style={{ display: "flex", gap: 18, alignItems: "center" }}>
          <span style={{ width: 64, height: 64, borderRadius: "50%", background: motifStyle(u.email || "me").background, display: "flex", alignItems: "center", justifyContent: "center", color: "#fff", fontSize: 22, fontWeight: 800 }}>{u.name[0]?.toUpperCase() || "i"}</span>
          <div><h3 className="h3">{u.name}</h3><p className="small">{u.email} · {u.plan === "free" ? "Free plan" : "Premium"}</p></div>
        </div>
        <div style={{ display: "flex", gap: 32, marginTop: 22 }}>{[["Streak", st.streak.count + "d"], ["Saved", st.favs.length], ["Played", st.history.length]].map(([l, v]) => <div key={l as string}><span className="t-meta">{l}</span><div style={{ fontSize: 24, fontWeight: 800 }}>{v}</div></div>)}</div>
        <div style={{ marginTop: 22, display: "flex", gap: 10, flexWrap: "wrap" }}>
          <Link className="btn btn-ghost btn-sm" href="/app/settings">Settings</Link>
          <Link className="btn btn-ghost btn-sm" href="/app/subscription">Subscription</Link>
          <Link className="btn btn-ghost btn-sm" href="/app/achievements">Achievements</Link>
          <button className="btn btn-ghost btn-sm" onClick={() => { mutate((s) => { s.user = null; }); toast("Logged out"); router.push("/"); }}>Log out</button>
        </div>
      </div>
    </>
  );
}

function Playback() {
  const st = useApp();
  return (
    <>
      <Head title="Playback" sub="One sound, tuned to you — saved across this device" />
      <div className="form-card" style={{ maxWidth: 560, marginBottom: 16 }}>
        <div className="setting-row"><div><b>Narration voice</b><span>Curated catalog · rights-tracked</span></div><Link className="btn btn-ghost btn-sm" href="/app/voices">{st.settings.voiceId === "grace" ? "Grace" : (st.settings.voiceId.charAt(0).toUpperCase() + st.settings.voiceId.slice(1))} · change</Link></div>
        <Switch title="Loudness normalisation" sub="Even volume across every confession and session" path="normalize" />
        <div className="setting-row"><div><b>Voice EQ preset</b><span>Legacy tone shaping</span></div><div className="chip-row">{[["warm", "Warm"], ["bright", "Bright"], ["calm", "Calm"]].map(([v, l]) => <button className="chip" key={v} aria-pressed={st.settings.eq === v} onClick={() => { mutate((x) => { x.settings.eq = v; }); }}>{l}</button>)}</div></div>
        <div className="setting-row"><div><b>Voice warmth preset</b><span>Premium · pace and tone per part of day</span></div><div className="chip-row">{[["dawn", "Calm dawn"], ["steady", "Steady midday"], ["night", "Soft night"]].map(([v, l]) => <button className="chip" key={v} aria-pressed={st.settings.preset === v} onClick={() => { mutate((x) => { x.settings.preset = v; x.settings.rate = v === "night" ? 0.9 : 1; }); }}>{l}</button>)}</div></div>
        <div className="setting-row"><div><b>Offline quality</b><span>Premium · lossless packs for saved sessions</span></div><div className="chip-row">{[["std", "Standard"], ["lossless", "Lossless"]].map(([v, l]) => <button className="chip" key={v} aria-pressed={st.settings.dlQuality === v} onClick={() => { mutate((x) => { x.settings.dlQuality = v; }); }}>{l}</button>)}</div></div>
      </div>
      <div style={{ maxWidth: 560 }}><SoundPanel compact /></div>
    </>
  );
}

function Sub({ manage }: { manage?: boolean }) {
  const st = useApp(); const toast = useToast();
  const prem = st.user!.plan !== "free";
  return (
    <>
      <Head title={manage ? "Manage subscription" : "Subscription"} sub={prem ? "Premium active — 7-day trial" : "Free plan"} />
      {prem && (
        <div className="form-card" style={{ maxWidth: 640, marginBottom: 24 }}>
          {[["Current plan", st.user!.plan === "annual" ? "Premium Annual" : "Premium Monthly"], ["Renewal", "In 30 days (demo)"], ["Benefits", "Premium voices · offline downloads · long sessions"]].map(([b, s]) => <div className="setting-row" key={b}><div><b>{b}</b><span>{s}</span></div></div>)}
          <div style={{ display: "flex", gap: 10, marginTop: 16 }}>
            <button className="btn btn-ghost btn-sm" onClick={() => { mutate((s) => { s.user!.plan = "free"; }); track("subscription_cancelled"); toast("Subscription cancelled — you keep Premium until period end"); }}>Cancel subscription</button>
            <button className="btn btn-ghost btn-sm" onClick={() => toast("Purchase restore checked — nothing to restore (demo).")}>Restore purchase</button>
          </div>
        </div>
      )}
      <div className="plan-grid">
        {PLANS_LOCAL.map((p) => (
          <div className={"plan-card" + (p.interval === "year" ? " gold" : "")} key={p.id}>
            <h3 className="h3">{p.name}</h3>
            <div><span className="price">{p.prices.USD}</span> <span className="per">/ {p.interval} · {p.prices.NGN}</span></div>
            <ul>{["Premium voices", "Offline downloads (mobile)", "Longer sessions", "7-day trial"].map((f) => <li key={f}><Icon n="check" s={14} /> {f}</li>)}</ul>
            <button className={"btn " + (p.interval === "year" ? "btn-light" : "btn-primary")} onClick={() => { mutate((s) => { s.user!.plan = p.id; }); track("subscription_started", { plan: p.id }); toast("Premium trial started"); }}>{prem ? "Switch to " + p.name : "Start 7-day trial"}</button>
          </div>
        ))}
      </div>
    </>
  );
}

function SearchPage() {
  const open = useSearch(); const st = useApp();
  return (
    <>
      <Head title="Search" />
      <div className="form-card" style={{ maxWidth: 640 }}>
        <button className="btn btn-primary" onClick={open}><Icon n="search" s={14} /> Open global search</button>
        <p className="small" style={{ marginTop: 12 }}>Finds confessions, categories, sessions, voices and journal essays.</p>
        {st.recentSearches.length > 0 && <div className="chip-row" style={{ marginTop: 14 }}>{st.recentSearches.map((r) => <span className="chip" key={r}>{r}</span>)}</div>}
      </div>
    </>
  );
}
function Shared({ token }: { token: string }) {
  const st = useApp(); const sh = st.shared[token]; const c = sh && confBySlug(sh.slug);
  if (!c) return <div style={{ maxWidth: 520, margin: "40px auto" }}><EmptyState icon="share" title="This share link has expired or doesn't exist" sub="Shared links live only as long as the sharer wants." /></div>;
  return <><Head title="Shared with you" sub={c.category} /><div className="form-card"><p style={{ fontSize: 18, lineHeight: 1.7 }}>“{c.long}”</p>{c.scriptures.map((s, i) => <div className="scripture" key={i}><b>{s.book} {s.chapter}:{s.verse}</b></div>)}</div><div style={{ marginTop: 16 }}><PlayBtn slug={c.slug} /></div></>;
}
function Invite() {
  const toast = useToast();
  return (
    <>
      <Head title="Invite" sub="The practice is better shared." />
      <div className="form-card" style={{ maxWidth: 560 }}>
        <p className="small" style={{ marginBottom: 14 }}>Your invite link</p>
        <div className="list-row"><div className="lr-main"><h4 style={{ whiteSpace: "normal" }}>{typeof location !== "undefined" ? location.origin : ""}/register</h4></div><button className="btn btn-ghost btn-sm" onClick={() => { navigator.clipboard?.writeText(location.origin + "/register"); toast("Invite link copied"); }}><Icon n="share" s={13} /> Copy</button></div>
        <p className="small">No reward mechanics. If it's good, they'll stay; if not, they won't.</p>
      </div>
    </>
  );
}
function FeedbackForm() {
  const toast = useToast();
  return <form className="form-card" style={{ maxWidth: 560 }} onSubmit={(e) => { e.preventDefault(); toast("Thank you — feedback recorded"); (e.target as HTMLFormElement).reset(); }}>
    <div className="field"><label>What's on your mind?</label><textarea rows={5} required /></div>
    <button className="btn btn-primary" type="submit" style={{ marginTop: 14 }}>Send</button>
  </form>;
}

function SaveOffline({ slug }: { slug: string }) {
  const st = useApp(); const toast = useToast();
  const on = st.downloads.includes(slug);
  return (
    <button className="btn btn-ghost" onClick={() => {
      if (!on && st.user?.plan === "free") { toast("Offline downloads are a Premium feature"); return; }
      mutate((s) => { s.downloads = on ? s.downloads.filter((x) => x !== slug) : [...s.downloads, slug]; });
      toast(on ? "Removed from offline" : "Saved for offline");
    }}>{on ? "Saved offline ✓" : "Save for offline"}</button>
  );
}

function JournalApp() {
  const st = useApp(); const toast = useToast();
  const [title, setTitle] = useState(""); const [text, setText] = useState(""); const [mood, setMood] = useState("Grateful"); const [audioNote, setAudioNote] = useState("");
  const save = (e: React.FormEvent) => {
    e.preventDefault();
    if (!text.trim()) { toast("Write a line first"); return; }
    mutate((s) => { s.journals.unshift({ id: Date.now(), date: new Date().toISOString(), title: title.trim() || "Entry — " + new Date().toLocaleDateString(), text: text.trim(), mood, audio: audioNote || undefined }); }); setAudioNote("");
    track("content_saved", { kind: "journal" });
    toast("Journal saved — private to you");
    setTitle(""); setText("");
  };
  return (
    <>
      <Head title="Journal" sub="Private by design. No review, no publishing — a page kept for you." />
      <form className="form-card rv in" style={{ maxWidth: 640 }} onSubmit={save}>
        <div className="field"><label>Today feels</label>
          <div className="chip-row">{["Grateful", "Hopeful", "Heavy", "Quiet", "Bold"].map((m) => <button type="button" className="chip" key={m} aria-pressed={mood === m} onClick={() => setMood(m)}>{m}</button>)}</div></div>
        <div className="field"><label>Title (optional)</label><input value={title} onChange={(e) => setTitle(e.target.value)} maxLength={80} placeholder={new Date().toLocaleDateString(undefined, { weekday: "long", month: "long", day: "numeric" })} /></div>
        <div className="field"><label>Entry</label><textarea rows={6} value={text} onChange={(e) => setText(e.target.value)} placeholder="What did the words meet in you today?" /></div>
        <div className="field"><label>Voice note (optional, 30s, stays private)</label>
          <div style={{ display: "flex", gap: 10, alignItems: "center" }}>
            <button type="button" className="btn btn-ghost" onClick={() => {
              const MR = (window as any).MediaRecorder; if (!MR) { toast("Recording not supported here"); return; }
              navigator.mediaDevices.getUserMedia({ audio: true }).then((m) => {
                const rec = new MR(m); const chunks: Blob[] = [];
                rec.ondataavailable = (e: any) => chunks.push(e.data);
                rec.onstop = () => { m.getTracks().forEach((t) => t.stop()); const b = new Blob(chunks, { type: "audio/webm" }); if (b.size > 700000) { toast("Keep it under ~30 seconds"); return; } const r = new FileReader(); r.onload = () => { setAudioNote(String(r.result)); toast("Voice note attached"); }; r.readAsDataURL(b); };
                rec.start(); toast("Recording — 30s max"); setTimeout(() => { if (rec.state === "recording") rec.stop(); }, 30000);
              }).catch(() => toast("Microphone permission denied"));
            }}><Icon n="mic" s={14} /> Record</button>
            {audioNote && <><span className="tag private">attached</span><audio controls src={audioNote} style={{ height: 36, flex: 1 }} /><button type="button" className="icon-btn" aria-label="Remove voice note" onClick={() => setAudioNote("")}><Icon n="x" s={12} /></button></>}
          </div></div>
        <button className="btn btn-primary btn-lg" type="submit" style={{ marginTop: 16 }}>Save entry</button>
      </form>
      <div style={{ maxWidth: 640, marginTop: 24, display: "flex", flexDirection: "column", gap: 14 }}>
        {st.journals.map((j) => (
          <div className="form-card rv in" key={j.id}>
            <div style={{ display: "flex", justifyContent: "space-between", gap: 12, alignItems: "flex-start" }}>
              <div><span className="tag private">{j.mood}</span><h4 style={{ marginTop: 8 }}>{j.title}</h4><p className="small">{new Date(j.date).toLocaleString()}</p></div>
              <button className="icon-btn" aria-label="Delete entry" onClick={() => { mutate((s) => { s.journals = s.journals.filter((x) => x.id !== j.id); }); toast("Entry deleted"); }}><Icon n="x" s={14} /></button>
            </div>
            <p style={{ marginTop: 12, lineHeight: 1.7 }}>{j.text}</p>{j.audio && <audio controls src={j.audio} style={{ marginTop: 10, width: "100%", height: 36 }} />}
          </div>
        ))}
      </div>
    </>
  );
}

function MemoryApp() {
  const st = useApp(); const toast = useToast();
  const now = Date.now();
  const due = st.memory.filter((m) => m.due <= now);
  const [reveal, setReveal] = useState<string | null>(null);
  const addToday = () => {
    const c = CONFESSIONS[(new Date().getDate() + st.memory.length) % CONFESSIONS.length];
    const sc = c.scriptures[0];
    if (st.memory.some((m) => m.ref === `${sc.book} ${sc.chapter}:${sc.verse}`)) { toast("Already in your memory box"); return; }
    mutate((s) => { s.memory.push({ ref: `${sc.book} ${sc.chapter}:${sc.verse}`, text: c.short, title: c.title, box: 0, due: Date.now() }); });
    toast("Added to memory practice");
  };
  const grade = (ref: string, up: boolean) => {
    mutate((s) => { const m = s.memory.find((x) => x.ref === ref); if (!m) return; m.box = up ? Math.min(m.box + 1, 5) : 0; m.due = Date.now() + [1, 1, 3, 7, 14, 30][m.box] * 864e5; });
    setReveal(null);
  };
  return (
    <>
      <Head title="Scripture memory" sub="Spaced returns to the verses your confessions stand on. Say them out loud." right={<button className="btn btn-primary btn-sm" onClick={addToday}><Icon n="plus" s={12} /> Add today's verse</button>} />
      {due.length ? due.map((m) => (
        <div className="form-card rv in" key={m.ref} style={{ maxWidth: 640, marginBottom: 14 }}>
          <div style={{ display: "flex", justifyContent: "space-between", gap: 12 }}><h4>{m.ref}</h4><span className="tag private">Box {m.box + 1}/6</span></div>
          <p className="small" style={{ marginTop: 6 }}>Stands under: {m.title}</p>
          {reveal === m.ref ? (
            <><p style={{ marginTop: 12, lineHeight: 1.7 }}>“{m.text}”</p>
              <div style={{ display: "flex", gap: 10, marginTop: 14 }}><button className="btn btn-primary" onClick={() => grade(m.ref, true)}>I said it</button><button className="btn btn-ghost" onClick={() => grade(m.ref, false)}>Missed it</button></div></>
          ) : <button className="btn btn-ghost" style={{ marginTop: 12 }} onClick={() => setReveal(m.ref)}>Say it from memory, then reveal</button>}
        </div>
      )) : <EmptyState icon="ast" title="Nothing due today" sub="Add today's verse and it will return on a spaced schedule — 1, 3, 7, 14, 30 days." cta={<button className="btn btn-primary" onClick={addToday}>Add today's verse</button>} />}
      {st.memory.length > 0 && <p className="small">{st.memory.length} verse{st.memory.length === 1 ? "" : "s"} in practice · {due.length} due now</p>}
    </>
  );
}

function PartnersApp() {
  const st = useApp(); const toast = useToast();
  const [name, setName] = useState("");
  const week = Math.floor(Date.now() / (7 * 864e5));
  const shared = CONFESSIONS[week % CONFESSIONS.length];
  const pair = (e: React.FormEvent) => {
    e.preventDefault();
    mutate((s) => { s.partners = [{ name: name.trim() || "Partner", code: "IC-" + Math.random().toString(36).slice(2, 7).toUpperCase(), since: Date.now() }]; });
    track("partner_paired"); toast("Paired — one confession a week, both ways");
  };
  return (
    <>
      <Head title="Practice partner" sub="A consensual 1:1 pairing. One confession a week, shared both ways. Unpair any time." />
      {st.partners.length ? (
        <>
          <div className="big-tile rv in"><div><span className="eyebrow on-dark">This week, you and {st.partners[0].name}</span><h3 style={{ marginTop: 8 }}>{shared.title}</h3><p>“{shared.medium}”</p><p className="small" style={{ marginTop: 8, color: "var(--tint2)" }}>Paired since {new Date(st.partners[0].since).toLocaleDateString()} · code {st.partners[0].code}</p></div><div style={{ display: "flex", gap: 10 }}><PlayBtn slug={shared.slug} /><button className="btn btn-ghost on-dark" onClick={() => { mutate((s) => { s.partners = []; }); toast("Unpaired"); }}>Unpair</button></div></div>
          <div className="form-card rv in" style={{ marginTop: 16, maxWidth: 640 }}><p className="small">Sharing is double opt-in: your partner sees only the weekly confession and never your history, journal or private words.</p></div>
        </>
      ) : (
        <form className="form-card rv in" style={{ maxWidth: 560 }} onSubmit={pair}>
          <div className="field"><label>Partner's first name</label><input value={name} onChange={(e) => setName(e.target.value)} required maxLength={30} /></div>
          <div className="field"><label>Their pairing code (they get one when they accept)</label><input placeholder="IC-XXXXX" pattern="IC-[A-Z0-9]{5}" required /></div>
          <button className="btn btn-primary btn-lg" type="submit" style={{ marginTop: 16 }}>Pair up</button>
        </form>
      )}
    </>
  );
}

function FamilyApp() {
  const st = useApp(); const toast = useToast();
  const [nm, setNm] = useState(""); const [role, setRole] = useState("guardian"); const [band, setBand] = useState("16+");
  if (st.user?.plan === "free") return <><Head title="Family space" /><EmptyState icon="lock" title="A Premium space" sub="Two guardians plus children, an age-appropriate library and one shared family routine." cta={<Link className="btn btn-primary" href="/premium">See Premium</Link>} /></>;
  const suggest = (b: string) => b === "Under 10" ? ["children", "family", "gratitude", "joy", "hope"] : b === "11–15" ? ["identity", "confidence", "purpose", "relationships", "discipline"] : [];
  return (
    <>
      <Head title="Family space" sub="Guardians and kids. Age-appropriate words, one shared routine, nothing tracked on children." />
      <form className="form-card rv in" style={{ maxWidth: 640 }} onSubmit={(e) => { e.preventDefault(); mutate((s) => { s.family.push({ id: Date.now(), name: nm.trim(), role, band }); }); toast(nm + " added"); setNm(""); }}>
        <div className="field"><label>Name</label><input value={nm} onChange={(e) => setNm(e.target.value)} required maxLength={30} /></div>
        <div className="field"><label>Role</label><div className="chip-row">{["guardian", "child"].map((r) => <button type="button" className="chip" key={r} aria-pressed={role === r} onClick={() => setRole(r)}>{r}</button>)}</div></div>
        <div className="field"><label>Age band</label><div className="chip-row">{["Under 10", "11–15", "16+"].map((b) => <button type="button" className="chip" key={b} aria-pressed={band === b} onClick={() => setBand(b)}>{b}</button>)}</div></div>
        <button className="btn btn-primary btn-lg" type="submit" style={{ marginTop: 14 }}>Add member</button>
      </form>
      {st.family.map((f) => (
        <div className="form-card rv in" key={f.id} style={{ maxWidth: 640, marginTop: 14 }}>
          <div style={{ display: "flex", justifyContent: "space-between" }}><div><h4>{f.name}</h4><p className="small">{f.role} · {f.band}</p></div><button className="icon-btn" aria-label={"Remove " + f.name} onClick={() => mutate((s) => { s.family = s.family.filter((x) => x.id !== f.id); })}><Icon n="x" s={12} /></button></div>
          {suggest(f.band).length > 0 && <><p className="small" style={{ marginTop: 10 }}>Suggested library:</p><div className="chip-row" style={{ marginTop: 6 }}>{suggest(f.band).map((c) => <Link className="chip" key={c} href={`/categories/${c}`}>{catBySlug(c)?.name}</Link>)}</div></>}
        </div>
      ))}
    </>
  );
}

function LanguagesApp() {
  const st = useApp(); const toast = useToast();
  const join = (lang: string) => { mutate((s) => { const n = s.settings.notifications as any; n.langWait = (n.langWait || []).includes(lang) ? n.langWait : [...(n.langWait || []), lang]; }); toast("You're on the " + lang + " reviewer waitlist"); };
  return (
    <>
      <Head title="Languages" sub="English today. First-language sets are reviewed by native speakers before they ship — never machine-published." />
      <div className="card-row snap">
        <div className="tile"><span className="t-meta">Live</span><h4>English</h4><p>39 categories · 78 confessions · Grace narration.</p></div>
        {[["Yoruba", "Ìtẹ̀wọ́gbà"], ["Igbo", "Nkwupụta"], ["Hausa", "Furci"]].map(([l, native]) => (
          <div className="tile" key={l} style={{ opacity: .8 }}><span className="t-meta">In review</span><h4>{l} · {native}</h4><p>Native-speaker review in progress. No ship date until it's honest.</p><button className="btn btn-ghost btn-sm" style={{ marginTop: 8 }} onClick={() => join(l)}>Join reviewer waitlist</button></div>
        ))}
      </div>
      <div className="form-card rv in" style={{ maxWidth: 640, marginTop: 16 }}><p className="small">Waitlisted: {((st.settings.notifications as any).langWait || []).join(", ") || "none yet"}. Reviewers read drafts against the source text and record every decision.</p></div>
    </>
  );
}

function WidgetApp() {
  const st = useApp(); const t = CONFESSIONS[new Date().getDate() % CONFESSIONS.length];
  const [canInstall, setCanInstall] = useState(false);
  const evtRef = React.useRef<any>(null);
  React.useEffect(() => {
    const h = (e: any) => { e.preventDefault(); evtRef.current = e; setCanInstall(true); };
    window.addEventListener("beforeinstallprompt", h);
    return () => window.removeEventListener("beforeinstallprompt", h);
  }, []);
  return (
    <>
      <Head title="Home-screen widget" sub="Today's one line and a play button, one tap from your phone. The web equivalent ships now; native widgets land with the mobile beta." />
      <div className="widget-mock rv in" aria-label="Widget preview">
        <span className="small" style={{ color: "var(--tint2)" }}>iCONFESS · today</span>
        <h4 style={{ color: "#fff", marginTop: 6 }}>{t.title}</h4>
        <p className="small" style={{ color: "var(--tint2)", marginTop: 4 }}>“{t.short}”</p>
        <span className="orb" style={{ marginTop: 12 }} aria-hidden="true"><Icon n="play" s={14} /></span>
      </div>
      <div className="card-row snap" style={{ marginTop: 16 }}>
        <div className="tile"><h4>Android</h4><p>Menu → “Add to Home screen”. The widget appears in your widget picker after install.</p></div>
        <div className="tile"><h4>iPhone</h4><p>Share → “Add to Home Screen”. iOS widgets arrive with the TestFlight beta.</p></div>
        <div className="tile"><h4>Install now</h4><p>{canInstall ? "Your browser offered an install — one tap." : "Not offered yet — use your browser's install action."}</p>{canInstall && <button className="btn btn-primary btn-sm" style={{ marginTop: 8 }} onClick={() => evtRef.current?.prompt()}>Install app</button>}</div>
      </div>
    </>
  );
}

function PackCard({ id, title, desc, cats }: { id: string; title: string; desc: string; cats: string[] }) {
  const st = useApp(); const toast = useToast();
  const on = st.downloads.includes("pack:" + id);
  return (
    <div className="tile"><span className="t-meta">Pack</span><h4>{title}</h4><p>{desc}</p>
      <div style={{ display: "flex", gap: 10, marginTop: 10, flexWrap: "wrap" }}><MixBtn cats={cats} label="Queue pack" />
      <button className="btn btn-ghost btn-sm" onClick={() => { if (st.user?.plan === "free") { toast("Mixtapes are a Premium feature"); return; } mutate((x) => { x.downloads = on ? x.downloads.filter((d) => d !== "pack:" + id) : [...x.downloads, "pack:" + id]; }); toast(on ? "Pack removed" : "Pack saved offline"); }}>{on ? "Saved ✓" : "Save offline"}</button></div>
    </div>
  );
}

function HandoffCard() {
  const audio = useAudio(); const toast = useToast();
  const [ho, setHo] = useState<{ slugs: string[]; idx: number } | null>(null);
  useEffect(() => { try { const raw = localStorage.getItem("ic-handoff"); if (raw) setHo(JSON.parse(raw)); } catch { } }, []);
  if (!ho || !ho.slugs.length) return null;
  return (
    <div className="app-section" style={{ marginTop: 12 }}>
      <div className="list-row"><div className="lr-main"><h4>Continue from another device</h4><p>{ho.slugs.length} items in the handed-off queue</p></div>
        <div style={{ display: "flex", gap: 8 }}>
          <button className="btn btn-primary btn-sm" onClick={() => { const q = ho.slugs.map((sl) => confBySlug(sl)).filter(Boolean).map((c) => queueItem(c!)); if (q.length) { audio.play(q, Math.min(ho.idx, q.length - 1)); localStorage.removeItem("ic-handoff"); setHo(null); toast("Resumed from handoff"); } }}><Icon n="play" s={12} /> Resume</button>
          <button className="icon-btn" aria-label="Dismiss handoff" onClick={() => { localStorage.removeItem("ic-handoff"); setHo(null); }}><Icon n="x" s={12} /></button>
        </div></div>
    </div>
  );
}

function DataSaverSwitch() {
  const st = useApp();
  useEffect(() => { if (st.settings.dataSaver) document.documentElement.setAttribute("data-saver", "on"); else document.documentElement.removeAttribute("data-saver"); }, [st.settings.dataSaver]);
  return <Switch title="Data saver" sub="Text and voice only — hides imagery to save data on slow networks" path="dataSaver" />;
}

function WrappedApp() {
  const st = useApp(); const toast = useToast();
  const cnt: Record<string, number> = {}; const catCnt: Record<string, number> = {};
  st.history.forEach((h) => { cnt[h.slug] = (cnt[h.slug] || 0) + 1; catCnt[h.category] = (catCnt[h.category] || 0) + 1; });
  const topCats = Object.entries(catCnt).sort((a, b) => b[1] - a[1]).slice(0, 3);
  const top = Object.entries(cnt).sort((a, b) => b[1] - a[1])[0];
  const days = [...new Set(st.history.map((h) => new Date(h.at).toDateString()))].length;
  const mins = st.history.length * 2;
  const summary = `My Year in Words — ${st.history.length} returns across ${days} days (~${mins} min). Top areas: ${topCats.map(([c]) => c).join(", ") || "—"}. Most returned: ${top ? confBySlug(top[0])?.title : "—"}.`;
  return (
    <>
      <Head title="Year in Words" sub="Your private annual review. Sharing is opt-in, one tap, plain text." />
      <div className="wrapped-card rv in">
        <span className="eyebrow on-dark">Year in Words</span>
        <div className="wrap-grid">
          <div><b>{st.history.length}</b><span>returns</span></div>
          <div><b>{days}</b><span>days practised</span></div>
          <div><b>~{mins}</b><span>honest minutes</span></div>
          <div><b>{st.journals.length}</b><span>journal entries</span></div>
        </div>
        {topCats.length > 0 && <p style={{ color: "var(--tint1)", marginTop: 18 }}>You returned most to <b style={{ color: "#fff" }}>{topCats.map(([c]) => c).join(", ")}</b>.</p>}
        {top && <p style={{ color: "var(--tint2)", marginTop: 8 }}>Your most-returned confession: “{confBySlug(top[0])?.title}”.</p>}
        <div style={{ display: "flex", gap: 10, marginTop: 22, flexWrap: "wrap" }}>
          <button className="btn btn-light" onClick={() => { (navigator.clipboard?.writeText(summary) || Promise.reject()).then(() => toast("Summary copied — share only if you want to")).catch(() => toast(summary)); }}>Copy share summary</button>
          <Link className="btn btn-ghost on-dark" href="/app/streaks">Returns map</Link>
        </div>
      </div>
    </>
  );
}


/* ---------- Audio + Voice Engine: in-app voice browser (§12–§15) ---------- */
function AppVoices() {
  const [q, setQ] = useState("");
  const [region, setRegion] = useState("");
  const [free, setFree] = useState(false);
  const [voices, setVoices] = useState<ReturnType<typeof effectiveVoices>>(VOICE_CATALOG);
  useEffect(() => { setVoices(effectiveVoices()); }, []);
  const pool = voices.filter((v) => v.status !== "SUSPENDED" && v.rightsStatus !== "REVOKED");
  const shown = searchVoices(pool, q, { region: region || undefined, free: free || undefined });
  return <>
    <Head title="Voices" sub="Curated, licensed, rights-tracked narration" />
    <div className="form-card" style={{ maxWidth: 760, marginBottom: 20 }}>
      <input type="search" aria-label="Search voices" placeholder="Search: Nigerian · Pidgin · calm · preacher · British…" value={q} onChange={(e) => setQ(e.target.value)} style={{ width: "100%" }} />
      <div className="chip-row" style={{ marginTop: 10 }}>
        {["", "Nigeria", "West Africa", "Europe", "North America", "Global"].map((r) => <button key={r || "all"} className="chip" aria-pressed={region === r} onClick={() => setRegion(r)}>{r || "All regions"}</button>)}
        <button className="chip" aria-pressed={free} onClick={() => setFree(!free)}>Free</button>
      </div>
    </div>
    {shown.length ? <div className="voice-grid vc-grid">{shown.map((v) => <VoiceCard key={v.id} v={v} />)}</div> : <EmptyState icon="mic" title="No voices match" sub="Clear a filter, or check back as new voices clear rights review." />}
    <div className="form-card" style={{ maxWidth: 760, marginTop: 24 }}><SoundPanel /></div>
  </>;
}
function AppVoiceDetail({ slug }: { slug: string }) {
  const st = useApp(); const toast = useToast();
  const [v, setV] = useState<ReturnType<typeof effectiveVoices>[number] | null>(() => VOICE_CATALOG.find((x) => x.slug === slug) || null);
  useEffect(() => { setV(effectiveVoices().find((x) => x.slug === slug) || null); }, [slug]);
  if (!v) return <EmptyState icon="mic" title="Voice not found" sub="It may have been retired — rights first, always." cta={<Link className="btn btn-primary" href="/app/voices">All voices</Link>} />;
  const avail = voiceAvailable(v);
  const cats = v.categories.map(catBySlug).filter(Boolean);
  const featured = cats.flatMap((c) => CONFESSIONS.filter((x) => x.category === c!.name).slice(0, 2)).slice(0, 4);
  return <>
    <Link className="textlink" href="/app/voices"><Icon n="arrowL" s={12} /> Voices</Link>
    <div className="form-card rv in" style={{ marginTop: 12, display: "flex", gap: 18, alignItems: "center", flexWrap: "wrap" }}>
      <VoiceOrb v={v} size={92} />
      <div style={{ flex: 1, minWidth: 220 }}>
        <h3 style={{ margin: 0 }}>{v.displayName}</h3>
        <p className="small">{v.description}</p>
        <div className="chip-row" style={{ marginTop: 8 }}>{v.styles.map((x) => <span className="chip" key={x} style={{ pointerEvents: "none" }}>{x}</span>)}</div>
      </div>
      <div style={{ display: "flex", gap: 8 }}>
        <VoicePreview v={v} />
        <button type="button" className="btn btn-primary btn-sm" onClick={() => {
          if (!avail) { toast(v.displayName + " is not in production yet"); return; }
          if (v.premium && st.user?.plan === "free") { toast("Premium voices unlock with a plan"); return; }
          mutate((x) => { x.settings.voiceId = v.slug; }); logAE("voice_selected", { voiceId: v.id }); toast("Narration voice: " + v.displayName);
        }}>Use this voice</button>
      </div>
    </div>
    {featured.length > 0 && <div className="app-section"><div className="section-head"><h3>Curated for</h3></div><div className="grid4 snap">{featured.map((c) => <ConfTile key={c.slug} c={c} />)}</div></div>}
  </>;
}
