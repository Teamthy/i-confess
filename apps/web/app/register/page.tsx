import type { Metadata } from "next";
import Link from "next/link";
import { RegisterForm } from "./RegisterForm";

export const metadata: Metadata = {
  title: "Create your account",
  description: "One account for the practice — web and mobile. Private confessions stay private.",
  alternates: { canonical: "/register" },
  robots: { index: true, follow: true },
};

export default async function RegisterPage({
  searchParams,
}: {
  searchParams: Promise<{ email?: string | string[] }>;
}) {
  const sp = await searchParams;
  const raw = Array.isArray(sp.email) ? sp.email[0] : sp.email;
  const initialEmail = raw && /^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(raw) ? raw : "";
  return (
    <>
      <RegisterForm initialEmail={initialEmail} />
      <noscript>
        <p style={{ position: "fixed", inset: "auto 0 0 0", background: "#212927", color: "#E9EDEC", padding: "16px", textAlign: "center" }}>
          Enable JavaScript to create an account — or{" "}
          <Link href="/explore" style={{ color: "#7288C4" }}>explore the public library</Link> first.
        </p>
      </noscript>
    </>
  );
}
