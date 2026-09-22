#!/usr/bin/env node
// scripts/dev-api.mjs — the local API FIXTURE, not the product API.
//
// The development sandbox cannot run Go (no toolchain; module proxy
// unreachable), so the web app previews point IC_API_URL at this server. It
// serves the same shapes the Go handlers serve, reading the SAME seed of
// record: scripts/dev-api-corpus.py extracts /tmp/cats.json and
// /tmp/confs.json from server/internal/seed/*_canonical.go (run
// automatically on first start if the files are missing).
//
// Fixture rules:
//  - Content (categories, confessions, voices) is the real canonical seed.
//    Nothing here invents catalogue data.
//  - Accounts, sessions, favourites and community posts live in memory and
//    reset with the process. They are test state, clearly not durable.
//  - "Audio" is generated silence: signed URLs point at /media/* so the real
//    player code paths (src assignment, timeupdate, ended) execute.
//  - Auth is deliberately permissive but explicit: any well-formed email with
//    password "devpassword" signs in as one shared fixture account. The Go
//    server enforces everything for real; never ship this.
//
// Start:  node scripts/dev-api.mjs [--port 8081]
// Wire:   IC_API_URL=http://127.0.0.1:8081 npm run dev (apps/web)

import { spawnSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import http from "node:http";
import { randomUUID } from "node:crypto";
import path from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const TMP = "/tmp";
const PORT = Number(process.env.IC_FIXTURE_PORT || argVal("--port") || 8081);

function argVal(flag) {
  const i = process.argv.indexOf(flag);
  return i > -1 ? process.argv[i + 1] : null;
}

// ---------------------------------------------------------------- corpus

function loadCorpus() {
  const catsPath = path.join(TMP, "cats.json");
  const confsPath = path.join(TMP, "confs.json");
  if (!existsSync(catsPath) || !existsSync(confsPath)) {
    const r = spawnSync("python3", [path.join(ROOT, "scripts", "dev-api-corpus.py")], { stdio: "inherit" });
    if (r.status !== 0) {
      console.error("fixture: could not extract the corpus from the Go seed");
      process.exit(1);
    }
  }
  const cats = JSON.parse(readFileSync(catsPath, "utf8"));
  const confs = JSON.parse(readFileSync(confsPath, "utf8"));
  return { cats, confs };
}

const { cats: RAW_CATS, confs: RAW_CONFS } = loadCorpus();

// Mirror the API projections exactly: categories carry the fields
// cachedListCategories serves; confessions the fields GET /confessions/{id}
// serves, with variants sized by word count (~150 wpm reading pace).
const CATS = RAW_CATS.map((c, i) => ({
  id: `cat-${c.slug}`,
  name: c.name,
  slug: c.slug,
  description: c.description,
  icon: c.icon,
  premium: false,
  status: "published",
  sort_order: i,
  updated_at: "2026-01-01T00:00:00Z",
}));
const CAT_BY_NAME = new Map(RAW_CATS.map((c, i) => [c.name, CATS[i]]));
const WORDS = (t) => Math.max(1, t.trim().split(/\s+/).length);
const SECS = (t) => Math.max(20, Math.round((WORDS(t) / 150) * 60));
const CONFS = RAW_CONFS.map((f, i) => {
  const cat = CAT_BY_NAME.get(f.category);
  return {
    id: `conf-${i + 1}`,
    category_id: cat ? cat.id : "cat-unknown",
    title: f.title,
    short_text: f.short,
    medium_text: f.medium,
    long_text: f.long,
    tags: [],
    intensity: f.intensity,
    language: "en",
    status: "published",
    author: "Canonical corpus",
    version: 1,
    published_at: "2026-01-01T00:00:00Z",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    variants: [
      { id: `v-${i + 1}-s`, label: "short", duration_seconds: SECS(f.short) },
      { id: `v-${i + 1}-m`, label: "medium", duration_seconds: SECS(f.medium) },
      { id: `v-${i + 1}-l`, label: "long", duration_seconds: SECS(f.long) },
    ],
    scriptures: f.scriptures.map((s, j) => ({ id: `scr-${i}-${j}`, ...s })),
  };
});
const CONFS_BY_CAT = new Map();
for (const c of CONFS) {
  if (!CONFS_BY_CAT.has(c.category_id)) CONFS_BY_CAT.set(c.category_id, []);
  CONFS_BY_CAT.get(c.category_id).push(c);
}

// Mirrors the three canonical voices seeded by internal/seed/ensure.go.
const VOICES = [
  { id: "voice-grace", name: "Grace", description: "Warm, calm professional narration voice.", type: "professional", provider: "i-confess", gender: "female", language: "en", premium: false, status: "active", sample_url: "/media/sample.wav", created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
  { id: "voice-david", name: "David", description: "Clear, grounded and reflective voice.", type: "professional", provider: "i-confess", gender: "male", language: "en", premium: false, status: "active", sample_url: "/media/sample.wav", created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
  { id: "voice-faith", name: "Faith", description: "Uplifting, resonant expressive voice.", type: "professional", provider: "i-confess", gender: "female", language: "en", premium: true, status: "active", sample_url: "/media/sample.wav", created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z" },
];

// ------------------------------------------------------------- app state

const DEV_TOKEN = "dev-token-" + "fixture";
const NOW = () => new Date().toISOString();

const state = {
  user: { id: "user-dev", email: "dev@iconfess.test", display_name: "Dev Listener", timezone: "UTC", status: "active", email_verified: true },
  profile: {
    id: "profile-dev", user_id: "user-dev", display_name: "Dev Listener", username: "devlistener",
    bio: "", avatar_url: "", timezone: "UTC", locale: "en", country_code: "", language: "en",
    created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
  },
  prefs: {
    id: "prefs-dev", user_id: "user-dev", default_duration: 900, default_voice_id: "", autoplay: true,
    preferred_quality: "standard", download_over_wifi: true, notifications_enabled: true,
    recommendations_enabled: true, personalization_enabled: true, language: "en", theme: "system",
    updated_at: "2026-01-01T00:00:00Z",
  },
  notifPrefs: { user_id: "user-dev", scheduled_sessions: true, new_content: true, recommendations: true, product_updates: false, updated_at: "2026-01-01T00:00:00Z" },
  sessions: new Map(), // id -> session
  favorites: new Map(), // `${type}:${id}` -> {id, entity_type, entity_id, created_at}
  history: [],
  communityPosts: [
    { id: "post-fixture-1", body: "Fixture post — this string exists only in the local dev API so the feed UI has something to render.", visibility: "public", status: "published", created_at: "2026-09-01T08:00:00Z" },
  ],
};

// -------------------------------------------------------------- helpers

function json(res, status, body) {
  const text = JSON.stringify(body);
  res.writeHead(status, { "content-type": "application/json", "cache-control": "no-store" });
  res.end(text);
}

function readBody(req) {
  return new Promise((resolve) => {
    let data = "";
    req.on("data", (c) => (data += c));
    req.on("end", () => {
      try { resolve(data ? JSON.parse(data) : {}); } catch { resolve(null); }
    });
  });
}

function authed(req) {
  const h = req.headers.authorization || "";
  return h === `Bearer ${DEV_TOKEN}`;
}

// Silent 2-second 16-bit PCM WAV. The <audio> element behaves identically to
// real content for every event the player wires (loadedmetadata, timeupdate,
// ended), which is the point of a fixture: exercise the plumbing, fake only
// the bytes.
function silentWav(seconds = 2, rate = 8000) {
  const n = seconds * rate;
  const buf = Buffer.alloc(44 + n * 2);
  buf.write("RIFF", 0); buf.writeUInt32LE(36 + n * 2, 4); buf.write("WAVE", 8);
  buf.write("fmt ", 12); buf.writeUInt32LE(16, 16); buf.writeUInt16LE(1, 20);
  buf.writeUInt16LE(1, 22); buf.writeUInt32LE(rate, 24); buf.writeUInt32LE(rate * 2, 28);
  buf.writeUInt16LE(2, 32); buf.writeUInt16LE(16, 34);
  buf.write("data", 36); buf.writeUInt32LE(n * 2, 40);
  return buf;
}
const SAMPLE_WAV = silentWav(2);

function signKey(key) {
  const exp = Date.now() + 15 * 60_000;
  return `/media/${encodeURIComponent(key)}?token=${DEV_TOKEN}&exp=${exp}`;
}

function buildSession(body) {
  const cats = (body.category_ids || []).filter(Boolean);
  const target = Math.max(60, Math.min(Number(body.duration_seconds) || 900, 3 * 3600));
  const planCap = 15 * 60; // free — the fixture models a free account
  if (target > planCap) return { planLimit: true };
  const pool = [];
  for (const cid of cats) for (const c of CONFS_BY_CAT.get(cid) || []) pool.push(c);
  if (pool.length === 0) return { code: "CONTENT_UNAVAILABLE", reason: "no published content available for the selected categories and voice" };
  const items = [];
  let total = 0;
  for (const c of pool) {
    if (total >= target * 0.6) {
      // stop once at least 60% of the target is covered; items are never cut
      if (total >= target) break;
    }
    const variant = c.variants[1]; // medium, as the engine prefers near-target
    const dur = variant.duration_seconds;
    items.push({
      id: `qi-${randomUUID()}`, session_id: "", confession_id: c.id, variant_id: variant.id,
      voice_id: body.voice_id || "voice-grace", position: items.length + 1, duration_seconds: dur,
      status: "QUEUED", title: c.title, category: (CATS.find((x) => x.id === c.category_id) || {}).name,
      text: c.medium_text,
    });
    total += dur;
    if (total >= target) break;
  }
  return { items, total };
}

function sessionView(sess, { fresh = true } = {}) {
  const out = { ...sess };
  out.items = sess.items.map((it) => ({
    ...it,
    audio_url: fresh && !it.locked ? signKey(`${it.confession_id}/${it.variant_id}.wav`) : it.audio_url,
  }));
  return out;
}

// --------------------------------------------------------------- router

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url, "http://fixture");
  let p = url.pathname;
  if (p.startsWith("/v1/")) p = "/" + p.slice(4); // parity with the Go router
  const method = req.method;
  const body = method === "GET" || method === "DELETE" ? {} : await readBody(req);
  if (method !== "GET" && body === null) return json(res, 400, { error: "invalid request body" });

  // media (signed URL target) — silence for everyone, token ignored
  if (p.startsWith("/media/")) {
    res.writeHead(200, { "content-type": "audio/wav", "content-length": SAMPLE_WAV.length, "accept-ranges": "none" });
    return res.end(SAMPLE_WAV);
  }

  // ---- public content
  if (method === "GET" && p === "/categories") return json(res, 200, CATS);
  let m;
  if ((m = p.match(/^\/categories\/([^/]+)\/confessions$/)) && method === "GET") {
    const key = decodeURIComponent(m[1]);
    const cat = CATS.find((c) => c.id === key || c.slug === key);
    if (!cat) return json(res, 404, { error: "category not found" });
    return json(res, 200, CONFS_BY_CAT.get(cat.id) || []);
  }
  if ((m = p.match(/^\/confessions\/([^/]+)$/)) && method === "GET") {
    const c = CONFS.find((x) => x.id === m[1] || x.title === decodeURIComponent(m[1]));
    if (!c) return json(res, 404, { error: "confession not found" });
    return json(res, 200, c);
  }
  if (method === "GET" && p === "/voices") return json(res, 200, VOICES);
  if (method === "GET" && p === "/health/live") return json(res, 200, { ok: true, fixture: true });
  if (method === "GET" && p === "/community/feed") {
    return json(res, 200, { posts: state.communityPosts.filter((x) => x.status === "published").map(({ author_id, ...rest }) => rest) });
  }
  if (method === "GET" && p === "/community/confessions") return json(res, 200, { confessions: [] });
  if (method === "GET" && p === "/search") {
    const q = (url.searchParams.get("q") || "").toLowerCase();
    const results = [];
    for (const c of CONFS) if (!q || c.title.toLowerCase().includes(q)) results.push({ id: c.id, type: "confession", title: c.title, description: c.short_text });
    for (const c of CATS) if (!q || c.name.toLowerCase().includes(q)) results.push({ id: c.id, type: "category", title: c.name, description: c.description });
    return json(res, 200, { results: results.slice(0, 20), count: Math.min(results.length, 20) });
  }

  // ---- auth endpoints (fixture-strict on shape, lenient on credentials)
  if (method === "POST" && p === "/auth/login") {
    if (!body || !/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(String(body.email || ""))) {
      return json(res, 401, { error: "invalid credentials", code: "AUTH_INVALID_CREDENTIALS" });
    }
    return json(res, 200, { token: DEV_TOKEN, user: state.user });
  }
  if (method === "POST" && p === "/auth/register") {
    return json(res, 200, { token: DEV_TOKEN, user: { ...state.user, email: body?.email || state.user.email, display_name: body?.display_name || state.user.display_name } });
  }
  if (method === "POST" && p === "/auth/logout") return json(res, 200, { message: "logged out" });
  if (method === "POST" && p === "/auth/change-password") {
    if (String(body?.new_password || "").length < 8) return json(res, 400, { error: "password too short" });
    return json(res, 200, { message: "password changed; other sessions revoked" });
  }

  // ---- everything below requires the fixture token
  const needsAuth =
    p.startsWith("/me") || p.startsWith("/sessions") || p.startsWith("/home") ||
    p.startsWith("/entitlements") || p.startsWith("/subscription") ||
    p.startsWith("/recommendations") || p.startsWith("/schedules") ||
    (method === "POST" && p.startsWith("/community/"));
  if (needsAuth && !authed(req)) return json(res, 401, { error: "authentication required" });

  if (method === "GET" && p === "/me") return json(res, 200, { user: state.user, plan: "free" });
  if (method === "GET" && p === "/me/profile") return json(res, 200, state.profile);
  if (method === "PATCH" && p === "/me/profile") {
    const errs = [];
    for (const k of ["display_name", "username", "bio", "avatar_url", "timezone", "locale", "language", "country_code"]) {
      if (!(k in body)) continue;
      const v = body[k];
      if (k === "display_name" && String(v).length > 60) errs.push("display name must be 60 characters or fewer");
      if (k === "username" && ["admin", "root", "api"].includes(String(v))) return json(res, 400, { error: "that username is reserved", code: "PROFILE_INVALID" });
      if (k === "avatar_url" && v && !String(v).startsWith("https://")) errs.push("url must start with https://");
      if (k === "timezone" && String(v).trim() === "") errs.push("timezone is required");
    }
    if (errs.length) return json(res, 400, { error: errs[0], code: "PROFILE_INVALID" });
    Object.assign(state.profile, body, { updated_at: NOW() });
    return json(res, 200, state.profile);
  }
  if (method === "GET" && p === "/me/preferences") return json(res, 200, state.prefs);
  if (method === "PATCH" && p === "/me/preferences") {
    if ("preferred_quality" in body && !["low", "standard", "high"].includes(body.preferred_quality))
      return json(res, 400, { error: "preferred_quality must be low, standard or high", code: "PROFILE_INVALID" });
    if ("default_duration" in body && (body.default_duration < 60 || body.default_duration > 3 * 3600))
      return json(res, 400, { error: "default_duration must be between 60 seconds and 3 hours", code: "PROFILE_INVALID" });
    if (body.default_voice_id) {
      const v = VOICES.find((x) => x.id === body.default_voice_id);
      if (!v || v.status !== "active") return json(res, 422, { error: "that voice is not available", code: "VOICE_UNAVAILABLE" });
      if (v.premium) return json(res, 402, { error: "that voice is available on Premium", code: "ENTITLEMENT_REQUIRED" });
    }
    Object.assign(state.prefs, body, { updated_at: NOW() });
    return json(res, 200, state.prefs);
  }
  if (method === "GET" && p === "/me/notifications")
    return json(res, 200, { preferences: state.notifPrefs, note: "Security notifications are always sent and cannot be disabled." });
  if (method === "PATCH" && p === "/me/notifications") {
    for (const k of ["scheduled_sessions", "new_content", "recommendations", "product_updates"])
      if (k in body) state.notifPrefs[k] = Boolean(body[k]);
    state.notifPrefs.updated_at = NOW();
    return json(res, 200, state.notifPrefs);
  }
  if (method === "GET" && p === "/entitlements")
    return json(res, 200, { plan: "free", premium_voices: false, premium_content: false, offline_downloads: false, personal_confessions: true, max_session_seconds: 15 * 60, max_concurrent_downloads: 0, playback_ttl_seconds: 900 });
  if (method === "GET" && p === "/subscription")
    return json(res, 200, { plan: "free", status: "none", active: false, max_session_seconds: 15 * 60 });

  if (method === "GET" && p === "/home") {
    const recent = [...state.sessions.values()].sort((a, b) => (a.created_at < b.created_at ? 1 : -1));
    const resumable = recent.find((s) => ["ACTIVE", "PAUSED", "INTERRUPTED", "READY", "DRAFT"].includes(s.status)) || null;
    return json(res, 200, { continue: resumable, recent_sessions: recent.slice(0, 12), collections: [], categories: CATS });
  }
  if (method === "GET" && p === "/recommendations") {
    return json(res, 200, {
      categories: CATS.slice(0, 6),
      confessions: CONFS.slice(0, 4).map(({ variants, scriptures, ...rest }) => rest),
      count: 10,
      personalized: false,
      listen_again: [],
    });
  }

  if (method === "POST" && p === "/sessions") {
    const built = buildSession(body);
    if (built.planLimit)
      return json(res, 402, { error: "longer sessions are part of Premium", code: "PLAN_LIMIT", max_session_seconds: 15 * 60 });
    if (built.code)
      return json(res, 422, { error: built.reason, code: built.code, reason: "content_unavailable" });
    const id = `sess-${randomUUID()}`;
    const sess = {
      id, user_id: state.user.id, type: "LISTEN", duration_seconds: built.total, target_duration: Math.max(60, Math.min(Number(body.duration_seconds) || 900, 3 * 3600)),
      actual_duration: built.total, strategy: body.strategy || "BALANCED", title: body.title || "", description: body.description || "",
      voice_id: body.voice_id || "voice-grace", status: "DRAFT", created_at: NOW(), items: built.items.map((it) => ({ ...it, session_id: id })),
    };
    state.sessions.set(id, sess);
    return json(res, 201, sessionView(sess));
  }
  if ((m = p.match(/^\/sessions\/([^/]+)$/)) && method === "GET") {
    const sess = state.sessions.get(m[1]);
    if (!sess) return json(res, 404, { error: "session not found" });
    return json(res, 200, sessionView(sess));
  }
  if ((m = p.match(/^\/sessions\/([^/]+)\/(start|pause|resume|complete|interrupt)$/)) && method === "POST") {
    const sess = state.sessions.get(m[1]);
    if (!sess) return json(res, 404, { error: "session not found" });
    const verb = m[2];
    const next = { start: "ACTIVE", pause: "PAUSED", resume: "ACTIVE", complete: "COMPLETED", interrupt: "INTERRUPTED" }[verb];
    sess.status = next;
    if (verb === "start") sess.started_at = sess.started_at || NOW();
    if (verb === "complete") {
      sess.completed_at = NOW();
      state.history.unshift({ session_id: sess.id, confession_id: sess.items[0]?.confession_id || "", duration_seconds: sess.actual_duration, completed: true, skipped: false, at: NOW() });
    }
    if (verb === "start" || verb === "resume") {
      const q = sess.items.find((it) => it.status === "QUEUED" || it.status === "PLAYING");
      if (q) q.status = "PLAYING";
    }
    return json(res, 200, sessionView(sess, { fresh: false }));
  }
  if ((m = p.match(/^\/sessions\/([^/]+)\/progress$/)) && method === "POST") {
    const sess = state.sessions.get(m[1]);
    if (!sess) return json(res, 404, { error: "session not found" });
    let applied = false;
    if (body.queue_item_id) {
      const it = sess.items.find((x) => x.id === body.queue_item_id);
      if (!it) return json(res, 404, { error: "item is not in this session" });
      if (body.item_status) it.status = String(body.item_status).toUpperCase();
      applied = true;
    }
    return json(res, 200, { applied, progress: { session_id: sess.id, position_ms: Number(body.position_ms) || 0, queue_item_id: body.queue_item_id || "", last_updated_at: NOW() } });
  }
  if ((m = p.match(/^\/sessions\/([^/]+)\/skip$/)) && method === "POST") {
    const sess = state.sessions.get(m[1]);
    if (!sess) return json(res, 404, { error: "session not found" });
    const it = sess.items.find((x) => x.id === body.queue_item_id);
    if (it) it.status = "SKIPPED";
    return json(res, 200, { ok: true });
  }
  if (method === "GET" && p === "/sessions") return json(res, 200, [...state.sessions.values()].map((s) => sessionView(s, { fresh: false })));

  if (method === "GET" && p === "/me/favorites") return json(res, 200, [...state.favorites.values()]);
  if (method === "POST" && p === "/me/favorites") {
    const key = `${body.entity_type}:${body.entity_id}`;
    if (state.favorites.has(key)) return json(res, 409, { error: "already saved" });
    const fav = { id: `fav-${randomUUID()}`, entity_type: body.entity_type, entity_id: body.entity_id, created_at: NOW() };
    state.favorites.set(key, fav);
    return json(res, 201, fav);
  }
  if (method === "DELETE" && p === "/me/favorites") {
    state.favorites.delete(`${body.entity_type}:${body.entity_id}`);
    return json(res, 200, { ok: true });
  }
  if (method === "GET" && p === "/me/history") return json(res, 200, { items: state.history });

  if (method === "GET" && p === "/me/confessions") return json(res, 200, []);
  if (method === "POST" && p === "/me/confessions") {
    if (!body?.title || !body?.text) return json(res, 400, { error: "title and text are required" });
    const uc = {
      id: `uc-${randomUUID()}`, user_id: state.user.id, title: body.title, text: body.text,
      category_id: body.category_id || "", is_private: (body.visibility || "private") === "private",
      status: "draft", visibility: body.visibility || "private", created_at: NOW(),
    };
    state.userConfessions = state.userConfessions || [];
    state.userConfessions.push(uc);
    return json(res, 201, uc);
  }
  if ((m = p.match(/^\/me\/confessions\/([^/]+)\/submit$/)) && method === "POST") {
    const uc = (state.userConfessions || []).find((x) => x.id === m[1]);
    if (!uc) return json(res, 404, { error: "confession not found", code: "RESOURCE_NOT_FOUND" });
    uc.status = "submitted";
    return json(res, 200, uc);
  }

  if (method === "POST" && p === "/community/posts") {
    const b = String(body?.body || "");
    if (!b || b.length > 2000) return json(res, 400, { error: "body 1..2000 chars" });
    const vis = body?.visibility || "private";
    if (!["private", "shared", "public"].includes(vis)) return json(res, 400, { error: "invalid visibility" });
    const post = { id: `post-${randomUUID()}`, author_id: state.user.id, body: b, visibility: vis, status: vis === "private" ? "draft" : "submitted", created_at: NOW() };
    state.communityPosts.push(post);
    return json(res, 201, post);
  }
  if ((m = p.match(/^\/community\/posts\/([^/]+)\/react$/)) && method === "POST") {
    const r = body?.reaction;
    if (!["amen", "heart", "pray"].includes(r)) return json(res, 400, { error: "invalid reaction" });
    const post = state.communityPosts.find((x) => x.id === m[1]);
    if (!post) return json(res, 404, { error: "no such post" });
    return json(res, 200, { ok: true });
  }

  return json(res, 404, { error: "not found", fixture: "dev-api" });
});

server.listen(PORT, "0.0.0.0", () => {
  console.log(`iCONFESS dev-api FIXTURE on http://127.0.0.1:${PORT}`);
  console.log(`  corpus: ${CATS.length} categories, ${CONFS.length} confessions (from server/internal/seed)`);
  console.log(`  sign in with any email + password "devpassword" — this is test state, not accounts`);
});
