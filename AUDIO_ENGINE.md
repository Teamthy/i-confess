# iCONFESS Audio + Voice Engine — Production Architecture

Status: **v10 shipped in-app** (web platform) · backend layers specified below for the API/worker build.
Companion code: `web/lib/voices.ts` (domain + catalog + rights), `web/lib/audio-engine.ts` (providers, preprocessor, jobs, cache keys, events), `web/lib/dsp.ts` (client DSP), `web/lib/ui.tsx` (global player engine), `web/components/voice-catalog.tsx` (discovery, sound panel), `web/components/admin.tsx` (studio console).

---

## 1. What is live in the web platform today

| Layer | Implementation |
|---|---|
| Voice catalog | 18 original studio voices: Nigerian English (Grace, Emeka, Amina, Tunde), **Nigerian Pidgin as its own locale `en-NG-PIDGIN`** (Adaeze, Chidi), Ghanaian (Kwame, Ama), British/Irish/French/German, American/Canadian, neutral International. Full metadata per §05 incl. style profiles, capabilities, curation flags. No impersonation of any real person, ever. |
| Rights registry | `rightsStatus` PENDING/APPROVED/RESTRICTED/EXPIRED/REVOKED + owner, licence ref, territories, expiry. **Enforced**: `voiceAvailable()` gates selection AND generation; expired/revoked voices cannot generate (§06). |
| Approval workflow | DRAFT → TESTING → QUALITY_REVIEW → RIGHTS_REVIEW → APPROVED → PRODUCTION; admin console can move states; nothing auto-ships (§54). |
| Provider abstraction | `VoiceProvider` interface (`listVoices`, `validateVoice`, `resolve`, capabilities). Live adapter: `InternalSpeechProvider` (device speech engine, distinctiveness via style profile). Stubs: ElevenLabs/OpenAI/Google/Azure behind the same interface — activation = credentials + backend, zero product-code change (§07, §75). |
| Preprocessor | whitespace/punct normalisation, scripture-notation expansion ("Romans 8:28" → spoken form), pronunciation dictionary with `iCONFESS` override, admin-extensible (§09, §58). |
| Generation jobs | `AudioGenerationJob` QUEUED→PROCESSING→MASTERING→ENCODING→READY/FAILED, cache-key idempotency (`contentVersion+voiceId+voiceVersion+locale+rate+style`), correlation IDs, reuse of cached masters (§47–§50, §88). |
| Global player | one engine across navigation; states idle/playing/paused/completed/error; queue reorder/remove; repeat 1/3/7; **resume from saved position** ("Continuing from X%", sentence-accurate); Media Session API (OS controls, lock-screen on Android); keyboard: Space play/pause, ←/→ seek, ↑/↓ volume (§38–§42, §46, §93). |
| Client DSP | ambience engine (Rain/Ocean/Night/Fireplace/Room) — procedural, seamless loops, fades; **smart ducking** (voice → 22% in 0.6s, back in 1.4s); biquad EQ chain (lowshelf 180Hz / peaking 1.2kHz / highshelf 4.8kHz + compressor) for real audio elements; voice character (warmth/clarity/presence) mapped into the synthesis path (§21–§26, §59–§62). |
| Sound panel | Basic: volume, speed, bass, treble. Advanced: full EQ + warmth/clarity/presence + ambience. Profiles: Default, Voice Focus, Deep, Warm, Night, Quiet + user-saved ("My Evening") (§25, §63, §65–§66). |
| Discovery | `/voices` search + region/pidgin/free filters + curated sections; `/voices/[slug]` profile with preview, style profile, rights record, featured content, on-demand master generation; in-app `/app/voices` browser + use-voice (§12–§15, §97–§98). |
| Admin studio | `/admin`: overview, voice catalog + rights actions (approve/suspend/revoke/expire — enforcement immediate), generation queue, quality, cost, event log (§53, §80–§85, §113–§114). |
| Analytics | `voice_previewed/selected`, `audio_started/paused/resumed/completed/seeked/error`, `eq_changed`, `speed_changed`, `ambience_changed`, `generation_*`, cache hits. Never records private content text (§69). |

Honest constraint: browser speech output can't be routed through biquad filters, so EQ applies to ambience + real audio today; voice character maps to synthesis parameters. When studio masters (real audio files) ship through the pipeline below, the same EQ chain applies to them unmodified — that's why the chain is built as a reusable `attachEQ()`.

## 2. Production backend (to build)

```
CLIENTS (web/Flutter) → Audio API (Go, next to existing server)
  ├─ PostgreSQL: voices, voice_versions, voice_rights, audio_assets, audio_versions,
  │   audio_generation_jobs, audio_waveforms, audio_transcripts, audio_settings,
  │   audio_profiles, audio_progress, audio_queue_items, ambience_assets,
  │   provider_usage, audio_events   (indexes on voiceId, status, cacheKey; FKs; soft deletes)
  ├─ Redis: generation queue (BullMQ-style), idempotency keys, progress sync pub/sub
  ├─ Workers: TTS provider call → cleanup → loudness -16 LUFS / -1 dBTP → DR → EQ →
  │   de-ess → optional ambience → encode (opus/aac/mp3) → QC → waveform peaks → transcript → R2
  └─ Cloudflare R2 + CDN: audio/{voices|previews|confessions|sessions|ambience|masters|transcripts}/…
      immutable hashes, range requests, signed URLs for private assets, cache-control/ETag
```

- **API surface** (§86): `GET /voices`, `GET /voices/:id[/preview]`, `POST /audio/generate` (idempotency key), `GET /audio/jobs/:id`, `GET/POST /audio/settings|profiles`, `POST /audio/:id/progress` (multi-device: last-writer-wins by timestamp), admin namespace with rights endpoints.
- **Streaming** (§19): progressive + HTTP range from day one; segmented (HLS) for long sessions; prefetch next-in-queue with bandwidth caps.
- **Security** (§76–§77): provider keys server-only; signed short-lived URLs; private confession audio excluded from search/feeds/stats; moderation pipeline stays separate from TTS; rate limits per key.
- **Cost control** (§72–§73): dedupe by cache key, pre-generation for featured/popular/daily, per-org quotas, provider fallback only across rights-compatible equivalent voices with the swap surfaced.
- **Observability** (§71, §113): correlation IDs through queue→worker→storage; metrics for time-to-first-audio, buffering ratio, completion, TTS failures, provider latency, CDN hit rate, cost per voice/day.
- **Mobile** (§27, §43–§45): Flutter audio abstraction → AVAudioEngine (iOS) / native DSP (Android); background playback + lock-screen + audio focus (duck on calls); offline downloads encrypted with expiry per subscription state.

## 3. Definition of Done — current position

- Voice: catalog ✅ filtering ✅ search ✅ preview ✅ metadata ✅ rights ✅ versioning ✅ approval ✅ provider abstraction ✅ (cloud adapters pending keys)
- TTS: generation ✅(on-device)+✅(job model) queue ✅(sim)→Redis worker ⏳ retry ✅(idempotent keys) caching ✅ dedupe ✅ fallback ✅(interface)
- Audio: mastering ⏳(targets set: -16 LUFS/-1 dBTP) encoding ⏳ CDN ⏳ streaming ✅(progressive) waveform ⏳ transcript ✅(synced sentences)
- Player: play/pause/seek/queue/resume/speed/volume ✅ · background+OS controls ✅(web Media Session) · mobile ⏳
- DSP: bass/mid/treble/warmth/clarity/presence/ambience/ducking/profiles ✅
- Content: en ✅ en-NG ✅ en-NG-PIDGIN ✅ · fr/de seeded, gated behind rights review ✅ · pronunciation dict ✅
- User: favorites ✅ history ✅ settings ✅ downloads ✅(offline pins/packs) multi-device ✅(handoff)→server sync ⏳
- Admin: voice studio ✅ rights mgmt ✅ audio mgmt ✅ queue ✅ quality ✅ cost ✅ (demo-local persistence → Postgres)
- Quality: build ✅ 78/78 · unit/integration/load/security suites ⏳ with backend

⏳ = production backend work; the in-app system is complete and demo-true; nothing claims a capability the backend can't yet deliver.
