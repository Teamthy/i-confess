import type { Metadata, Viewport } from "next";
// styles/tokens.css is a symlink to design/generated/tokens.css: the app
// consumes the repository's generated tokens in place — one source of truth,
// nothing copied, drift impossible by construction.
import "../styles/tokens.css";
import "../styles/globals.css";
import "../styles/site.css";
import { SITE_URL, SITE_NAME } from "@/lib/site";

export const metadata: Metadata = {
  metadataBase: new URL(SITE_URL),
  title: {
    default: "iCONFESS — A new way to speak what you believe",
    template: "%s — iCONFESS",
  },
  description:
    "iCONFESS turns spoken confession and reflection into an intentional daily experience. Choose an area of life, hear the words spoken, and return to them until they become part of you.",
  applicationName: SITE_NAME,
  keywords: [
    "confession",
    "daily confession",
    "scripture",
    "reflection",
    "guided sessions",
    "spoken word",
    "christian meditation",
  ],
  openGraph: {
    type: "website",
    siteName: SITE_NAME,
    url: SITE_URL,
    title: "iCONFESS — A new way to speak what you believe",
    description:
      "Spoken confession and reflection, made a daily practice. 39 areas of life, guided sessions, curated voices.",
  },
  twitter: {
    card: "summary_large_image",
    title: "iCONFESS — A new way to speak what you believe",
    description:
      "Spoken confession and reflection, made a daily practice. 39 areas of life, guided sessions, curated voices.",
  },
  robots: { index: true, follow: true },
};

export const viewport: Viewport = {
  themeColor: "#0A0E0D",
  width: "device-width",
  initialScale: 1,
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>
        <a href="#main" className="ic-skip-link">
          Skip to content
        </a>
        {children}
      </body>
    </html>
  );
}
