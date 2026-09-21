/**
 * Authenticated application layout.
 *
 * Wraps all /app/* routes in the AuthProvider + AppShell.
 * Noindex envelope: authenticated surfaces must never be indexed.
 */

import type { Metadata } from "next";
import { AuthProvider } from "@/lib/auth-context";
import { AppShell } from "@/components/AppShell";

export const metadata: Metadata = {
  robots: { index: false, follow: false },
};

export default function AppLayout({ children }: { children: React.ReactNode }) {
  return (
    <AuthProvider>
      <AppShell>{children}</AppShell>
    </AuthProvider>
  );
}
