import { Wordmark } from "./Wordmark";

/**
 * Shared shell for the auth screens: ink canvas, centered card, wordmark.
 * The forms themselves are client components (see app/login, app/register,
 * app/forgot-password, app/reset-password, app/verify-email).
 */
export function AuthCard({
  eyebrow,
  title,
  lede,
  children,
  footer,
}: {
  eyebrow: string;
  title: string;
  lede?: string;
  children: React.ReactNode;
  footer?: React.ReactNode;
}) {
  return (
    <main
      id="main"
      className="on-ink"
      style={{
        minHeight: "100vh",
        display: "grid",
        placeItems: "center",
        background: "var(--ic-color-neutral-950)",
        color: "var(--ic-color-neutral-100)",
        padding: "var(--ic-spacing-6)",
        position: "relative",
        overflow: "hidden",
      }}
    >
      <div
        aria-hidden="true"
        style={{
          position: "absolute",
          inset: 0,
          background:
            "radial-gradient(48rem 28rem at 80% -10%, rgba(42,157,118,0.14), transparent 60%)",
        }}
      />
      <div style={{ position: "relative", width: "100%", maxWidth: "26rem" }}>
        <div style={{ display: "flex", justifyContent: "center", marginBottom: "var(--ic-spacing-7)" }}>
          <a href="/" aria-label="iCONFESS home" style={{ color: "var(--ic-color-neutral-0)", textDecoration: "none" }}>
            <Wordmark />
          </a>
        </div>
        <div
          className="ic-card"
          style={{
            padding: "clamp(var(--ic-spacing-6), 5vw, var(--ic-spacing-8))",
            display: "grid",
            gap: "var(--ic-spacing-5)",
          }}
        >
          <div>
            <p className="ic-eyebrow" style={{ color: "var(--ic-color-brand-600)" }}>{eyebrow}</p>
            <h1
              style={{
                fontFamily: "var(--ic-font-family-serif)",
                fontSize: "var(--ic-web-title)",
                fontWeight: "var(--ic-font-weight-regular)",
                marginTop: "var(--ic-spacing-3)",
                color: "var(--ic-color-neutral-950)",
              }}
            >
              {title}
            </h1>
            {lede && (
              <p style={{ marginTop: "var(--ic-spacing-3)", fontSize: "var(--ic-font-size-bodySm)", color: "var(--ic-color-neutral-600)", lineHeight: "var(--ic-font-lineHeight-relaxed)" }}>
                {lede}
              </p>
            )}
          </div>
          {children}
        </div>
        {footer && (
          <div
            style={{
              marginTop: "var(--ic-spacing-5)",
              textAlign: "center",
              fontSize: "var(--ic-font-size-bodySm)",
              color: "var(--ic-color-neutral-400)",
            }}
          >
            {footer}
          </div>
        )}
      </div>
    </main>
  );
}
