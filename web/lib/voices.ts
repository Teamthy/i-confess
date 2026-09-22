/* iCONFESS Audio + Voice Engine — domain model & curated voice catalog.
   Spec §02–§06, §11, §54, §56, §59, §63, §117–§118.
   All voices are original synthetic iCONFESS studio voices.
   No impersonation or cloning of any real, identifiable person. */

export type RightsStatus = "PENDING" | "APPROVED" | "RESTRICTED" | "EXPIRED" | "REVOKED";
export type VoiceStatus = "DRAFT" | "TESTING" | "QUALITY_REVIEW" | "RIGHTS_REVIEW" | "APPROVED" | "PRODUCTION" | "SUSPENDED";
export type StyleProfile = { warm: number; depth: number; energy: number; pace: number; presence: number };

export type CatalogVoice = {
  id: string;                    /* permanent internal ID, e.g. voice_ngr_female_001 */
  slug: string;
  displayName: string;
  internalName: string;
  provider: "internal" | "elevenlabs" | "openai" | "google" | "azure";
  providerVoiceId: string;
  locale: string;                /* en-NG, en-NG-PIDGIN, en-GB, fr-FR … */
  language: string;
  accent: string;
  region: "Nigeria" | "West Africa" | "Europe" | "North America" | "Global";
  genderPresentation: "female" | "male" | "neutral";
  ageRange: string;
  styles: string[];              /* PREACHER / SPEAKER style categories (§04) */
  tone: string;
  pace: "slow" | "measured" | "brisk";
  pitchRange: string;
  description: string;
  previewText: string;
  styleProfile: StyleProfile;    /* defaults §11 */
  status: VoiceStatus;           /* approval workflow §54 */
  rightsStatus: RightsStatus;    /* rights registry §06 */
  rights: {
    rightsOwner: string; licenseReference: string; usageScope: string;
    territories: string; platforms: string; commercialStatus: string;
    expirationDate: string | null; revocationDate: string | null;
  };
  categories: string[];          /* voiceCapabilities §117 — editorial curation */
  premium: boolean;
  version: number;               /* voiceVersion §51 */
  curated: "featured" | "editors" | "new" | "popular" | null;
  createdAt: string;
  updatedAt: string;
};

const R = (owner: string, ref: string, exp: string | null = null) => ({
  rightsOwner: owner, licenseReference: ref, usageScope: "In-app narration, previews, offline packs",
  territories: "Worldwide", platforms: "Web, iOS, Android", commercialStatus: "Approved for commercial use",
  expirationDate: exp, revocationDate: null,
});

export const VOICE_CATALOG: CatalogVoice[] = [
  {
    id: "voice_ngr_female_001", slug: "grace", displayName: "Grace", internalName: "ngr_grace_warm",
    provider: "internal", providerVoiceId: "internal.grace.v1", locale: "en-NG", language: "English", accent: "Nigerian English",
    region: "Nigeria", genderPresentation: "female", ageRange: "28–38", styles: ["Calm Speaker", "Reflective", "Mentor"],
    tone: "Warm", pace: "measured", pitchRange: "M3–A4", description: "Our founding voice. Warm, steady Nigerian English — built for morning confession and evening rest.",
    previewText: "Peace beyond understanding. Speak it, hear it, and let the words return to you.",
    styleProfile: { warm: 85, depth: 60, energy: 40, pace: 52, presence: 75 },
    status: "PRODUCTION", rightsStatus: "APPROVED", rights: R("iCONFESS Studio", "IC-VOICE-0001"),
    categories: ["peace", "rest", "hope", "forgiveness", "gratitude", "prayer", "family", "love"],
    premium: false, version: 1, curated: "featured", createdAt: "2026-01-12", updatedAt: "2026-01-12",
  },
  {
    id: "voice_ngr_male_001", slug: "emeka", displayName: "Emeka", internalName: "ngr_emeka_firm",
    provider: "internal", providerVoiceId: "internal.emeka.v1", locale: "en-NG", language: "English", accent: "Nigerian English",
    region: "Nigeria", genderPresentation: "male", ageRange: "32–45", styles: ["Authoritative", "Mentor", "Reflective"],
    tone: "Firm, grounded", pace: "measured", pitchRange: "G2–D4", description: "A grounded Nigerian baritone for confessions that need weight — purpose, discipline, direction.",
    previewText: "You were not built to drift. Speak the direction, then walk it.",
    styleProfile: { warm: 55, depth: 85, energy: 50, pace: 48, presence: 85 },
    status: "PRODUCTION", rightsStatus: "APPROVED", rights: R("iCONFESS Studio", "IC-VOICE-0002"),
    categories: ["purpose", "discipline", "direction", "leadership", "confidence", "career", "success"],
    premium: false, version: 1, curated: "featured", createdAt: "2026-02-03", updatedAt: "2026-02-03",
  },
  {
    id: "voice_ngr_female_002", slug: "adaeze", displayName: "Adaeze", internalName: "ngr_adaeze_pidgin",
    provider: "internal", providerVoiceId: "internal.adaeze.v1", locale: "en-NG-PIDGIN", language: "Nigerian Pidgin", accent: "Nigerian Pidgin",
    region: "Nigeria", genderPresentation: "female", ageRange: "25–35", styles: ["Gentle", "Conversational", "Warm"],
    tone: "Gentle, close", pace: "measured", pitchRange: "A3–C5", description: "Nigerian Pidgin, spoken the way it lives — for words that should feel like home, not a translation.",
    previewText: "Make you take small time breathe. God dey with you, e no dey far.",
    styleProfile: { warm: 90, depth: 50, energy: 45, pace: 50, presence: 70 },
    status: "PRODUCTION", rightsStatus: "APPROVED", rights: R("iCONFESS Studio", "IC-VOICE-0003"),
    categories: ["peace", "gratitude", "family", "love", "hope", "faith"],
    premium: false, version: 1, curated: "featured", createdAt: "2026-02-20", updatedAt: "2026-02-20",
  },
  {
    id: "voice_ngr_male_002", slug: "chidi", displayName: "Chidi", internalName: "ngr_chidi_pidgin",
    provider: "internal", providerVoiceId: "internal.chidi.v1", locale: "en-NG-PIDGIN", language: "Nigerian Pidgin", accent: "Nigerian Pidgin",
    region: "Nigeria", genderPresentation: "male", ageRange: "28–40", styles: ["Storyteller", "Conversational"],
    tone: "Easy, friendly", pace: "brisk", pitchRange: "A2–E4", description: "Pidgin storytelling voice — for testimonies and evening words that sit close.",
    previewText: "See, the story no finish. Wetin you go tell tomorrow, start am today.",
    styleProfile: { warm: 75, depth: 60, energy: 60, pace: 62, presence: 65 },
    status: "PRODUCTION", rightsStatus: "APPROVED", rights: R("iCONFESS Studio", "IC-VOICE-0004"),
    categories: ["testimony", "gratitude", "purpose", "faith", "family"],
    premium: false, version: 1, curated: "popular", createdAt: "2026-03-01", updatedAt: "2026-03-01",
  },
  {
    id: "voice_ngr_female_003", slug: "amina", displayName: "Amina", internalName: "ngr_amina_measured",
    provider: "internal", providerVoiceId: "internal.amina.v1", locale: "en-NG", language: "English", accent: "Northern Nigerian English",
    region: "Nigeria", genderPresentation: "female", ageRange: "30–42", styles: ["Counselor-style", "Measured", "Reflective"],
    tone: "Calm, counseling", pace: "slow", pitchRange: "G3–B4", description: "A measured Northern Nigerian voice for heavy weeks — forgiveness, healing, rest.",
    previewText: "Lay it down here. What you carry was never meant to be carried alone.",
    styleProfile: { warm: 80, depth: 65, energy: 30, pace: 38, presence: 70 },
    status: "PRODUCTION", rightsStatus: "APPROVED", rights: R("iCONFESS Studio", "IC-VOICE-0005"),
    categories: ["healing", "forgiveness", "rest", "grief", "anxiety", "prayer"],
    premium: true, version: 1, curated: "editors", createdAt: "2026-03-14", updatedAt: "2026-03-14",
  },
  {
    id: "voice_ngr_male_003", slug: "tunde", displayName: "Tunde", internalName: "ngr_tunde_pulpit",
    provider: "internal", providerVoiceId: "internal.tunde.v1", locale: "en-NG", language: "English", accent: "Nigerian English",
    region: "Nigeria", genderPresentation: "male", ageRange: "35–50", styles: ["Nigerian Preacher", "Energetic", "Authoritative"],
    tone: "Charged, declarative", pace: "brisk", pitchRange: "G2–F4", description: "Pulpit-energy declaration voice — for confessions spoken like they mean business.",
    previewText: "Say it like the heavens are listening — because they are. I am favored. I am kept.",
    styleProfile: { warm: 50, depth: 75, energy: 90, pace: 70, presence: 95 },
    status: "RIGHTS_REVIEW", rightsStatus: "PENDING", rights: R("Talent — pending clearance", "IC-VOICE-0006 (draft)"),
    categories: ["faith", "favor", "victory", "confidence", "purpose"],
    premium: true, version: 1, curated: null, createdAt: "2026-08-30", updatedAt: "2026-09-10",
  },
  {
    id: "voice_gha_male_001", slug: "kwame", displayName: "Kwame", internalName: "gha_kwame_teacher",
    provider: "internal", providerVoiceId: "internal.kwame.v1", locale: "en-GH", language: "English", accent: "Ghanaian English",
    region: "West Africa", genderPresentation: "male", ageRange: "30–45", styles: ["Teacher", "Measured", "Reflective"],
    tone: "Clear, pedagogic", pace: "measured", pitchRange: "A2–D4", description: "Ghanaian English with a teacher's clarity — memory work and scripture reading.",
    previewText: "Read it slowly. Let each word land before the next one comes.",
    styleProfile: { warm: 60, depth: 70, energy: 45, pace: 45, presence: 75 },
    status: "PRODUCTION", rightsStatus: "APPROVED", rights: R("iCONFESS Studio", "IC-VOICE-0007"),
    categories: ["memory", "wisdom", "direction", "discipline", "purpose"],
    premium: true, version: 1, curated: "new", createdAt: "2026-05-02", updatedAt: "2026-05-02",
  },
  {
    id: "voice_gha_female_001", slug: "ama", displayName: "Ama", internalName: "gha_ama_story",
    provider: "internal", providerVoiceId: "internal.ama.v1", locale: "en-GH", language: "English", accent: "Ghanaian English",
    region: "West Africa", genderPresentation: "female", ageRange: "26–38", styles: ["Storyteller", "Warm", "Gentle"],
    tone: "Story-warm", pace: "measured", pitchRange: "A3–D5", description: "A Ghanaian storytelling voice — testimonies, evening reflection, family words.",
    previewText: "Every testimony begins as a whisper someone refused to let die.",
    styleProfile: { warm: 85, depth: 55, energy: 50, pace: 52, presence: 68 },
    status: "QUALITY_REVIEW", rightsStatus: "APPROVED", rights: R("iCONFESS Studio", "IC-VOICE-0008"),
    categories: ["testimony", "family", "gratitude", "hope", "love"],
    premium: true, version: 1, curated: null, createdAt: "2026-07-18", updatedAt: "2026-09-01",
  },
  {
    id: "voice_gbr_male_001", slug: "oliver", displayName: "Oliver", internalName: "gbr_oliver_formal",
    provider: "internal", providerVoiceId: "internal.oliver.v1", locale: "en-GB", language: "English", accent: "British English",
    region: "Europe", genderPresentation: "male", ageRange: "30–45", styles: ["British Preacher", "Authoritative", "Measured"],
    tone: "Formal, resonant", pace: "measured", pitchRange: "G2–C4", description: "Received British resonance — scripture reading and formal declaration.",
    previewText: "The word stands, though the world moves. Speak it plainly.",
    styleProfile: { warm: 55, depth: 80, energy: 45, pace: 46, presence: 85 },
    status: "PRODUCTION", rightsStatus: "APPROVED", rights: R("iCONFESS Studio", "IC-VOICE-0009"),
    categories: ["scripture", "faith", "purpose", "wisdom", "leadership"],
    premium: true, version: 1, curated: "popular", createdAt: "2026-04-11", updatedAt: "2026-04-11",
  },
  {
    id: "voice_gbr_female_001", slug: "charlotte", displayName: "Charlotte", internalName: "gbr_charlotte_reflect",
    provider: "internal", providerVoiceId: "internal.charlotte.v1", locale: "en-GB", language: "English", accent: "British English",
    region: "Europe", genderPresentation: "female", ageRange: "28–40", styles: ["Reflective", "Meditative", "Calm Speaker"],
    tone: "Soft, precise", pace: "slow", pitchRange: "A3–C5", description: "A quiet British voice for sleep, meditation and long reflection.",
    previewText: "Breathe out. Nothing needs solving in this minute.",
    styleProfile: { warm: 75, depth: 55, energy: 25, pace: 35, presence: 65 },
    status: "PRODUCTION", rightsStatus: "APPROVED", rights: R("iCONFESS Studio", "IC-VOICE-0010"),
    categories: ["rest", "sleep", "anxiety", "peace", "meditation"],
    premium: true, version: 1, curated: "editors", createdAt: "2026-04-11", updatedAt: "2026-04-11",
  },
  {
    id: "voice_irl_female_001", slug: "fiona", displayName: "Fiona", internalName: "irl_fiona_warm",
    provider: "internal", providerVoiceId: "internal.fiona.v1", locale: "en-IE", language: "English", accent: "Irish English",
    region: "Europe", genderPresentation: "female", ageRange: "30–45", styles: ["Warm", "Storyteller", "Gentle"],
    tone: "Warm, lilting", pace: "measured", pitchRange: "A3–D5", description: "Irish warmth — blessings, family words, evening gratitude.",
    previewText: "May the road rise to meet the words you speak this morning.",
    styleProfile: { warm: 90, depth: 50, energy: 55, pace: 55, presence: 70 },
    status: "PRODUCTION", rightsStatus: "APPROVED", rights: R("iCONFESS Studio", "IC-VOICE-0011"),
    categories: ["family", "gratitude", "hope", "love", "favor"],
    premium: true, version: 1, curated: null, createdAt: "2026-05-20", updatedAt: "2026-05-20",
  },
  {
    id: "voice_usa_male_001", slug: "marcus", displayName: "Marcus", internalName: "usa_marcus_drive",
    provider: "internal", providerVoiceId: "internal.marcus.v1", locale: "en-US", language: "English", accent: "American English",
    region: "North America", genderPresentation: "male", ageRange: "28–42", styles: ["Motivational Speaker", "Energetic", "American Preacher"],
    tone: "Driven, bright", pace: "brisk", pitchRange: "A2–E4", description: "American drive — career, confidence and morning momentum.",
    previewText: "Stand up before the day stands over you. This is your ground.",
    styleProfile: { warm: 50, depth: 65, energy: 85, pace: 68, presence: 90 },
    status: "PRODUCTION", rightsStatus: "APPROVED", rights: R("iCONFESS Studio", "IC-VOICE-0012"),
    categories: ["career", "confidence", "success", "leadership", "discipline"],
    premium: true, version: 1, curated: "popular", createdAt: "2026-03-29", updatedAt: "2026-03-29",
  },
  {
    id: "voice_usa_female_001", slug: "harper", displayName: "Harper", internalName: "usa_harper_intimate",
    provider: "internal", providerVoiceId: "internal.harper.v1", locale: "en-US", language: "English", accent: "American English",
    region: "North America", genderPresentation: "female", ageRange: "24–36", styles: ["Conversational", "Intimate", "Calm Speaker"],
    tone: "Close, intimate", pace: "measured", pitchRange: "A3–E5", description: "A close American voice — journaling companions and quiet confession.",
    previewText: "Just between you and the quiet. Say what's true.",
    styleProfile: { warm: 80, depth: 45, energy: 40, pace: 52, presence: 60 },
    status: "PRODUCTION", rightsStatus: "APPROVED", rights: R("iCONFESS Studio", "IC-VOICE-0013"),
    categories: ["journaling", "anxiety", "rest", "self-worth", "forgiveness"],
    premium: true, version: 1, curated: null, createdAt: "2026-06-08", updatedAt: "2026-06-08",
  },
  {
    id: "voice_can_male_001", slug: "liam", displayName: "Liam", internalName: "can_liam_easy",
    provider: "internal", providerVoiceId: "internal.liam.v1", locale: "en-CA", language: "English", accent: "Canadian English",
    region: "North America", genderPresentation: "male", ageRange: "26–38", styles: ["Conversational", "Gentle", "Calm Speaker"],
    tone: "Easy, unhurried", pace: "measured", pitchRange: "A2–D4", description: "Unhurried Canadian ease — daily practice and long walks.",
    previewText: "No rush here. One word at a time is still forward.",
    styleProfile: { warm: 70, depth: 60, energy: 45, pace: 50, presence: 65 },
    status: "TESTING", rightsStatus: "APPROVED", rights: R("iCONFESS Studio", "IC-VOICE-0014"),
    categories: ["daily", "patience", "peace", "gratitude"],
    premium: true, version: 1, curated: null, createdAt: "2026-08-12", updatedAt: "2026-09-05",
  },
  {
    id: "voice_int_female_001", slug: "vera", displayName: "Vera", internalName: "int_vera_neutral",
    provider: "internal", providerVoiceId: "internal.vera.v1", locale: "en", language: "English", accent: "International neutral",
    region: "Global", genderPresentation: "female", ageRange: "30–45", styles: ["Meditative", "Measured", "Teacher"],
    tone: "Neutral, centered", pace: "slow", pitchRange: "G3–C5", description: "Accent-neutral international English — the default when no accent is preferred.",
    previewText: "Center first. Then speak. The words will find their weight.",
    styleProfile: { warm: 60, depth: 60, energy: 35, pace: 42, presence: 70 },
    status: "PRODUCTION", rightsStatus: "APPROVED", rights: R("iCONFESS Studio", "IC-VOICE-0015"),
    categories: ["meditation", "scripture", "wisdom", "peace", "memory"],
    premium: false, version: 1, curated: "featured", createdAt: "2026-02-27", updatedAt: "2026-02-27",
  },
  {
    id: "voice_fra_female_001", slug: "elise", displayName: "Élise", internalName: "fra_elise_douce",
    provider: "internal", providerVoiceId: "internal.elise.v1", locale: "fr-FR", language: "French", accent: "French",
    region: "Europe", genderPresentation: "female", ageRange: "28–40", styles: ["Gentle", "Reflective"],
    tone: "Soft, French", pace: "slow", pitchRange: "A3–D5", description: "French voice in rights review — first-language sets ship only after native-speaker review.",
    previewText: "Respire. Les mots viennent quand le cœur est calme.",
    styleProfile: { warm: 80, depth: 50, energy: 35, pace: 40, presence: 65 },
    status: "RIGHTS_REVIEW", rightsStatus: "PENDING", rights: R("Talent — pending clearance", "IC-VOICE-0016 (draft)"),
    categories: ["rest", "peace", "love"],
    premium: true, version: 1, curated: null, createdAt: "2026-08-25", updatedAt: "2026-09-12",
  },
  {
    id: "voice_deu_male_001", slug: "lukas", displayName: "Lukas", internalName: "deu_lukas_klar",
    provider: "internal", providerVoiceId: "internal.lukas.v1", locale: "de-DE", language: "German", accent: "German",
    region: "Europe", genderPresentation: "male", ageRange: "30–45", styles: ["Measured", "Authoritative"],
    tone: "Clear, German", pace: "measured", pitchRange: "G2–C4", description: "German voice in testing — precision for scripture and memory work.",
    previewText: "Ein Wort nach dem anderen. So wächst Vertrauen.",
    styleProfile: { warm: 50, depth: 75, energy: 45, pace: 48, presence: 80 },
    status: "TESTING", rightsStatus: "APPROVED", rights: R("iCONFESS Studio", "IC-VOICE-0017"),
    categories: ["scripture", "memory", "discipline"],
    premium: true, version: 1, curated: null, createdAt: "2026-08-18", updatedAt: "2026-09-08",
  },
  {
    id: "voice_leg_male_000", slug: "legacy-voice", displayName: "Legacy Announcer", internalName: "legacy_announcer_v0",
    provider: "internal", providerVoiceId: "internal.legacy.v0", locale: "en", language: "English", accent: "International neutral",
    region: "Global", genderPresentation: "male", ageRange: "—", styles: ["Conversational"],
    tone: "Neutral", pace: "measured", pitchRange: "—", description: "Retired first-generation voice. Rights window closed — kept for archive only, never selectable.",
    previewText: "This voice is retired and cannot generate new audio.",
    styleProfile: { warm: 50, depth: 50, energy: 50, pace: 50, presence: 50 },
    status: "SUSPENDED", rightsStatus: "EXPIRED", rights: { ...R("Third-party licensor", "IC-VOICE-0000"), expirationDate: "2026-06-30" },
    categories: [], premium: false, version: 1, curated: null, createdAt: "2025-11-02", updatedAt: "2026-07-01",
  },
];

/* §06/§54 — a voice is only usable in production generation when BOTH gates pass */
export const voiceAvailable = (v: CatalogVoice) =>
  v.rightsStatus === "APPROVED" && v.status === "PRODUCTION" &&
  (!v.rights.expirationDate || new Date(v.rights.expirationDate).getTime() > Date.now());

/* local admin overrides (§53) — status/rights edits persist client-side in the demo */
export type VoiceOverride = { status?: VoiceStatus; rightsStatus?: RightsStatus; displayName?: string; description?: string; expirationDate?: string | null };
const OVR_KEY = "ic-ae-voice-overrides";
export function loadOverrides(): Record<string, VoiceOverride> {
  if (typeof window === "undefined") return {};
  try { return JSON.parse(localStorage.getItem(OVR_KEY) || "{}"); } catch { return {}; }
}
export function saveOverrides(o: Record<string, VoiceOverride>) { try { localStorage.setItem(OVR_KEY, JSON.stringify(o)); } catch { } }
export function effectiveVoices(): CatalogVoice[] {
  const o = loadOverrides();
  return VOICE_CATALOG.map((v) => {
    const ov = o[v.id]; if (!ov) return v;
    return { ...v, ...("status" in ov ? { status: ov.status! } : {}), ...("rightsStatus" in ov ? { rightsStatus: ov.rightsStatus! } : {}), ...("displayName" in ov ? { displayName: ov.displayName! } : {}), ...("description" in ov ? { description: ov.description! } : {}), rights: { ...v.rights, ...("expirationDate" in ov ? { expirationDate: ov.expirationDate! } : {}) } };
  });
}

/* §59 — ambience library (procedurally rendered on-device; masters in R2 when backend ships) */
export type Ambience = { id: string; name: string; category: string; loopable: boolean; volumeDefault: number; rights: string; status: "READY" };
export const AMBIENCES: Ambience[] = [
  { id: "rain", name: "Rain", category: "Weather", loopable: true, volumeDefault: 45, rights: "iCONFESS Studio", status: "READY" },
  { id: "ocean", name: "Ocean", category: "Nature", loopable: true, volumeDefault: 40, rights: "iCONFESS Studio", status: "READY" },
  { id: "night", name: "Night", category: "Nature", loopable: true, volumeDefault: 35, rights: "iCONFESS Studio", status: "READY" },
  { id: "fireplace", name: "Fireplace", category: "Indoor", loopable: true, volumeDefault: 40, rights: "iCONFESS Studio", status: "READY" },
  { id: "room", name: "Quiet room", category: "Indoor", loopable: true, volumeDefault: 30, rights: "iCONFESS Studio", status: "READY" },
];

/* §63/§25 — sound profiles: a profile is only a saved configuration */
export type SoundConfig = { bass: number; mid: number; treble: number; warmth: number; clarity: number; presence: number; ambienceLevel: number; rate: number };
export type SoundProfile = { id: string; name: string; builtin: boolean; config: SoundConfig };
export const SOUND_PROFILES: SoundProfile[] = [
  { id: "default", name: "Default", builtin: true, config: { bass: 0, mid: 0, treble: 0, warmth: 60, clarity: 55, presence: 60, ambienceLevel: 45, rate: 1 } },
  { id: "voice-focus", name: "Voice Focus", builtin: true, config: { bass: -2, mid: 2, treble: 1, warmth: 50, clarity: 70, presence: 80, ambienceLevel: 15, rate: 1 } },
  { id: "deep", name: "Deep", builtin: true, config: { bass: 4, mid: -1, treble: -2, warmth: 75, clarity: 45, presence: 65, ambienceLevel: 40, rate: 0.95 } },
  { id: "warm", name: "Warm", builtin: true, config: { bass: 2, mid: 1, treble: -1, warmth: 80, clarity: 50, presence: 60, ambienceLevel: 40, rate: 0.95 } },
  { id: "night", name: "Night", builtin: true, config: { bass: 1, mid: 0, treble: -3, warmth: 70, clarity: 45, presence: 50, ambienceLevel: 30, rate: 0.9 } },
  { id: "quiet", name: "Quiet", builtin: true, config: { bass: 0, mid: 1, treble: 0, warmth: 65, clarity: 55, presence: 55, ambienceLevel: 20, rate: 0.9 } },
];

/* §14/§15 — search + filter over metadata */
export type VoiceFilters = { region?: string; language?: string; style?: string; gender?: string; free?: boolean; premium?: boolean; curated?: string };
export function searchVoices(list: CatalogVoice[], q: string, f: VoiceFilters): CatalogVoice[] {
  const s = q.trim().toLowerCase();
  return list.filter((v) => {
    if (f.region && v.region !== f.region) return false;
    if (f.language && v.language !== f.language) return false;
    if (f.style && !v.styles.some((x) => x.toLowerCase().includes(f.style!.toLowerCase()))) return false;
    if (f.gender && v.genderPresentation !== f.gender) return false;
    if (f.free && v.premium) return false;
    if (f.premium && !v.premium) return false;
    if (f.curated && v.curated !== f.curated) return false;
    if (!s) return true;
    const hay = [v.displayName, v.accent, v.locale, v.language, v.region, v.tone, v.description, ...v.styles].join(" ").toLowerCase();
    return hay.includes(s);
  });
}
export const voiceBySlug = (slug: string) => effectiveVoices().find((v) => v.slug === slug);
