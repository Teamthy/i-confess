"use client";

import React, { useMemo, useState } from "react";
import Link from "next/link";
import { Icon } from "./ui";
import { CONFESSIONS, type QueueItem } from "@/lib/data";
import { useAudio, useToast } from "@/lib/ui";
import { useApp, mutate } from "@/lib/store";
import { useVoiceCatalogue, playableSample, preferredVoiceId } from "@/lib/catalogue";
import { selectDeviceVoice } from "@/lib/audio-playback";
import { DeviceVoicePicker } from "./voice-preferences";

type VoiceSample = {
  id: string;
  title: string;
  category: string;
  description: string;
  text: string;
};

const SAMPLE_CATEGORIES = ["Peace", "Gratitude", "Confidence"];
const SAMPLE_COPY: VoiceSample[] = SAMPLE_CATEGORIES.flatMap((category) => {
  const confession = CONFESSIONS.find((item) => item.category.toLowerCase() === category.toLowerCase());
  return confession
    ? [{
        id: confession.slug,
        title: confession.title,
        category: confession.category,
        description: confession.short,
        text: confession.medium,
      }]
    : [];
});
const FALLBACK_SAMPLES: VoiceSample[] = CONFESSIONS.slice(0, 3).map((item) => ({
  id: item.slug,
  title: item.title,
  category: item.category,
  description: item.short,
  text: item.medium,
}));
const SAMPLES = SAMPLE_COPY.length >= 3 ? SAMPLE_COPY : FALLBACK_SAMPLES;
const CURATION_STEPS = [
  { id: "identity", focus: "Voice identity & sample approval", detail: "A voice is named and previewed only after permission to publish its identity and sample is confirmed." },
  { id: "rights", focus: "Recording & usage rights", detail: "Recording, distribution and commercial use are reviewed separately before a voice becomes selectable." },
  { id: "language", focus: "Language & accent review", detail: "Language coverage is listed only after the voice and its sample pass review." },
];

const waveformBars = Array.from({ length: 46 }, (_, index) => {
  const shape = Math.abs(Math.sin(index * 0.47) * 0.58 + Math.sin(index * 0.17 + 1.2) * 0.42);
  return 18 + Math.round(shape * 78);
});

function sampleQueueItem(sample: VoiceSample): QueueItem {
  return {
    slug: `voice-preview-${sample.id}`,
    title: `Device speech sample · ${sample.title}`,
    category: sample.category,
    text: sample.text,
    preview: true,
  };
}

function Waveform({ progress, active = false, compact = false }: { progress: number; active?: boolean; compact?: boolean }) {
  const value = Math.max(0, Math.min(1, progress));
  return (
    <div
      className={`voice-waveform${active ? " is-active" : ""}${compact ? " is-compact" : ""}`}
      role="progressbar"
      aria-label="Spoken sample progress"
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={Math.round(value * 100)}
    >
      {waveformBars.map((height, index) => (
        <i
          key={index}
          className={index / waveformBars.length < value ? "played" : undefined}
          style={{ height: `${height}%` }}
        />
      ))}
    </div>
  );
}

function useSamplePlayback(sample: VoiceSample) {
  const audio = useAudio();
  const item = useMemo(() => sampleQueueItem(sample), [sample]);
  const current = audio.queue[audio.idx];
  const isCurrent = current?.slug === item.slug;
  const playing = isCurrent && audio.status === "playing";
  const progress = isCurrent ? audio.progress : 0;

  const toggle = () => {
    if (isCurrent && audio.status === "playing") {
      audio.pause();
      return;
    }
    if (isCurrent && audio.status === "paused") {
      audio.resume();
      return;
    }
    audio.play([item]);
  };

  return { toggle, playing, progress, error: isCurrent ? audio.error : null };
}

function PlayIcon({ playing }: { playing: boolean }) {
  return <Icon n={playing ? "pause" : "play"} s={14} />;
}

function HeroPlayer({ sample }: { sample: VoiceSample }) {
  const playback = useSamplePlayback(sample);
  const label = playback.playing ? "Pause device-speech sample" : "Play device-speech sample";
  const audio = useAudio();
  const { settings } = useApp();
  const voice = selectDeviceVoice(audio.deviceVoices, settings.deviceVoice);
  return (
    <div className="voice-feature-player">
      <div className="voice-feature-person">
        <div className="voice-portrait-wrap">
          <span className="device-voice-portrait"><Icon n="mic" s={44} /></span>
        </div>
        <div className="voice-person-copy">
          <span className="voice-person-eyebrow">Your device voice</span>
          <h2>{voice?.name || "Browser default"}</h2>
          <p>{voice?.lang || "Device speech"}</p>
          <span className="voice-license"><Icon n="info" s={12} /> {voice?.localService ? "On-device speech engine" : "Browser-provided speech engine"}</span>
        </div>
      </div>

      <div className="voice-player-main">
        <div className="voice-player-heading">
          <div>
            <span className="voice-player-kicker">A sample to begin with</span>
            <h3>{sample.title}</h3>
          </div>
          <span className="voice-device-tag">Device speech preview</span>
        </div>
        <p className="voice-player-script">“{sample.text}”</p>
        <Waveform progress={playback.progress} active={playback.playing} />
        <div className="voice-player-controls">
          <button className="voice-play-button" type="button" onClick={playback.toggle} aria-label={label}>
            <PlayIcon playing={playback.playing} />
          </button>
          <div className="voice-progress-copy" aria-live="polite">
            <b>{playback.playing ? "Speaking" : "Listen to a sample"}</b>
            <span>{playback.playing ? `${Math.round(playback.progress * 100)}% through this sample` : "Progress follows spoken words"}</span>
          </div>
          <Link className="voice-player-link" href="/app/session-builder">
            Build a session <Icon n="arrow" s={13} />
          </Link>
        </div>
        <p className="voice-disclosure">
          This preview uses the device voice shown here, not a recording or imitation of any published narrator. Some device voices require a network connection.
        </p>
        {playback.error && <p role="alert" className="voice-disclosure">{playback.error}</p>}
      </div>
    </div>
  );
}

function SampleCard({ sample, index }: { sample: VoiceSample; index: number }) {
  const playback = useSamplePlayback(sample);
  return (
    <article className="voice-sample-card">
      <div className="voice-sample-top">
        <span className="voice-sample-index">0{index + 1}</span>
        <span className="voice-sample-category">{sample.category}</span>
      </div>
      <h3>{sample.title}</h3>
      <p>{sample.description}</p>
      <div className="voice-sample-controls">
        <button type="button" onClick={playback.toggle} aria-label={`${playback.playing ? "Pause" : "Play"} device-speech sample: ${sample.title}`}>
          <PlayIcon playing={playback.playing} />
        </button>
        <Waveform progress={playback.progress} active={playback.playing} compact />
        <span>{playback.playing ? `${Math.round(playback.progress * 100)}%` : "Sample"}</span>
      </div>
    </article>
  );
}

function CurationCard({ focus, detail }: (typeof CURATION_STEPS)[number]) {
  return (
    <article className="voice-curation-card">
      <span className="voice-curation-lock"><Icon n="lock" s={17} /></span>
      <span className="voice-curation-status">Review checkpoint · not available</span>
      <h3>{focus}</h3>
      <p>{detail}</p>
    </article>
  );
}

export function VoiceLibrary({ inApp = false }: { inApp?: boolean }) {
  const sample = SAMPLES[0];
  const catalogue = useVoiceCatalogue();
  const audio = useAudio();
  const toast = useToast();
  const { settings } = useApp();
  const [query, setQuery] = useState("");
  const voices = (catalogue.data || []).filter((v) => v.status === "active");
  const filtered = voices.filter((v) => `${v.name} ${v.language} ${v.description || ""}`.toLowerCase().includes(query.toLowerCase()));
  const selected = preferredVoiceId(voices, settings.voice);
  return (
    <section className={`voice-library${inApp ? " in-app" : " public-page"}`}>
      <header className="voice-library-intro">
        <div>
          <span className="eyebrow">Voice library</span>
          <h1>Let the words <span>find their pace.</span></h1>
          <p>Choose published narration for account sessions, or a voice from your own device for browser speech. They are distinct sources, always labelled.</p>
        </div>
        <div className="voice-library-counts" aria-label="Voice availability">
          <div><b>{catalogue.loading ? "…" : String(voices.length).padStart(2, "0")}</b><span>published voices</span></div>
          <div><b>{String(audio.deviceVoices.length).padStart(2, "0")}</b><span>device voices</span></div>
        </div>
      </header>

      <section className="voice-library-section" aria-labelledby="published-voices-title">
        <div className="voice-section-heading"><div><span className="voice-section-overline">From the live catalogue</span><h2 id="published-voices-title">Published narration</h2></div><p>Your choice is used by the session builder and saved schedules. The server checks voice availability and access when a session starts.</p></div>
        {catalogue.loading && <p className="small" role="status">Loading published voices…</p>}
        {catalogue.error && <div className="feature-notice" role="alert"><p>{catalogue.error.message} Device samples below are still available.</p><button className="btn btn-ghost btn-sm" onClick={catalogue.reload}>Retry voice catalogue</button></div>}
        {!catalogue.loading && !catalogue.error && <>
          <div className="field voice-search"><label htmlFor="voice-filter">Find a published voice</label><input id="voice-filter" type="search" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Name or language" /></div>
          <div className="published-voice-grid">
            {filtered.map((voice) => {
              const current = audio.current()?.slug === `voice-preview-published-${voice.id}`;
              return <article className={`published-voice-card${selected === voice.id ? " selected" : ""}`} key={voice.id}>
                <div className="published-voice-heading"><span className="published-voice-avatar"><Icon n="mic" s={22} /></span><span className="tag private">{voice.playable === false ? "Audio in review" : voice.premium ? "Premium" : "Standard"}</span></div>
                <h3>{voice.name}</h3><p>{voice.description || "Published narration voice."}</p><span className="small">{voice.language} · {voice.type}{voice.playable === false ? " · no playable render yet" : " · ready to use"}</span>
                <div className="published-voice-actions">
                  <button type="button" className="btn btn-primary btn-sm" disabled={voice.playable === false} aria-pressed={selected === voice.id} onClick={() => { mutate((s) => { s.settings.voice = voice.id; }); toast(`${voice.name} selected for published sessions`); }}>{selected === voice.id ? "Selected ✓" : voice.playable === false ? "Not available yet" : "Use this voice"}</button>
                  {voice.playable !== false && playableSample(voice.sample_url) ? <button className="btn btn-ghost btn-sm" type="button" onClick={() => {
                    if (current && audio.status === "playing") audio.pause();
                    else if (current && audio.status === "paused") audio.resume();
                    else audio.play([{ slug: `voice-preview-published-${voice.id}`, title: `${voice.name} · published sample`, category: voice.language, text: "", src: voice.sample_url!, preview: true }]);
                  }}><Icon n={current && audio.status === "playing" ? "pause" : "play"} s={13} /> {current && audio.status === "playing" ? "Pause sample" : "Published sample"}</button> : <span className="small">No published sample yet</span>}
                </div>
                {current && audio.error && <p className="feature-error" role="alert">{audio.error}</p>}
              </article>;
            })}
          </div>
          {!filtered.length && <p className="small">{voices.length ? "No voices match that search." : "No active published voices are available. You can still use your device's speech engine below."}</p>}
        </>}
      </section>

      <section className="voice-library-section" aria-labelledby="device-voices-title">
        <div className="voice-section-heading"><div><span className="voice-section-overline">Available on this browser</span><h2 id="device-voices-title">Device speech</h2></div><p>No cloning, no narrator impersonation. These samples use the actual voice supplied by your browser.</p></div>
        <div className="form-card voice-device-settings"><DeviceVoicePicker id="library-device-voice" /><Link className="textlink" href="/app/settings/playback">Speed, sound and listening preferences →</Link></div>
        <HeroPlayer sample={sample} />
      </section>
      <section className="voice-library-section" aria-labelledby="voice-samples-title">
        <div className="voice-section-heading"><div><span className="voice-section-overline">Hear a few words</span><h2 id="voice-samples-title">Samples for different moments</h2></div><p>Reviewed wording, spoken in your selected device voice. Samples never repeat or count towards listening history.</p></div>
        <div className="voice-sample-grid">{SAMPLES.map((item, index) => <SampleCard key={item.id} sample={item} index={index} />)}</div>
      </section>
      <section className="voice-library-section voice-curation" aria-labelledby="voice-curation-title">
        <div className="voice-section-heading"><div><span className="voice-section-overline">Rights before release</span><h2 id="voice-curation-title">The curation pipeline</h2></div><p>Review checkpoints, not additional voice listings. Minister-voice models and consent are managed in the admin Voice Studio.</p></div>
        <div className="voice-curation-grid">{CURATION_STEPS.map((step) => <CurationCard key={step.id} {...step} />)}</div>
      </section>
    </section>
  );
}
