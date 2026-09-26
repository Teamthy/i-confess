/*
 * The brand mark.
 *
 * The repository contains no approved iCONFESS logo asset — both mobile app
 * icons are the unmodified Flutter template icon (docs/44-WEB-PLATFORM-AUDIT.md
 * §1). Per the build rules, the fallback is restrained and intentional: a
 * typographic wordmark with a three-bar "spoken word" glyph drawn from the
 * design tokens. It is explicitly a fallback, not a redesign of an official
 * mark; when the approved logo lands in the repository, this component is the
 * single file to replace.
 */

export function Mark({ className }: { className?: string }) {
  return (
    <svg
      className={className}
      viewBox="0 0 28 28"
      fill="none"
      aria-hidden="true"
      focusable="false"
    >
      <rect x="4" y="15" width="3.2" height="8" rx="1.6" fill="currentColor" opacity="0.55" />
      <rect x="10.4" y="10" width="3.2" height="13" rx="1.6" fill="currentColor" />
      <rect x="16.8" y="5" width="3.2" height="18" rx="1.6" fill="currentColor" opacity="0.8" />
      <circle cx="24" cy="7" r="1.6" fill="currentColor" opacity="0.55" />
    </svg>
  );
}

export function Wordmark({ onDark = false }: { onDark?: boolean }) {
  return (
    <span
      className="ic-brand"
      style={{ color: "inherit" }}
      data-on-dark={onDark || undefined}
    >
      <Mark />
      <span>
        I&nbsp;Confess
      </span>
    </span>
  );
}
