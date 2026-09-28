import type { Metadata } from "next";
import Link from "next/link";

/* /t/{token} — a shared session template.
 *
 * Carried over from the app being retired because share links exist in the
 * wild and must keep resolving. The token is resolved server-side, so an
 * unauthenticated visitor sees exactly what the token permits and nothing
 * more; the route is excluded from robots.ts and never indexed. */

export const metadata: Metadata = {
  title: "Shared with you",
  robots: { index: false, follow: false },
};

async function getTemplate(token: string) {
  try {
    const res = await fetch(`${process.env.IC_API_URL || "http://127.0.0.1:8080"}/t/${encodeURIComponent(token)}`, {
      cache: "no-store",
    });
    if (!res.ok) return null;
    return (await res.json()) as { id?: string; title?: string; description?: string; duration_minutes?: number };
  } catch {
    return null;
  }
}

export default async function SharedTemplatePage({ params }: { params: Promise<{ token: string }> }) {
  const { token } = await params;
  const template = await getTemplate(token);

  return (
    <main id="main" className="container" style={{ minHeight: "70vh", display: "grid", placeItems: "center", padding: "80px 24px" }}>
      <div className="auth-card" style={{ width: "min(560px,100%)", textAlign: "center" }}>
        {template ? (
          <>
            <span className="scripture-eyebrow">Shared with you</span>
            <h1 className="h2" style={{ marginTop: 18 }}>{template.title || "A session"}</h1>
            {template.description && <p className="lede" style={{ marginTop: 12 }}>{template.description}</p>}
            {template.duration_minutes ? (
              <p className="small" style={{ marginTop: 10 }}>{template.duration_minutes} minutes</p>
            ) : null}
            <Link className="btn btn-primary btn-lg" style={{ marginTop: 24 }} href="/register">
              Create your own
            </Link>
          </>
        ) : (
          <>
            <span className="scripture-eyebrow">Link</span>
            <h1 className="h2" style={{ marginTop: 18 }}>This link has expired</h1>
            <p className="lede" style={{ marginTop: 12 }}>
              Shared templates stop resolving when their owner revokes them or the link ages out. Nothing is shown in
              its place.
            </p>
            <Link className="btn btn-ghost btn-lg" style={{ marginTop: 24 }} href="/">
              Go to iCONFESS
            </Link>
          </>
        )}
      </div>
    </main>
  );
}
