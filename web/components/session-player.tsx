"use client";
import { useEffect, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Icon } from "./ui";
import { SoundControls } from "./sound-settings";
import { DeviceVoicePicker } from "./voice-preferences";
import { useAudio, useToast } from "@/lib/ui";
import { mutate, useApp } from "@/lib/store";
import { useAuth } from "@/lib/auth-context";
import { useApiData } from "@/lib/app-api";
import { useVoiceCatalogue } from "@/lib/catalogue";
import { selectDeviceVoice } from "@/lib/audio-playback";
import type { ApiSession } from "@/lib/session-playback";
import { CAT_IMAGES, catBySlug } from "@/lib/data";

function clock(seconds: number) {
  return `${Math.floor(seconds / 60)}:${String(Math.floor(seconds % 60)).padStart(2, "0")}`;
}
export function SessionPlayer() {
  const audio = useAudio();
  const auth = useAuth();
  const params = useSearchParams();
  const sessionId = params.get("session");
  const requested = useApiData<ApiSession>(sessionId ? auth.token : null, `/sessions/${encodeURIComponent(sessionId || "")}`);
  const voices = useVoiceCatalogue();
  const { settings } = useApp();
  const toast = useToast();
  const [launching, setLaunching] = useState(false);
  const [launchError, setLaunchError] = useState<string | null>(null);
  const [still, setStill] = useState(0);
  const [now, setNow] = useState(() => Date.now());
  const item = audio.current();
  const active = audio.status === "playing" || audio.status === "loading";
  const isApiSession = !!item?.sessionId && item.sessionId !== "bible";
  const anotherSession = sessionId && item?.sessionId !== sessionId;
  useEffect(() => { if (still <= 0) return; const t = setTimeout(() => setStill((s) => s - 1), 1000); return () => clearTimeout(t); }, [still]);
  useEffect(() => { const id = setInterval(() => setNow(Date.now()), 1000); return () => clearInterval(id); }, []);
  const startRequested = async () => {
    if (!requested.data || !auth.token || launching) return;
    setLaunching(true); setLaunchError(null);
    const result = await audio.playSession(requested.data, auth.token);
    setLaunching(false);
    if (!result.ok) setLaunchError(result.message);
  };
  if (anotherSession) return <div className="form-card session-ready">
    <span className="eyebrow">Published session</span><h1 className="h2">{requested.data?.title || "Your session"}</h1>
    {!auth.token ? <p>Sign in to load and play this account's session. <Link className="textlink" href="/login">Sign in</Link></p> : requested.loading ? <p role="status">Loading your queue…</p> : requested.error ? <div role="alert"><p>{requested.error.message}</p><button className="btn btn-ghost" onClick={requested.reload}>Retry session</button></div> : <>
      <p>{requested.data?.items.length || 0} confessions · {Math.round((requested.data?.actual_duration || requested.data?.duration_seconds || 0) / 60)} minutes</p>
      <p className="small">The player uses authorized published audio. No browser speech will be substituted for an unavailable recording.</p>
      <button className="btn btn-primary" disabled={launching} onClick={() => { void startRequested(); }}><Icon n="play" s={16} /> {launching ? "Starting…" : "Start listening"}</button>
      {launchError && <p className="feature-error" role="alert">{launchError}</p>}
    </>}
    <Link className="textlink" href="/app/schedule">Back to your practice →</Link>
  </div>;

  const category = item ? catBySlug(item.category)?.slug || "" : "";
  const sentences = item?.text.match(/[^.!?]+[.!?]?/g) || (item?.text ? [item.text] : []);
  const activeLine = Math.min(sentences.length - 1, Math.floor(audio.progress * sentences.length));
  const deviceVoice = selectDeviceVoice(audio.deviceVoices, settings.deviceVoice);
  const namedVoice = requested.data?.voice_id ? voices.data?.find((v) => v.id === requested.data?.voice_id)?.name : null;
  const canReplay = audio.status === "completed" && !isApiSession;
  return <div className="player-stage finished-player">
    {item && CAT_IMAGES[category] && <img className="ps-canvas" src={`/assets/${CAT_IMAGES[category]}`} alt="" aria-hidden="true" />}
    <div className="ps-voice"><span className="ps-voice-icon"><Icon n="wave" s={15} /></span><span>{audio.source === "file" ? namedVoice ? `${namedVoice} · published audio` : item?.disclosure || "Published audio" : `${deviceVoice?.name || "Browser default"} · device speech`}</span></div>
    <Link className="btn btn-ghost on-dark btn-sm ps-exit" href="/app"><Icon n="exit" s={14} /> Back</Link>
    <span className="ps-cat">{item?.category || "Your listening space"}</span><h1>{item?.title || "A few words, whenever you're ready."}</h1>
    {item && settings.accessibility.captions && sentences.length ? <p className="ps-text tr-sync">{sentences.map((sentence, i) => <span key={i} className={`tr-line${i === activeLine && active ? " on" : ""}`}>{sentence} </span>)}</p> : <p className="ps-text">{item ? audio.source === "file" ? "Listen to the published recording. Your voice, pace and sound preferences stay with this player." : "Device speech is reading your selected words. Captions are off." : "Choose a confession, build a session, or start one of your saved routines. Sound is never forced, and nothing starts without you."}</p>}
    {!item && <div className="player-empty-actions"><Link className="btn btn-light" href="/app/session-builder">Build a session</Link><Link className="btn btn-ghost on-dark" href="/app/schedule">Your routines</Link></div>}
    {item && <>
      <div className="player-controls">
        <button className="pc-btn" type="button" onClick={audio.prev} disabled={audio.idx === 0} aria-label="Previous confession"><Icon n="prev" s={18} /></button>
        {audio.status === "completed" && isApiSession ? <Link className="pc-btn main" href="/app/session-builder" aria-label="Build another session"><Icon n="plus" s={22} /></Link> : <button className="pc-btn main" type="button" onClick={() => audio.status === "error" ? audio.retry() : active ? audio.pause() : audio.resume()} aria-label={audio.status === "error" ? "Retry playback" : active ? "Pause playback" : canReplay ? "Replay device-speech queue" : "Play audio"}><Icon n={active ? "pause" : "play"} s={22} /></button>}
        <button className="pc-btn" type="button" onClick={audio.next} disabled={audio.idx >= audio.queue.length - 1} aria-label="Next confession"><Icon n="next" s={18} /></button>
        <button className="pc-btn" type="button" style={{ fontSize: 12, fontWeight: 700 }} aria-label="Playback speed" title="Playback speed" onClick={() => audio.setRate(settings.rate >= 1.4 ? 0.8 : Math.round((settings.rate + 0.2) * 10) / 10)}>{settings.rate}×</button>
        <button className="pc-btn" type="button" style={{ fontSize: 12, fontWeight: 700 }} aria-label="Cycle sleep timer" title="Sleep timer" onClick={() => { const minutes = audio.sleepUntil ? 0 : 15; audio.setSleepTimer(minutes); toast(minutes ? "Sleep timer: 15 minutes" : "Sleep timer cancelled"); }}>{audio.sleepUntil ? `${Math.max(1, Math.ceil((audio.sleepUntil - now) / 60000))}m` : <Icon n="moon" s={16} />}</button>
        <button className="pc-btn" type="button" aria-label="Toggle background sound" title="Background sound" aria-pressed={settings.ambient} onClick={() => { mutate((s) => { s.settings.ambient = !s.settings.ambient; }); audio.unlockSound(); }}><Icon n="spark" s={16} /></button>
        <button className="pc-btn" type="button" aria-label="30 seconds of stillness" title="Pause for stillness" onClick={() => { audio.pause(); setStill(30); }}><Icon n="ast" s={16} /></button>
        <button className="pc-btn" type="button" style={{ fontSize: 12, fontWeight: 700 }} aria-label="Cycle confession repetitions" title="Applied to the next confession; samples always play once" onClick={() => { const options = [1, 3, 7]; const next = options[(options.indexOf(settings.repeat) + 1) % options.length]; mutate((s) => { s.settings.repeat = next; }); toast(next === 1 ? "Next confession plays once" : `Next confession repeats ${next} times`); }}>{settings.repeat}×</button>
        <button className="pc-btn" type="button" aria-label="Toggle captions" title="Captions" aria-pressed={settings.accessibility.captions} onClick={() => mutate((s) => { s.settings.accessibility.captions = !s.settings.accessibility.captions; })}><Icon n="book" s={16} /></button>
        <button className="pc-btn" type="button" aria-label="Stop session" title="Stop playback and background sound" onClick={audio.stop}><Icon n="x" s={18} /></button>
      </div>
      <div className="player-volume"><label htmlFor="player-volume">Volume · {Math.round(settings.volume * 100)}%</label><input id="player-volume" type="range" min={0} max={1} step={0.05} value={settings.volume} onChange={(e) => audio.setVolume(Number(e.target.value))} /></div>
      {(audio.error || audio.syncError) && <div className="player-error" role="alert"><p>{audio.error || audio.syncError}</p>{audio.error && <button className="btn btn-light btn-sm" onClick={audio.retry}>Retry audio</button>}</div>}
      {audio.status === "completed" && <div className="player-completed" role="status"><b>A moment kept.</b><p>Your queue is complete. Background sound has stopped.</p><Link className="textlink on-dark" href="/app/journal">Write a reflection →</Link></div>}
      <details className="player-sound-panel"><summary><Icon n="gear" s={16} /> Voice & sound controls</summary><div className="form-card">{audio.source === "speech" && <DeviceVoicePicker id="player-device-voice" />}<SoundControls compact /><Link className="textlink" href="/app/settings/playback">All listening preferences →</Link></div></details>
      {audio.queue.length > audio.idx + 1 && <div className="ps-queue" aria-label="Up next"><h4>Up next</h4>{audio.queue.slice(audio.idx + 1).map((queued, i) => <div className="psq-row" key={queued.itemId || `${queued.slug}-${i}`}><div className="lr-main"><h4>{queued.title}</h4><p>{queued.category}{queued.src === null ? " · audio unavailable" : ""}</p></div>{i > 0 && <button className="pc-btn" type="button" aria-label={`Move ${queued.title} to next`} onClick={() => { const queue = [...audio.queue]; const [move] = queue.splice(audio.idx + 1 + i, 1); queue.splice(audio.idx + 1, 0, move); audio.updateQueue(queue, audio.idx); }}><Icon n="next" s={13} /></button>}<button className="pc-btn" type="button" aria-label={`Remove ${queued.title} from queue`} onClick={() => audio.updateQueue(audio.queue.filter((_, index) => index !== audio.idx + 1 + i), audio.idx)}><Icon n="x" s={13} /></button></div>)}</div>}
      <div className="ps-progress"><input aria-label={audio.source === "speech" ? "Seek in spoken words (estimated)" : "Seek in recording"} type="range" min={0} max={audio.duration || 1} step={0.1} value={Math.min(audio.position, audio.duration || 1)} onChange={(e) => audio.seek(Number(e.target.value))} /><div className="ps-meta"><span>{audio.status}{audio.source === "file" ? ` · ${clock(audio.position)} / ${clock(audio.duration)}` : ` · ${Math.round(audio.progress * 100)}% of spoken words`}</span><span>{audio.idx + 1} / {audio.queue.length}</span></div></div>
    </>}
    <div role="status" aria-live="polite" className="sr-only">{item ? `Now ${audio.status}: ${item.title}, ${item.category}` : "Nothing playing"}</div>
    {still > 0 && <div className="still-veil" role="dialog" aria-modal="true" aria-label="Stillness"><div className="still-card"><span className="eyebrow on-dark">Stillness</span><div className="still-num">{still}</div><p>Let the words settle. Nothing else is asked of you.</p><button className="btn btn-ghost on-dark" autoFocus onClick={() => setStill(0)}>End stillness</button></div></div>}
  </div>;
}
