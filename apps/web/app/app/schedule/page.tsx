/** Recurring session reminders — the time-based companion to routines. */
import type { Metadata } from "next";
import { ScheduleClient } from "@/components/ScheduleClient";

export const metadata: Metadata = { title: "Schedule" };

export default function SchedulePage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <p className="ia-section-title">Ritual</p>
        <h1>Schedule</h1>
        <p>Quiet appointments with yourself. iCONFESS builds the session; you just show up.</p>
      </div>
      <ScheduleClient />
    </div>
  );
}
