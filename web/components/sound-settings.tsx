"use client";
import { useEffect, useState } from "react";
import Link from "next/link";
import { Icon } from "./ui";
import { VoicePreferences } from "./voice-preferences";
import { mutate, useApp } from "@/lib/store";
import { usePlayer } from "@/lib/player";
import { useAudio, useToast } from "@/lib/ui";
import { EQ_PRESETS, SOUNDS, type SoundName } from "@/lib/sound-engine";
import { CONFESSIONS } from "@/lib/data";

const PRESETS = [
  { id: "dawn", name: "Calm dawn", description: "Warm speech, a slower start.", rate: 0.9, eq: "warm", pitch: 1 },
  { id: "steady", name: "Steady day", description: "Natural pace, as mastered.", rate: 1, eq: "flat", pitch: 1 },
  { id: "night", name: "Soft night", description: "A gentle pace and softer highs.", rate: 0.8, eq: "night", pitch: 0.95 },
];
export function SoundControls({ compact = false }: { compact?: boolean }) {
  const { settings } = useApp();
  const player = usePlayer();
  const changeSound = (sound: SoundName) => {
    mutate((s) => { s.settings.ambientSound = sound; s.settings.ambient = true; });
    player.unlockSound();
  };
  return <div className={compact ? "sound-controls compact" : "sound-controls"}>
    <div className="setting-row"><div><b>Background sound</b><span>Fades in under narration; stops when you pause, stop or finish.</span></div><button type="button" className="switch" role="switch" aria-label="Background sound" aria-checked={settings.ambient} onClick={() => { mutate((s) => { s.settings.ambient = !s.settings.ambient; }); player.unlockSound(); }} /></div>
    <div className="sound-choice-grid">
      {Object.entries(SOUNDS).map(([id, sound]) => <button key={id} type="button" className="sound-choice" aria-pressed={settings.ambient && settings.ambientSound === id} onClick={() => changeSound(id as SoundName)}><Icon n="wave" s={22} /><b>{sound.label}</b>{!compact && <span>{sound.description}</span>}</button>)}
    </div>
    <div className="setting-row"><div><label htmlFor={compact ? "compact-ambient-volume" : "ambient-volume"}><b>Background level</b></label><span>{Math.round(settings.ambientVolume * 100)}% · kept beneath the words</span></div><input id={compact ? "compact-ambient-volume" : "ambient-volume"} type="range" min={0} max={0.6} step={0.02} value={settings.ambientVolume} onChange={(e) => { mutate((s) => { s.settings.ambientVolume = Number(e.target.value); }); player.unlockSound(); }} /></div>
    <div className="field"><label htmlFor={compact ? "compact-eq" : "listener-eq"}>Recording EQ</label><select id={compact ? "compact-eq" : "listener-eq"} value={settings.eq} onChange={(e) => { mutate((s) => { s.settings.eq = e.target.value; }); player.unlockSound(); }}>{Object.entries(EQ_PRESETS).map(([id, preset]) => <option key={id} value={id}>{preset.label}</option>)}</select><p className="small" style={{ marginTop: 6 }}>EQ shapes published audio on this device. Browsers do not expose device-speech audio for EQ.</p></div>
    <div className="setting-row"><div><b>Even listening level</b><span>Gentle compression for recordings. Does not alter the stored master.</span></div><button type="button" className="switch" role="switch" aria-label="Even listening level" aria-checked={settings.normalize} onClick={() => { mutate((s) => { s.settings.normalize = !s.settings.normalize; }); player.unlockSound(); }} /></div>
    {player.soundError && <p className="feature-error" role="alert">{player.soundError}</p>}
  </div>;
}

export function SoundSettings() {
  const { settings } = useApp();
  const player = usePlayer();
  const audio = useAudio();
  const toast = useToast();
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => { const id = setInterval(() => setNow(Date.now()), 15000); return () => clearInterval(id); }, []);
  const sample = CONFESSIONS.find((c) => c.category === "Peace") || CONFESSIONS[0];
  return <section className="listening-settings">
    <div className="app-top"><div><h2 className="h2">Voice & sound</h2><p className="small">Your listening preferences, saved on this browser. One player uses them everywhere.</p></div><Link className="btn btn-ghost btn-sm" href="/app/voices"><Icon n="mic" s={14} /> Voice library</Link></div>
    <div className="listening-grid">
      <div className="form-card">
        <span className="eyebrow">Narration</span><h3 className="h3" style={{ margin: "8px 0 20px" }}>A voice you can return to</h3>
        <VoicePreferences />
        <div className="setting-row"><div><label htmlFor="narration-rate"><b>Playback speed</b></label><span>{settings.rate.toFixed(2).replace(/0$/, "")}× · speech and recordings</span></div><input id="narration-rate" type="range" min={0.5} max={2} step={0.05} value={settings.rate} onChange={(e) => player.setRate(Number(e.target.value))} /></div>
        <div className="chip-row">{[0.8, 1, 1.2, 1.5].map((rate) => <button className="chip" type="button" key={rate} aria-pressed={settings.rate === rate} onClick={() => player.setRate(rate)}>{rate}×</button>)}</div>
        <div className="setting-row"><div><label htmlFor="narration-volume"><b>Narration volume</b></label><span>{Math.round(settings.volume * 100)}% · zero mutes all listening sound</span></div><input id="narration-volume" type="range" min={0} max={1} step={0.05} value={settings.volume} onChange={(e) => player.setVolume(Number(e.target.value))} /></div>
        <div className="setting-row"><div><label htmlFor="speech-pitch"><b>Device speech pitch</b></label><span>{settings.pitch.toFixed(2)} · does not change published voices</span></div><input id="speech-pitch" type="range" min={0.5} max={1.5} step={0.05} value={settings.pitch} onChange={(e) => mutate((s) => { s.settings.pitch = Number(e.target.value); })} /></div>
        <div className="field"><label htmlFor="confession-repeat">Repetitions per confession</label><select id="confession-repeat" value={settings.repeat} onChange={(e) => mutate((s) => { s.settings.repeat = Number(e.target.value); })}>{[1, 3, 7].map((n) => <option key={n} value={n}>{n === 1 ? "Once" : `${n} times`}</option>)}</select><p className="small" style={{ marginTop: 6 }}>Applied when each next item starts. Voice samples always play once.</p></div>
        <button className="btn btn-primary" type="button" disabled={!player.speechAvailable} onClick={() => audio.play([{ slug: "voice-preview-listening-test", title: "Listening test", category: "Device speech", text: sample.medium, preview: true }])}><Icon n="play" s={14} /> Test your settings</button>
        {player.error && <p className="feature-error" role="alert">{player.error}</p>}
      </div>
      <div className="form-card">
        <span className="eyebrow">Your listening space</span><h3 className="h3" style={{ margin: "8px 0 20px" }}>Keep the words in front</h3>
        <SoundControls />
        <div className="setting-row"><div><b>Completion chime</b><span>One soft note after a finished session. Off by default; never on samples.</span></div><button className="switch" role="switch" type="button" aria-label="Completion chime" aria-checked={settings.chime} onClick={() => { mutate((s) => { s.settings.chime = !s.settings.chime; }); player.unlockSound(); }} /></div>
        <div className="field"><label htmlFor="sleep-timer">Sleep timer</label><select id="sleep-timer" value={player.sleepUntil ? "running" : "0"} onChange={(e) => { player.setSleepTimer(Number(e.target.value)); toast(Number(e.target.value) ? `Playback will pause in ${e.target.value} minutes` : "Sleep timer cancelled"); }}>{player.sleepUntil && <option value="running">{Math.max(1, Math.ceil((player.sleepUntil - now) / 60000))} minutes remaining</option>}<option value="0">Off</option>{[5, 10, 15, 30, 60].map((n) => <option value={n} key={n}>{n} minutes</option>)}</select><p className="small" style={{ marginTop: 6 }}>The timer follows the player across pages. Browsers may delay timers while the device sleeps.</p></div>
      </div>
    </div>
    <div className="form-card listening-presets"><div><span className="eyebrow">A starting point</span><h3 className="h3" style={{ marginTop: 8 }}>Listening presets</h3></div><div className="sound-choice-grid">{PRESETS.map((p) => <button key={p.id} className="sound-choice" type="button" aria-pressed={settings.preset === p.id && settings.rate === p.rate && settings.eq === p.eq} onClick={() => { mutate((s) => { s.settings.preset = p.id; s.settings.rate = p.rate; s.settings.eq = p.eq; s.settings.pitch = p.pitch; }); player.unlockSound(); toast(`${p.name} applied`); }}><b>{p.name}</b><span>{p.description}</span></button>)}</div><button className="btn btn-ghost btn-sm" type="button" onClick={() => { mutate((s) => { Object.assign(s.settings, { rate: 1, volume: 1, pitch: 1, repeat: 1, eq: "flat", ambient: false, ambientSound: "rain", ambientVolume: 0.2, normalize: true, chime: false, preset: "steady" }); }); player.setSleepTimer(0); toast("Listening defaults restored"); }}>Reset sound preferences</button></div>
  </section>;
}
