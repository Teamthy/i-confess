/* iCONFESS Audio + Voice Engine — provider abstraction (§07), preprocessor (§09),
   speech markup (§10), generation jobs (§47–§49), cache keys (§50/§102), events (§69).
   On the web today, the live provider is the device speech engine (InternalSpeechProvider);
   cloud providers are wired as adapters behind the same interface and activate
   the moment credentials + backend ship — no product code changes. */
import type { CatalogVoice } from "./voices";
import { effectiveVoices, voiceAvailable } from "./voices";

/* ---------- §09 pronunciation dictionary (admin-extensible) ---------- */
export const PRONUNCIATION: { word: string; pronunciation: string; locale: string }[] = [
  { word: "iCONFESS", pronunciation: "eye confess", locale: "en" },
  { word: "iConfess", pronunciation: "eye confess", locale: "en" },
  { word: "Selah", pronunciation: "seh-lah", locale: "en" },
  { word: "Jehovah", pronunciation: "jeh-HO-vah", locale: "en" },
  { word: "Adaeze", pronunciation: "ah-DIE-eh-zeh", locale: "en-NG" },
  { word: "Emeka", pronunciation: "eh-MEH-kah", locale: "en-NG" },
];
const SCRIPTURE_RE = /\b((?:[1-3]\s)?[A-Z][a-z]+)\s(\d{1,3}):(\d{1,3})\b/g;
export function preprocessSpeech(text: string): string {
  let t = text.replace(/\s+/g, " ").trim();
  t = t.replace(SCRIPTURE_RE, (_m, b, c, v) => `${b} chapter ${c} verse ${v}`);
  for (const p of PRONUNCIATION) t = t.split(p.word).join(p.pronunciation);
  return t;
}
/* §10 internal speech markup → plain utterance chunks (provider-specific markup stays in adapters) */
export function toChunks(text: string): string[] {
  return preprocessSpeech(text).split(/(?<=[.!?])\s+/).map((s) => s.trim()).filter(Boolean);
}

/* ---------- §07 provider abstraction ---------- */
export type ProviderCapabilities = { streaming: boolean; ssml: boolean; wordTimestamps: boolean; locales: string[]; styles: boolean };
export interface VoiceProvider {
  name: string;
  getCapabilities(): ProviderCapabilities;
  listVoices(): CatalogVoice[];
  validateVoice(v: CatalogVoice): { ok: boolean; reason?: string };
  resolve(v: CatalogVoice): ResolvedVoice | null;
  generateSpeech?(): never; /* cloud generation arrives with backend; see AUDIO_ENGINE.md */
}
export type ResolvedVoice = { synthVoice: SpeechSynthesisVoice | null; rateMul: number; pitchMul: number; note: string };

class InternalSpeechProvider implements VoiceProvider {
  name = "internal";
  getCapabilities(): ProviderCapabilities { return { streaming: true, ssml: false, wordTimestamps: false, locales: ["en", "en-NG", "en-GB", "en-US", "fr", "de", "es", "it"], styles: false }; }
  listVoices() { return effectiveVoices(); }
  validateVoice(v: CatalogVoice) {
    if (v.rightsStatus === "REVOKED") return { ok: false, reason: "Voice rights revoked — generation stopped." };
    if (v.rightsStatus === "EXPIRED") return { ok: false, reason: "Voice licence expired — removed from generation." };
    if (!voiceAvailable(v)) return { ok: false, reason: `Voice is in ${v.status} / rights ${v.rightsStatus} — not production-ready.` };
    return { ok: true };
  }
  resolve(v: CatalogVoice): ResolvedVoice | null {
    if (typeof window === "undefined" || !window.speechSynthesis) return null;
    const all = window.speechSynthesis.getVoices();
    const base = v.locale.split("-")[0];
    const isFem = v.genderPresentation === "female";
    const femRe = /female|zira|samantha|aria|moira|tessa|karen|victoria|fiona|google uk english female|amelie|anna/i;
    const maleRe = /male|david|mark|daniel|george|alex|fred|google uk english male|thomas|nicolas/i;
    const pool = all.filter((x) => x.lang.toLowerCase().startsWith(base.toLowerCase()));
    const want = pool.find((x) => (isFem ? femRe : maleRe).test(x.name)) ||
      (base === "en-NG" ? all.find((x) => x.lang.toLowerCase().startsWith("en-gb") && (isFem ? femRe : maleRe).test(x.name)) : null) ||
      pool[0] || all.find((x) => x.lang.toLowerCase().startsWith("en")) || all[0] || null;
    /* distinctiveness via style profile until studio masters ship */
    const sp = v.styleProfile;
    const rateMul = 0.86 + sp.pace / 250;            /* 0.86–1.26 */
    const pitchMul = 0.9 + sp.depth / 500 + (isFem ? 0.12 : 0) - (sp.warm - 60) / 800;
    const note = want ? `Rendered on-device · ${want.name}` : "Rendered on-device · default system voice";
    return { synthVoice: want, rateMul, pitchMul, note };
  }
}
class CloudProviderStub implements VoiceProvider {
  constructor(public name: string) { }
  getCapabilities(): ProviderCapabilities { return { streaming: true, ssml: true, wordTimestamps: true, locales: [], styles: true }; }
  listVoices() { return []; }
  validateVoice() { return { ok: false, reason: `${this.name} provider not configured in this environment.` }; }
  resolve() { return null; }
}
export const PROVIDERS: VoiceProvider[] = [new InternalSpeechProvider(), new CloudProviderStub("elevenlabs"), new CloudProviderStub("openai"), new CloudProviderStub("google"), new CloudProviderStub("azure")];
export const getProvider = (name: string) => PROVIDERS.find((p) => p.name === name) || PROVIDERS[0];
export function resolveVoice(idOrSlug: string): { voice: CatalogVoice; resolved: ResolvedVoice | null; gate: { ok: boolean; reason?: string } } {
  const v = effectiveVoices().find((x) => x.id === idOrSlug || x.slug === idOrSlug) || effectiveVoices()[0];
  const provider = getProvider(v.provider);
  const gate = provider.validateVoice(v);
  return { voice: v, resolved: gate.ok ? provider.resolve(v) : null, gate };
}

/* ---------- §50/§102 cache keys ---------- */
export function hashKey(s: string): string { let h = 5381; for (let i = 0; i < s.length; i++) h = ((h << 5) + h + s.charCodeAt(i)) >>> 0; return h.toString(36); }
export function audioCacheKey(o: { contentVersion: number; voiceId: string; voiceVersion: number; locale: string; rate: number; style: string }) {
  return hashKey([o.contentVersion, o.voiceId, o.voiceVersion, o.locale, o.rate, o.style].join("|"));
}

/* ---------- §47–§49 generation jobs (simulated worker in the web demo; Redis+worker in production) ---------- */
export type JobStatus = "QUEUED" | "PROCESSING" | "MASTERING" | "ENCODING" | "READY" | "FAILED" | "CANCELLED";
export type AudioJob = { id: string; cacheKey: string; contentId: string; contentTitle: string; voiceId: string; voiceName: string; locale: string; provider: string; outputFormat: string; status: JobStatus; error?: string; createdAt: number; completedAt?: number; correlationId: string };
const JOB_KEY = "ic-ae-jobs";
export function loadJobs(): AudioJob[] { if (typeof window === "undefined") return []; try { return JSON.parse(localStorage.getItem(JOB_KEY) || "[]"); } catch { return []; } }
function saveJobs(j: AudioJob[]) { try { localStorage.setItem(JOB_KEY, JSON.stringify(j.slice(-40))); } catch { } }
export function createJob(contentId: string, contentTitle: string, voice: CatalogVoice): { job: AudioJob; reused: boolean } {
  const cacheKey = audioCacheKey({ contentVersion: 1, voiceId: voice.id, voiceVersion: voice.version, locale: voice.locale, rate: 1, style: "default" });
  const jobs = loadJobs();
  const hit = jobs.find((j) => j.cacheKey === cacheKey && j.status === "READY");
  if (hit) { logAE("audio_cache_hit", { voiceId: voice.id }); return { job: hit, reused: true }; }  /* §88 idempotent */
  const job: AudioJob = { id: "job_" + hashKey(cacheKey + Date.now()), cacheKey, contentId, contentTitle, voiceId: voice.id, voiceName: voice.displayName, locale: voice.locale, provider: "internal", outputFormat: "web.opus", status: "QUEUED", createdAt: Date.now(), correlationId: "corr_" + hashKey(String(Date.now()) + Math.random()) };
  saveJobs([job, ...jobs]); logAE("generation_queued", { voiceId: voice.id });
  const flow: [JobStatus, number][] = [["PROCESSING", 900], ["MASTERING", 900], ["ENCODING", 800], ["READY", 600]];
  let i = 0;
  const step = () => {
    if (i >= flow.length) return;
    const [status, wait] = flow[i++];
    window.setTimeout(() => {
      const cur = loadJobs().map((j) => (j.id === job.id ? { ...j, status, ...(status === "READY" ? { completedAt: Date.now() } : {}) } : j));
      saveJobs(cur); logAE("generation_status", { job: job.id, status });
      step();
    }, wait);
  };
  step();
  return { job, reused: false };
}

/* ---------- §69 analytics events (never carries private content text) ---------- */
export type AEEvent = { name: string; props: Record<string, unknown>; at: number };
const EV_KEY = "ic-ae-events";
export function logAE(name: string, props: Record<string, unknown> = {}) {
  try {
    const evs: AEEvent[] = JSON.parse(localStorage.getItem(EV_KEY) || "[]");
    evs.push({ name, props, at: Date.now() });
    localStorage.setItem(EV_KEY, JSON.stringify(evs.slice(-200)));
  } catch { }
}
export function readAE(): AEEvent[] { if (typeof window === "undefined") return []; try { return JSON.parse(localStorage.getItem(EV_KEY) || "[]"); } catch { return []; } }
