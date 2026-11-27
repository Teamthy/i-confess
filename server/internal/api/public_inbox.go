package api

import (
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Teamthy/i-confess/internal/auth"
	"github.com/Teamthy/i-confess/internal/email"
	"github.com/Teamthy/i-confess/internal/httpx"
)

// The website's newsletter and contact forms. Both are public, rate-limited
// per address, and answer the same way whether or not an address is already
// known, so neither can be used to test which emails are on file.

func validEmail(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) > 254 {
		return false
	}
	a, err := mail.ParseAddress(s)
	return err == nil && a.Address == s
}

// newsletterSubscribe stores a newsletter address.
func (h *Handler) newsletterSubscribe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || !validEmail(req.Email) {
		writeCode(w, http.StatusBadRequest, "EMAIL_INVALID", "enter a valid email address")
		return
	}
	token, hash, err := auth.NewOneTimeToken()
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not save your subscription")
		return
	}
	created, err := h.inbox.Subscribe(r.Context(), req.Email, "web", hash, time.Now())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not save your subscription")
		return
	}
	// The welcome mail carries the unsubscribe link, so the address's owner
	// can leave even if someone else typed it in. It goes only to a new
	// address: a known one already has a working link, and the answer below
	// is the same either way so the form does not reveal who is on the list.
	if !created {
		// Nothing to send.
	} else if h.devTokenSink != nil {
		h.devTokenSink("newsletter_unsubscribe", req.Email, token)
	} else if h.mail != nil {
		h.mail.Enqueue(h.mailCfg.NewsletterWelcomeMessage(strings.TrimSpace(req.Email), token))
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "subscribed"})
}

// newsletterUnsubscribe takes an address off the list using the token from
// the link in any newsletter mail.
func (h *Handler) newsletterUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || strings.TrimSpace(req.Token) == "" {
		writeCode(w, http.StatusBadRequest, "TOKEN_INVALID", "this unsubscribe link is incomplete")
		return
	}
	ok, err := h.inbox.Unsubscribe(r.Context(), auth.HashToken(strings.TrimSpace(req.Token)), time.Now())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not unsubscribe")
		return
	}
	if !ok {
		writeCode(w, http.StatusNotFound, "TOKEN_UNKNOWN", "this unsubscribe link is no longer valid")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "unsubscribed"})
}

// contactMessage stores a message for support and, when mail is configured,
// forwards it to the support address.
func (h *Handler) contactMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Email   string `json:"email"`
		Message string `json:"message"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		writeCode(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}
	name, msg := strings.TrimSpace(req.Name), strings.TrimSpace(req.Message)
	switch {
	case name == "" || utf8.RuneCountInString(name) > 120:
		writeCode(w, http.StatusBadRequest, "NAME_INVALID", "enter your name (up to 120 characters)")
		return
	case !validEmail(req.Email):
		writeCode(w, http.StatusBadRequest, "EMAIL_INVALID", "enter a valid email address")
		return
	case msg == "" || utf8.RuneCountInString(msg) > 5000:
		writeCode(w, http.StatusBadRequest, "MESSAGE_INVALID", "write a message (up to 5,000 characters)")
		return
	}
	id, err := h.inbox.SaveContact(r.Context(), name, req.Email, msg, time.Now())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "could not send your message")
		return
	}
	// The message is stored first, so a mail outage never loses it.
	if h.mail != nil && h.mailCfg.SupportAddress != "" {
		body := "From: " + name + " <" + strings.TrimSpace(req.Email) + ">\nReference: " + id + "\n\n" + msg
		h.mail.Enqueue(email.Message{
			To: h.mailCfg.SupportAddress, Subject: "Contact form: " + name, Text: body, Tag: "contact",
		})
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "received", "reference": id})
}
