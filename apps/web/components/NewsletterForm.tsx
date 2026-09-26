"use client";

/**
 * Newsletter block (motif M12).
 *
 * There is no newsletter endpoint — and this form does not pretend otherwise.
 * "The weekly words" is an account benefit: entering an address carries it
 * into registration, where notification preferences (including new-content
 * mail) are managed honestly. No fake subscribe POST, no fabricated list.
 */

import { useState } from "react";
import { useRouter } from "next/navigation";

export function NewsletterForm() {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [error, setError] = useState("");

  function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(email.trim())) {
      setError("Enter a valid email address.");
      return;
    }
    router.push(`/register?email=${encodeURIComponent(email.trim())}`);
  }

  return (
    <form className="ic-news__form" onSubmit={submit} noValidate>
      <label className="ic-visually-hidden" htmlFor="news-email">Email address</label>
      <input
        id="news-email"
        type="email"
        autoComplete="email"
        className="ic-input"
        placeholder="you@example.com"
        value={email}
        onChange={(e) => {
          setEmail(e.target.value);
          if (error) setError("");
        }}
        aria-invalid={error ? true : undefined}
        aria-describedby={error ? "news-email-error" : undefined}
      />
      <button type="submit" className="ic-btn ic-btn--on-dark">
        Get the weekly words
      </button>
      {error && (
        <p id="news-email-error" role="alert" style={{ fontSize: "var(--ic-font-size-caption)", color: "#E8776E", margin: 0 }}>
          {error}
        </p>
      )}
    </form>
  );
}
