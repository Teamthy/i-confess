import { AdminGate } from "@/components/admin";
import { VoiceStudio } from "@/components/voice-studio";

export const metadata = { title: "Minister Voices" };

export default function Page() {
  return (
    <>
      <h1 className="adm-title">Minister voices</h1>
      <p className="adm-lede">
        Licensed voice cloning, in order: rights, recordings, review, datasets, training, models and preview. Each step is
        checked against the voice&apos;s current rights on the server. Revoking a voice stops every step immediately, including
        queued work.
      </p>
      <AdminGate>
        <VoiceStudio />
      </AdminGate>
    </>
  );
}
