import React from "react";
import type { Metadata } from "next";
import "./globals.css";
import { UiProvider } from "@/lib/ui";

export const metadata: Metadata = {
  metadataBase: new URL("https://iconfess.app"),
  title: { default: "iCONFESS — Speak it. Hear it. Live it.", template: "%s — iCONFESS" },
  description: "Turn the words you believe in into an experience you can return to every day. 39 categories, reviewed confessions, curated voices and guided sessions.",
  openGraph: { siteName: "iCONFESS", type: "website" },
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
        <UiProvider>{children}</UiProvider>
      </body>
    </html>
  );
}
