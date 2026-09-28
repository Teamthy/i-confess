"use client";

import React, { useMemo } from "react";
import Link from "next/link";
import { Icon } from "./ui";
import { CONFESSIONS, VOICES, type QueueItem } from "@/lib/data";
import { useAudio } from "@/lib/ui";

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
const LISTED_VOICES = VOICES.filter((voice) => voice.status === "active" && /licensed/i.test(voice.rights));
const ACTIVE_VOICE = LISTED_VOICES.find((voice) => voice.slug === "grace") || LISTED_VOICES[0]!;
const LISTED_VOICE_COUNT = String(LISTED_VOICES.length).padStart(2, "0");
const ACTIVE_VOICE_STYLE = "Warm · Calm";
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

  return { toggle, playing, progress };
}

function PlayIcon({ playing }: { playing: boolean }) {
  return <Icon n={playing ? "pause" : "play"} s={14} />;
}

function HeroPlayer({ sample }: { sample: VoiceSample }) {
  const playback = useSamplePlayback(sample);
  const label = playback.playing ? "Pause device-speech sample" : "Play device-speech sample";
  return (
    <div className="voice-feature-player">
      <div className="voice-feature-person">
        <div className="voice-portrait-wrap">
          <img src="/assets/voice-grace.jpg" alt="Portrait of Grace" />
          <span className="voice-live-dot" aria-label="Available" />
        </div>
        <div className="voice-person-copy">
          <span className="voice-person-eyebrow">Available voice</span>
          <h2>{ACTIVE_VOICE.name}</h2>
          <p>{ACTIVE_VOICE_STYLE}</p>
          <span className="voice-license"><Icon n="check" s={12} /> {ACTIVE_VOICE.rights}</span>
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
          The web preview uses your device’s speech engine; it is not a studio recording of Grace. Published audio stays rights-cleared.
        </p>
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
  return (
    <section className={`voice-library${inApp ? " in-app" : " public-page"}`}>
      <header className="voice-library-intro">
        <div>
          <span className="eyebrow">Voice library</span>
          <h1>Let the words <span>find their pace.</span></h1>
          <p>Grace is the licensed narration voice listed today. This web preview uses your device’s speech engine; any future voice will be named only after its identity, sample and usage rights are approved.</p>
        </div>
        <div className="voice-library-counts" aria-label="Voice availability">
          <div><b>{LISTED_VOICE_COUNT}</b><span>{LISTED_VOICES.length === 1 ? "listed voice" : "listed voices"}</span></div>
          <div><b>Rights</b><span>reviewed first</span></div>
        </div>
      </header>

      <HeroPlayer sample={sample} />

      <section className="voice-library-section" aria-labelledby="voice-samples-title">
        <div className="voice-section-heading">
          <div>
            <span className="voice-section-overline">Hear a few words</span>
            <h2 id="voice-samples-title">Samples for different moments</h2>
          </div>
          <p>Each sample uses reviewed iCONFESS wording. Tap to listen, pause, or resume.</p>
        </div>
        <div className="voice-sample-grid">
          {SAMPLES.map((item, index) => <SampleCard key={item.id} sample={item} index={index} />)}
        </div>
      </section>

      <section className="voice-library-section voice-curation" aria-labelledby="voice-curation-title">
        <div className="voice-section-heading">
          <div>
            <span className="voice-section-overline">No names before approval</span>
            <h2 id="voice-curation-title">The curation pipeline</h2>
          </div>
          <p>These are review checkpoints, not additional voice listings. We publish identities and samples only after permission and usage rights are confirmed.</p>
        </div>
        <div className="voice-curation-grid">
          {CURATION_STEPS.map((step) => <CurationCard key={step.id} {...step} />)}
        </div>
      </section>

      <div className="voice-library-footnote">
        <Icon n="shield" s={17} />
        <p>Rights are checked before a voice or recording is published. A review checkpoint never implies that another voice has already been licensed.</p>
      </div>
    </section>
  );
}
