// Package email delivers transactional authentication mail (§57, §58).
//
// Two rules shape this package:
//
//  1. Sending never happens inline in an authentication request. SMTP latency
//     and provider outages must not turn "register" into a 500 — the request
//     enqueues and returns, and a worker delivers.
//  2. Message bodies are built here, not by callers. Centralising them is what
//     makes it auditable that a verification link is never logged and that we
//     never put a token in a subject line or a query string that ends up in a
//     referrer header.
package email

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Message is a rendered email ready to send.
type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
	// Tag groups messages for provider-side analytics and lets the worker apply
	// per-type retry policy.
	Tag string
}

// Sender delivers a message. Implementations must be safe for concurrent use.
//
// The interface is intentionally tiny so a provider swap (Postmark, SES,
// Resend) touches one file.
type Sender interface {
	Name() string
	Send(ctx context.Context, msg Message) error
}

// Errors a Sender may return. The worker uses these to decide whether to retry.
var (
	// ErrRetryable is a transient fault: timeout, 429, 5xx.
	ErrRetryable = errors.New("retryable email error")
	// ErrPermanent will recur: malformed address, hard bounce, rejected
	// content. Retrying wastes quota and can harm sender reputation.
	ErrPermanent = errors.New("permanent email error")
)

// Retryable wraps a transient fault.
func Retryable(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrRetryable, fmt.Sprintf(format, args...))
}

// Permanent wraps a fault that must not be retried.
func Permanent(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrPermanent, fmt.Sprintf(format, args...))
}

// IsRetryable reports whether delivery should be attempted again.
func IsRetryable(err error) bool { return errors.Is(err, ErrRetryable) }

// ---------------------------------------------------------------------------
// Message templates
// ---------------------------------------------------------------------------

// Config holds the values templates need.
type Config struct {
	// AppName appears in subjects and bodies.
	AppName string
	// BaseURL is the public origin used to build links, e.g. https://iconfess.app
	BaseURL string
	// FromAddress is the envelope sender.
	FromAddress string
	// SupportAddress is offered as a reply path in security mail.
	SupportAddress string
}

// VerificationMessage builds the email-verification mail (§15).
//
// The token travels in the URL path fragment of our own link and nowhere else:
// not in the subject, not in a logged field, and not in any third-party pixel.
func (c Config) VerificationMessage(to, token string) Message {
	link := c.link("/verify-email", token)
	return Message{
		To:      to,
		Tag:     "email_verification",
		Subject: "Confirm your email for " + c.AppName,
		Text: strings.Join([]string{
			"Welcome to " + c.AppName + ".",
			"",
			"Confirm your email address to finish setting up your account:",
			link,
			"",
			"This link expires in 60 minutes and can be used once.",
			"If you did not create an account, you can ignore this message.",
		}, "\n"),
		HTML: c.wrapHTML("Confirm your email",
			"<p>Welcome to "+esc(c.AppName)+".</p>"+
				"<p>Confirm your email address to finish setting up your account.</p>"+
				c.button(link, "Confirm email")+
				"<p class=\"muted\">This link expires in 60 minutes and can be used once. "+
				"If you did not create an account, you can ignore this message.</p>"),
	}
}

// PasswordResetMessage builds the reset mail (§32).
//
// The copy deliberately does not confirm that an account exists — the same
// message is only ever sent to a real address, but its wording must not become
// the thing that leaks membership if it is ever forwarded or screenshotted.
func (c Config) PasswordResetMessage(to, token string) Message {
	link := c.link("/reset-password", token)
	return Message{
		To:      to,
		Tag:     "password_reset",
		Subject: "Reset your " + c.AppName + " password",
		Text: strings.Join([]string{
			"A password reset was requested for this email address.",
			"",
			"Set a new password:",
			link,
			"",
			"This link expires in 30 minutes and can be used once.",
			"If you did not request this, no action is needed and your password is unchanged.",
		}, "\n"),
		HTML: c.wrapHTML("Reset your password",
			"<p>A password reset was requested for this email address.</p>"+
				c.button(link, "Set a new password")+
				"<p class=\"muted\">This link expires in 30 minutes and can be used once. "+
				"If you did not request this, no action is needed and your password is unchanged.</p>"),
	}
}

// SecurityAlertMessage notifies a user of a meaningful account event (§52).
//
// It carries no token and no link that performs an action: a security alert
// that can be acted on directly is itself a phishing vector.
func (c Config) SecurityAlertMessage(to, event, detail string) Message {
	return Message{
		To:      to,
		Tag:     "security_alert",
		Subject: c.AppName + " security notice",
		Text: strings.Join([]string{
			event,
			"",
			detail,
			"",
			"If this was you, no action is needed.",
			"If it was not, change your password and sign out of all devices from Settings.",
			"Questions: " + c.SupportAddress,
		}, "\n"),
		HTML: c.wrapHTML("Security notice",
			"<p><strong>"+esc(event)+"</strong></p>"+
				"<p>"+esc(detail)+"</p>"+
				"<p class=\"muted\">If this was you, no action is needed. If it was not, "+
				"change your password and sign out of all devices from Settings.</p>"),
	}
}

func (c Config) link(path, token string) string {
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = "https://localhost"
	}
	return base + path + "?token=" + token
}

func (c Config) button(href, label string) string {
	return `<p><a class="btn" href="` + esc(href) + `">` + esc(label) + `</a></p>` +
		`<p class="muted">Or paste this link into your browser:<br><span class="link">` + esc(href) + `</span></p>`
}

// wrapHTML applies a plain, inline-styled shell. Inline styles because email
// clients strip stylesheets, and restrained because authentication mail that
// looks like marketing gets filed as marketing.
func (c Config) wrapHTML(title, body string) string {
	return `<!doctype html><html><body style="margin:0;padding:24px;background:#f6f1e7;font-family:-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;color:#12100e">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0"><tr><td align="center">
<table role="presentation" width="100%" style="max-width:520px;background:#ffffff;border-radius:12px;padding:32px">
<tr><td>
<h1 style="margin:0 0 16px;font-size:20px;font-weight:600">` + esc(title) + `</h1>
` + strings.NewReplacer(
		`class="btn"`, `style="display:inline-block;padding:12px 24px;background:#12100e;color:#ffffff;text-decoration:none;border-radius:999px;font-weight:600"`,
		`class="muted"`, `style="color:#8c8272;font-size:13px;line-height:1.5"`,
		`class="link"`, `style="color:#8c8272;font-size:12px;word-break:break-all"`,
	).Replace(body) + `
<p style="color:#8c8272;font-size:12px;margin-top:24px">` + esc(c.AppName) + `</p>
</td></tr></table></td></tr></table></body></html>`
}

// esc escapes text for HTML interpolation. Addresses and app names are not
// attacker-controlled today, but templates outlive their assumptions.
func esc(s string) string {
	return strings.NewReplacer(
		"&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;",
	).Replace(s)
}

// MaskAddress redacts an address for logging (§70).
func MaskAddress(addr string) string {
	at := strings.IndexByte(addr, '@')
	if at <= 1 {
		return "***"
	}
	return addr[:1] + "***" + addr[at:]
}
