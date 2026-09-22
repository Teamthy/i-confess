/** Personal confession shelf — private by default, reviewable on request. */
import type { Metadata } from "next";
import Link from "next/link";
import { ConfessionsClient } from "@/components/ConfessionsClient";

export const metadata: Metadata = { title: "My confessions" };

export default function ConfessionsPage() {
  return (
    <div className="ia-page">
      <div className="ia-page__head">
        <p className="ia-section-title">Create</p>
        <h1>My confessions</h1>
        <p>What you have written, and where each piece stands.</p>
      </div>
      <Link href="/app/community/create" className="ic-btn ic-btn--primary" style={{ width: "fit-content" }}>
        Write a confession
      </Link>
      <ConfessionsClient />
    </div>
  );
}
