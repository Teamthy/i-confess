"use client";

import { useState } from "react";

/**
 * The contact form (client island). The page itself is a server component so
 * it keeps real metadata; only this form ships JavaScript.
 */
export function ContactForm() {
  const [status, setStatus] = useState<"idle" | "sending" | "sent" | "error">("idle");
  const [error, setError] = useState("");
  const [fields, setFields] = useState({ name: "", email: "", subject: "", message: "" });
  const [issues, setIssues] = useState<Record<string, string>>({});

  function validate() {
    const next: Record<string, string> = {};
    if (!fields.name.trim()) next.name = "Please tell us your name.";
    if (!/^[^@\s]+@[^@\s]+\.[^@\s]+$/.test(fields.email)) next.email = "Please use a valid email address.";
    if (fields.message.trim().length < 10) next.message = "A little more detail helps us help you.";
    setIssues(next);
    return Object.keys(next).length === 0;
  }

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!validate()) return;
    setStatus("sending");
    try {
      const res = await fetch("/api/support/contact", {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(fields),
      });
      if (res.ok) {
        setStatus("sent");
      } else {
        setStatus("error");
        setError("Your message could not be sent just now. Please try again, or write to us directly.");
      }
    } catch {
      setStatus("error");
      setError("Your message could not be sent just now. Please try again, or write to us directly.");
    }
  }

  const set = (k: keyof typeof fields) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) =>
    setFields((f) => ({ ...f, [k]: e.target.value }));

  if (status === "sent") {
    return (
      <div className="ic-state" role="status" style={{ borderStyle: "solid", borderColor: "var(--ic-color-brand-300)" }}>
        <h3>Message received.</h3>
        <p>Thank you — we read everything, and we will reply to {fields.email} as soon as we can.</p>
      </div>
    );
  }

  return (
    <form onSubmit={onSubmit} noValidate style={{ display: "grid", gap: "var(--ic-spacing-5)" }}>
      <div className="ic-field">
        <label htmlFor="contact-name">Name</label>
        <input
          id="contact-name"
          name="name"
          autoComplete="name"
          value={fields.name}
          onChange={set("name")}
          aria-invalid={issues.name ? true : undefined}
          aria-describedby={issues.name ? "contact-name-error" : undefined}
        />
        {issues.name && (
          <p className="ic-field__error" id="contact-name-error">{issues.name}</p>
        )}
      </div>
      <div className="ic-field">
        <label htmlFor="contact-email">Email</label>
        <input
          id="contact-email"
          name="email"
          type="email"
          autoComplete="email"
          value={fields.email}
          onChange={set("email")}
          aria-invalid={issues.email ? true : undefined}
          aria-describedby={issues.email ? "contact-email-error" : undefined}
        />
        {issues.email && (
          <p className="ic-field__error" id="contact-email-error">{issues.email}</p>
        )}
      </div>
      <div className="ic-field">
        <label htmlFor="contact-subject">Subject <span style={{ color: "var(--ic-color-neutral-500)", fontWeight: "var(--ic-font-weight-regular)" }}>(optional)</span></label>
        <input id="contact-subject" name="subject" value={fields.subject} onChange={set("subject")} />
      </div>
      <div className="ic-field">
        <label htmlFor="contact-message">Message</label>
        <textarea
          id="contact-message"
          name="message"
          value={fields.message}
          onChange={set("message")}
          aria-invalid={issues.message ? true : undefined}
          aria-describedby={issues.message ? "contact-message-error" : undefined}
        />
        {issues.message && (
          <p className="ic-field__error" id="contact-message-error">{issues.message}</p>
        )}
      </div>
      {status === "error" && (
        <div className="ic-state" role="alert" style={{ padding: "var(--ic-spacing-5)" }}>
          <p>{error}</p>
        </div>
      )}
      <div className="ic-btn-row">
        <button type="submit" className="ic-btn ic-btn--primary" disabled={status === "sending"}>
          {status === "sending" ? "Sending…" : "Send message"}
        </button>
      </div>
      <p className="ic-field__hint">
        Support: <a href="mailto:support@iconfess.app" style={{ color: "var(--ic-color-brand-700)" }}>support@iconfess.app</a>
      </p>
    </form>
  );
}
