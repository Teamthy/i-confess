/** Write a personal confession — saved privately, or submitted to moderation. */
import { ConfessionCreateClient } from "@/components/ConfessionCreateClient";

export default function CreateConfessionPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <h1>Write a confession</h1>
        <p>Write in your own words. Keep it private, share it with people you choose, or offer it for community review.</p>
      </div>
      <ConfessionCreateClient />
    </div>
  );
}
