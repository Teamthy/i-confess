/**
 * Authenticated application layout.
 *
 * Wraps all /app/* routes in the AuthProvider + AppShell.
 * Noindex envelope: authenticated surfaces must never be indexed.
 */

import type { Metadata } from "next";
import { AppShell } from "@/components/AppShell";
import { PlayerProvider } from "@/lib/player-context";

export const metadata: Metadata = {
  robots: { index: false, follow: false },
};

export default function AppLayout({ children }: { children: React.ReactNode }) {
  // AuthProvider is mounted at the root layout (shared with /login, which
  // establishes the session). PlayerProvider lives here — above the shell —
  // so the single audio element survives navigation between /app pages.
  return (
    <PlayerProvider>
      <AppShell>{children}</AppShell>
    </PlayerProvider>
  );
}
