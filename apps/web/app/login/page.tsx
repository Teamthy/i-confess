import type { Metadata } from "next";
import Link from "next/link";
import { LoginForm } from "./LoginForm";

export const metadata: Metadata = {
  title: "Sign in",
  description: "Sign in to iCONFESS. Your practice is where you left it.",
  alternates: { canonical: "/login" },
  robots: { index: true, follow: true },
};

export default function LoginPage() {
  return (
    <>
      <LoginForm />
      {/* Server-rendered route for people and crawlers without the form island. */}
      <noscript>
        <p style={{ position: "fixed", inset: "auto 0 0 0", background: "#212927", color: "#E9EDEC", padding: "16px", textAlign: "center" }}>
          Enable JavaScript to sign in — or{" "}
          <Link href="/explore" style={{ color: "#74C9AB" }}>explore the public library</Link> without an account.
        </p>
      </noscript>
    </>
  );
}
