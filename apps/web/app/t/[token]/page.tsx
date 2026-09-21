import { notFound } from "next/navigation";

/**
 * Shareable template — /t/{token}.
 *
 * Kept from the retired scaffold because live share links exist in the wild,
 * rebuilt against the new client: server fetch, human error state, no
 * crawlable output (robots.ts disallows /t/). GET /t/{token} resolves the
 * template server-side; unauthenticated visitors see exactly what the token
 * permits and nothing more.
 */
async function getTemplate(token: string) {
  try {
    const res = await fetch(
      `${process.env.IC_API_URL || "http://127.0.0.1:8080"}/t/${encodeURIComponent(token)}`,
      { cache: "no-store" },
    );
    if (!res.ok) return null;
    return (await res.json()) as {
      id?: string;
      title?: string;
      description?: string;
      duration_minutes?: number;
    };
  } catch {
    return null;
  }
}

export default async function SharedTemplatePage({
  params,
}: {
  params: Promise<{ token: string }>;
}) {
  const { token } = await params;
  const template = await getTemplate(token);

  return (
    <main
      id="main"
      style={{
        minHeight: "100vh",
        display: "grid",
        placeItems: "center",
        background: "var(--ic-color-neutral-950)",
        color: "var(--ic-color-neutral-100)",
        padding: "var(--ic-spacing-6)",
      }}
      className="on-ink"
    >
      <div
        className="ic-card"
        style={{ maxWidth: "28rem", width: "100%", padding: "var(--ic-spacing-8)", textAlign: "center" }}
      >
        {template ? (
          <>
            <p className="ic-eyebrow" style={{ justifyContent: "center" }}>Shared session</p>
            <h1 style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-web-title)", marginTop: "var(--ic-spacing-4)" }}>
              {template.title || "A shared session"}
            </h1>
            {template.description && (
              <p className="ic-lede" style={{ margin: "var(--ic-spacing-4) auto 0" }}>
                {template.description}
              </p>
            )}
            <a href="/register" className="ic-btn ic-btn--primary" style={{ marginTop: "var(--ic-spacing-6)" }}>
              Start your experience
            </a>
          </>
        ) : (
          <>
            <p className="ic-eyebrow" style={{ justifyContent: "center" }}>Link not valid</p>
            <h1 style={{ fontFamily: "var(--ic-font-family-serif)", fontSize: "var(--ic-web-title)", marginTop: "var(--ic-spacing-4)" }}>
              This shared session isn't available.
            </h1>
            <p className="ic-lede" style={{ margin: "var(--ic-spacing-4) auto 0" }}>
              The link may have been revoked or mistyped. Everything public is on
              the explore page.
            </p>
            <a href="/explore" className="ic-btn ic-btn--secondary" style={{ marginTop: "var(--ic-spacing-6)" }}>
              Explore confessions
            </a>
          </>
        )}
      </div>
    </main>
  );
}
