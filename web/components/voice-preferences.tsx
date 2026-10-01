"use client";
import Link from "next/link";
import { mutate, useApp } from "@/lib/store";
import { usePlayer } from "@/lib/player";
import { preferredVoiceId, useVoiceCatalogue } from "@/lib/catalogue";
import { selectDeviceVoice } from "@/lib/audio-playback";
import type { Voice } from "@/lib/api";

export function PlatformVoicePicker({ voices, value, onChange, id = "platform-voice", disabled = false }: {
  voices: Voice[]; value: string; onChange: (value: string) => void; id?: string; disabled?: boolean;
}) {
  const selected = preferredVoiceId(voices, value);
  const list = voices.filter((v) => v.status === "active" && v.playable !== false);
  return <div className="field">
    <label htmlFor={id}>Published narration voice</label>
    <select id={id} value={selected} onChange={(e) => onChange(e.target.value)} disabled={disabled}>
      <option value="">Automatic available voice</option>
      {selected && !list.some((v) => v.id === selected) && <option value={selected}>Saved voice is no longer available — choose another</option>}
      {list.map((voice) => <option key={voice.id} value={voice.id}>{voice.name} · {voice.language}{voice.premium ? " · Premium" : ""}</option>)}
    </select>
    <p className="small" style={{ marginTop: 6 }}>Used by published sessions and account schedules. Availability and your plan are checked by the server.</p>
  </div>;
}
export function DeviceVoicePicker({ id = "device-voice" }: { id?: string }) {
  const { settings } = useApp();
  const audio = usePlayer();
  const selected = selectDeviceVoice(audio.deviceVoices, settings.deviceVoice);
  return <div className="field">
    <label htmlFor={id}>Device speech voice</label>
    <select id={id} value={settings.deviceVoice} disabled={!audio.speechAvailable} onChange={(e) => mutate((s) => { s.settings.deviceVoice = e.target.value; })}>
      <option value="">Automatic device voice</option>
      {settings.deviceVoice && !audio.deviceVoices.some((v) => v.voiceURI === settings.deviceVoice) && <option value={settings.deviceVoice}>Saved voice unavailable on this device — using automatic</option>}
      {audio.deviceVoices.map((voice) => <option key={`${voice.voiceURI}-${voice.lang}`} value={voice.voiceURI}>{voice.name} · {voice.lang}{voice.localService ? " · on-device" : " · online"}</option>)}
    </select>
    <p className="small" style={{ marginTop: 6 }}>{!audio.speechAvailable ? "Speech synthesis is not supported by this browser. Published audio can still play." : selected ? `Currently using ${selected.name}. Device speech is not a recording of a published iCONFESS voice.` : "Your browser has not listed its voices yet. It can use its default voice; installed voices vary by device."}</p>
  </div>;
}
export function VoicePreferences() {
  const { settings } = useApp();
  const catalogue = useVoiceCatalogue();
  return <>
    <DeviceVoicePicker />
    {catalogue.loading ? <p className="small" role="status">Loading published voices…</p> : catalogue.error ? <div className="feature-notice" role="alert"><p>{catalogue.error.message}</p><button className="btn btn-ghost btn-sm" onClick={catalogue.reload}>Retry voice catalogue</button></div> : <PlatformVoicePicker voices={catalogue.data || []} value={settings.voice} onChange={(value) => mutate((s) => { s.settings.voice = value; })} />}
    <Link className="textlink" href="/app/voices">Listen to voice samples →</Link>
  </>;
}
