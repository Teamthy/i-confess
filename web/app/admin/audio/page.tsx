import { AdminAudio, AdminGate } from "@/components/admin";

export const metadata = { title: "Audio" };

export default function Page() {
  return (
    <>
      <h1 className="adm-title">Audio</h1>
      <p className="adm-lede">
        Generation jobs and the QA bench. Every render carries a human decision before listeners hear it — approve,
        reject with a recorded reason, or publish an approved one. The server re-authorises each transition.
      </p>
      <AdminGate>
        <AdminAudio />
      </AdminGate>
    </>
  );
}
