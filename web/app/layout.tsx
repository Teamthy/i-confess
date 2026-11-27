import React from "react";
import type { Metadata } from "next";
import "./globals.css";
import { UiProvider } from "@/lib/ui";
import { AuthProvider } from "@/lib/auth-context";
import { PlayerProvider } from "@/lib/player";
import { ScheduleProvider } from "@/lib/schedule-context";
import { AccountSync } from "@/lib/account-sync";
import { ScheduleReminders } from "@/components/schedules";
import { CardShapeDefs } from "@/components/ui";

export const metadata: Metadata = {
  metadataBase: new URL("https://iconfess.app"),
  title: { default: "I CONFESS — Speak it. Believe it. Live it.", template: "%s — I CONFESS" },
  description: "Turn the words you believe in into an experience you can return to every day. 39 categories, reviewed confessions, curated voices and guided sessions.",
  openGraph: { siteName: "I CONFESS", type: "website", images: [{ url: "/brand/og-mark.png", width: 1200, height: 630, alt: "I CONFESS" }] },
};
export const viewport = { themeColor: "#051650" };

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <head>
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="anonymous" />
        <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=Space+Grotesk:wght@400;500;600;700&display=swap" rel="stylesheet" />
        <link rel="manifest" href="/manifest.webmanifest" />
        <meta name="mobile-web-app-capable" content="yes" />
        <meta name="apple-mobile-web-app-status-bar-style" content="black-translucent" />
      </head>
      <body>
        <CardShapeDefs />
        {/* The real API session wraps everything: the public site renders
            signed-out, the app hydrates from it, and the admin console needs
            it. Local UI state (player, toasts, preferences) stays in
            UiProvider. */}
        <AuthProvider>
          {/* One persistent queue for device speech, published session and
              Bible audio, with shared sound controls and Media Session. */}
          <PlayerProvider>
            <UiProvider><ScheduleProvider><AccountSync />{children}<ScheduleReminders /></ScheduleProvider></UiProvider>
          </PlayerProvider>
        </AuthProvider>
      </body>
    </html>
  );
}
